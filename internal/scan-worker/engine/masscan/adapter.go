package masscan

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/martinezpascualdani/heimdall/internal/scan-worker/engine"
)

// Adapter implements PortDiscoveryEngine by invoking masscan as an external process.
// Masscan uses raw sockets but often works in environments where zmap hangs (e.g. Docker).
type Adapter struct {
	MasscanPath   string
	WorkDir       string
	Timeout       time.Duration
	Rate          int // packets per second
	DefaultPort   int
}

// -oL output can be "Host: IP () Ports: PORT/open/..." or "open tcp PORT IP timestamp"
var (
	listLineRE   = regexp.MustCompile(`Host:\s*([\d.]+)\s*\([^)]*\)\s*Ports:\s*(\d+)/open`)
	listLineRE2  = regexp.MustCompile(`open\s+tcp\s+(\d+)\s+([\d.]+)\s+\d+`)
)

func resolveMasscanPath() string {
	if p := os.Getenv("MASSCAN_PATH"); p != "" {
		return p
	}
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), "bin", "masscan")
	}
	return "masscan"
}

func NewAdapter() *Adapter {
	return &Adapter{
		MasscanPath: resolveMasscanPath(),
		WorkDir:     os.TempDir(),
		Timeout:     10 * time.Minute,
		Rate:        1000,
		DefaultPort: 443,
	}
}

func (a *Adapter) portsToScan(payload *engine.JobPayload) []int {
	if payload.PortRangeStart > 0 && payload.PortRangeEnd >= payload.PortRangeStart {
		var out []int
		for p := payload.PortRangeStart; p <= payload.PortRangeEnd && p <= 65535; p++ {
			out = append(out, p)
		}
		return out
	}
	if len(payload.Ports) > 0 {
		return payload.Ports
	}
	if payload.Port > 0 {
		return []int{payload.Port}
	}
	return []int{a.DefaultPort}
}

// Run runs masscan once with all requested ports (masscan supports multiple ports in one run).
func (a *Adapter) Run(ctx context.Context, payload *engine.JobPayload) (*engine.PortDiscoveryResult, error) {
	if payload == nil || len(payload.Prefixes) == 0 {
		return &engine.PortDiscoveryResult{Observations: nil}, nil
	}
	ports := a.portsToScan(payload)
	if len(ports) == 0 {
		return &engine.PortDiscoveryResult{Observations: nil}, nil
	}
	portList := make([]string, len(ports))
	for i, p := range ports {
		portList[i] = strconv.Itoa(p)
	}
	portArg := strings.Join(portList, ",")
	log.Printf("masscan: scanning %d ports: %s", len(ports), portArg)

	dir, err := os.MkdirTemp(a.WorkDir, "masscan-")
	if err != nil {
		return nil, fmt.Errorf("masscan work dir: %w", err)
	}
	defer os.RemoveAll(dir)
	targetPath := filepath.Join(dir, "targets.txt")
	f, err := os.Create(targetPath)
	if err != nil {
		return nil, fmt.Errorf("masscan targets file: %w", err)
	}
	for _, p := range payload.Prefixes {
		fmt.Fprintln(f, strings.TrimSpace(p))
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithTimeout(ctx, a.Timeout)
	defer cancel()
	obs, err := a.runMasscan(runCtx, targetPath, portArg, len(payload.Prefixes))
	if err != nil {
		return &engine.PortDiscoveryResult{Observations: nil, Error: err.Error()}, nil
	}
	return &engine.PortDiscoveryResult{Observations: obs}, nil
}

func (a *Adapter) runMasscan(ctx context.Context, targetPath, portArg string, numPrefixes int) ([]engine.Observation, error) {
	outPath := filepath.Join(filepath.Dir(targetPath), "masscan-out.txt")
	args := []string{
		"-iL", targetPath,
		"-p", portArg,
		"--rate", strconv.Itoa(a.Rate),
		"-oL", outPath,
	}
	cmd := exec.CommandContext(ctx, a.MasscanPath, args...)
	cmd.Dir = filepath.Dir(targetPath)
	log.Printf("masscan: targets=%s ports=%s", targetPath, portArg)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("masscan stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("masscan start: %w", err)
	}

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				for _, line := range strings.Split(strings.TrimSpace(string(buf[:n])), "\n") {
					line = strings.TrimSpace(line)
					if line != "" {
						log.Printf("masscan stderr: %s", line)
					}
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
		}
	}()

	waitErr := cmd.Wait()
	if waitErr != nil {
		os.Remove(outPath)
		if ctx.Err() != nil {
			return nil, fmt.Errorf("masscan timeout: %w", ctx.Err())
		}
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			log.Printf("masscan: exit code=%d", exitErr.ExitCode())
		}
		return nil, waitErr
	}

	f, err := os.Open(outPath)
	if err != nil {
		log.Printf("masscan: could not read output file: %v", err)
		return nil, nil
	}
	defer f.Close()
	defer os.Remove(outPath)

	var observations []engine.Observation
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var ip string
		var port int
		if m := listLineRE.FindStringSubmatch(line); len(m) >= 3 {
			ip = m[1]
			port, _ = strconv.Atoi(m[2])
		} else if m := listLineRE2.FindStringSubmatch(line); len(m) >= 3 {
			port, _ = strconv.Atoi(m[1])
			ip = m[2]
		}
		if ip != "" {
			observations = append(observations, engine.Observation{IP: ip, Port: port, Status: "open"})
		}
	}
	log.Printf("masscan: finished observations=%d", len(observations))
	return observations, nil
}

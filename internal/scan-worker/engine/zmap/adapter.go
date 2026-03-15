package zmap

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/martinezpascualdani/heimdall/internal/scan-worker/engine"
)

// Adapter implements PortDiscoveryEngine by invoking zmap as an external process.
// Supports single port, Ports list (portscan-basic), or port range (portscan-full). Uses -v 5; streams stdout/stderr in real time.
// PerPortTimeout: if zmap hangs (e.g. no raw socket in Docker on Mac after "found gateway IP"), we stop after this and try next port.
type Adapter struct {
	ZmapPath        string
	WorkDir         string
	Timeout         time.Duration // max total time for entire Run (all ports)
	PerPortTimeout  time.Duration // max time per single port (avoids hanging forever when raw socket is blocked)
	DefaultPort     int
}

func ResolveZmapPath() string {
	if p := os.Getenv("ZMAP_PATH"); p != "" {
		return p
	}
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), "bin", "zmap")
	}
	return "zmap"
}

func NewAdapter() *Adapter {
	return &Adapter{
		ZmapPath:       ResolveZmapPath(),
		WorkDir:        os.TempDir(),
		Timeout:        10 * time.Minute,
		PerPortTimeout: 2 * time.Minute, // when zmap hangs (e.g. no raw socket), fail this port and continue
		DefaultPort:    443,
	}
}

// portsToScan returns the list of ports to scan from the payload.
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

func (a *Adapter) Run(ctx context.Context, payload *engine.JobPayload) (*engine.PortDiscoveryResult, error) {
	if payload == nil || len(payload.Prefixes) == 0 {
		return &engine.PortDiscoveryResult{Observations: nil}, nil
	}
	ports := a.portsToScan(payload)
	if len(ports) == 0 {
		return &engine.PortDiscoveryResult{Observations: nil}, nil
	}
	log.Printf("zmap: scanning %d ports: %v", len(ports), ports)

	dir, err := os.MkdirTemp(a.WorkDir, "zmap-")
	if err != nil {
		return nil, fmt.Errorf("zmap work dir: %w", err)
	}
	defer os.RemoveAll(dir)
	targetPath := filepath.Join(dir, "targets.txt")
	f, err := os.Create(targetPath)
	if err != nil {
		return nil, fmt.Errorf("zmap targets file: %w", err)
	}
	for _, p := range payload.Prefixes {
		fmt.Fprintln(f, strings.TrimSpace(p))
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	perPort := a.PerPortTimeout
	if perPort <= 0 {
		perPort = 2 * time.Minute
	}
	var allObs []engine.Observation
	for _, port := range ports {
		select {
		case <-ctx.Done():
			return &engine.PortDiscoveryResult{Observations: allObs, Error: ctx.Err().Error()}, nil
		default:
		}
		runCtx, cancel := context.WithTimeout(ctx, perPort)
		obs, err := a.runZmapOnePort(runCtx, targetPath, port, len(payload.Prefixes))
		cancel()
		if err != nil {
			log.Printf("zmap: port %d failed or timeout (e.g. raw socket blocked): %v", port, err)
			// continue to next port instead of aborting the whole job
			continue
		}
		allObs = append(allObs, obs...)
	}
	return &engine.PortDiscoveryResult{Observations: allObs}, nil
}

// runZmapOnePort runs zmap for one port with -v 5 (verbose), streams stdout/stderr in real time, returns observations.
// -w = allowlist file (CIDRs to scan). -l in zmap is log-file, so we must use -w for targets or zmap won't scan our IPs and returns 0.
// -c = cooldown: seconds to keep receiving after sending ends (default 8). Higher = more time for late SYN-ACKs, fewer missed hosts.
// --disable-syslog + --log-file=/dev/stderr: in containers /dev/log often doesn't exist.
func (a *Adapter) runZmapOnePort(ctx context.Context, targetPath string, port, numPrefixes int) ([]engine.Observation, error) {
	cmd := exec.CommandContext(ctx, a.ZmapPath, "--disable-syslog", "--log-file=/dev/stderr", "-v", "5", "-p", strconv.Itoa(port), "-w", targetPath, "-c", "20", "-o", "-")
	cmd.Dir = filepath.Dir(targetPath)
	log.Printf("zmap: port=%d targets=%s num_prefixes=%d", port, targetPath, numPrefixes)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("zmap stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("zmap stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("zmap start: %w", err)
	}

	var observations []engine.Observation
	var stderrBuf strings.Builder
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				observations = append(observations, engine.Observation{IP: line, Port: port, Status: "open"})
			} else if line != "" {
				log.Printf("zmap [port %d]: %s", port, line)
			}
		}
	}()
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				s := strings.TrimSpace(string(buf[:n]))
				stderrBuf.Write(buf[:n])
				for _, line := range strings.Split(s, "\n") {
					line = strings.TrimSpace(line)
					if line != "" {
						log.Printf("zmap [port %d] stderr: %s", port, line)
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
	wg.Wait()

	waitErr := cmd.Wait()
	if waitErr != nil {
		if ctx.Err() != nil {
			return observations, fmt.Errorf("zmap timeout: %w", ctx.Err())
		}
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			log.Printf("zmap [port %d]: exit code=%d", port, exitErr.ExitCode())
		}
		return observations, waitErr
	}
	log.Printf("zmap [port %d]: finished observations=%d", port, len(observations))
	return observations, nil
}

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/martinezpascualdani/heimdall/internal/scan-worker/engine"
	"github.com/martinezpascualdani/heimdall/internal/scan-worker/engine/masscan"
	"github.com/martinezpascualdani/heimdall/internal/scan-worker/engine/zmap"
	"github.com/martinezpascualdani/heimdall/internal/scan-worker/runner"
)

func main() {
	executionURL := strings.TrimSuffix(os.Getenv("EXECUTION_SERVICE_URL"), "/")
	if executionURL == "" {
		executionURL = "http://localhost:8085"
	}
	name := os.Getenv("WORKER_NAME")
	if name == "" {
		name = "scan-worker-1"
	}
	region := os.Getenv("WORKER_REGION")
	version := os.Getenv("WORKER_VERSION")
	if version == "" {
		version = "0.1.0"
	}
	capabilitiesStr := os.Getenv("WORKER_CAPABILITIES")
	var capabilities []string
	if capabilitiesStr != "" {
		for _, c := range strings.Split(capabilitiesStr, ",") {
			capabilities = append(capabilities, strings.TrimSpace(c))
		}
	}
	maxConcurrency := 1
	if s := os.Getenv("WORKER_MAX_CONCURRENCY"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxConcurrency = n
		}
	}
	heartbeatInterval := 30 * time.Second
	if s := os.Getenv("HEARTBEAT_INTERVAL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			heartbeatInterval = d
		}
	}
	claimInterval := 5 * time.Second
	if s := os.Getenv("CLAIM_INTERVAL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			claimInterval = d
		}
	}
	jobTimeout := 10 * time.Minute
	if s := os.Getenv("JOB_TIMEOUT"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			jobTimeout = d
		}
	}

	cfg := runner.Config{
		ExecutionServiceURL: executionURL,
		Name:                name,
		Region:              region,
		Version:             version,
		Capabilities:        capabilities,
		MaxConcurrency:      maxConcurrency,
		HeartbeatInterval:   heartbeatInterval,
		ClaimInterval:       claimInterval,
		JobTimeout:          jobTimeout,
	}
	// Default: masscan (works in Docker where zmap often hangs). Set SCAN_ENGINE=zmap to use zmap.
	var portEngine engine.PortDiscoveryEngine
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SCAN_ENGINE"))) {
	case "zmap":
		portEngine = zmap.NewAdapter()
		log.Println("scan-worker: using engine zmap")
	default:
		portEngine = masscan.NewAdapter()
		log.Println("scan-worker: using engine masscan")
	}
	r := runner.NewRunner(cfg, portEngine)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("scan-worker: shutting down")
		cancel()
	}()
	r.Run(ctx)
}

package runner

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/martinezpascualdani/heimdall/internal/scan-worker/client"
	"github.com/martinezpascualdani/heimdall/internal/scan-worker/engine"
	"github.com/martinezpascualdani/heimdall/internal/scan-worker/engine/masscan"
)

// Config holds runner configuration.
type Config struct {
	ExecutionServiceURL string
	Name                string
	Region              string
	Version             string
	Capabilities        []string
	MaxConcurrency      int
	HeartbeatInterval   time.Duration
	ClaimInterval       time.Duration
	JobTimeout          time.Duration
}

func (c *Config) setDefaults() {
	if c.MaxConcurrency <= 0 {
		c.MaxConcurrency = 1
	}
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = 30 * time.Second
	}
	if c.ClaimInterval <= 0 {
		c.ClaimInterval = 5 * time.Second
	}
	if c.JobTimeout <= 0 {
		c.JobTimeout = 10 * time.Minute
	}
	if len(c.Capabilities) == 0 {
		c.Capabilities = []string{"discovery-basic", "portscan-basic", "portscan-full", "portscan-medium", "discovery-plus-ports"}
	}
}

// Runner runs the worker loop: register, heartbeat, claim, execute, report.
type Runner struct {
	Config Config
	Engine engine.PortDiscoveryEngine
	client *client.Client
	logger *log.Logger
	mu     sync.Mutex
	workerID uuid.UUID
}

// NewRunner creates a runner. If Engine is nil, a default Masscan adapter is used.
func NewRunner(cfg Config, portEngine engine.PortDiscoveryEngine) *Runner {
	cfg.setDefaults()
	if portEngine == nil {
		portEngine = masscan.NewAdapter()
	}
	return &Runner{
		Config: cfg,
		Engine: portEngine,
		client: client.NewClient(cfg.ExecutionServiceURL, 30*time.Second),
		logger: log.Default(),
	}
}

// SetLogger sets the logger (optional).
func (r *Runner) SetLogger(l *log.Logger) {
	if l != nil {
		r.logger = l
	}
}

// WorkerID returns the registered worker ID (valid after Run or after first successful register).
func (r *Runner) WorkerID() uuid.UUID {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.workerID
}

// Run registers the worker (with retries), starts heartbeat and claim loops, and blocks until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) {
	for {
		resp, err := r.client.Register(ctx, client.RegisterRequest{
			Name:           r.Config.Name,
			Region:         r.Config.Region,
			Version:        r.Config.Version,
			Capabilities:   r.Config.Capabilities,
			MaxConcurrency: r.Config.MaxConcurrency,
		})
		if err != nil {
			r.logger.Printf("runner: register failed: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
				continue
			}
		}
		r.mu.Lock()
		r.workerID = resp.WorkerID
		r.mu.Unlock()
		r.logger.Printf("runner: registered worker_id=%s", resp.WorkerID)
		break
	}

	go r.heartbeatLoop(ctx)
	// Run MaxConcurrency claim loops in parallel so we process up to N jobs at once
	for i := 0; i < r.Config.MaxConcurrency; i++ {
		go r.claimLoop(ctx)
	}
	<-ctx.Done()
}

func (r *Runner) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(r.Config.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			wid := r.WorkerID()
			if wid == uuid.Nil {
				continue
			}
			if err := r.client.Heartbeat(ctx, wid, nil); err != nil {
				r.logger.Printf("runner: heartbeat failed: %v", err)
			}
		}
	}
}

// claimLoop runs in a single goroutine; run MaxConcurrency of these to process jobs in parallel.
func (r *Runner) claimLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			wid := r.WorkerID()
			if wid == uuid.Nil {
				time.Sleep(r.Config.ClaimInterval)
				continue
			}
			job, err := r.client.Claim(ctx, wid, r.Config.Capabilities)
			if err != nil {
				r.logger.Printf("runner: claim failed: %v", err)
				time.Sleep(r.Config.ClaimInterval)
				continue
			}
			if job == nil {
				time.Sleep(r.Config.ClaimInterval)
				continue
			}
			r.logger.Printf("runner: claimed job job_id=%s execution_id=%s", job.ID, job.ExecutionID)
			r.runJob(ctx, job)
		}
	}
}

func (r *Runner) runJob(ctx context.Context, job *client.ClaimedJob) {
	jobCtx, cancel := context.WithTimeout(ctx, r.Config.JobTimeout)
	defer cancel()

	// Renew lease periodically while the job runs so it doesn't get requeued while worker is still online
	renewCtx, renewCancel := context.WithCancel(ctx)
	defer renewCancel()
	go func() {
		ticker := time.NewTicker(90 * time.Second) // before 5min lease expiry
		defer ticker.Stop()
		for {
			select {
			case <-renewCtx.Done():
				return
			case <-ticker.C:
				if err := r.client.Renew(renewCtx, job.ID, r.WorkerID(), job.LeaseID); err != nil {
					r.logger.Printf("runner: lease renew failed for job %s: %v", job.ID, err)
				}
			}
		}
	}()

	payload, err := engine.ParseJobPayload(job.Payload)
	if err != nil {
		r.logger.Printf("runner: job job_id=%s invalid payload: %v", job.ID, err)
		_ = r.client.Fail(ctx, job.ID, r.WorkerID(), job.LeaseID, "invalid payload: "+err.Error())
		return
	}

	r.logger.Printf("runner: running job job_id=%s (%d prefixes)", job.ID, len(payload.Prefixes))
	result, err := r.Engine.Run(jobCtx, payload)
	if err != nil {
		r.logger.Printf("runner: job job_id=%s failed: %v", job.ID, err)
		_ = r.client.Fail(ctx, job.ID, r.WorkerID(), job.LeaseID, err.Error())
		return
	}

	obsCount := 0
	if result != nil {
		obsCount = len(result.Observations)
	}
	r.logger.Printf("runner: job job_id=%s completed (%d observations)", job.ID, obsCount)
	resultSummary, _ := json.Marshal(result)
	if err := r.client.Complete(ctx, job.ID, r.WorkerID(), job.LeaseID, resultSummary); err != nil {
		r.logger.Printf("runner: complete failed for job %s: %v", job.ID, err)
	}
}

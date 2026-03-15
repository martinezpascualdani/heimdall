package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/martinezpascualdani/heimdall/internal/execution-service/domain"
)

// Store is the persistence interface for the scheduler.
type Store interface {
	ListJobsWithExpiredLease(limit int) ([]*domain.ExecutionJob, error)
	ListJobsByAssignedWorker(workerID uuid.UUID, limit int) ([]*domain.ExecutionJob, error)
	ListWorkersWithStaleHeartbeat(threshold time.Time, limit int) ([]uuid.UUID, error)
	UpdateWorkerStatus(id uuid.UUID, status string) error
	RequeueJob(jobID uuid.UUID) (failed bool, err error)
}

// Scheduler runs periodic lease expiry and worker heartbeat timeout handling.
type Scheduler struct {
	Store            Store
	HeartbeatTimeout time.Duration
	LeaseBatchSize   int
	WorkerBatchSize  int
	Interval         time.Duration
	logger           *log.Logger
}

// NewScheduler creates a scheduler. Interval is how often the tick runs; heartbeatTimeout is used to consider workers offline.
func NewScheduler(store Store, heartbeatTimeout, interval time.Duration) *Scheduler {
	if heartbeatTimeout <= 0 {
		heartbeatTimeout = 2 * time.Minute
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Scheduler{
		Store:            store,
		HeartbeatTimeout: heartbeatTimeout,
		LeaseBatchSize:   100,
		WorkerBatchSize:  50,
		Interval:         interval,
		logger:           log.Default(),
	}
}

// SetLogger sets the logger (optional).
func (s *Scheduler) SetLogger(l *log.Logger) {
	if l != nil {
		s.logger = l
	}
}

// Run runs the tick loop until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	// 1) Requeue jobs with expired lease
	jobs, err := s.Store.ListJobsWithExpiredLease(s.LeaseBatchSize)
	if err != nil {
		s.logger.Printf("scheduler: list expired leases: %v", err)
		return
	}
	for _, j := range jobs {
		failed, err := s.Store.RequeueJob(j.ID)
		if err != nil {
			s.logger.Printf("scheduler: requeue job %s: %v", j.ID, err)
			continue
		}
		if failed {
			s.logger.Printf("scheduler: job %s marked failed (max attempts)", j.ID)
		}
	}

	// 2) Mark workers with stale heartbeat as offline and requeue their jobs
	threshold := time.Now().Add(-s.HeartbeatTimeout)
	workerIDs, err := s.Store.ListWorkersWithStaleHeartbeat(threshold, s.WorkerBatchSize)
	if err != nil {
		s.logger.Printf("scheduler: list stale workers: %v", err)
		return
	}
	for _, wid := range workerIDs {
		if err := s.Store.UpdateWorkerStatus(wid, domain.WorkerStatusOffline); err != nil {
			s.logger.Printf("scheduler: mark worker %s offline: %v", wid, err)
			continue
		}
		jobs, err := s.Store.ListJobsByAssignedWorker(wid, 1000)
		if err != nil {
			s.logger.Printf("scheduler: list jobs for worker %s: %v", wid, err)
			continue
		}
		for _, j := range jobs {
			failed, err := s.Store.RequeueJob(j.ID)
			if err != nil {
				s.logger.Printf("scheduler: requeue job %s (worker offline): %v", j.ID, err)
				continue
			}
			if failed {
				s.logger.Printf("scheduler: job %s marked failed (max attempts)", j.ID)
			}
		}
	}
}

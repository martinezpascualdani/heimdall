package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/martinezpascualdani/heimdall/internal/execution-service/domain"
	"github.com/martinezpascualdani/heimdall/pkg/events"
)

// PostgresStore implements execution-service persistence.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore opens a connection and runs migrations.
func NewPostgresStore(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	if err := migrateExecutionService(db); err != nil {
		return nil, err
	}
	return &PostgresStore{db: db}, nil
}

func migrateExecutionService(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS workers (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name TEXT DEFAULT '',
			region TEXT DEFAULT '',
			version TEXT DEFAULT '',
			capabilities JSONB NOT NULL DEFAULT '[]',
			status VARCHAR(32) NOT NULL DEFAULT 'online',
			last_heartbeat_at TIMESTAMPTZ,
			max_concurrency INT NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_workers_status ON workers(status);
		CREATE INDEX IF NOT EXISTS idx_workers_last_heartbeat_at ON workers(last_heartbeat_at);

		CREATE TABLE IF NOT EXISTS executions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			run_id UUID NOT NULL,
			campaign_id UUID NOT NULL,
			target_id UUID NOT NULL,
			target_materialization_id UUID NOT NULL,
			scan_profile_slug TEXT NOT NULL,
			scan_profile_config JSONB,
			status VARCHAR(32) NOT NULL,
			total_jobs INT NOT NULL DEFAULT 0,
			completed_jobs INT NOT NULL DEFAULT 0,
			failed_jobs INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			completed_at TIMESTAMPTZ,
			error_summary TEXT DEFAULT ''
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_executions_run_id ON executions(run_id);
		CREATE INDEX IF NOT EXISTS idx_executions_status ON executions(status);
		CREATE INDEX IF NOT EXISTS idx_executions_campaign_id ON executions(campaign_id);

		CREATE TABLE IF NOT EXISTS execution_jobs (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			execution_id UUID NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
			payload JSONB NOT NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'pending',
			assigned_worker_id UUID REFERENCES workers(id),
			lease_expires_at TIMESTAMPTZ,
			lease_id TEXT DEFAULT '',
			attempt INT NOT NULL DEFAULT 0,
			max_attempts INT NOT NULL DEFAULT 3,
			result_summary JSONB,
			error_message TEXT DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			started_at TIMESTAMPTZ,
			completed_at TIMESTAMPTZ
		);
		CREATE INDEX IF NOT EXISTS idx_execution_jobs_execution_id ON execution_jobs(execution_id);
		CREATE INDEX IF NOT EXISTS idx_execution_jobs_status ON execution_jobs(status);
		CREATE INDEX IF NOT EXISTS idx_execution_jobs_assigned_worker_id ON execution_jobs(assigned_worker_id);
		CREATE INDEX IF NOT EXISTS idx_execution_jobs_pending_claim ON execution_jobs(execution_id, status) WHERE status = 'pending';

		CREATE TABLE IF NOT EXISTS outbox_events (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			aggregate_type VARCHAR(64) NOT NULL,
			aggregate_id UUID NOT NULL,
			event_type VARCHAR(64) NOT NULL,
			payload JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			published_at TIMESTAMPTZ,
			stream_id TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_outbox_events_published_at ON outbox_events(published_at) WHERE published_at IS NULL;
	`)
	return err
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) Ping() error {
	return s.db.Ping()
}

func jsonbArg(b json.RawMessage) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}

func capabilitiesArg(c []string) interface{} {
	if c == nil {
		return []byte("[]")
	}
	b, _ := json.Marshal(c)
	return b
}

// Workers

func (s *PostgresStore) CreateWorker(w *domain.Worker) error {
	w.CreatedAt = time.Now()
	w.UpdatedAt = w.CreatedAt
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	if w.Status == "" {
		w.Status = domain.WorkerStatusOnline
	}
	_, err := s.db.Exec(`
		INSERT INTO workers (id, name, region, version, capabilities, status, last_heartbeat_at, max_concurrency, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, w.ID, w.Name, w.Region, w.Version, capabilitiesArg(w.Capabilities), w.Status, w.LastHeartbeatAt, w.MaxConcurrency, w.CreatedAt, w.UpdatedAt)
	return err
}

func (s *PostgresStore) GetWorkerByID(id uuid.UUID) (*domain.Worker, error) {
	var w domain.Worker
	var capsRaw []byte
	var lastHB pq.NullTime
	err := s.db.QueryRow(`
		SELECT id, name, region, version, capabilities, status, last_heartbeat_at, max_concurrency, created_at, updated_at
		FROM workers WHERE id = $1
	`, id).Scan(&w.ID, &w.Name, &w.Region, &w.Version, &capsRaw, &w.Status, &lastHB, &w.MaxConcurrency, &w.CreatedAt, &w.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(capsRaw, &w.Capabilities)
	if lastHB.Valid {
		w.LastHeartbeatAt = &lastHB.Time
	}
	return &w, nil
}

func (s *PostgresStore) UpdateWorkerHeartbeat(id uuid.UUID, at time.Time) error {
	_, err := s.db.Exec(`
		UPDATE workers SET last_heartbeat_at = $1, updated_at = NOW() WHERE id = $2
	`, at, id)
	return err
}

func (s *PostgresStore) UpdateWorkerStatus(id uuid.UUID, status string) error {
	_, err := s.db.Exec(`UPDATE workers SET status = $1, updated_at = NOW() WHERE id = $2`, status, id)
	return err
}

func (s *PostgresStore) UpdateWorker(id uuid.UUID, capabilities []string, maxConcurrency int) error {
	_, err := s.db.Exec(`
		UPDATE workers SET capabilities = $1, max_concurrency = $2, updated_at = NOW() WHERE id = $3
	`, capabilitiesArg(capabilities), maxConcurrency, id)
	return err
}

// CountActiveJobsByWorker returns the number of jobs in assigned or running for the worker (for current_load derivation).
func (s *PostgresStore) CountActiveJobsByWorker(workerID uuid.UUID) (int, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM execution_jobs
		WHERE assigned_worker_id = $1 AND status IN ('assigned', 'running')
	`, workerID).Scan(&n)
	return n, err
}

// Executions

func (s *PostgresStore) CreateExecution(e *domain.Execution) error {
	e.CreatedAt = time.Now()
	e.UpdatedAt = e.CreatedAt
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	_, err := s.db.Exec(`
		INSERT INTO executions (id, run_id, campaign_id, target_id, target_materialization_id, scan_profile_slug, scan_profile_config, status, total_jobs, completed_jobs, failed_jobs, created_at, updated_at, completed_at, error_summary)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, e.ID, e.RunID, e.CampaignID, e.TargetID, e.TargetMaterializationID, e.ScanProfileSlug, jsonbArg(e.ScanProfileConfig), e.Status, e.TotalJobs, e.CompletedJobs, e.FailedJobs, e.CreatedAt, e.UpdatedAt, e.CompletedAt, e.ErrorSummary)
	return err
}

// CreateExecutionWithJobs creates an execution and all jobs in a single transaction. Execution status is set to running and total_jobs to len(jobs).
func (s *PostgresStore) CreateExecutionWithJobs(e *domain.Execution, jobs []*domain.ExecutionJob) error {
	e.CreatedAt = time.Now()
	e.UpdatedAt = e.CreatedAt
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	e.Status = domain.ExecutionStatusPlanning
	e.TotalJobs = len(jobs)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
		INSERT INTO executions (id, run_id, campaign_id, target_id, target_materialization_id, scan_profile_slug, scan_profile_config, status, total_jobs, completed_jobs, failed_jobs, created_at, updated_at, completed_at, error_summary)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, e.ID, e.RunID, e.CampaignID, e.TargetID, e.TargetMaterializationID, e.ScanProfileSlug, jsonbArg(e.ScanProfileConfig), e.Status, e.TotalJobs, e.CompletedJobs, e.FailedJobs, e.CreatedAt, e.UpdatedAt, e.CompletedAt, e.ErrorSummary)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		j.ExecutionID = e.ID
		j.CreatedAt = time.Now()
		j.UpdatedAt = j.CreatedAt
		if j.ID == uuid.Nil {
			j.ID = uuid.New()
		}
		if j.Status == "" {
			j.Status = domain.JobStatusPending
		}
		if j.MaxAttempts == 0 {
			j.MaxAttempts = 3
		}
		_, err = tx.Exec(`
			INSERT INTO execution_jobs (id, execution_id, payload, status, attempt, max_attempts, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, j.ID, j.ExecutionID, jsonbArg(j.Payload), j.Status, j.Attempt, j.MaxAttempts, j.CreatedAt, j.UpdatedAt)
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(`UPDATE executions SET status = $1, updated_at = NOW() WHERE id = $2`, domain.ExecutionStatusRunning, e.ID)
	if err != nil {
		return err
	}
	e.Status = domain.ExecutionStatusRunning
	return tx.Commit()
}

func (s *PostgresStore) GetExecutionByID(id uuid.UUID) (*domain.Execution, error) {
	var e domain.Execution
	var config []byte
	var completedAt pq.NullTime
	err := s.db.QueryRow(`
		SELECT id, run_id, campaign_id, target_id, target_materialization_id, scan_profile_slug, scan_profile_config, status, total_jobs, completed_jobs, failed_jobs, created_at, updated_at, completed_at, error_summary
		FROM executions WHERE id = $1
	`, id).Scan(&e.ID, &e.RunID, &e.CampaignID, &e.TargetID, &e.TargetMaterializationID, &e.ScanProfileSlug, &config, &e.Status, &e.TotalJobs, &e.CompletedJobs, &e.FailedJobs, &e.CreatedAt, &e.UpdatedAt, &completedAt, &e.ErrorSummary)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.ScanProfileConfig = config
	if completedAt.Valid {
		e.CompletedAt = &completedAt.Time
	}
	return &e, nil
}

func (s *PostgresStore) GetExecutionByRunID(runID uuid.UUID) (*domain.Execution, error) {
	var e domain.Execution
	var config []byte
	var completedAt pq.NullTime
	err := s.db.QueryRow(`
		SELECT id, run_id, campaign_id, target_id, target_materialization_id, scan_profile_slug, scan_profile_config, status, total_jobs, completed_jobs, failed_jobs, created_at, updated_at, completed_at, error_summary
		FROM executions WHERE run_id = $1
	`, runID).Scan(&e.ID, &e.RunID, &e.CampaignID, &e.TargetID, &e.TargetMaterializationID, &e.ScanProfileSlug, &config, &e.Status, &e.TotalJobs, &e.CompletedJobs, &e.FailedJobs, &e.CreatedAt, &e.UpdatedAt, &completedAt, &e.ErrorSummary)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.ScanProfileConfig = config
	if completedAt.Valid {
		e.CompletedAt = &completedAt.Time
	}
	return &e, nil
}

func (s *PostgresStore) UpdateExecution(e *domain.Execution) error {
	e.UpdatedAt = time.Now()
	_, err := s.db.Exec(`
		UPDATE executions SET status = $1, total_jobs = $2, completed_jobs = $3, failed_jobs = $4, updated_at = $5, completed_at = $6, error_summary = $7
		WHERE id = $8
	`, e.Status, e.TotalJobs, e.CompletedJobs, e.FailedJobs, e.UpdatedAt, e.CompletedAt, e.ErrorSummary, e.ID)
	return err
}

func (s *PostgresStore) ListExecutions(limit, offset int, runID, campaignID *uuid.UUID, status string) ([]*domain.Execution, int, error) {
	qCount := `SELECT COUNT(*) FROM executions WHERE 1=1`
	q := `SELECT id, run_id, campaign_id, target_id, target_materialization_id, scan_profile_slug, scan_profile_config, status, total_jobs, completed_jobs, failed_jobs, created_at, updated_at, completed_at, error_summary FROM executions WHERE 1=1`
	args := []interface{}{}
	n := 0
	if runID != nil {
		n++
		qCount += fmt.Sprintf(" AND run_id = $%d", n)
		q += fmt.Sprintf(" AND run_id = $%d", n)
		args = append(args, *runID)
	}
	if campaignID != nil {
		n++
		qCount += fmt.Sprintf(" AND campaign_id = $%d", n)
		q += fmt.Sprintf(" AND campaign_id = $%d", n)
		args = append(args, *campaignID)
	}
	if status != "" {
		n++
		qCount += fmt.Sprintf(" AND status = $%d", n)
		q += fmt.Sprintf(" AND status = $%d", n)
		args = append(args, status)
	}
	var total int
	if err := s.db.QueryRow(qCount, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	n++
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", n, n+1)
	args = append(args, limit, offset)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.Execution
	for rows.Next() {
		var e domain.Execution
		var config []byte
		var completedAt pq.NullTime
		if err := rows.Scan(&e.ID, &e.RunID, &e.CampaignID, &e.TargetID, &e.TargetMaterializationID, &e.ScanProfileSlug, &config, &e.Status, &e.TotalJobs, &e.CompletedJobs, &e.FailedJobs, &e.CreatedAt, &e.UpdatedAt, &completedAt, &e.ErrorSummary); err != nil {
			return nil, 0, err
		}
		e.ScanProfileConfig = config
		if completedAt.Valid {
			e.CompletedAt = &completedAt.Time
		}
		out = append(out, &e)
	}
	return out, total, rows.Err()
}

// Jobs

func (s *PostgresStore) CreateJob(j *domain.ExecutionJob) error {
	j.CreatedAt = time.Now()
	j.UpdatedAt = j.CreatedAt
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	if j.MaxAttempts == 0 {
		j.MaxAttempts = 3
	}
	_, err := s.db.Exec(`
		INSERT INTO execution_jobs (id, execution_id, payload, status, assigned_worker_id, lease_expires_at, lease_id, attempt, max_attempts, result_summary, error_message, created_at, updated_at, started_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, j.ID, j.ExecutionID, jsonbArg(j.Payload), j.Status, nullableUUID(j.AssignedWorkerID), j.LeaseExpiresAt, j.LeaseID, j.Attempt, j.MaxAttempts, jsonbArg(j.ResultSummary), j.ErrorMessage, j.CreatedAt, j.UpdatedAt, j.StartedAt, j.CompletedAt)
	return err
}

func nullableUUID(u *uuid.UUID) interface{} {
	if u == nil || *u == uuid.Nil {
		return nil
	}
	return *u
}

func (s *PostgresStore) GetJobByID(id uuid.UUID) (*domain.ExecutionJob, error) {
	var j domain.ExecutionJob
	var workerID uuid.NullUUID
	var leaseExp pq.NullTime
	var resultSummary []byte
	var startedAt, completedAt pq.NullTime
	err := s.db.QueryRow(`
		SELECT id, execution_id, payload, status, assigned_worker_id, lease_expires_at, lease_id, attempt, max_attempts, result_summary, error_message, created_at, updated_at, started_at, completed_at
		FROM execution_jobs WHERE id = $1
	`, id).Scan(&j.ID, &j.ExecutionID, &j.Payload, &j.Status, &workerID, &leaseExp, &j.LeaseID, &j.Attempt, &j.MaxAttempts, &resultSummary, &j.ErrorMessage, &j.CreatedAt, &j.UpdatedAt, &startedAt, &completedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if workerID.Valid {
		j.AssignedWorkerID = &workerID.UUID
	}
	if leaseExp.Valid {
		j.LeaseExpiresAt = &leaseExp.Time
	}
	j.ResultSummary = resultSummary
	if startedAt.Valid {
		j.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		j.CompletedAt = &completedAt.Time
	}
	return &j, nil
}

// ClaimJob selects a pending job compatible with scanProfileSlug, assigns it to workerID with lease, in one transaction. Uses FOR UPDATE SKIP LOCKED.
// Returns nil if no suitable job or worker would exceed max_concurrency.
func (s *PostgresStore) ClaimJob(workerID uuid.UUID, maxConcurrency int, scanProfileSlug string, leaseDuration time.Duration) (*domain.ExecutionJob, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var activeCount int
	if err := tx.QueryRow(`
		SELECT COUNT(*) FROM execution_jobs WHERE assigned_worker_id = $1 AND status IN ('assigned', 'running')
	`, workerID).Scan(&activeCount); err != nil {
		return nil, err
	}
	if activeCount >= maxConcurrency {
		return nil, nil
	}

	leaseExpires := time.Now().Add(leaseDuration)
	leaseID := uuid.New().String()

	var j domain.ExecutionJob
	err = tx.QueryRow(`
		SELECT id, execution_id, payload, status, attempt, max_attempts, created_at, updated_at
		FROM execution_jobs
		WHERE status = 'pending'
		AND execution_id IN (SELECT id FROM executions WHERE status = 'running' AND scan_profile_slug = $1)
		ORDER BY created_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`, scanProfileSlug).Scan(&j.ID, &j.ExecutionID, &j.Payload, &j.Status, &j.Attempt, &j.MaxAttempts, &j.CreatedAt, &j.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(`
		UPDATE execution_jobs SET status = 'assigned', assigned_worker_id = $1, lease_expires_at = $2, lease_id = $3, updated_at = NOW()
		WHERE id = $4
	`, workerID, leaseExpires, leaseID, j.ID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	j.Status = domain.JobStatusAssigned
	j.AssignedWorkerID = &workerID
	j.LeaseExpiresAt = &leaseExpires
	j.LeaseID = leaseID
	return &j, nil
}

// JobComplete marks the job as completed, updates result_summary and completed_at, and increments execution completed_jobs.
// Returns error if job not found, not assigned to worker, or lease_id mismatch.
func (s *PostgresStore) JobComplete(jobID, workerID uuid.UUID, leaseID string, resultSummary json.RawMessage) error {
	j, err := s.GetJobByID(jobID)
	if err != nil || j == nil {
		return err
	}
	if j.AssignedWorkerID == nil || *j.AssignedWorkerID != workerID || j.LeaseID != leaseID {
		return fmt.Errorf("job not assigned to worker or lease mismatch")
	}
	if j.Status != domain.JobStatusAssigned && j.Status != domain.JobStatusRunning {
		return fmt.Errorf("job not in assigned/running state")
	}
	now := time.Now()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
		UPDATE execution_jobs SET status = $1, result_summary = $2, completed_at = $3, updated_at = $3 WHERE id = $4
	`, domain.JobStatusCompleted, jsonbArg(resultSummary), now, jobID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		UPDATE executions SET completed_jobs = completed_jobs + 1, updated_at = $1
		WHERE id = (SELECT execution_id FROM execution_jobs WHERE id = $2)
	`, now, jobID)
	if err != nil {
		return err
	}
	_, _ = tx.Exec(`
		UPDATE executions SET status = 'completed', completed_at = $1
		WHERE id = (SELECT execution_id FROM execution_jobs WHERE id = $2)
		AND total_jobs = completed_jobs + 1
	`, now, jobID)

	// Outbox: same tx, insert job_completed event for inventory-service consumer
	var runID, campaignID, targetID, targetMatID uuid.UUID
	var scanProfileSlug string
	err = tx.QueryRow(`
		SELECT run_id, campaign_id, target_id, target_materialization_id, scan_profile_slug
		FROM executions WHERE id = $1
	`, j.ExecutionID).Scan(&runID, &campaignID, &targetID, &targetMatID, &scanProfileSlug)
	if err != nil {
		return err
	}
	observedAt := now
	if j.CompletedAt != nil {
		observedAt = *j.CompletedAt
	}
	var observations []events.JobCompletedObservation
	if len(resultSummary) > 0 {
		var result struct {
			Observations []events.JobCompletedObservation `json:"observations"`
		}
		if err := json.Unmarshal(resultSummary, &result); err == nil && result.Observations != nil {
			observations = result.Observations
		}
	}
	evt := events.JobCompletedEvent{
		EventType:               events.JobCompletedEventType,
		PayloadVersion:          events.JobCompletedPayloadVersion,
		ExecutionID:             j.ExecutionID.String(),
		JobID:                   j.ID.String(),
		RunID:                   runID.String(),
		CampaignID:              campaignID.String(),
		TargetID:                targetID.String(),
		TargetMaterializationID: targetMatID.String(),
		ScanProfileSlug:         scanProfileSlug,
		ObservedAt:              observedAt,
		Observations:            observations,
	}
	payloadBytes, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, created_at, published_at, stream_id)
		VALUES ('job', $1, 'job_completed', $2, NOW(), NULL, NULL)
	`, jobID, jsonbArg(json.RawMessage(payloadBytes)))
	if err != nil {
		return err
	}
	return tx.Commit()
}

// JobFail marks the job as failed, updates error_message and completed_at, increments execution failed_jobs.
func (s *PostgresStore) JobFail(jobID, workerID uuid.UUID, leaseID string, errorMessage string) error {
	j, err := s.GetJobByID(jobID)
	if err != nil || j == nil {
		return err
	}
	if j.AssignedWorkerID == nil || *j.AssignedWorkerID != workerID || j.LeaseID != leaseID {
		return fmt.Errorf("job not assigned to worker or lease mismatch")
	}
	if j.Status != domain.JobStatusAssigned && j.Status != domain.JobStatusRunning {
		return fmt.Errorf("job not in assigned/running state")
	}
	now := time.Now()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
		UPDATE execution_jobs SET status = $1, error_message = $2, completed_at = $3, updated_at = $3 WHERE id = $4
	`, domain.JobStatusFailed, errorMessage, now, jobID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		UPDATE executions SET failed_jobs = failed_jobs + 1, updated_at = $1
		WHERE id = (SELECT execution_id FROM execution_jobs WHERE id = $2)
	`, now, jobID)
	if err != nil {
		return err
	}
	_, _ = tx.Exec(`
		UPDATE executions SET status = 'completed', completed_at = $1
		WHERE id = (SELECT execution_id FROM execution_jobs WHERE id = $2)
		AND total_jobs = completed_jobs + failed_jobs + 1
	`, now, jobID)
	return tx.Commit()
}

// JobRenew extends lease_expires_at for the job if worker and lease_id match.
func (s *PostgresStore) JobRenew(jobID, workerID uuid.UUID, leaseID string, newExpiresAt time.Time) (ok bool, err error) {
	j, err := s.GetJobByID(jobID)
	if err != nil || j == nil {
		return false, err
	}
	if j.AssignedWorkerID == nil || *j.AssignedWorkerID != workerID || j.LeaseID != leaseID {
		return false, nil
	}
	if j.Status != domain.JobStatusAssigned && j.Status != domain.JobStatusRunning {
		return false, nil
	}
	res, err := s.db.Exec(`
		UPDATE execution_jobs SET lease_expires_at = $1, updated_at = NOW() WHERE id = $2 AND assigned_worker_id = $3 AND lease_id = $4
	`, newExpiresAt, jobID, workerID, leaseID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *PostgresStore) UpdateJob(j *domain.ExecutionJob) error {
	j.UpdatedAt = time.Now()
	_, err := s.db.Exec(`
		UPDATE execution_jobs SET status = $1, assigned_worker_id = $2, lease_expires_at = $3, lease_id = $4, attempt = $5, result_summary = $6, error_message = $7, updated_at = $8, started_at = $9, completed_at = $10
		WHERE id = $11
	`, j.Status, nullableUUID(j.AssignedWorkerID), j.LeaseExpiresAt, j.LeaseID, j.Attempt, jsonbArg(j.ResultSummary), j.ErrorMessage, j.UpdatedAt, j.StartedAt, j.CompletedAt, j.ID)
	return err
}

func (s *PostgresStore) ListJobsByExecution(executionID uuid.UUID, limit, offset int) ([]*domain.ExecutionJob, int, error) {
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM execution_jobs WHERE execution_id = $1`, executionID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`
		SELECT id, execution_id, payload, status, assigned_worker_id, lease_expires_at, lease_id, attempt, max_attempts, result_summary, error_message, created_at, updated_at, started_at, completed_at
		FROM execution_jobs WHERE execution_id = $1 ORDER BY created_at ASC LIMIT $2 OFFSET $3
	`, executionID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.ExecutionJob
	for rows.Next() {
		j, err := scanJobRow(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, j)
	}
	return out, total, rows.Err()
}

func scanJobRow(rows *sql.Rows) (*domain.ExecutionJob, error) {
	var j domain.ExecutionJob
	var workerID uuid.NullUUID
	var leaseExp pq.NullTime
	var resultSummary []byte
	var startedAt, completedAt pq.NullTime
	err := rows.Scan(&j.ID, &j.ExecutionID, &j.Payload, &j.Status, &workerID, &leaseExp, &j.LeaseID, &j.Attempt, &j.MaxAttempts, &resultSummary, &j.ErrorMessage, &j.CreatedAt, &j.UpdatedAt, &startedAt, &completedAt)
	if err != nil {
		return nil, err
	}
	if workerID.Valid {
		j.AssignedWorkerID = &workerID.UUID
	}
	if leaseExp.Valid {
		j.LeaseExpiresAt = &leaseExp.Time
	}
	j.ResultSummary = resultSummary
	if startedAt.Valid {
		j.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		j.CompletedAt = &completedAt.Time
	}
	return &j, nil
}

// RequeueJob sets job back to pending, clears assignment, increments attempt. If attempt > max_attempts, sets job to failed and updates execution.
func (s *PostgresStore) RequeueJob(jobID uuid.UUID) (failed bool, err error) {
	j, err := s.GetJobByID(jobID)
	if err != nil || j == nil {
		return false, err
	}
	if j.Status != domain.JobStatusAssigned && j.Status != domain.JobStatusRunning {
		return false, nil
	}
	attempt := j.Attempt + 1
	if attempt > j.MaxAttempts {
		now := time.Now()
		tx, err := s.db.Begin()
		if err != nil {
			return false, err
		}
		defer tx.Rollback()
		_, err = tx.Exec(`
			UPDATE execution_jobs SET status = $1, assigned_worker_id = NULL, lease_expires_at = NULL, lease_id = '', attempt = $2, error_message = 'max attempts exceeded', completed_at = $3, updated_at = $3 WHERE id = $4
		`, domain.JobStatusFailed, attempt, now, jobID)
		if err != nil {
			return false, err
		}
		_, _ = tx.Exec(`
			UPDATE executions SET failed_jobs = failed_jobs + 1, updated_at = $1
			WHERE id = (SELECT execution_id FROM execution_jobs WHERE id = $2)
		`, now, jobID)
		_, _ = tx.Exec(`
			UPDATE executions SET status = 'completed', completed_at = $1
			WHERE id = (SELECT execution_id FROM execution_jobs WHERE id = $2)
			AND total_jobs = completed_jobs + failed_jobs + 1
		`, now, jobID)
		return true, tx.Commit()
	}
	_, err = s.db.Exec(`
		UPDATE execution_jobs SET status = $1, assigned_worker_id = NULL, lease_expires_at = NULL, lease_id = '', attempt = $2, updated_at = NOW() WHERE id = $3
	`, domain.JobStatusPending, attempt, jobID)
	return false, err
}

// ListWorkersWithStaleHeartbeat returns workers with last_heartbeat_at < threshold (for scheduler to mark offline).
func (s *PostgresStore) ListWorkersWithStaleHeartbeat(threshold time.Time, limit int) ([]uuid.UUID, error) {
	rows, err := s.db.Query(`
		SELECT id FROM workers WHERE status = 'online' AND (last_heartbeat_at IS NULL OR last_heartbeat_at < $1) LIMIT $2
	`, threshold, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListJobsByAssignedWorker returns jobs in assigned or running for the given worker (for requeue when worker goes offline).
func (s *PostgresStore) ListJobsByAssignedWorker(workerID uuid.UUID, limit int) ([]*domain.ExecutionJob, error) {
	rows, err := s.db.Query(`
		SELECT id, execution_id, payload, status, assigned_worker_id, lease_expires_at, lease_id, attempt, max_attempts, result_summary, error_message, created_at, updated_at, started_at, completed_at
		FROM execution_jobs WHERE assigned_worker_id = $1 AND status IN ('assigned', 'running') LIMIT $2
	`, workerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.ExecutionJob
	for rows.Next() {
		j, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// RequeueFailedJobsForExecution sets all failed jobs of the execution (with attempt < max_attempts) back to pending,
// clears assignment and error_message, increments attempt. Updates execution: failed_jobs -= count, status = running.
// Returns the number of jobs requeued.
func (s *PostgresStore) RequeueFailedJobsForExecution(executionID uuid.UUID) (requeued int, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`
		UPDATE execution_jobs SET status = $1, assigned_worker_id = NULL, lease_expires_at = NULL, lease_id = '',
			error_message = '', completed_at = NULL, attempt = attempt + 1, updated_at = NOW()
		WHERE execution_id = $2 AND status = $3 AND attempt < max_attempts
	`, domain.JobStatusPending, executionID, domain.JobStatusFailed)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	requeued = int(n)
	if requeued == 0 {
		return 0, tx.Commit()
	}
	_, err = tx.Exec(`
		UPDATE executions SET failed_jobs = failed_jobs - $1, status = $2, completed_at = NULL, updated_at = NOW()
		WHERE id = $3
	`, requeued, domain.ExecutionStatusRunning, executionID)
	if err != nil {
		return 0, err
	}
	return requeued, tx.Commit()
}

// CancelExecution sets execution status to canceled and all non-terminal jobs (pending, assigned, running) to canceled.
// Returns the number of jobs canceled. Execution must exist and not already be in a terminal state (optional check in handler).
func (s *PostgresStore) CancelExecution(executionID uuid.UUID) (canceled int, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`
		UPDATE execution_jobs SET status = $1, assigned_worker_id = NULL, lease_expires_at = NULL, lease_id = '', updated_at = NOW()
		WHERE execution_id = $2 AND status IN ($3, $4, $5)
	`, domain.JobStatusCanceled, executionID, domain.JobStatusPending, domain.JobStatusAssigned, domain.JobStatusRunning)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	canceled = int(n)
	_, err = tx.Exec(`
		UPDATE executions SET status = $1, updated_at = NOW(), completed_at = NOW() WHERE id = $2
	`, domain.ExecutionStatusCanceled, executionID)
	if err != nil {
		return 0, err
	}
	return canceled, tx.Commit()
}

// ListJobsWithExpiredLease returns jobs in assigned or running with lease_expires_at < now (for scheduler requeue).
func (s *PostgresStore) ListJobsWithExpiredLease(limit int) ([]*domain.ExecutionJob, error) {
	rows, err := s.db.Query(`
		SELECT id, execution_id, payload, status, assigned_worker_id, lease_expires_at, lease_id, attempt, max_attempts, result_summary, error_message, created_at, updated_at, started_at, completed_at
		FROM execution_jobs WHERE status IN ('assigned', 'running') AND lease_expires_at IS NOT NULL AND lease_expires_at < NOW() LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.ExecutionJob
	for rows.Next() {
		j, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// ListWorkers returns workers with optional status filter.
func (s *PostgresStore) ListWorkers(limit, offset int, status string) ([]*domain.Worker, int, error) {
	qCount := `SELECT COUNT(*) FROM workers WHERE 1=1`
	q := `SELECT id, name, region, version, capabilities, status, last_heartbeat_at, max_concurrency, created_at, updated_at FROM workers WHERE 1=1`
	args := []interface{}{}
	n := 0
	if status != "" {
		n++
		qCount += fmt.Sprintf(" AND status = $%d", n)
		q += fmt.Sprintf(" AND status = $%d", n)
		args = append(args, status)
	}
	var total int
	if err := s.db.QueryRow(qCount, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	n++
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", n, n+1)
	args = append(args, limit, offset)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.Worker
	for rows.Next() {
		var w domain.Worker
		var capsRaw []byte
		var lastHB pq.NullTime
		if err := rows.Scan(&w.ID, &w.Name, &w.Region, &w.Version, &capsRaw, &w.Status, &lastHB, &w.MaxConcurrency, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal(capsRaw, &w.Capabilities)
		if lastHB.Valid {
			w.LastHeartbeatAt = &lastHB.Time
		}
		out = append(out, &w)
	}
	return out, total, rows.Err()
}

// FetchUnpublishedOutboxEvents returns up to limit unpublished outbox rows.
// It uses SELECT ... FOR UPDATE SKIP LOCKED in a short transaction; the lock is released on Commit(),
// so the same row could be fetched by another caller after this returns. To avoid double publication,
// v1 assumes a single active outbox-publisher per execution-service DB (e.g. one execution-service instance).
// For multiple publishers you would need a claim (e.g. publishing_started_at / claimed_by) updated in the same tx.
func (s *PostgresStore) FetchUnpublishedOutboxEvents(limit int) ([]events.OutboxEventRow, error) {
	if limit <= 0 {
		limit = 50
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`
		SELECT id, payload FROM outbox_events
		WHERE published_at IS NULL
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []events.OutboxEventRow
	for rows.Next() {
		var row events.OutboxEventRow
		if err := rows.Scan(&row.ID, &row.Payload); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// MarkOutboxPublished sets published_at and stream_id for the outbox event. Call only after XADD succeeds.
func (s *PostgresStore) MarkOutboxPublished(id uuid.UUID, streamID string) error {
	_, err := s.db.Exec(`
		UPDATE outbox_events SET published_at = NOW(), stream_id = $2 WHERE id = $1
	`, id, streamID)
	return err
}

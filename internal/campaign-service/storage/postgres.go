package storage

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/domain"
)

// PostgresStore implements campaign-service persistence.
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
	if err := migrateCampaignService(db); err != nil {
		return nil, err
	}
	return &PostgresStore{db: db}, nil
}

func migrateCampaignService(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS campaigns (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			active BOOLEAN NOT NULL DEFAULT true,
			target_id UUID NOT NULL,
			scan_profile_id UUID NOT NULL,
			schedule_type VARCHAR(32) NOT NULL,
			schedule_config JSONB,
			materialization_policy VARCHAR(32) NOT NULL,
			next_run_at TIMESTAMPTZ,
			run_once_done BOOLEAN NOT NULL DEFAULT false,
			concurrency_policy VARCHAR(32) NOT NULL DEFAULT 'allow',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_campaigns_active ON campaigns(active);
		CREATE INDEX IF NOT EXISTS idx_campaigns_next_run_at ON campaigns(next_run_at);
		CREATE INDEX IF NOT EXISTS idx_campaigns_scan_profile_id ON campaigns(scan_profile_id);

		CREATE TABLE IF NOT EXISTS scan_profiles (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name TEXT NOT NULL,
			slug VARCHAR(128) NOT NULL UNIQUE,
			description TEXT DEFAULT '',
			config JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_scan_profiles_slug ON scan_profiles(slug);

		CREATE TABLE IF NOT EXISTS campaign_runs (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			campaign_id UUID NOT NULL,
			target_id UUID NOT NULL,
			target_materialization_id UUID NOT NULL,
			scan_profile_id UUID NOT NULL,
			scan_profile_slug TEXT NOT NULL,
			scan_profile_config_snapshot JSONB,
			status VARCHAR(32) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			started_at TIMESTAMPTZ,
			completed_at TIMESTAMPTZ,
			dispatched_at TIMESTAMPTZ,
			dispatch_ref TEXT DEFAULT '',
			error_message TEXT DEFAULT '',
			stats JSONB
		);
		CREATE INDEX IF NOT EXISTS idx_campaign_runs_campaign_id ON campaign_runs(campaign_id);
		CREATE INDEX IF NOT EXISTS idx_campaign_runs_status ON campaign_runs(status);
		CREATE INDEX IF NOT EXISTS idx_campaign_runs_created_at ON campaign_runs(created_at DESC);
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

// CreateCampaign inserts a campaign.
func (s *PostgresStore) CreateCampaign(c *domain.Campaign) error {
	c.CreatedAt = time.Now()
	c.UpdatedAt = c.CreatedAt
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	if c.ConcurrencyPolicy == "" {
		c.ConcurrencyPolicy = domain.ConcurrencyPolicyAllow
	}
	_, err := s.db.Exec(`
		INSERT INTO campaigns (id, name, description, active, target_id, scan_profile_id, schedule_type, schedule_config, materialization_policy, next_run_at, run_once_done, concurrency_policy, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, c.ID, c.Name, c.Description, c.Active, c.TargetID, c.ScanProfileID, c.ScheduleType, jsonbArg(c.ScheduleConfig), c.MaterializationPolicy, c.NextRunAt, c.RunOnceDone, c.ConcurrencyPolicy, c.CreatedAt, c.UpdatedAt)
	return err
}

// GetCampaignByID returns a campaign by ID or nil if not found.
func (s *PostgresStore) GetCampaignByID(id uuid.UUID) (*domain.Campaign, error) {
	var c domain.Campaign
	var config []byte
	var nextRunAt pq.NullTime
	err := s.db.QueryRow(`
		SELECT id, name, description, active, target_id, scan_profile_id, schedule_type, schedule_config, materialization_policy, next_run_at, run_once_done, concurrency_policy, created_at, updated_at
		FROM campaigns WHERE id = $1
	`, id).Scan(&c.ID, &c.Name, &c.Description, &c.Active, &c.TargetID, &c.ScanProfileID, &c.ScheduleType, &config, &c.MaterializationPolicy, &nextRunAt, &c.RunOnceDone, &c.ConcurrencyPolicy, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.ScheduleConfig = config
	if nextRunAt.Valid {
		c.NextRunAt = &nextRunAt.Time
	}
	return &c, nil
}

// ListCampaigns returns campaigns with pagination. activeOnly filters to active=true when true.
func (s *PostgresStore) ListCampaigns(limit, offset int, activeOnly bool) ([]*domain.Campaign, int, error) {
	var total int
	q := `SELECT COUNT(*) FROM campaigns`
	if activeOnly {
		q += ` WHERE active = true`
	}
	if err := s.db.QueryRow(q).Scan(&total); err != nil {
		return nil, 0, err
	}
	q = `
		SELECT id, name, description, active, target_id, scan_profile_id, schedule_type, schedule_config, materialization_policy, next_run_at, run_once_done, concurrency_policy, created_at, updated_at
		FROM campaigns
	`
	args := []interface{}{}
	if activeOnly {
		q += ` WHERE active = true `
	}
	q += ` ORDER BY name ASC LIMIT $1 OFFSET $2`
	args = append(args, limit, offset)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.Campaign
	for rows.Next() {
		var c domain.Campaign
		var config []byte
		var nextRunAt pq.NullTime
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.Active, &c.TargetID, &c.ScanProfileID, &c.ScheduleType, &config, &c.MaterializationPolicy, &nextRunAt, &c.RunOnceDone, &c.ConcurrencyPolicy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, 0, err
		}
		c.ScheduleConfig = config
		if nextRunAt.Valid {
			c.NextRunAt = &nextRunAt.Time
		}
		out = append(out, &c)
	}
	return out, total, rows.Err()
}

// UpdateCampaign full replacement of campaign fields.
func (s *PostgresStore) UpdateCampaign(c *domain.Campaign) error {
	c.UpdatedAt = time.Now()
	_, err := s.db.Exec(`
		UPDATE campaigns SET name = $1, description = $2, active = $3, target_id = $4, scan_profile_id = $5, schedule_type = $6, schedule_config = $7, materialization_policy = $8, next_run_at = $9, run_once_done = $10, concurrency_policy = $11, updated_at = $12
		WHERE id = $13
	`, c.Name, c.Description, c.Active, c.TargetID, c.ScanProfileID, c.ScheduleType, jsonbArg(c.ScheduleConfig), c.MaterializationPolicy, c.NextRunAt, c.RunOnceDone, c.ConcurrencyPolicy, c.UpdatedAt, c.ID)
	return err
}

// SoftDeleteCampaign sets active = false. Idempotent.
func (s *PostgresStore) SoftDeleteCampaign(id uuid.UUID) error {
	_, err := s.db.Exec(`UPDATE campaigns SET active = false, updated_at = NOW() WHERE id = $1`, id)
	return err
}

// CreateScanProfile inserts a scan profile.
func (s *PostgresStore) CreateScanProfile(p *domain.ScanProfile) error {
	p.CreatedAt = time.Now()
	p.UpdatedAt = p.CreatedAt
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	_, err := s.db.Exec(`
		INSERT INTO scan_profiles (id, name, slug, description, config, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, p.ID, p.Name, p.Slug, p.Description, jsonbArg(p.Config), p.CreatedAt, p.UpdatedAt)
	return err
}

// GetScanProfileByID returns a scan profile by ID or nil.
func (s *PostgresStore) GetScanProfileByID(id uuid.UUID) (*domain.ScanProfile, error) {
	var p domain.ScanProfile
	var config []byte
	err := s.db.QueryRow(`
		SELECT id, name, slug, description, config, created_at, updated_at
		FROM scan_profiles WHERE id = $1
	`, id).Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &config, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.Config = config
	return &p, nil
}

// GetScanProfileBySlug returns a scan profile by slug or nil.
func (s *PostgresStore) GetScanProfileBySlug(slug string) (*domain.ScanProfile, error) {
	var p domain.ScanProfile
	var config []byte
	err := s.db.QueryRow(`
		SELECT id, name, slug, description, config, created_at, updated_at
		FROM scan_profiles WHERE slug = $1
	`, slug).Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &config, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.Config = config
	return &p, nil
}

// ListScanProfiles returns scan profiles with pagination.
func (s *PostgresStore) ListScanProfiles(limit, offset int) ([]*domain.ScanProfile, int, error) {
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM scan_profiles`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`
		SELECT id, name, slug, description, config, created_at, updated_at
		FROM scan_profiles ORDER BY name ASC LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.ScanProfile
	for rows.Next() {
		var p domain.ScanProfile
		var config []byte
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &config, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, 0, err
		}
		p.Config = config
		out = append(out, &p)
	}
	return out, total, rows.Err()
}

// UpdateScanProfile full replacement.
func (s *PostgresStore) UpdateScanProfile(p *domain.ScanProfile) error {
	p.UpdatedAt = time.Now()
	_, err := s.db.Exec(`
		UPDATE scan_profiles SET name = $1, slug = $2, description = $3, config = $4, updated_at = $5
		WHERE id = $6
	`, p.Name, p.Slug, p.Description, jsonbArg(p.Config), p.UpdatedAt, p.ID)
	return err
}

// CountCampaignsByScanProfileID returns how many campaigns (any) reference this scan profile.
func (s *PostgresStore) CountCampaignsByScanProfileID(scanProfileID uuid.UUID) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM campaigns WHERE scan_profile_id = $1`, scanProfileID).Scan(&n)
	return n, err
}

// DeleteScanProfile removes a scan profile. Call only when no campaigns reference it.
func (s *PostgresStore) DeleteScanProfile(id uuid.UUID) error {
	_, err := s.db.Exec(`DELETE FROM scan_profiles WHERE id = $1`, id)
	return err
}

// CreateCampaignRun inserts a run.
func (s *PostgresStore) CreateCampaignRun(r *domain.CampaignRun) error {
	r.CreatedAt = time.Now()
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	_, err := s.db.Exec(`
		INSERT INTO campaign_runs (id, campaign_id, target_id, target_materialization_id, scan_profile_id, scan_profile_slug, scan_profile_config_snapshot, status, created_at, started_at, completed_at, dispatched_at, dispatch_ref, error_message, stats)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, r.ID, r.CampaignID, r.TargetID, r.TargetMaterializationID, r.ScanProfileID, r.ScanProfileSlug, jsonbArg(r.ScanProfileConfigSnapshot), r.Status, r.CreatedAt, r.StartedAt, r.CompletedAt, r.DispatchedAt, r.DispatchRef, r.ErrorMessage, jsonbArg(r.Stats))
	return err
}

// GetCampaignRunByID returns a run by ID or nil.
func (s *PostgresStore) GetCampaignRunByID(id uuid.UUID) (*domain.CampaignRun, error) {
	var r domain.CampaignRun
	var configSnap, stats []byte
	var startedAt, completedAt, dispatchedAt pq.NullTime
	err := s.db.QueryRow(`
		SELECT id, campaign_id, target_id, target_materialization_id, scan_profile_id, scan_profile_slug, scan_profile_config_snapshot, status, created_at, started_at, completed_at, dispatched_at, dispatch_ref, error_message, stats
		FROM campaign_runs WHERE id = $1
	`, id).Scan(&r.ID, &r.CampaignID, &r.TargetID, &r.TargetMaterializationID, &r.ScanProfileID, &r.ScanProfileSlug, &configSnap, &r.Status, &r.CreatedAt, &startedAt, &completedAt, &dispatchedAt, &r.DispatchRef, &r.ErrorMessage, &stats)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.ScanProfileConfigSnapshot = configSnap
	r.Stats = stats
	if startedAt.Valid {
		r.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		r.CompletedAt = &completedAt.Time
	}
	if dispatchedAt.Valid {
		r.DispatchedAt = &dispatchedAt.Time
	}
	return &r, nil
}

// ListCampaignRuns returns runs for a campaign, ordered by created_at DESC.
func (s *PostgresStore) ListCampaignRuns(campaignID uuid.UUID, limit, offset int) ([]*domain.CampaignRun, int, error) {
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM campaign_runs WHERE campaign_id = $1`, campaignID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`
		SELECT id, campaign_id, target_id, target_materialization_id, scan_profile_id, scan_profile_slug, scan_profile_config_snapshot, status, created_at, started_at, completed_at, dispatched_at, dispatch_ref, error_message, stats
		FROM campaign_runs WHERE campaign_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3
	`, campaignID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.CampaignRun
	for rows.Next() {
		var r domain.CampaignRun
		var configSnap, stats []byte
		var startedAt, completedAt, dispatchedAt pq.NullTime
		if err := rows.Scan(&r.ID, &r.CampaignID, &r.TargetID, &r.TargetMaterializationID, &r.ScanProfileID, &r.ScanProfileSlug, &configSnap, &r.Status, &r.CreatedAt, &startedAt, &completedAt, &dispatchedAt, &r.DispatchRef, &r.ErrorMessage, &stats); err != nil {
			return nil, 0, err
		}
		r.ScanProfileConfigSnapshot = configSnap
		r.Stats = stats
		if startedAt.Valid {
			r.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			r.CompletedAt = &completedAt.Time
		}
		if dispatchedAt.Valid {
			r.DispatchedAt = &dispatchedAt.Time
		}
		out = append(out, &r)
	}
	return out, total, rows.Err()
}

// UpdateCampaignRun updates status and optional timestamps/ref/stats.
func (s *PostgresStore) UpdateCampaignRun(r *domain.CampaignRun) error {
	_, err := s.db.Exec(`
		UPDATE campaign_runs SET status = $1, started_at = $2, completed_at = $3, dispatched_at = $4, dispatch_ref = $5, error_message = $6, stats = $7
		WHERE id = $8
	`, r.Status, r.StartedAt, r.CompletedAt, r.DispatchedAt, r.DispatchRef, r.ErrorMessage, jsonbArg(r.Stats), r.ID)
	return err
}

// ListCampaignsDueForScheduler returns active once/interval campaigns that are due (next_run_at <= now or null for once with run_once_done=false). Respects forbid_if_active by excluding campaigns that have a run in pending/dispatching/dispatched.
func (s *PostgresStore) ListCampaignsDueForScheduler(limit int) ([]*domain.Campaign, error) {
	// Subquery: campaign_ids that have at least one run in pending, dispatching, or dispatched (for forbid_if_active).
	rows, err := s.db.Query(`
		SELECT c.id, c.name, c.description, c.active, c.target_id, c.scan_profile_id, c.schedule_type, c.schedule_config, c.materialization_policy, c.next_run_at, c.run_once_done, c.concurrency_policy, c.created_at, c.updated_at
		FROM campaigns c
		WHERE c.active = true
		  AND c.schedule_type IN ('once', 'interval')
		  AND (c.run_once_done = false OR c.schedule_type = 'interval')
		  AND (c.next_run_at IS NULL OR c.next_run_at <= NOW())
		  AND (
		    c.concurrency_policy = 'allow'
		    OR NOT EXISTS (
		      SELECT 1 FROM campaign_runs r WHERE r.campaign_id = c.id AND r.status IN ('pending', 'dispatching', 'dispatched')
		    )
		  )
		ORDER BY c.next_run_at ASC NULLS FIRST
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Campaign
	for rows.Next() {
		var c domain.Campaign
		var config []byte
		var nextRunAt pq.NullTime
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.Active, &c.TargetID, &c.ScanProfileID, &c.ScheduleType, &config, &c.MaterializationPolicy, &nextRunAt, &c.RunOnceDone, &c.ConcurrencyPolicy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.ScheduleConfig = config
		if nextRunAt.Valid {
			c.NextRunAt = &nextRunAt.Time
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// HasActiveRun returns true if the campaign has at least one run in pending, dispatching, or dispatched.
func (s *PostgresStore) HasActiveRun(campaignID uuid.UUID) (bool, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM campaign_runs WHERE campaign_id = $1 AND status IN ('pending', 'dispatching', 'dispatched')
	`, campaignID).Scan(&n)
	return n > 0, err
}

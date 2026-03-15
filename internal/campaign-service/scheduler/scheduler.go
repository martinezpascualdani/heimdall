package scheduler

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/domain"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/dispatch"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/targetclient"
)

// CampaignStore provides campaigns due for scheduling and updates.
type CampaignStore interface {
	ListCampaignsDueForScheduler(limit int) ([]*domain.Campaign, error)
	GetScanProfileByID(uuid.UUID) (*domain.ScanProfile, error)
	CreateCampaignRun(*domain.CampaignRun) error
	UpdateCampaignRun(*domain.CampaignRun) error
	UpdateCampaign(*domain.Campaign) error
}

// Scheduler runs a tick every interval and processes due campaigns.
type Scheduler struct {
	Store      CampaignStore
	Target     *targetclient.Client
	Dispatcher dispatch.Dispatcher
	Interval   time.Duration
	MaxRuns    int
	Enabled    bool
	stopCh     chan struct{}
}

// DefaultInterval is the default tick interval.
const DefaultInterval = 60 * time.Second

// DefaultMaxRunsPerTick is the max runs to create per tick.
const DefaultMaxRunsPerTick = 10

// Start starts the scheduler goroutine. No-op if Enabled is false.
func (s *Scheduler) Start() {
	if !s.Enabled {
		return
	}
	if s.Interval == 0 {
		s.Interval = DefaultInterval
	}
	if s.MaxRuns == 0 {
		s.MaxRuns = DefaultMaxRunsPerTick
	}
	s.stopCh = make(chan struct{})
	go s.loop()
	log.Printf("campaign-scheduler: started (interval=%v, max_runs_per_tick=%d)", s.Interval, s.MaxRuns)
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	if s.stopCh != nil {
		close(s.stopCh)
		s.stopCh = nil
	}
}

func (s *Scheduler) loop() {
	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.tick(context.Background())
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	campaigns, err := s.Store.ListCampaignsDueForScheduler(s.MaxRuns)
	if err != nil {
		log.Printf("campaign-scheduler: list due: %v", err)
		return
	}
	for _, c := range campaigns {
		s.runOne(ctx, c)
	}
}

func (s *Scheduler) runOne(ctx context.Context, campaign *domain.Campaign) {
	profile, err := s.Store.GetScanProfileByID(campaign.ScanProfileID)
	if err != nil || profile == nil {
		log.Printf("campaign-scheduler: campaign %s scan_profile not found", campaign.ID)
		return
	}
	var materializationID uuid.UUID
	var prefixCount int
	if campaign.MaterializationPolicy == domain.MaterializationPolicyUseLatest {
		resp, err := s.Target.ListMaterializations(ctx, campaign.TargetID, 1, 0)
		if err != nil {
			log.Printf("campaign-scheduler: campaign %s use_latest: %v", campaign.ID, err)
			return
		}
		if len(resp.Items) == 0 {
			log.Printf("campaign-scheduler: campaign %s no materializations (use_latest)", campaign.ID)
			return
		}
		materializationID = resp.Items[0].ID
		prefixCount = resp.Items[0].TotalPrefixCount
	} else {
		matResp, err := s.Target.Materialize(ctx, campaign.TargetID)
		if err != nil {
			log.Printf("campaign-scheduler: campaign %s materialize: %v", campaign.ID, err)
			return
		}
		materializationID = matResp.MaterializationID
		prefixCount = matResp.TotalPrefixCount
	}
	now := time.Now()
	statsBytes, _ := json.Marshal(map[string]int{"prefix_count": prefixCount})
	run := &domain.CampaignRun{
		CampaignID:                campaign.ID,
		TargetID:                  campaign.TargetID,
		TargetMaterializationID:   materializationID,
		ScanProfileID:             profile.ID,
		ScanProfileSlug:           profile.Slug,
		ScanProfileConfigSnapshot: profile.Config,
		Status:                    domain.StatusDispatching,
		StartedAt:                 &now,
		Stats:                     json.RawMessage(statsBytes),
	}
	if err := s.Store.CreateCampaignRun(run); err != nil {
		log.Printf("campaign-scheduler: create run: %v", err)
		return
	}
	if prefixCount == 0 {
		run.Status = domain.StatusCompleted
		run.CompletedAt = &now
		_ = s.Store.UpdateCampaignRun(run)
		if campaign.ScheduleType == domain.ScheduleTypeOnce {
			campaign.RunOnceDone = true
			_ = s.Store.UpdateCampaign(campaign)
		}
		return
	}
	payload := &dispatch.Payload{
		RunID:                   run.ID.String(),
		CampaignID:              campaign.ID.String(),
		TargetID:                campaign.TargetID.String(),
		TargetMaterializationID: materializationID.String(),
		ScanProfileSlug:         profile.Slug,
		ScanProfileConfig:       profile.Config,
		CreatedAt:               run.CreatedAt,
		DispatchAttempt:         1,
	}
	streamID, err := s.Dispatcher.Dispatch(ctx, payload)
	if err != nil {
		run.Status = domain.StatusDispatchFailed
		run.ErrorMessage = err.Error()
		_ = s.Store.UpdateCampaignRun(run)
		log.Printf("campaign-scheduler: dispatch: %v", err)
		return
	}
	run.Status = domain.StatusDispatched
	run.DispatchRef = streamID
	run.DispatchedAt = &now
	if err := s.Store.UpdateCampaignRun(run); err != nil {
		log.Printf("campaign-scheduler: update run: %v", err)
		return
	}
	if campaign.ScheduleType == domain.ScheduleTypeOnce {
		campaign.RunOnceDone = true
	} else if campaign.ScheduleType == domain.ScheduleTypeInterval {
		next := nextRunAt(campaign, now)
		campaign.NextRunAt = &next
	}
	_ = s.Store.UpdateCampaign(campaign)
}

// nextRunAt computes next_run_at from schedule_config (interval_seconds or interval_hours).
func nextRunAt(c *domain.Campaign, from time.Time) time.Time {
	if len(c.ScheduleConfig) == 0 {
		return from.Add(24 * time.Hour)
	}
	var cfg struct {
		IntervalSeconds int `json:"interval_seconds"`
		IntervalHours   int `json:"interval_hours"`
	}
	_ = json.Unmarshal(c.ScheduleConfig, &cfg)
	if cfg.IntervalSeconds > 0 {
		return from.Add(time.Duration(cfg.IntervalSeconds) * time.Second)
	}
	if cfg.IntervalHours > 0 {
		return from.Add(time.Duration(cfg.IntervalHours) * time.Hour)
	}
	return from.Add(24 * time.Hour)
}

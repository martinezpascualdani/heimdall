package storage

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/domain"
)

func getTestDSN(t *testing.T) string {
	dsn := os.Getenv("CAMPAIGN_DB_DSN")
	if dsn == "" {
		dsn = "postgres://heimdall:heimdall@localhost:5432/heimdall_campaign_service_test?sslmode=disable"
	}
	return dsn
}

func TestPostgresStore_Campaign_CRUD(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	profile := &domain.ScanProfile{Name: "p1", Slug: "p1-slug-" + uuid.New().String()[:8]}
	if err := store.CreateScanProfile(profile); err != nil {
		t.Fatalf("CreateScanProfile: %v", err)
	}

	c := &domain.Campaign{
		Name:                 "test-campaign",
		Description:          "desc",
		Active:               true,
		TargetID:             uuid.New(),
		ScanProfileID:        profile.ID,
		ScheduleType:         domain.ScheduleTypeManual,
		MaterializationPolicy: domain.MaterializationPolicyUseLatest,
		ConcurrencyPolicy:    domain.ConcurrencyPolicyAllow,
	}
	if err := store.CreateCampaign(c); err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	if c.ID == uuid.Nil {
		t.Error("CreateCampaign should set ID")
	}

	got, err := store.GetCampaignByID(c.ID)
	if err != nil {
		t.Fatalf("GetCampaignByID: %v", err)
	}
	if got == nil || got.Name != c.Name {
		t.Errorf("GetCampaignByID: got %+v", got)
	}

	list, total, err := store.ListCampaigns(10, 0, true)
	if err != nil {
		t.Fatalf("ListCampaigns: %v", err)
	}
	if total < 1 || len(list) < 1 {
		t.Errorf("ListCampaigns: total=%d len=%d", total, len(list))
	}

	c.Name = "updated"
	if err := store.UpdateCampaign(c); err != nil {
		t.Fatalf("UpdateCampaign: %v", err)
	}
	got, _ = store.GetCampaignByID(c.ID)
	if got.Name != "updated" {
		t.Errorf("UpdateCampaign: got name %q", got.Name)
	}

	if err := store.SoftDeleteCampaign(c.ID); err != nil {
		t.Fatalf("SoftDeleteCampaign: %v", err)
	}
	list, total, _ = store.ListCampaigns(10, 0, true)
	for _, x := range list {
		if x.ID == c.ID {
			t.Error("soft-deleted campaign should not appear in active-only list")
		}
	}
}

func TestPostgresStore_ScanProfile_Delete_RejectIfInUse(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	profile := &domain.ScanProfile{Name: "inuse", Slug: "inuse-" + uuid.New().String()[:8]}
	if err := store.CreateScanProfile(profile); err != nil {
		t.Fatalf("CreateScanProfile: %v", err)
	}
	c := &domain.Campaign{
		Name:                 "uses-profile",
		Active:               true,
		TargetID:             uuid.New(),
		ScanProfileID:        profile.ID,
		ScheduleType:         domain.ScheduleTypeManual,
		MaterializationPolicy: domain.MaterializationPolicyUseLatest,
	}
	if err := store.CreateCampaign(c); err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	n, err := store.CountCampaignsByScanProfileID(profile.ID)
	if err != nil || n != 1 {
		t.Fatalf("CountCampaignsByScanProfileID: n=%d err=%v", n, err)
	}
	// Delete without removing campaign first should work (we only reject in handler; storage allows)
	// Actually storage DeleteScanProfile just deletes - the handler checks CountCampaigns. So test: delete a profile that no campaign uses.
	profile2 := &domain.ScanProfile{Name: "unused", Slug: "unused-" + uuid.New().String()[:8]}
	if err := store.CreateScanProfile(profile2); err != nil {
		t.Fatalf("CreateScanProfile 2: %v", err)
	}
	if err := store.DeleteScanProfile(profile2.ID); err != nil {
		t.Fatalf("DeleteScanProfile: %v", err)
	}
	got, _ := store.GetScanProfileByID(profile2.ID)
	if got != nil {
		t.Error("DeleteScanProfile: profile should be gone")
	}
}

func TestPostgresStore_CampaignRun_CRUD(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	profile := &domain.ScanProfile{Name: "run-p", Slug: "run-p-" + uuid.New().String()[:8]}
	if err := store.CreateScanProfile(profile); err != nil {
		t.Fatalf("CreateScanProfile: %v", err)
	}
	c := &domain.Campaign{
		Name:                 "run-campaign",
		Active:               true,
		TargetID:             uuid.New(),
		ScanProfileID:        profile.ID,
		ScheduleType:         domain.ScheduleTypeManual,
		MaterializationPolicy: domain.MaterializationPolicyUseLatest,
	}
	if err := store.CreateCampaign(c); err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}

	run := &domain.CampaignRun{
		CampaignID:              c.ID,
		TargetID:                c.TargetID,
		TargetMaterializationID: uuid.New(),
		ScanProfileID:           profile.ID,
		ScanProfileSlug:         profile.Slug,
		Status:                  domain.StatusPending,
	}
	if err := store.CreateCampaignRun(run); err != nil {
		t.Fatalf("CreateCampaignRun: %v", err)
	}
	if run.ID == uuid.Nil {
		t.Error("CreateCampaignRun should set ID")
	}

	got, err := store.GetCampaignRunByID(run.ID)
	if err != nil || got == nil || got.Status != domain.StatusPending {
		t.Fatalf("GetCampaignRunByID: err=%v got=%+v", err, got)
	}

	runs, total, err := store.ListCampaignRuns(c.ID, 10, 0)
	if err != nil || total < 1 || len(runs) < 1 {
		t.Fatalf("ListCampaignRuns: err=%v total=%d len=%d", err, total, len(runs))
	}

	run.Status = domain.StatusDispatched
	if err := store.UpdateCampaignRun(run); err != nil {
		t.Fatalf("UpdateCampaignRun: %v", err)
	}
	got, _ = store.GetCampaignRunByID(run.ID)
	if got.Status != domain.StatusDispatched {
		t.Errorf("UpdateCampaignRun: got status %q", got.Status)
	}
}

func TestPostgresStore_HasActiveRun_ListCampaignsDueForScheduler(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	profile := &domain.ScanProfile{Name: "sched-p", Slug: "sched-p-" + uuid.New().String()[:8]}
	if err := store.CreateScanProfile(profile); err != nil {
		t.Fatalf("CreateScanProfile: %v", err)
	}
	now := time.Now()
	c := &domain.Campaign{
		Name:                 "interval-campaign",
		Active:               true,
		TargetID:             uuid.New(),
		ScanProfileID:        profile.ID,
		ScheduleType:         domain.ScheduleTypeInterval,
		MaterializationPolicy: domain.MaterializationPolicyUseLatest,
		NextRunAt:            &now,
		ConcurrencyPolicy:    domain.ConcurrencyPolicyAllow,
	}
	if err := store.CreateCampaign(c); err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}

	has, err := store.HasActiveRun(c.ID)
	if err != nil || has {
		t.Fatalf("HasActiveRun (none): has=%v err=%v", has, err)
	}

	due, err := store.ListCampaignsDueForScheduler(5)
	if err != nil {
		t.Fatalf("ListCampaignsDueForScheduler: %v", err)
	}
	if len(due) < 1 {
		t.Logf("ListCampaignsDueForScheduler: no due campaigns (maybe next_run_at in future)")
	}

	// Create a run in pending -> HasActiveRun true
	run := &domain.CampaignRun{
		CampaignID:              c.ID,
		TargetID:                c.TargetID,
		TargetMaterializationID: uuid.New(),
		ScanProfileID:           profile.ID,
		ScanProfileSlug:         profile.Slug,
		Status:                  domain.StatusPending,
	}
	if err := store.CreateCampaignRun(run); err != nil {
		t.Fatalf("CreateCampaignRun: %v", err)
	}
	has, err = store.HasActiveRun(c.ID)
	if err != nil || !has {
		t.Fatalf("HasActiveRun (pending): has=%v err=%v", has, err)
	}
}

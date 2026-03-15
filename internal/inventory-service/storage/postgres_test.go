package storage

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/inventory-service/domain"
)

func getTestDSN(t *testing.T) string {
	dsn := os.Getenv("INVENTORY_DB_DSN")
	if dsn == "" {
		dsn = "postgres://heimdall:heimdall@localhost:5432/heimdall_inventory_service_test?sslmode=disable"
	}
	return dsn
}

func TestPostgresStore_IngestJobCompleted_Idempotency(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	execID := uuid.New()
	jobID := uuid.New()
	runID := uuid.New()
	campaignID := uuid.New()
	targetID := uuid.New()
	targetMatID := uuid.New()
	observedAt := time.Now().UTC()
	observations := []IngestObservation{
		{IP: "192.168.1.1", Port: 443, Status: "open"},
		{IP: "192.168.1.1", Port: 80, Status: "open"},
	}

	err = store.IngestJobCompleted(execID, jobID, runID, campaignID, targetID, targetMatID, "portscan-basic", observedAt, observations)
	if err != nil {
		t.Fatalf("first IngestJobCompleted: %v", err)
	}

	// Count assets and exposures after first ingest (1 asset, 2 exposures for 192.168.1.1)
	list1, total1, _ := store.ListAssets("", nil, nil, nil, nil, nil, 100, 0)
	expList1, expTotal1, _ := store.ListExposures(nil, "", "", nil, nil, 100, 0)
	assetsAfterFirst := total1
	exposuresAfterFirst := expTotal1

	// Second ingest same job must return ErrAlreadyIngested and must not create duplicate assets/exposures
	err = store.IngestJobCompleted(execID, jobID, runID, campaignID, targetID, targetMatID, "portscan-basic", observedAt, observations)
	if err == nil {
		t.Fatal("second ingest should fail with ErrAlreadyIngested")
	}
	if !errors.Is(err, ErrAlreadyIngested) {
		t.Errorf("expected ErrAlreadyIngested, got %v", err)
	}
	_, total2, _ := store.ListAssets("", nil, nil, nil, nil, nil, 100, 0)
	_, expTotal2, _ := store.ListExposures(nil, "", "", nil, nil, 100, 0)
	if total2 != assetsAfterFirst {
		t.Errorf("repeated ingest created duplicate assets: had %d, now %d", assetsAfterFirst, total2)
	}
	if expTotal2 != exposuresAfterFirst {
		t.Errorf("repeated ingest created duplicate exposures: had %d, now %d", exposuresAfterFirst, expTotal2)
	}
	_ = list1
	_ = expList1
}

func TestPostgresStore_IngestJobCompleted_ListAssets_GetAsset(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	execID := uuid.New()
	jobID := uuid.New()
	runID := uuid.New()
	campaignID := uuid.New()
	targetID := uuid.New()
	targetMatID := uuid.New()
	observedAt := time.Now().UTC()
	observations := []IngestObservation{
		{IP: "10.0.0.1", Port: 22, Status: "open"},
		{IP: "10.0.0.2", Port: 443, Status: "open"},
	}

	err = store.IngestJobCompleted(execID, jobID, runID, campaignID, targetID, targetMatID, "portscan-basic", observedAt, observations)
	if err != nil {
		t.Fatalf("IngestJobCompleted: %v", err)
	}

	list, total, err := store.ListAssets("host", &campaignID, nil, nil, nil, nil, 10, 0)
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if total < 2 {
		t.Errorf("ListAssets: expected at least 2 assets, got total=%d", total)
	}
	if len(list) < 2 {
		t.Errorf("ListAssets: expected at least 2 items, got %d", len(list))
	}

	// Get first asset and its exposures
	a := list[0]
	got, err := store.GetAsset(a.ID)
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if got == nil || got.ID != a.ID {
		t.Errorf("GetAsset: got %+v", got)
	}
	exposures, err := store.ListExposuresByAssetID(a.ID)
	if err != nil {
		t.Fatalf("ListExposuresByAssetID: %v", err)
	}
	if len(exposures) < 1 {
		t.Errorf("ListExposuresByAssetID: expected at least 1 exposure, got %d", len(exposures))
	}
}

// TestPostgresStore_TwoJobsSameHostPort_OneAssetOneExposureTwoObservations verifies that two
// different jobs (different execution_id, job_id) for the same host/port produce one asset,
// one exposure, two observations; and the second job updates last_seen_at only (first_seen_at unchanged).
func TestPostgresStore_TwoJobsSameHostPort_OneAssetOneExposureTwoObservations(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	runID := uuid.New()
	campaignID := uuid.New()
	targetID := uuid.New()
	targetMatID := uuid.New()
	ip := "172.16.0.1"
	port := 22
	obs1 := time.Now().UTC().Add(-1 * time.Hour)
	obs2 := time.Now().UTC()

	// Job 1
	err = store.IngestJobCompleted(uuid.New(), uuid.New(), runID, campaignID, targetID, targetMatID, "portscan-basic", obs1, []IngestObservation{{IP: ip, Port: port, Status: "open"}})
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	assets1, _, _ := store.ListAssets("host", nil, nil, nil, nil, nil, 10, 0)
	var asset *domain.Asset
	for _, a := range assets1 {
		if a.IdentityValue == ip {
			asset = a
			break
		}
	}
	if asset == nil {
		t.Fatal("asset not found after first ingest")
	}
	firstSeenAfterJob1 := asset.FirstSeenAt
	lastSeenAfterJob1 := asset.LastSeenAt
	exposures1, _ := store.ListExposuresByAssetID(asset.ID)
	if len(exposures1) != 1 {
		t.Fatalf("expected 1 exposure, got %d", len(exposures1))
	}
	expFirstSeen1 := exposures1[0].FirstSeenAt
	expLastSeen1 := exposures1[0].LastSeenAt

	// Job 2: same host/port, different execution/job
	err = store.IngestJobCompleted(uuid.New(), uuid.New(), runID, campaignID, targetID, targetMatID, "portscan-basic", obs2, []IngestObservation{{IP: ip, Port: port, Status: "open"}})
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	assets2, total2, _ := store.ListAssets("host", nil, nil, nil, nil, nil, 10, 0)
	if total2 != 1 {
		t.Errorf("expected 1 asset total, got %d", total2)
	}
	gotAsset, _ := store.GetAssetByTypeIdentity("host", ip)
	if gotAsset == nil {
		t.Fatal("asset not found after second ingest")
	}
	if gotAsset.FirstSeenAt != firstSeenAfterJob1 {
		t.Errorf("first_seen_at must not change: was %v, now %v", firstSeenAfterJob1, gotAsset.FirstSeenAt)
	}
	if !gotAsset.LastSeenAt.After(lastSeenAfterJob1) && !gotAsset.LastSeenAt.Equal(obs2) {
		t.Errorf("last_seen_at should be updated to second observation time: %v", gotAsset.LastSeenAt)
	}
	exposures2, _ := store.ListExposuresByAssetID(gotAsset.ID)
	if len(exposures2) != 1 {
		t.Errorf("expected still 1 exposure, got %d", len(exposures2))
	}
	if exposures2[0].FirstSeenAt != expFirstSeen1 {
		t.Errorf("exposure first_seen_at must not change: was %v, now %v", expFirstSeen1, exposures2[0].FirstSeenAt)
	}
	if !exposures2[0].LastSeenAt.After(expLastSeen1) || !exposures2[0].LastSeenAt.Equal(obs2) {
		t.Errorf("exposure last_seen_at should be updated: was %v, now %v", expLastSeen1, exposures2[0].LastSeenAt)
	}
	obsList, obsTotal, _ := store.ListObservations(nil, nil, &runID, nil, nil, nil, nil, nil, 100, 0)
	if obsTotal < 2 {
		t.Errorf("expected at least 2 observations (one per job), got %d", obsTotal)
	}
	_ = obsList
}

// TestPostgresStore_DiffExecution_NewGoneUnchanged verifies diff by execution uses observation
// sets only (not last_seen). Execution A: host1:80. Execution B: host1:80, host1:443.
// A→B: one exposure new (443), one unchanged (80). B→A: one exposure gone (443).
func TestPostgresStore_DiffExecution_NewGoneUnchanged(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	runID := uuid.New()
	campaignID := uuid.New()
	targetID := uuid.New()
	targetMatID := uuid.New()
	ip := "10.10.10.10"
	now := time.Now().UTC()

	execA := uuid.New()
	jobA := uuid.New()
	err = store.IngestJobCompleted(execA, jobA, runID, campaignID, targetID, targetMatID, "portscan-basic", now, []IngestObservation{{IP: ip, Port: 80, Status: "open"}})
	if err != nil {
		t.Fatalf("ingest A: %v", err)
	}

	execB := uuid.New()
	jobB := uuid.New()
	err = store.IngestJobCompleted(execB, jobB, runID, campaignID, targetID, targetMatID, "portscan-basic", now.Add(time.Minute), []IngestObservation{
		{IP: ip, Port: 80, Status: "open"},
		{IP: ip, Port: 443, Status: "open"},
	})
	if err != nil {
		t.Fatalf("ingest B: %v", err)
	}

	pairsA, _ := store.ListObservationPairsByExecution(execA)
	pairsB, _ := store.ListObservationPairsByExecution(execB)
	if len(pairsA) != 1 {
		t.Fatalf("exec A should have 1 pair, got %d", len(pairsA))
	}
	if len(pairsB) != 2 {
		t.Fatalf("exec B should have 2 pairs, got %d", len(pairsB))
	}

	setA := make(map[string]struct{})
	for _, p := range pairsA {
		setA[p.AssetID.String()+"\x00"+p.ExposureID.String()] = struct{}{}
	}
	setB := make(map[string]struct{})
	for _, p := range pairsB {
		setB[p.AssetID.String()+"\x00"+p.ExposureID.String()] = struct{}{}
	}
	var newCount, goneCount, unchangedCount int
	for _, p := range pairsB {
		key := p.AssetID.String() + "\x00" + p.ExposureID.String()
		if _, in := setA[key]; in {
			unchangedCount++
		} else {
			newCount++
		}
	}
	for _, p := range pairsA {
		key := p.AssetID.String() + "\x00" + p.ExposureID.String()
		if _, in := setB[key]; !in {
			goneCount++
		}
	}
	if newCount != 1 {
		t.Errorf("expected 1 new (443), got %d", newCount)
	}
	if unchangedCount != 1 {
		t.Errorf("expected 1 unchanged (80), got %d", unchangedCount)
	}
	if goneCount != 0 {
		t.Errorf("A→B: expected 0 gone, got %d", goneCount)
	}
	// B→A: 443 is in B but not A, so "gone" when comparing B to A means exposure present in B but not A... no, "gone" means in from_execution but not in to_execution. So from=B to=A: gone = in B not in A = 1 (443).
	goneFromBToA := 0
	for _, p := range pairsB {
		key := p.AssetID.String() + "\x00" + p.ExposureID.String()
		if _, in := setA[key]; !in {
			goneFromBToA++
		}
	}
	// Actually "from_execution_id" and "to_execution_id" semantics: from=A to=B gives new (in B not A), gone (in A not B). So from=B to=A: new = in A not B = 0, gone = in B not A = 1.
	if goneFromBToA != 1 {
		t.Errorf("B→A: expected 1 gone (443), got %d", goneFromBToA)
	}
}

// TestPostgresStore_Filters_CampaignAndTarget_SameObservation verifies that listing assets
// with both campaign_id and target_id requires the same observation to have both (no cross-context).
func TestPostgresStore_Filters_CampaignAndTarget_SameObservation(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	campaign1 := uuid.New()
	target1 := uuid.New()
	target2 := uuid.New()
	runID := uuid.New()
	targetMatID := uuid.New()
	now := time.Now().UTC()

	// Asset 1: only observed in (campaign1, target1)
	exec1 := uuid.New()
	job1 := uuid.New()
	err = store.IngestJobCompleted(exec1, job1, runID, campaign1, target1, targetMatID, "portscan-basic", now, []IngestObservation{{IP: "203.0.113.1", Port: 80, Status: "open"}})
	if err != nil {
		t.Fatalf("ingest 1: %v", err)
	}

	// Asset 2: only observed in (campaign1, target2)
	exec2 := uuid.New()
	job2 := uuid.New()
	err = store.IngestJobCompleted(exec2, job2, runID, campaign1, target2, targetMatID, "portscan-basic", now, []IngestObservation{{IP: "203.0.113.2", Port: 80, Status: "open"}})
	if err != nil {
		t.Fatalf("ingest 2: %v", err)
	}

	// List with campaign1 + target1: must return only asset 1 (203.0.113.1), not asset 2
	list, total, err := store.ListAssets("", &campaign1, &target1, nil, nil, nil, 10, 0)
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 asset for campaign1+target1 (same observation), got %d", total)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 item, got %d", len(list))
	}
	if len(list) > 0 && list[0].IdentityValue != "203.0.113.1" {
		t.Errorf("expected 203.0.113.1, got %s", list[0].IdentityValue)
	}

	// List with campaign1 + target2: must return only asset 2
	list2, total2, _ := store.ListAssets("", &campaign1, &target2, nil, nil, nil, 10, 0)
	if total2 != 1 || (len(list2) > 0 && list2[0].IdentityValue != "203.0.113.2") {
		t.Errorf("expected 1 asset 203.0.113.2 for campaign1+target2, got total=%d identity=%s", total2, safeIdentity(list2))
	}
}

func safeIdentity(list []*domain.Asset) string {
	if len(list) == 0 {
		return ""
	}
	return list[0].IdentityValue
}

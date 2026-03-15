package storage

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/domain"
)

func getTestDSN(t *testing.T) string {
	dsn := os.Getenv("SCOPE_DB_DSN")
	if dsn == "" {
		dsn = "postgres://heimdall:heimdall@localhost:5432/heimdall_scope_service?sslmode=disable"
	}
	return dsn
}

func TestPostgresStore_HasImportedDataset(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	datasetID := uuid.New()
	impID := uuid.New()
	configEff := "status=allocated,assigned"
	imp := &domain.ScopeImport{
		ID: impID, DatasetID: datasetID, Registry: "ripencc", ConfigEffective: configEff,
		State: domain.ImportStateImported, BlocksPersisted: 10, DurationMs: 100, CreatedAt: time.Now(),
	}
	if err := store.CreateImport(imp); err != nil {
		t.Fatalf("CreateImport: %v", err)
	}

	ok, err := store.HasImportedDataset(datasetID)
	if err != nil {
		t.Fatalf("HasImportedDataset: %v", err)
	}
	if !ok {
		t.Error("HasImportedDataset(datasetID) should be true")
	}

	ok, _ = store.HasImportedDataset(uuid.New())
	if ok {
		t.Error("HasImportedDataset(random) should be false")
	}
}

func TestPostgresStore_GetLatestImportedDatasetIDsPerRegistry(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	d1, d2 := uuid.New(), uuid.New()
	configEff := "status=allocated,assigned"
	now := time.Now()
	for i, reg := range []string{"ripencc", "arin"} {
		id := uuid.New()
		datasetID := d1
		if i == 1 {
			datasetID = d2
		}
		imp := &domain.ScopeImport{
			ID: id, DatasetID: datasetID, Registry: reg, ConfigEffective: configEff,
			State: domain.ImportStateImported, BlocksPersisted: 1, CreatedAt: now.Add(time.Duration(i) * time.Second),
		}
		if err := store.CreateImport(imp); err != nil {
			t.Fatalf("CreateImport: %v", err)
		}
	}

	ids, err := store.GetLatestImportedDatasetIDsPerRegistry()
	if err != nil {
		t.Fatalf("GetLatestImportedDatasetIDsPerRegistry: %v", err)
	}
	if len(ids) < 2 {
		t.Errorf("expected at least 2 dataset IDs (ripencc, arin), got %d", len(ids))
	}
}

func TestPostgresStore_GetLatestImportedDatasetIDForScope(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	datasetID := uuid.New()
	impID := uuid.New()
	configEff := "status=allocated,assigned"
	imp := &domain.ScopeImport{
		ID: impID, DatasetID: datasetID, Registry: "ripencc", ConfigEffective: configEff,
		State: domain.ImportStateImported, BlocksPersisted: 1, CreatedAt: time.Now(),
	}
	if err := store.CreateImport(imp); err != nil {
		t.Fatalf("CreateImport: %v", err)
	}
	b := &domain.ScopeBlock{
		DatasetID: datasetID, ScopeType: "country", ScopeValue: "ES", AddressFamily: "ipv4",
		BlockRawIdentity: "ripencc|ES|ipv4|1.2.3.0|256|20240101|allocated",
		Start: "1.2.3.0", Value: "256", Status: "allocated", CC: "ES", Date: "20240101", CreatedAt: time.Now(),
	}
	if err := store.UpsertBlock(b); err != nil {
		t.Fatalf("UpsertBlock: %v", err)
	}

	got, err := store.GetLatestImportedDatasetIDForScope("country", "ES")
	if err != nil {
		t.Fatalf("GetLatestImportedDatasetIDForScope: %v", err)
	}
	if got == nil || *got != datasetID {
		t.Errorf("GetLatestImportedDatasetIDForScope: got %v, want %s", got, datasetID)
	}

	got, _ = store.GetLatestImportedDatasetIDForScope("country", "XX")
	if got != nil {
		t.Errorf("scope XX should have no import, got %v", got)
	}
}

func TestPostgresStore_ListBlocksByScope_CountBlocksByScope(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	datasetID := uuid.New()
	impID := uuid.New()
	configEff := "status=allocated,assigned"
	imp := &domain.ScopeImport{
		ID: impID, DatasetID: datasetID, Registry: "apnic", ConfigEffective: configEff,
		State: domain.ImportStateImported, BlocksPersisted: 2, CreatedAt: time.Now(),
	}
	if err := store.CreateImport(imp); err != nil {
		t.Fatalf("CreateImport: %v", err)
	}
	b1 := &domain.ScopeBlock{
		DatasetID: datasetID, ScopeType: "country", ScopeValue: "ES", AddressFamily: "ipv4",
		BlockRawIdentity: "apnic|ES|ipv4|1.0.0.0|256|20240101|allocated",
		Start: "1.0.0.0", Value: "256", Status: "allocated", CC: "ES", Date: "20240101", CreatedAt: time.Now(),
	}
	store.UpsertBlock(b1)
	b2 := &domain.ScopeBlock{
		DatasetID: datasetID, ScopeType: "country", ScopeValue: "ES", AddressFamily: "ipv6",
		BlockRawIdentity: "apnic|ES|ipv6|2001:db8::|32|20240101|allocated",
		Start: "2001:db8::", Value: "32", Status: "allocated", CC: "ES", Date: "20240101", CreatedAt: time.Now(),
	}
	store.UpsertBlock(b2)

	ids := []uuid.UUID{datasetID}
	total, err := store.CountBlocksByScope("country", "ES", ids, "")
	if err != nil {
		t.Fatalf("CountBlocksByScope: %v", err)
	}
	if total != 2 {
		t.Errorf("CountBlocksByScope: want 2, got %d", total)
	}

	list, err := store.ListBlocksByScope("country", "ES", ids, "", 10, 0)
	if err != nil {
		t.Fatalf("ListBlocksByScope: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListBlocksByScope: want 2, got %d", len(list))
	}

	total, _ = store.CountBlocksByScope("country", "ES", ids, "ipv4")
	if total != 1 {
		t.Errorf("CountBlocksByScope ipv4: want 1, got %d", total)
	}

	list, _ = store.ListBlocksByScope("country", "ES", ids, "", 0, 0)
	if len(list) != 0 {
		t.Errorf("limit=0 should return 0 items, got %d", len(list))
	}
	total, _ = store.CountBlocksByScope("country", "ES", ids, "")
	list, _ = store.ListBlocksByScope("country", "ES", ids, "", 10, int(total)+1)
	if len(list) != 0 {
		t.Errorf("offset >= total should return 0 items, got %d", len(list))
	}
}

func TestPostgresStore_FindBlockByIP(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	datasetID := uuid.New()
	impID := uuid.New()
	imp := &domain.ScopeImport{
		ID: impID, DatasetID: datasetID, Registry: "ripencc", ConfigEffective: "status=allocated,assigned",
		State: domain.ImportStateImported, BlocksPersisted: 1, CreatedAt: time.Now(),
	}
	if err := store.CreateImport(imp); err != nil {
		t.Fatalf("CreateImport: %v", err)
	}
	b := &domain.ScopeBlock{
		DatasetID: datasetID, ScopeType: "country", ScopeValue: "ES", AddressFamily: "ipv4",
		BlockRawIdentity: "ripencc|ES|ipv4|1.2.3.0|256|20240101|allocated",
		Start: "1.2.3.0", Value: "256", Status: "allocated", CC: "ES", Date: "20240101", CreatedAt: time.Now(),
	}
	if err := store.UpsertBlock(b); err != nil {
		t.Fatalf("UpsertBlock: %v", err)
	}

	// 1.2.3.0 - 1.2.3.255
	ipIn := net.ParseIP("1.2.3.1")
	match, err := store.FindBlockByIP(ipIn, datasetID)
	if err != nil {
		t.Fatalf("FindBlockByIP: %v", err)
	}
	if match == nil {
		t.Fatal("FindBlockByIP(1.2.3.1): expected match")
	}
	if match.ScopeValue != "ES" || match.ScopeType != "country" {
		t.Errorf("match: %+v", match)
	}

	ipOut := net.ParseIP("9.9.9.9")
	match, _ = store.FindBlockByIP(ipOut, datasetID)
	if match != nil {
		t.Error("FindBlockByIP(9.9.9.9): expected no match")
	}
}

func TestPostgresStore_FindBlockByIPInLatestPerRegistry(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	datasetID := uuid.New()
	impID := uuid.New()
	imp := &domain.ScopeImport{
		ID: impID, DatasetID: datasetID, Registry: "ripencc", ConfigEffective: "status=allocated,assigned",
		State: domain.ImportStateImported, BlocksPersisted: 1, CreatedAt: time.Now(),
	}
	if err := store.CreateImport(imp); err != nil {
		t.Fatalf("CreateImport: %v", err)
	}
	b := &domain.ScopeBlock{
		DatasetID: datasetID, ScopeType: "country", ScopeValue: "DE", AddressFamily: "ipv4",
		BlockRawIdentity: "ripencc|DE|ipv4|5.6.7.0|512|20240101|allocated",
		Start: "5.6.7.0", Value: "512", Status: "allocated", CC: "DE", Date: "20240101", CreatedAt: time.Now(),
	}
	if err := store.UpsertBlock(b); err != nil {
		t.Fatalf("UpsertBlock: %v", err)
	}

	ip := net.ParseIP("5.6.7.10")
	match, err := store.FindBlockByIPInLatestPerRegistry(ip)
	if err != nil {
		t.Fatalf("FindBlockByIPInLatestPerRegistry: %v", err)
	}
	if match == nil {
		t.Fatal("expected match for 5.6.7.10 in latest ripencc")
	}
	if match.ScopeValue != "DE" {
		t.Errorf("scope_value: got %s", match.ScopeValue)
	}
}

func TestPostgresStore_ASNs(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	datasetID := uuid.New()
	impID := uuid.New()
	imp := &domain.ScopeImport{
		ID: impID, DatasetID: datasetID, Registry: "ripencc", ConfigEffective: "status=allocated,assigned",
		State: domain.ImportStateImported, BlocksPersisted: 0, AsnsPersisted: 2, CreatedAt: time.Now(),
	}
	if err := store.CreateImport(imp); err != nil {
		t.Fatalf("CreateImport: %v", err)
	}

	// Single ASN (asn_count=1) and a range
	a1 := &domain.ScopeASN{
		DatasetID: datasetID, ScopeType: "country", ScopeValue: "ES",
		ASNStart: 65536, ASNCount: 1, Status: "allocated", CC: "ES", Date: "20240101",
		Registry: "ripencc", RawIdentity: "ripencc|ES|asn|65536|1|20240101|allocated", CreatedAt: time.Now(),
	}
	a2 := &domain.ScopeASN{
		DatasetID: datasetID, ScopeType: "country", ScopeValue: "ES",
		ASNStart: 65540, ASNCount: 10, Status: "assigned", CC: "ES", Date: "20240101",
		Registry: "ripencc", RawIdentity: "ripencc|ES|asn|65540|10|20240101|assigned", CreatedAt: time.Now(),
	}
	if err := store.UpsertASN(a1); err != nil {
		t.Fatalf("UpsertASN: %v", err)
	}
	if err := store.UpsertASN(a2); err != nil {
		t.Fatalf("UpsertASN: %v", err)
	}

	ids := []uuid.UUID{datasetID}
	rangeCount, err := store.CountASNRangeByScope("country", "ES", ids)
	if err != nil {
		t.Fatalf("CountASNRangeByScope: %v", err)
	}
	if rangeCount != 2 {
		t.Errorf("CountASNRangeByScope: want 2, got %d", rangeCount)
	}
	sum, err := store.SumASNCountByScope("country", "ES", ids)
	if err != nil {
		t.Fatalf("SumASNCountByScope: %v", err)
	}
	if sum != 11 {
		t.Errorf("SumASNCountByScope: want 11 (1+10), got %d", sum)
	}

	list, err := store.ListASNsByScope("country", "ES", ids, 10, 0)
	if err != nil {
		t.Fatalf("ListASNsByScope: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListASNsByScope: want 2, got %d", len(list))
	}
	// Order: asn_start ASC, raw_identity
	if list[0].ASNStart != 65536 || list[0].ASNCount != 1 {
		t.Errorf("first ASN: want 65536/1, got %d/%d", list[0].ASNStart, list[0].ASNCount)
	}
	if list[1].ASNStart != 65540 || list[1].ASNCount != 10 {
		t.Errorf("second ASN: want 65540/10, got %d/%d", list[1].ASNStart, list[1].ASNCount)
	}

	// Country with no ASNs
	rangeCount, _ = store.CountASNRangeByScope("country", "XX", ids)
	if rangeCount != 0 {
		t.Errorf("CountASNRangeByScope XX: want 0, got %d", rangeCount)
	}
}

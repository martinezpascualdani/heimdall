package storage

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/dataset-service/domain"
)

func getTestDSN(t *testing.T) string {
	dsn := os.Getenv("DATASET_DB_DSN")
	if dsn == "" {
		dsn = "postgres://heimdall:heimdall@localhost:5432/heimdall_datasets?sslmode=disable"
	}
	return dsn
}

// uniqueSerial returns a serial unlikely to collide with previous test runs
func uniqueSerial(base int64) int64 {
	return base + (time.Now().UnixNano() % 10000000)
}

func TestPostgresStore_CreateVersion_GetByID(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	serial := uniqueSerial(999001001)
	id := uuid.New()
	now := time.Now()
	// Use test_ prefix to avoid collision with real data in shared DB
	v := &domain.DatasetVersion{
		ID: id, Registry: "test_ripencc", Serial: serial,
		StartDate: "20240101", EndDate: "20240102", RecordCount: 1000,
		State: domain.StateValidated, StoragePath: "test_ripencc/delegated-serial-abc.txt",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateVersion(v); err != nil {
		t.Fatalf("CreateVersion: %v", err)
	}

	got, err := store.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.Registry != "test_ripencc" || got.Serial != serial || got.State != domain.StateValidated {
		t.Errorf("GetByID: got %+v", got)
	}

	got, _ = store.GetByID(uuid.New())
	if got != nil {
		t.Error("GetByID(unknown) should be nil")
	}
}

func TestPostgresStore_GetByRegistrySerial_PrefersValidated(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	serial := uniqueSerial(999002001)
	reg := "test_apnic"
	id1 := uuid.New()
	now := time.Now()
	// Insert failed first, then validated (same registry+serial)
	store.CreateVersion(&domain.DatasetVersion{ID: id1, Registry: reg, Serial: serial, State: domain.StateFailed, Error: "err", CreatedAt: now, UpdatedAt: now})
	id2 := uuid.New()
	store.CreateVersion(&domain.DatasetVersion{ID: id2, Registry: reg, Serial: serial, State: domain.StateValidated, StoragePath: "/x", CreatedAt: now.Add(time.Second), UpdatedAt: now})

	got, err := store.GetByRegistrySerial(reg, serial)
	if err != nil {
		t.Fatalf("GetByRegistrySerial: %v", err)
	}
	if got == nil || got.ID != id2 || got.State != domain.StateValidated {
		t.Errorf("GetByRegistrySerial should prefer validated: got %+v", got)
	}
}

func TestPostgresStore_List(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	serial := uniqueSerial(999003001)
	id := uuid.New()
	now := time.Now()
	store.CreateVersion(&domain.DatasetVersion{ID: id, Registry: "test_arin", Serial: serial, State: domain.StateValidated, CreatedAt: now, UpdatedAt: now})

	list, err := store.List(10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) < 1 {
		t.Errorf("List: expected at least 1, got %d", len(list))
	}
	// List(0) should default to 100
	list, _ = store.List(0)
	if len(list) > 100 {
		t.Errorf("List(0) should cap at 100, got %d", len(list))
	}
}

func TestPostgresStore_GetLatestByRegistry(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	base := uniqueSerial(999004001)
	// Use unique registry name so we don't pick up rows from other runs
	reg := fmt.Sprintf("test_lacnic_%d", base)
	now := time.Now()
	id1 := uuid.New()
	store.CreateVersion(&domain.DatasetVersion{ID: id1, Registry: reg, Serial: base, State: domain.StateValidated, CreatedAt: now, UpdatedAt: now})
	id2 := uuid.New()
	store.CreateVersion(&domain.DatasetVersion{ID: id2, Registry: reg, Serial: base + 1, State: domain.StateValidated, CreatedAt: now.Add(time.Second), UpdatedAt: now})

	got, err := store.GetLatestByRegistry(reg)
	if err != nil {
		t.Fatalf("GetLatestByRegistry: %v", err)
	}
	if got == nil || got.Serial != base+1 || got.ID != id2 {
		t.Errorf("GetLatestByRegistry: expected serial %d id %s, got %+v", base+1, id2, got)
	}

	got, _ = store.GetLatestByRegistry("test_nonexistent")
	if got != nil {
		t.Error("GetLatestByRegistry(nonexistent) should be nil")
	}
}

func TestPostgresStore_UpdateState(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	serial := uniqueSerial(999005001)
	id := uuid.New()
	now := time.Now()
	store.CreateVersion(&domain.DatasetVersion{ID: id, Registry: "test_afrinic", Serial: serial, State: domain.StateFetching, CreatedAt: now, UpdatedAt: now})

	if err := store.UpdateState(id, domain.StateValidated, "/path/to/file", ""); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	got, _ := store.GetByID(id)
	if got == nil || got.State != domain.StateValidated || got.StoragePath != "/path/to/file" {
		t.Errorf("UpdateState: got %+v", got)
	}
}

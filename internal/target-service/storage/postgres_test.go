package storage

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/target-service/domain"
)

func getTestDSN(t *testing.T) string {
	dsn := os.Getenv("TARGET_DB_DSN")
	if dsn == "" {
		dsn = "postgres://heimdall:heimdall@localhost:5432/heimdall_target_service_test?sslmode=disable"
	}
	return dsn
}

func TestPostgresStore_CreateTarget_GetTarget_ListTargets(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	target := &domain.Target{
		Name:        "test-target",
		Description: "desc",
		Active:      true,
	}
	if err := store.CreateTarget(target); err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	if target.ID == uuid.Nil {
		t.Error("CreateTarget should set ID")
	}

	got, err := store.GetTargetByID(target.ID)
	if err != nil {
		t.Fatalf("GetTargetByID: %v", err)
	}
	if got == nil || got.Name != target.Name || got.Description != target.Description {
		t.Errorf("GetTargetByID: got %+v", got)
	}

	list, err := store.ListTargets(10, 0, true)
	if err != nil {
		t.Fatalf("ListTargets: %v", err)
	}
	if len(list) < 1 {
		t.Error("ListTargets: expected at least one target")
	}
}

func TestPostgresStore_SoftDeleteTarget_Idempotent(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	target := &domain.Target{Name: "to-delete", Active: true}
	if err := store.CreateTarget(target); err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	if err := store.SoftDeleteTarget(target.ID); err != nil {
		t.Fatalf("SoftDeleteTarget: %v", err)
	}
	got, _ := store.GetTargetByID(target.ID)
	if got != nil && got.Active {
		t.Error("expected target to be inactive after soft delete")
	}
	// Idempotent: second call succeeds
	if err := store.SoftDeleteTarget(target.ID); err != nil {
		t.Fatalf("SoftDeleteTarget second call: %v", err)
	}
}

func TestPostgresStore_Materialization_Immutability(t *testing.T) {
	store, err := NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	target := &domain.Target{Name: "immut-test", Active: true}
	if err := store.CreateTarget(target); err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	if err := store.InsertRules(target.ID, []domain.TargetRule{
		{Kind: "include", SelectorType: "prefix", SelectorValue: "10.0.0.0/24"},
	}); err != nil {
		t.Fatalf("InsertRules: %v", err)
	}

	m1 := &domain.TargetMaterialization{TargetID: target.ID, Status: domain.MaterializationStatusRunning}
	if err := store.CreateMaterialization(m1); err != nil {
		t.Fatalf("CreateMaterialization: %v", err)
	}
	if err := store.InsertTargetEntries(m1.ID, []string{"10.0.0.0/24"}); err != nil {
		t.Fatalf("InsertTargetEntries: %v", err)
	}
	m1.TotalPrefixCount = 1
	m1.Status = domain.MaterializationStatusCompleted
	if err := store.UpdateMaterialization(m1); err != nil {
		t.Fatalf("UpdateMaterialization: %v", err)
	}

	// Second materialization (same logical result): new snapshot, does not mutate m1
	m2 := &domain.TargetMaterialization{TargetID: target.ID, Status: domain.MaterializationStatusRunning}
	if err := store.CreateMaterialization(m2); err != nil {
		t.Fatalf("CreateMaterialization m2: %v", err)
	}
	if err := store.InsertTargetEntries(m2.ID, []string{"10.0.0.0/24"}); err != nil {
		t.Fatalf("InsertTargetEntries m2: %v", err)
	}
	m2.TotalPrefixCount = 1
	m2.Status = domain.MaterializationStatusCompleted
	if err := store.UpdateMaterialization(m2); err != nil {
		t.Fatalf("UpdateMaterialization m2: %v", err)
	}

	if m1.ID == m2.ID {
		t.Error("repeated materialization must produce new snapshot ID")
	}
	prefixes1, _ := store.GetAllPrefixesForMaterialization(m1.ID)
	prefixes2, _ := store.GetAllPrefixesForMaterialization(m2.ID)
	if len(prefixes1) != 1 || len(prefixes2) != 1 {
		t.Errorf("each snapshot should have 1 prefix, got %d and %d", len(prefixes1), len(prefixes2))
	}
}

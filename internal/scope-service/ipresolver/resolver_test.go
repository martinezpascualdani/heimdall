package ipresolver

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/storage"
)

// mockStore implements Store for tests.
type mockStore struct {
	latestDatasetIDs []uuid.UUID
	blockMatch       *storage.IPBlockMatch
	findErr          error
}

func (m *mockStore) GetLatestImportedDatasetIDsPerRegistry() ([]uuid.UUID, error) {
	return m.latestDatasetIDs, nil
}

func (m *mockStore) FindBlockByIP(ip net.IP, datasetID uuid.UUID) (*storage.IPBlockMatch, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	return m.blockMatch, nil
}

func (m *mockStore) FindBlockByIPInLatestPerRegistry(ip net.IP) (*storage.IPBlockMatch, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	return m.blockMatch, nil
}

func TestResolve_InvalidIP(t *testing.T) {
	svc := &Service{Store: &mockStore{latestDatasetIDs: []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000001")}}}
	ctx := context.Background()

	_, err := svc.Resolve(ctx, "hola", nil)
	if !errors.Is(err, ErrInvalidIP) {
		t.Errorf("expected ErrInvalidIP, got %v", err)
	}

	_, err = svc.Resolve(ctx, "999.999.999.999", nil)
	if !errors.Is(err, ErrInvalidIP) {
		t.Errorf("expected ErrInvalidIP for malformed IP, got %v", err)
	}
}

func TestResolve_NoDataset(t *testing.T) {
	svc := &Service{Store: &mockStore{latestDatasetIDs: nil}}
	ctx := context.Background()

	_, err := svc.Resolve(ctx, "8.8.8.8", nil)
	if !errors.Is(err, ErrNoDataset) {
		t.Errorf("expected ErrNoDataset, got %v", err)
	}
}

func TestResolve_NotFound(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	svc := &Service{Store: &mockStore{latestDatasetIDs: []uuid.UUID{id}, blockMatch: nil}}
	ctx := context.Background()

	result, err := svc.Resolve(ctx, "8.8.8.8", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result (not found), got %+v", result)
	}
}

func TestResolve_IPv4OK(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	svc := &Service{Store: &mockStore{
		latestDatasetIDs: []uuid.UUID{id},
		blockMatch:       &storage.IPBlockMatch{ScopeType: "country", ScopeValue: "US", DatasetID: id},
	}}
	ctx := context.Background()

	result, err := svc.Resolve(ctx, "8.8.8.8", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if result.IP != "8.8.8.8" || result.ScopeType != "country" || result.ScopeValue != "US" || result.DatasetID != id {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestResolve_IPv6OK(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	svc := &Service{Store: &mockStore{
		latestDatasetIDs: []uuid.UUID{id},
		blockMatch:       &storage.IPBlockMatch{ScopeType: "country", ScopeValue: "IE", DatasetID: id},
	}}
	ctx := context.Background()

	result, err := svc.Resolve(ctx, "2a00:1450:4001:81b::200e", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if result.ScopeValue != "IE" {
		t.Errorf("unexpected scope_value: %s", result.ScopeValue)
	}
}

func TestResolve_WithDatasetID(t *testing.T) {
	customID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	svc := &Service{Store: &mockStore{
		latestDatasetIDs: []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000001")},
		blockMatch:       &storage.IPBlockMatch{ScopeType: "country", ScopeValue: "DE", DatasetID: customID},
	}}
	ctx := context.Background()

	result, err := svc.Resolve(ctx, "8.8.8.8", &customID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if result.DatasetID != customID || result.ScopeValue != "DE" {
		t.Errorf("unexpected result: %+v", result)
	}
}

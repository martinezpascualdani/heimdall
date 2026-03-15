package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/martinezpascualdani/heimdall/internal/dataset-service/storage"
	"github.com/martinezpascualdani/heimdall/pkg/registry"
)

func getTestDSN(t *testing.T) string {
	dsn := os.Getenv("DATASET_DB_DSN")
	if dsn == "" {
		dsn = "postgres://heimdall:heimdall@localhost:5432/heimdall_datasets?sslmode=disable"
	}
	return dsn
}

// mockFetcher returns a fixed body for Fetch calls.
type mockFetcher struct {
	body string
	err  error
}

func (m *mockFetcher) Fetch(ctx context.Context, cfg registry.Config) (io.ReadCloser, int64, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	r := io.NopCloser(strings.NewReader(m.body))
	return r, int64(len(m.body)), nil
}

// validDelegatedBodyWithSerial builds a delegated body so each test run uses a unique serial (avoids DB collision).
func validDelegatedBodyWithSerial(serial int64) string {
	return fmt.Sprintf("2|ripencc|%d|3|20240101|20240102|+00\nripencc|ES|ipv4|1.2.3.0|256|20240101|allocated\nripencc|ES|ipv6|2001:db8::|32|20240101|allocated\n", serial)
}

func TestFetchLatest_Integration_CreatedThenExisting(t *testing.T) {
	store, err := storage.NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	dir := t.TempDir()
	artifact, err := storage.NewArtifactStore(dir)
	if err != nil {
		t.Fatalf("NewArtifactStore: %v", err)
	}

	// Serial unique per run to avoid "existing" when DB already has data from previous runs
	serial := 999000000000 + (time.Now().UnixNano() % 1000000000)
	body := validDelegatedBodyWithSerial(serial)

	svc := &Service{
		Store:    store,
		Artifact: artifact,
		Fetcher:  &mockFetcher{body: body},
	}
	ctx := context.Background()
	cfg := registry.ConfigFor("ripencc")

	// First fetch: should create version and save artifact
	result1, err := svc.FetchLatest(ctx, cfg, "ripencc")
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if result1.Status != "created" {
		t.Errorf("first fetch: status=%q err=%q", result1.Status, result1.Error)
	}
	if result1.Registry != "ripencc" || result1.Serial != serial || result1.State != "validated" {
		t.Errorf("first fetch: %+v", result1)
	}

	// Second fetch with same serial (same body): idempotent, existing
	result2, err := svc.FetchLatest(ctx, cfg, "ripencc")
	if err != nil {
		t.Fatalf("FetchLatest second: %v", err)
	}
	if result2.Status != "existing" {
		t.Errorf("second fetch: expected existing, got %q", result2.Status)
	}
	if result2.DatasetID != result1.DatasetID {
		t.Errorf("second fetch: dataset_id should match first, got %s", result2.DatasetID)
	}
}

func TestFetchLatest_Integration_FetcherError(t *testing.T) {
	store, err := storage.NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	dir := t.TempDir()
	artifact, _ := storage.NewArtifactStore(dir)
	svc := &Service{
		Store:    store,
		Artifact: artifact,
		Fetcher:  &mockFetcher{err: errors.New("network error")},
	}
	ctx := context.Background()
	cfg := registry.ConfigFor("ripencc")

	result, err := svc.FetchLatest(ctx, cfg, "ripencc")
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if result.Status != "failed" {
		t.Errorf("expected failed when fetcher errors, got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestFetchLatest_Integration_InvalidHeaderRejected(t *testing.T) {
	store, err := storage.NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	dir := t.TempDir()
	artifact, _ := storage.NewArtifactStore(dir)
	// Body with invalid header (not enough fields)
	svc := &Service{
		Store:    store,
		Artifact: artifact,
		Fetcher:  &mockFetcher{body: "2|ripencc\n"},
	}
	ctx := context.Background()
	cfg := registry.ConfigFor("ripencc")

	result, err := svc.FetchLatest(ctx, cfg, "ripencc")
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if result.Status != "rejected" {
		t.Errorf("expected rejected for invalid header, got %q", result.Status)
	}
}

func TestFetchLatest_Integration_InvalidRegistryInHeaderRejected(t *testing.T) {
	store, err := storage.NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	dir := t.TempDir()
	artifact, _ := storage.NewArtifactStore(dir)
	// Header says "unknownreg" which is not in ValidRegistry
	body := "2|unknownreg|1773529199|0|20240101|20240102|+00\n"
	svc := &Service{
		Store:    store,
		Artifact: artifact,
		Fetcher:  &mockFetcher{body: body},
	}
	ctx := context.Background()
	cfg := registry.ConfigFor("ripencc")

	result, err := svc.FetchLatest(ctx, cfg, "ripencc")
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if result.Status != "rejected" {
		t.Errorf("expected rejected for invalid registry in header, got %q", result.Status)
	}
}

func TestNewArtifactStore_ValidDir(t *testing.T) {
	dir := t.TempDir()
	artifact, err := storage.NewArtifactStore(dir)
	if err != nil {
		t.Fatalf("NewArtifactStore: %v", err)
	}
	if artifact.BaseDir != dir {
		t.Errorf("BaseDir: got %q", artifact.BaseDir)
	}
}

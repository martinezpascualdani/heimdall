package importsvc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/storage"
)

func getTestDSN(t *testing.T) string {
	dsn := os.Getenv("SCOPE_DB_DSN")
	// Por defecto usa la base de test para no contaminar datos de desarrollo
	if dsn == "" {
		return "postgres://heimdall:heimdall@localhost:5432/heimdall_scope_service_test?sslmode=disable"
	}
	return dsn
}

func TestImport_Integration_ValidArtifactAndIdempotent(t *testing.T) {
	store, err := storage.NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	datasetID := uuid.New()
	// Minimal delegated file: header + 2 allocated IPv4 + 1 assigned IPv6; one reserved (ignored)
	artifactBody := "2|ripencc|1773529199|4|20240101|20240102|+00\n" +
		"ripencc|ES|ipv4|1.2.3.0|256|20240101|allocated\n" +
		"ripencc|ES|ipv4|1.2.4.0|512|20240101|assigned\n" +
		"ripencc|ES|ipv6|2001:db8::|32|20240101|allocated\n" +
		"ripencc|ES|ipv4|9.9.9.0|256|20240101|reserved\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/datasets/" + datasetID.String():
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"registry":"ripencc","serial":1773529199}`))
		case "/v1/datasets/" + datasetID.String() + "/artifact":
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(artifactBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := &Service{
		Store:       store,
		DatasetBase: server.URL,
		Client:      &http.Client{Timeout: 10 * time.Second},
	}
	ctx := context.Background()

	// First import: should persist 3 blocks (allocated/assigned only; reserved ignored)
	result1, err := svc.Import(ctx, datasetID)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result1.Status != "imported" {
		t.Errorf("first import: status=%q err=%q", result1.Status, result1.Error)
	}
	if result1.BlocksPersisted != 3 {
		t.Errorf("first import: expected 3 blocks (2 ipv4 allocated/assigned + 1 ipv6), got %d", result1.BlocksPersisted)
	}

	// Second import: idempotent, already_imported
	result2, err := svc.Import(ctx, datasetID)
	if err != nil {
		t.Fatalf("Import second: %v", err)
	}
	if result2.Status != "already_imported" {
		t.Errorf("second import: expected already_imported, got %q", result2.Status)
	}
	if result2.BlocksPersisted != 3 {
		t.Errorf("second import: BlocksPersisted should be 3 (from existing), got %d", result2.BlocksPersisted)
	}
}

func TestImport_Integration_EmptyCCIgnored(t *testing.T) {
	store, err := storage.NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	datasetID := uuid.New()
	artifactBody := "2|ripencc|1773529199|2|20240101|20240102|+00\n" +
		"ripencc||ipv4|1.2.3.0|256|20240101|allocated\n" +
		"ripencc|ES|ipv4|1.2.4.0|256|20240101|allocated\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/datasets/"+datasetID.String() {
			w.Write([]byte(`{"registry":"ripencc","serial":1}`))
			return
		}
		if r.URL.Path == "/v1/datasets/"+datasetID.String()+"/artifact" {
			w.Write([]byte(artifactBody))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	svc := &Service{Store: store, DatasetBase: server.URL, Client: &http.Client{Timeout: 5 * time.Second}}
	result, err := svc.Import(context.Background(), datasetID)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Status != "imported" {
		t.Errorf("status=%q", result.Status)
	}
	// Only ES block; empty cc row ignored
	if result.BlocksPersisted != 1 {
		t.Errorf("expected 1 block (empty cc ignored), got %d", result.BlocksPersisted)
	}
}

func TestImport_Integration_Artifact404(t *testing.T) {
	store, err := storage.NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	datasetID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/datasets/"+datasetID.String() {
			w.Write([]byte(`{"registry":"ripencc"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	svc := &Service{Store: store, DatasetBase: server.URL, Client: &http.Client{Timeout: 5 * time.Second}}
	result, err := svc.Import(context.Background(), datasetID)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Status != "failed" {
		t.Errorf("expected failed when artifact 404, got %q", result.Status)
	}
}

func TestImport_Integration_ASNPersistedAndIdempotent(t *testing.T) {
	store, err := storage.NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	datasetID := uuid.New()
	// Header + 1 IPv4 block + 2 ASN ranges (one single ASN asn_count=1, one range) + 1 ASN with invalid value (ignored)
	artifactBody := "2|ripencc|1773529200|5|20240101|20240102|+00\n" +
		"ripencc|ES|ipv4|1.2.3.0|256|20240101|allocated\n" +
		"ripencc|ES|asn|65536|1|20240101|allocated\n" +
		"ripencc|ES|asn|65540|10|20240101|assigned\n" +
		"ripencc|ES|asn|99999|not_a_number|20240101|allocated\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/datasets/" + datasetID.String():
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"registry":"ripencc","serial":1773529200}`))
		case "/v1/datasets/" + datasetID.String() + "/artifact":
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(artifactBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := &Service{
		Store:       store,
		DatasetBase: server.URL,
		Client:      &http.Client{Timeout: 10 * time.Second},
	}
	ctx := context.Background()

	// First import: 1 block + 2 ASN ranges (invalid ASN line ignored)
	result1, err := svc.Import(ctx, datasetID)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result1.Status != "imported" {
		t.Errorf("first import: status=%q", result1.Status)
	}
	if result1.BlocksPersisted != 1 {
		t.Errorf("first import: expected 1 block, got %d", result1.BlocksPersisted)
	}
	if result1.AsnsPersisted != 2 {
		t.Errorf("first import: expected 2 ASNs (one single, one range; invalid line ignored), got %d", result1.AsnsPersisted)
	}

	// Reimport: idempotent; ON CONFLICT DO NOTHING so no new rows → blocks_persisted/asns_persisted from existing
	result2, err := svc.Import(ctx, datasetID)
	if err != nil {
		t.Fatalf("Import second: %v", err)
	}
	if result2.Status != "already_imported" {
		t.Errorf("second import: expected already_imported, got %q", result2.Status)
	}
	if result2.AsnsPersisted != 2 {
		t.Errorf("second import: AsnsPersisted should be 2 (from existing), not re-counted; got %d", result2.AsnsPersisted)
	}
}

func TestSync_Integration_RequiresDatasetService(t *testing.T) {
	store, err := storage.NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	svc := &Service{Store: store, DatasetBase: "", Client: &http.Client{}}
	_, err = svc.Sync(context.Background())
	if err == nil {
		t.Fatal("Sync with empty DatasetBase should error")
	}
	if err.Error() == "" {
		t.Error("error message should be non-empty")
	}
}

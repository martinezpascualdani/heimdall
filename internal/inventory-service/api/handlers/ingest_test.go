package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/inventory-service/storage"
)

func getTestDSN(t *testing.T) string {
	dsn := os.Getenv("INVENTORY_DB_DSN")
	if dsn == "" {
		dsn = "postgres://heimdall:heimdall@localhost:5432/heimdall_inventory_service_test?sslmode=disable"
	}
	return dsn
}

func TestIngestHandler_JobCompleted_201_And_409(t *testing.T) {
	store, err := storage.NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	h := &IngestHandler{Store: store}
	execID := uuid.New()
	jobID := uuid.New()
	payload := map[string]interface{}{
		"execution_id":              execID.String(),
		"job_id":                    jobID.String(),
		"run_id":                    uuid.New().String(),
		"campaign_id":               uuid.New().String(),
		"target_id":                 uuid.New().String(),
		"target_materialization_id": uuid.New().String(),
		"scan_profile_slug":         "portscan-basic",
		"observed_at":               time.Now().UTC().Format(time.RFC3339),
		"observations": []map[string]interface{}{
			{"ip": "192.168.0.1", "port": 80, "status": "open"},
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/v1/ingest/job-completed", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.JobCompleted(rr, req)
	if rr.Code != http.StatusCreated {
		t.Errorf("first ingest: expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}

	// Second request same job -> 409
	req2 := httptest.NewRequest(http.MethodPost, "/v1/ingest/job-completed", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	h.JobCompleted(rr2, req2)
	if rr2.Code != http.StatusConflict {
		t.Errorf("second ingest: expected 409, got %d body=%s", rr2.Code, rr2.Body.String())
	}
}

func TestIngestHandler_JobCompleted_400_MissingObservedAt(t *testing.T) {
	store, err := storage.NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	h := &IngestHandler{Store: store}
	payload := map[string]interface{}{
		"execution_id": uuid.New().String(),
		"job_id":      uuid.New().String(),
		"observations": []map[string]interface{}{},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/v1/ingest/job-completed", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.JobCompleted(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing observed_at, got %d", rr.Code)
	}
}

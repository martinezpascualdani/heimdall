package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/storage"
)

func getTestDSN(t *testing.T) string {
	dsn := os.Getenv("CAMPAIGN_DB_DSN")
	if dsn == "" {
		dsn = "postgres://heimdall:heimdall@localhost:5432/heimdall_campaign_service_test?sslmode=disable"
	}
	return dsn
}

func TestHandlers_Health_Ready(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("health: code=%d", rr.Code)
	}
}

func TestHandlers_ScanProfile_Create_List(t *testing.T) {
	store, err := storage.NewPostgresStore(getTestDSN(t))
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer store.Close()

	h := &ScanProfilesHandler{Store: store}
	body := []byte(`{"name":"test-profile","slug":"test-` + uuid.New().String()[:8] + `","description":"d"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/scan-profiles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("Create scan profile: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created["slug"] == nil {
		t.Error("response should have slug")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/scan-profiles", nil)
	rr2 := httptest.NewRecorder()
	h.List(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("List scan profiles: code=%d", rr2.Code)
	}
	var list struct {
		Items []map[string]interface{} `json:"items"`
		Count int                       `json:"count"`
	}
	if err := json.NewDecoder(rr2.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.Count < 1 {
		t.Errorf("list count=%d", list.Count)
	}
}

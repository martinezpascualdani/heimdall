package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/http/handlers"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/importsvc"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/ipresolver"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/storage"
)

var (
	version   = "0.1.0"
	buildTime = "unknown"
)

func main() {
	dsn := os.Getenv("SCOPE_DB_DSN")
	if dsn == "" {
		dsn = "postgres://heimdall:heimdall@localhost:5432/heimdall_scope_service?sslmode=disable"
	}
	datasetBase := os.Getenv("DATASET_SERVICE_URL")
	if datasetBase == "" {
		datasetBase = "http://localhost:8080"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	store, err := storage.NewPostgresStore(dsn)
	if err != nil {
		log.Fatalf("scope-service: postgres: %v", err)
	}
	defer store.Close()

	importSvc := &importsvc.Service{
		Store:       store,
		DatasetBase: strings.TrimSuffix(datasetBase, "/"),
		Client:      &http.Client{Timeout: 15 * time.Minute},
	}

	ipResolveSvc := &ipresolver.Service{Store: store}
	datasetClientShort := &http.Client{Timeout: 5 * time.Second}
	datasetBaseURL := strings.TrimSuffix(datasetBase, "/")

	ipResolveHandler := &handlers.IPResolveHandler{
		Resolver:       ipResolveSvc,
		DatasetBaseURL: datasetBaseURL,
		DatasetClient:  datasetClientShort,
	}

	blocksHandler := &handlers.CountryBlocksHandler{
		Store:          store,
		DatasetBaseURL: datasetBaseURL,
		DatasetClient:  datasetClientShort,
	}
	summaryHandler := &handlers.CountrySummaryHandler{
		Store:          store,
		DatasetBaseURL: datasetBaseURL,
		DatasetClient:  datasetClientShort,
	}
	datasetsHandler := &handlers.CountryDatasetsHandler{
		Store:          store,
		DatasetBaseURL: datasetBaseURL,
		DatasetClient:  datasetClientShort,
	}
	asnsHandler := &handlers.CountryASNsHandler{
		Store:          store,
		DatasetBaseURL: datasetBaseURL,
		DatasetClient:  datasetClientShort,
	}
	asnSummaryHandler := &handlers.CountryASNSummaryHandler{
		Store:          store,
		DatasetBaseURL: datasetBaseURL,
		DatasetClient:  datasetClientShort,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if err := store.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"not ready"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"version": version, "build_time": buildTime})
	})

	mux.HandleFunc("POST /v1/import", func(w http.ResponseWriter, r *http.Request) {
		datasetIDStr := r.URL.Query().Get("dataset_id")
		if datasetIDStr == "" {
			http.Error(w, `{"error":"dataset_id required"}`, http.StatusBadRequest)
			return
		}
		datasetID, err := uuid.Parse(datasetIDStr)
		if err != nil {
			http.Error(w, "invalid dataset_id", http.StatusBadRequest)
			return
		}
		result, err := importSvc.Import(r.Context(), datasetID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		status := http.StatusOK
		if result.Status == "imported" {
			status = http.StatusCreated
		}
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("POST /v1/imports/sync", func(w http.ResponseWriter, r *http.Request) {
		resp, err := importSvc.Sync(r.Context())
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	})

	mux.Handle("GET /v1/scopes/by-ip/{ip}", ipResolveHandler)

	mux.Handle("GET /v1/scopes/country/{cc}/blocks", blocksHandler)
	mux.Handle("GET /v1/scopes/country/{cc}/summary", summaryHandler)
	mux.Handle("GET /v1/scopes/country/{cc}/asns", asnsHandler)
	mux.Handle("GET /v1/scopes/country/{cc}/asn-summary", asnSummaryHandler)
	mux.Handle("GET /v1/scopes/country/{cc}/datasets", datasetsHandler)

	srv := &http.Server{Addr: ":" + port, Handler: mux}
	log.Printf("scope-service listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/martinezpascualdani/heimdall/internal/inventory-service/api/handlers"
	"github.com/martinezpascualdani/heimdall/internal/inventory-service/storage"
)

var (
	version   = "0.1.0"
	buildTime = "unknown"
)

func main() {
	dsn := os.Getenv("INVENTORY_DB_DSN")
	if dsn == "" {
		dsn = "postgres://heimdall:heimdall@localhost:5432/heimdall_inventory_service?sslmode=disable"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8086"
	}

	store, err := storage.NewPostgresStore(dsn)
	if err != nil {
		log.Fatalf("inventory-service: postgres: %v", err)
	}
	defer store.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) {
		if err := store.Ping(); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"status": "not ready", "reason": "database"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"version": version, "build_time": buildTime})
	})

	ingestHandler := &handlers.IngestHandler{Store: store}
	assetsHandler := &handlers.AssetsHandler{Store: store}
	exposuresHandler := &handlers.ExposuresHandler{Store: store}
	observationsHandler := &handlers.ObservationsHandler{Store: store}
	diffsHandler := &handlers.DiffsHandler{Store: store}

	mux.HandleFunc("POST /v1/ingest/job-completed", ingestHandler.JobCompleted)
	mux.HandleFunc("GET /v1/assets", assetsHandler.List)
	mux.HandleFunc("GET /v1/assets/{id}/exposures", exposuresHandler.ListByAssetID)
	mux.HandleFunc("GET /v1/assets/{id}", assetsHandler.Get)
	mux.HandleFunc("GET /v1/exposures", exposuresHandler.List)
	mux.HandleFunc("GET /v1/observations", observationsHandler.List)
	mux.HandleFunc("GET /v1/diffs/executions", diffsHandler.ExecutionsDiff)

	srv := &http.Server{Addr: ":" + port, Handler: mux}
	log.Printf("inventory-service listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

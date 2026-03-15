package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/martinezpascualdani/heimdall/internal/routing-service/http/handlers"
	"github.com/martinezpascualdani/heimdall/internal/routing-service/importsvc"
	"github.com/martinezpascualdani/heimdall/internal/routing-service/storage"
)

var (
	version   = "0.1.0"
	buildTime = "unknown"
)

func main() {
	dsn := os.Getenv("ROUTING_DB_DSN")
	if dsn == "" {
		dsn = "postgres://heimdall:heimdall@localhost:5432/heimdall_routing_service?sslmode=disable"
	}
	datasetBase := os.Getenv("DATASET_SERVICE_URL")
	if datasetBase == "" {
		datasetBase = "http://localhost:8080"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	store, err := storage.NewPostgresStore(dsn)
	if err != nil {
		log.Fatalf("routing-service: postgres: %v", err)
	}
	defer store.Close()

	syncSvc := &importsvc.Service{
		Store:       store,
		DatasetBase: strings.TrimSuffix(datasetBase, "/"),
		Client:      &http.Client{Timeout: 30 * time.Minute},
	}

	byIPHandler := &handlers.ByIPHandler{Store: store}
	asnMetaHandler := &handlers.ASNMetaHandler{Store: store}
	asnPrefixesHandler := &handlers.ASNPrefixesHandler{Store: store}
	syncHandler := &handlers.SyncHandler{Sync: syncSvc}

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

	mux.Handle("GET /v1/asn/by-ip/{ip}", byIPHandler)
	mux.Handle("GET /v1/asn/prefixes/{asn}", asnPrefixesHandler)
	mux.Handle("GET /v1/asn/{asn}", asnMetaHandler)
	mux.Handle("POST /v1/imports/sync", syncHandler)

	srv := &http.Server{Addr: ":" + port, Handler: mux}
	go func() {
		log.Printf("routing-service listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
	log.Println("routing-service stopped")
}

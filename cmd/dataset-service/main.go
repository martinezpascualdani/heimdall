package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/dataset-service/fetch"
	"github.com/martinezpascualdani/heimdall/internal/dataset-service/storage"
	"github.com/martinezpascualdani/heimdall/pkg/registry"
)

var (
	version   = "0.1.0"
	buildTime = "unknown"
)

func main() {
	dsn := os.Getenv("DATASET_DB_DSN")
	if dsn == "" {
		dsn = "postgres://heimdall:heimdall@localhost:5432/heimdall_datasets?sslmode=disable"
	}
	baseDir := os.Getenv("DATASET_STORAGE_PATH")
	if baseDir == "" {
		baseDir = "./data/datasets"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	store, err := storage.NewPostgresStore(dsn)
	if err != nil {
		log.Fatalf("dataset-service: postgres: %v", err)
	}
	defer store.Close()

	artifactStore, err := storage.NewArtifactStore(baseDir)
	if err != nil {
		log.Fatalf("dataset-service: artifact store: %v", err)
	}

	cfg := registry.DefaultRIPENCCConfig()
	fetchSvc := &fetch.Service{
		Store:    store,
		Artifact: artifactStore,
		Fetcher:  registry.NewHTTPFetcher(cfg.Timeout),
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

	mux.HandleFunc("POST /v1/datasets/fetch", func(w http.ResponseWriter, r *http.Request) {
		sourceParam := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("source")))
		registryName := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("registry")))
		allRegistries := []string{"ripencc", "arin", "apnic", "lacnic", "afrinic"}
		caidaSources := []string{fetch.SourceCAIDAPfx2asIPv4, fetch.SourceCAIDAPfx2asIPv6, fetch.SourceCAIDAASOrg}

		// If ?source= is set, use CAIDA (do not use registry)
		if sourceParam != "" {
			validCAIDA := false
			for _, cs := range caidaSources {
				if sourceParam == cs {
					validCAIDA = true
					break
				}
			}
			if !validCAIDA {
				http.Error(w, `{"error":"unsupported source, use: caida_pfx2as_ipv4, caida_pfx2as_ipv6, caida_as_org"}`, http.StatusBadRequest)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
			defer cancel()
			result, err := fetchSvc.FetchCAIDA(ctx, sourceParam)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			status := http.StatusOK
			if result.Status == "created" {
				status = http.StatusCreated
			}
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(result)
			return
		}

		if registryName == "" || registryName == "all" {
			// Fetch all RIRs; timeout allows for all five
			ctx, cancel := context.WithTimeout(r.Context(), 55*time.Minute)
			defer cancel()
			var results []*fetch.FetchResult
			for _, reg := range allRegistries {
				cfg := registry.ConfigFor(reg)
				result, err := fetchSvc.FetchLatest(ctx, cfg, reg)
				if err != nil {
					results = append(results, &fetch.FetchResult{Registry: reg, Status: "failed", State: "failed", Error: err.Error()})
					continue
				}
				results = append(results, result)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{"results": results})
			return
		}

		valid := false
		for _, reg := range allRegistries {
			if registryName == reg {
				valid = true
				break
			}
		}
		if !valid {
			http.Error(w, `{"error":"unsupported registry, use: ripencc, arin, apnic, lacnic, afrinic or omit for all"}`, http.StatusBadRequest)
			return
		}
		cfg := registry.ConfigFor(registryName)
		ctx, cancel := context.WithTimeout(r.Context(), cfg.Timeout)
		defer cancel()
		result, err := fetchSvc.FetchLatest(ctx, cfg, registryName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var status int = http.StatusOK
		if result.Status == "created" {
			status = http.StatusCreated
		}
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("GET /v1/datasets", func(w http.ResponseWriter, r *http.Request) {
		source := strings.TrimSpace(r.URL.Query().Get("source"))
		sourceType := strings.TrimSpace(r.URL.Query().Get("source_type"))
		list, err := store.List(100, source, sourceType)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"datasets": list})
	})

	mux.HandleFunc("GET /v1/datasets/{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := uuid.Parse(idStr)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		v, err := store.GetByID(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if v == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	})

	mux.HandleFunc("GET /v1/datasets/{id}/artifact", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := uuid.Parse(idStr)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		v, err := store.GetByID(id)
		if err != nil || v == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if v.State != "validated" || v.StoragePath == "" {
			http.Error(w, "artifact not available", http.StatusNotFound)
			return
		}
		rc, err := artifactStore.Open(v.StoragePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", "inline")
		io.Copy(w, rc)
	})

	srv := &http.Server{Addr: ":" + port, Handler: mux}
	go func() {
		log.Printf("dataset-service listening on %s", srv.Addr)
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
	log.Println("dataset-service stopped")
}

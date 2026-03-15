package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/martinezpascualdani/heimdall/internal/campaign-service/dispatch"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/http/handlers"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/scheduler"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/storage"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/targetclient"
	"github.com/redis/go-redis/v9"
)

var (
	version   = "0.1.0"
	buildTime = "unknown"
)

func main() {
	dsn := os.Getenv("CAMPAIGN_DB_DSN")
	if dsn == "" {
		dsn = "postgres://heimdall:heimdall@localhost:5432/heimdall_campaign_service?sslmode=disable"
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	targetURL := strings.TrimSuffix(os.Getenv("TARGET_SERVICE_URL"), "/")
	if targetURL == "" {
		targetURL = "http://localhost:8083"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}
	schedulerEnabled := os.Getenv("CAMPAIGN_SCHEDULER_ENABLED")
	if schedulerEnabled == "" {
		schedulerEnabled = "true"
	}
	enableScheduler, _ := strconv.ParseBool(schedulerEnabled)
	materializeTimeout := 10 * time.Minute

	store, err := storage.NewPostgresStore(dsn)
	if err != nil {
		log.Fatalf("campaign-service: postgres: %v", err)
	}
	defer store.Close()

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()
	// Redis is required for /ready; service starts anyway and /ready will be false if Redis is down

	dispatcher := dispatch.NewRedisDispatcher(rdb, "")
	targetClient := targetclient.NewClient(targetURL, materializeTimeout)

	campaignsHandler := &handlers.CampaignsHandler{
		Store:         store,
		ProfileGetter: store,
		Validator:     targetClient,
	}
	scanProfilesHandler := &handlers.ScanProfilesHandler{Store: store}
	runsHandler := &handlers.RunsHandler{Store: store}
	launchHandler := &handlers.LaunchHandler{
		Store:            store,
		ProfileStore:     store,
		ActiveRunChecker: store,
		RunUpdater:       store,
		CampaignUpdater:  store,
		Target:           targetClient,
		Dispatcher:       dispatcher,
	}
	cancelHandler := &handlers.CancelHandler{RunStore: store, RunUpdater: store}

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
		if err := rdb.Ping(context.Background()).Err(); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"status": "not ready", "reason": "redis"})
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

	mux.HandleFunc("POST /v1/campaigns", campaignsHandler.Create)
	mux.HandleFunc("GET /v1/campaigns", campaignsHandler.List)
	mux.HandleFunc("GET /v1/campaigns/{id}", campaignsHandler.Get)
	mux.HandleFunc("PUT /v1/campaigns/{id}", campaignsHandler.Update)
	mux.HandleFunc("DELETE /v1/campaigns/{id}", campaignsHandler.Delete)

	mux.HandleFunc("POST /v1/scan-profiles", scanProfilesHandler.Create)
	mux.HandleFunc("GET /v1/scan-profiles", scanProfilesHandler.List)
	mux.HandleFunc("GET /v1/scan-profiles/{id}", scanProfilesHandler.Get)
	mux.HandleFunc("PUT /v1/scan-profiles/{id}", scanProfilesHandler.Update)
	mux.HandleFunc("DELETE /v1/scan-profiles/{id}", scanProfilesHandler.Delete)

	mux.HandleFunc("GET /v1/campaigns/{id}/runs", runsHandler.ListByCampaign)
	mux.HandleFunc("GET /v1/runs/{id}", runsHandler.GetRun)

	mux.HandleFunc("POST /v1/campaigns/{id}/launch", launchHandler.ServeHTTP)
	mux.HandleFunc("POST /v1/runs/{id}/cancel", cancelHandler.ServeHTTP)

	sched := &scheduler.Scheduler{
		Store:      store,
		Target:     targetClient,
		Dispatcher: dispatcher,
		Interval:   60 * time.Second,
		MaxRuns:    10,
		Enabled:    enableScheduler,
	}
	sched.Start()
	defer sched.Stop()

	srv := &http.Server{Addr: ":" + port, Handler: mux}
	log.Printf("campaign-service listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

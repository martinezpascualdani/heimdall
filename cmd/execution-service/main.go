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

	"github.com/martinezpascualdani/heimdall/internal/execution-service/api/handlers"
	"github.com/martinezpascualdani/heimdall/internal/execution-service/inventoryclient"
	"github.com/martinezpascualdani/heimdall/internal/execution-service/ingest"
	"github.com/martinezpascualdani/heimdall/internal/execution-service/scheduler"
	"github.com/martinezpascualdani/heimdall/internal/execution-service/storage"
	"github.com/martinezpascualdani/heimdall/internal/execution-service/targetclient"
	"github.com/redis/go-redis/v9"
)

var (
	version   = "0.1.0"
	buildTime = "unknown"
)

func main() {
	dsn := os.Getenv("EXECUTION_DB_DSN")
	if dsn == "" {
		dsn = "postgres://heimdall:heimdall@localhost:5432/heimdall_execution_service?sslmode=disable"
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
		port = "8085"
	}
	leaseDur := 5 * time.Minute
	if s := os.Getenv("JOB_LEASE_DURATION"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			leaseDur = d
		}
	}
	heartbeatTimeout := 2 * time.Minute
	if s := os.Getenv("WORKER_HEARTBEAT_TIMEOUT"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			heartbeatTimeout = d
		}
	}
	schedulerInterval := 30 * time.Second
	if s := os.Getenv("SCHEDULER_INTERVAL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			schedulerInterval = d
		}
	}
	ingestEnabled := true
	if s := os.Getenv("INGEST_ENABLED"); s != "" {
		if b, err := strconv.ParseBool(s); err == nil {
			ingestEnabled = b
		}
	}
	inventoryURL := strings.TrimSuffix(os.Getenv("INVENTORY_SERVICE_URL"), "/")

	store, err := storage.NewPostgresStore(dsn)
	if err != nil {
		log.Fatalf("execution-service: postgres: %v", err)
	}
	defer store.Close()

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()

	targetClient := targetclient.NewClient(targetURL, 5*time.Minute)

	ingestConsumer := ingest.NewConsumer(rdb, store, targetClient, ingest.Config{
		PrefixesPageSize:  1000,
		JobsPrefixBatch:   10,
		ReadBlockDuration: 5 * time.Second,
	})
	if ingestEnabled {
		if err := ingestConsumer.EnsureConsumerGroup(context.Background()); err != nil {
			log.Printf("execution-service: ensure consumer group: %v", err)
		}
		go ingestConsumer.Run(context.Background())
	}

	sched := scheduler.NewScheduler(store, heartbeatTimeout, schedulerInterval)
	go sched.Run(context.Background())

	inventoryNotifier := inventoryclient.NewClient(inventoryURL, store)

	workersHandler := &handlers.WorkersHandler{Store: store}
	jobsHandler := &handlers.JobsHandler{Store: store, LeaseDuration: leaseDur, InventoryNotifier: inventoryNotifier}
	executionsHandler := &handlers.ExecutionsHandler{Store: store}

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

	mux.HandleFunc("POST /v1/workers", workersHandler.Register)
	mux.HandleFunc("GET /v1/workers", workersHandler.List)
	mux.HandleFunc("GET /v1/workers/{id}/jobs", workersHandler.ListJobs)
	mux.HandleFunc("GET /v1/workers/{id}", workersHandler.Get)
	mux.HandleFunc("PATCH /v1/workers/{id}", workersHandler.Heartbeat)

	mux.HandleFunc("POST /v1/jobs/claim", jobsHandler.Claim)
	mux.HandleFunc("POST /v1/jobs/{id}/complete", jobsHandler.Complete)
	mux.HandleFunc("POST /v1/jobs/{id}/fail", jobsHandler.Fail)
	mux.HandleFunc("POST /v1/jobs/{id}/renew", jobsHandler.Renew)

	mux.HandleFunc("GET /v1/executions", executionsHandler.List)
	mux.HandleFunc("GET /v1/executions/{id}", executionsHandler.Get)
	mux.HandleFunc("GET /v1/executions/{id}/jobs", executionsHandler.ListJobs)
	mux.HandleFunc("POST /v1/executions/{id}/requeue", executionsHandler.Requeue)
	mux.HandleFunc("POST /v1/executions/{id}/cancel", executionsHandler.Cancel)

	srv := &http.Server{Addr: ":" + port, Handler: mux}
	log.Printf("execution-service listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

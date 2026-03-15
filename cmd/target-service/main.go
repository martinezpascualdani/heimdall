package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/martinezpascualdani/heimdall/internal/target-service/http/handlers"
	"github.com/martinezpascualdani/heimdall/internal/target-service/materializer"
	"github.com/martinezpascualdani/heimdall/internal/target-service/storage"
)

var (
	version   = "0.1.0"
	buildTime = "unknown"
)

func main() {
	dsn := os.Getenv("TARGET_DB_DSN")
	if dsn == "" {
		dsn = "postgres://heimdall:heimdall@localhost:5432/heimdall_target_service?sslmode=disable"
	}
	scopeURL := strings.TrimSuffix(os.Getenv("SCOPE_SERVICE_URL"), "/")
	if scopeURL == "" {
		scopeURL = "http://localhost:8081"
	}
	routingURL := strings.TrimSuffix(os.Getenv("ROUTING_SERVICE_URL"), "/")
	if routingURL == "" {
		routingURL = "http://localhost:8082"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}

	store, err := storage.NewPostgresStore(dsn)
	if err != nil {
		log.Fatalf("target-service: postgres: %v", err)
	}
	defer store.Close()

	httpClient := &http.Client{Timeout: 15 * time.Minute}
	scopeClient := &materializer.ScopeClient{BaseURL: scopeURL, Client: httpClient}
	routingClient := &materializer.RoutingClient{BaseURL: routingURL, Client: httpClient}
	matSvc := &materializer.Service{Store: store, Scope: scopeClient, Routing: routingClient}

	th := &handlers.TargetsHandler{Store: store}
	matHandler := &handlers.MaterializeHandler{Store: store, Materializer: matSvc}
	matListHandler := &handlers.MaterializationsHandler{Store: store}
	prefixesHandler := &handlers.PrefixesHandler{Store: store}
	diffHandler := &handlers.DiffHandler{Store: store}

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

	mux.HandleFunc("POST /v1/targets", th.Create)
	mux.HandleFunc("GET /v1/targets", th.List)
	mux.HandleFunc("GET /v1/targets/{id}", th.Get)
	mux.HandleFunc("PUT /v1/targets/{id}", th.Update)
	mux.HandleFunc("DELETE /v1/targets/{id}", th.Delete)

	mux.HandleFunc("POST /v1/targets/{id}/materialize", matHandler.ServeHTTP)
	mux.HandleFunc("GET /v1/targets/{id}/materializations/diff", diffHandler.ServeHTTP)
	mux.HandleFunc("GET /v1/targets/{id}/materializations/{mid}/prefixes", prefixesHandler.ServeHTTP)
	mux.HandleFunc("GET /v1/targets/{id}/materializations/{mid}", matListHandler.Get)
	mux.HandleFunc("GET /v1/targets/{id}/materializations", matListHandler.List)

	srv := &http.Server{Addr: ":" + port, Handler: mux}
	log.Printf("target-service listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

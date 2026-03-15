package importsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/routing-service/domain"
	"github.com/martinezpascualdani/heimdall/internal/routing-service/storage"
)

// Service runs the routing import pipeline (pfx2as + as-org from dataset-service).
type Service struct {
	Store       *storage.PostgresStore
	DatasetBase string // e.g. http://dataset-service:8080
	Client      *http.Client
}

// SyncResultItem is one result per source for POST /v1/imports/sync.
type SyncResultItem struct {
	Source        string    `json:"source"`
	DatasetID     uuid.UUID `json:"dataset_id"`
	Status        string    `json:"status"`
	RowsPersisted int64     `json:"rows_persisted"`
	DurationMs    int64     `json:"duration_ms"`
	Error         string    `json:"error,omitempty"`
}

// SyncResponse is the response of POST /v1/imports/sync.
type SyncResponse struct {
	Results []SyncResultItem `json:"results"`
}

// Routing sources (must match dataset-service and fetch constants).
const (
	SourcePfx2asIPv4 = "caida_pfx2as_ipv4"
	SourcePfx2asIPv6 = "caida_pfx2as_ipv6"
	SourceASOrg      = "caida_as_org"
)

var syncSources = []string{SourcePfx2asIPv4, SourcePfx2asIPv6, SourceASOrg}

// Sync fetches the latest validated dataset per source from dataset-service and imports; returns one result per source.
func (s *Service) Sync(ctx context.Context) (*SyncResponse, error) {
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	var results []SyncResultItem
	for _, source := range syncSources {
		item := s.syncOne(ctx, client, source)
		results = append(results, item)
	}
	return &SyncResponse{Results: results}, nil
}

func (s *Service) syncOne(ctx context.Context, client *http.Client, source string) SyncResultItem {
	start := time.Now()
	datasetID, err := s.fetchLatestValidatedBySource(ctx, client, source)
	if err != nil {
		return SyncResultItem{Source: source, Status: "failed", Error: err.Error(), DurationMs: time.Since(start).Milliseconds()}
	}
	if datasetID == nil {
		return SyncResultItem{Source: source, Status: "skipped", Error: "no validated dataset for source", DurationMs: time.Since(start).Milliseconds()}
	}

	existing, _ := s.Store.FindRoutingImportByDatasetAndSource(*datasetID, source)
	if existing != nil && (existing.State == domain.RoutingImportImported || existing.State == domain.RoutingImportAlready || existing.State == domain.RoutingImportReimported) {
		return SyncResultItem{
			Source:        source,
			DatasetID:     *datasetID,
			Status:        "already_imported",
			RowsPersisted: existing.RowsPersisted,
			DurationMs:    time.Since(start).Milliseconds(),
		}
	}

	importID := uuid.New()
	imp := &domain.RoutingImport{ID: importID, DatasetID: *datasetID, Source: source, State: domain.RoutingImportRunning, CreatedAt: start}
	if err := s.Store.CreateRoutingImport(imp); err != nil {
		return SyncResultItem{Source: source, DatasetID: *datasetID, Status: "failed", Error: err.Error(), DurationMs: time.Since(start).Milliseconds()}
	}

	var rowsPersisted int64
	switch source {
	case SourcePfx2asIPv4:
		rowsPersisted, err = s.importPfx2as(ctx, client, *datasetID, source, "ipv4")
	case SourcePfx2asIPv6:
		rowsPersisted, err = s.importPfx2as(ctx, client, *datasetID, source, "ipv6")
	case SourceASOrg:
		rowsPersisted, err = s.importASOrg(ctx, client, *datasetID, source)
	default:
		err = fmt.Errorf("unknown source %s", source)
	}

	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		_ = s.Store.UpdateRoutingImportState(importID, domain.RoutingImportFailed, 0, durationMs, err.Error())
		return SyncResultItem{Source: source, DatasetID: *datasetID, Status: "failed", RowsPersisted: rowsPersisted, DurationMs: durationMs, Error: err.Error()}
	}
	_ = s.Store.UpdateRoutingImportState(importID, domain.RoutingImportImported, rowsPersisted, durationMs, "")
	return SyncResultItem{
		Source:        source,
		DatasetID:     *datasetID,
		Status:        "imported",
		RowsPersisted: rowsPersisted,
		DurationMs:    durationMs,
	}
}

func (s *Service) fetchLatestValidatedBySource(ctx context.Context, client *http.Client, source string) (*uuid.UUID, error) {
	base := strings.TrimSuffix(s.DatasetBase, "/")
	url := base + "/v1/datasets?source=" + source + "&source_type=caida"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("datasets list: %s", resp.Status)
	}
	var out struct {
		Datasets []struct {
			ID        string `json:"id"`
			State     string `json:"state"`
			CreatedAt string `json:"created_at"`
		} `json:"datasets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	var latestID uuid.UUID
	var latestTime time.Time
	for _, d := range out.Datasets {
		if d.State != "validated" {
			continue
		}
		id, err := uuid.Parse(d.ID)
		if err != nil {
			continue
		}
		t, _ := time.Parse(time.RFC3339, d.CreatedAt)
		if t.After(latestTime) {
			latestTime = t
			latestID = id
		}
	}
	if latestTime.IsZero() {
		return nil, nil
	}
	return &latestID, nil
}

func (s *Service) importPfx2as(ctx context.Context, client *http.Client, datasetID uuid.UUID, source, ipVersion string) (int64, error) {
	url := strings.TrimSuffix(s.DatasetBase, "/") + "/v1/datasets/" + datasetID.String() + "/artifact"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("artifact: %s", resp.Status)
	}
	count, err := ParsePfx2asStream(resp.Body, datasetID, source, ipVersion, func(o *domain.BGPPrefixOrigin) error {
		return s.Store.UpsertBGPPrefixOrigin(o)
	})
	return count, err
}

func (s *Service) importASOrg(ctx context.Context, client *http.Client, datasetID uuid.UUID, source string) (int64, error) {
	url := strings.TrimSuffix(s.DatasetBase, "/") + "/v1/datasets/" + datasetID.String() + "/artifact"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("artifact: %s", resp.Status)
	}
	return ParseASOrgStream(resp.Body, source, &datasetID, func(m *domain.ASNMetadata) error {
		return s.Store.UpsertASNMetadata(m)
	})
}

// Import runs a single-dataset import (used when dataset_id is specified). Returns status, rows_persisted, error.
func (s *Service) Import(ctx context.Context, datasetID uuid.UUID, source string) (status string, rowsPersisted int64, err error) {
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	existing, _ := s.Store.FindRoutingImportByDatasetAndSource(datasetID, source)
	if existing != nil && existing.State != domain.RoutingImportFailed && existing.State != domain.RoutingImportRunning {
		return "already_imported", existing.RowsPersisted, nil
	}
	start := time.Now()
	importID := uuid.New()
	imp := &domain.RoutingImport{ID: importID, DatasetID: datasetID, Source: source, State: domain.RoutingImportRunning, CreatedAt: start}
	if err := s.Store.CreateRoutingImport(imp); err != nil {
		return "failed", 0, err
	}
	switch source {
	case SourcePfx2asIPv4:
		rowsPersisted, err = s.importPfx2as(ctx, client, datasetID, source, "ipv4")
	case SourcePfx2asIPv6:
		rowsPersisted, err = s.importPfx2as(ctx, client, datasetID, source, "ipv6")
	case SourceASOrg:
		rowsPersisted, err = s.importASOrg(ctx, client, datasetID, source)
	default:
		return "failed", 0, fmt.Errorf("unknown source %s", source)
	}
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		_ = s.Store.UpdateRoutingImportState(importID, domain.RoutingImportFailed, 0, durationMs, err.Error())
		return "failed", 0, err
	}
	_ = s.Store.UpdateRoutingImportState(importID, domain.RoutingImportImported, rowsPersisted, durationMs, "")
	return "imported", rowsPersisted, nil
}

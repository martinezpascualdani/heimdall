package importsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/domain"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/storage"
	"github.com/martinezpascualdani/heimdall/pkg/rirparser"
)

// Service runs the import pipeline.
type Service struct {
	Store       *storage.PostgresStore
	DatasetBase string // e.g. http://dataset-service:8080
	Client      *http.Client
}

// ImportResult is the result of POST /v1/import.
type ImportResult struct {
	Status          string    `json:"status"`
	ImportID        uuid.UUID `json:"import_id,omitempty"`
	DatasetID       uuid.UUID `json:"dataset_id,omitempty"`
	BlocksPersisted int64     `json:"blocks_persisted,omitempty"`
	AsnsPersisted   int64     `json:"asns_persisted,omitempty"`
	DurationMs      int64     `json:"duration_ms,omitempty"`
	Error           string    `json:"error,omitempty"`
}

// SyncResultItem is one result of POST /v1/imports/sync.
type SyncResultItem struct {
	Registry        string    `json:"registry"`
	DatasetID       uuid.UUID `json:"dataset_id"`
	Status          string    `json:"status"`
	BlocksPersisted int64     `json:"blocks_persisted,omitempty"`
	AsnsPersisted   int64     `json:"asns_persisted,omitempty"`
	DurationMs      int64     `json:"duration_ms,omitempty"`
	Error           string    `json:"error,omitempty"`
}

// SyncResponse is the response of POST /v1/imports/sync.
type SyncResponse struct {
	Results []SyncResultItem `json:"results"`
}

// AllowedStatuses for Fase 1 (config effective).
var AllowedStatuses = []string{"allocated", "assigned"}

func configEffectiveHash() string {
	return fmt.Sprintf("status=%s", strings.Join(AllowedStatuses, ","))
}

// Import fetches the artifact from dataset-service, parses, filters and persists blocks for all countries.
func (s *Service) Import(ctx context.Context, datasetID uuid.UUID) (*ImportResult, error) {
	configEff := configEffectiveHash()
	existing, _ := s.Store.FindImportByDatasetAndConfig(datasetID, configEff)
	if existing != nil {
		return &ImportResult{
			Status:          string(domain.ImportStateAlreadyImported),
			ImportID:        existing.ID,
			DatasetID:       datasetID,
			BlocksPersisted: existing.BlocksPersisted,
			AsnsPersisted:   existing.AsnsPersisted,
			DurationMs:      existing.DurationMs,
		}, nil
	}

	importID := uuid.New()
	start := time.Now()
	registry := fetchRegistryFromDatasetService(ctx, s.DatasetBase, s.Client, datasetID)
	imp := &domain.ScopeImport{ID: importID, DatasetID: datasetID, Registry: registry, ConfigEffective: configEff, State: domain.ImportStateRunning, CreatedAt: start}
	if err := s.Store.CreateImport(imp); err != nil {
		return &ImportResult{Status: string(domain.ImportStateFailed), Error: err.Error()}, nil
	}

	artifactURL := fmt.Sprintf("%s/v1/datasets/%s/artifact", s.DatasetBase, datasetID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifactURL, nil)
	if err != nil {
		_ = s.Store.UpdateImportState(importID, domain.ImportStateFailed, 0, 0, err.Error())
		return &ImportResult{Status: string(domain.ImportStateFailed), ImportID: importID, DatasetID: datasetID, Error: err.Error()}, nil
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		_ = s.Store.UpdateImportState(importID, domain.ImportStateFailed, 0, 0, err.Error())
		return &ImportResult{Status: string(domain.ImportStateFailed), ImportID: importID, DatasetID: datasetID, Error: err.Error()}, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_ = s.Store.UpdateImportState(importID, domain.ImportStateFailed, 0, 0, fmt.Sprintf("artifact status %d", resp.StatusCode))
		return &ImportResult{Status: string(domain.ImportStateFailed), ImportID: importID, DatasetID: datasetID, Error: fmt.Sprintf("artifact status %d", resp.StatusCode)}, nil
	}

	result, err := rirparser.ParseStream(resp.Body)
	if err != nil {
		_ = s.Store.UpdateImportState(importID, domain.ImportStateFailed, 0, 0, err.Error())
		return &ImportResult{Status: string(domain.ImportStateFailed), ImportID: importID, DatasetID: datasetID, Error: err.Error()}, nil
	}

	go func() {
		for range result.Err {
		}
	}()

	var count, asnsPersisted int64
	statusSet := map[string]bool{}
	for _, st := range AllowedStatuses {
		statusSet[st] = true
	}
	for rec := range result.Records {
		if rec.Type == rirparser.TypeIPv4 || rec.Type == rirparser.TypeIPv6 {
			if rec.CC == "" {
				continue
			}
			if !statusSet[strings.ToLower(rec.Status)] {
				continue
			}
			cc := strings.ToUpper(rec.CC)
			b := &domain.ScopeBlock{
				DatasetID:        datasetID,
				ScopeType:        "country",
				ScopeValue:       cc,
				AddressFamily:    string(rec.Type),
				BlockRawIdentity: rec.BlockRawIdentity(),
				Start:            rec.Start,
				Value:            rec.Value,
				Status:           rec.Status,
				CC:               rec.CC,
				Date:             rec.Date,
				CreatedAt:        time.Now(),
			}
			if err := s.Store.UpsertBlock(b); err != nil {
				continue
			}
			count++
			continue
		}
		if rec.Type == rirparser.TypeASN {
			if rec.CC == "" {
				continue
			}
			if !statusSet[strings.ToLower(rec.Status)] {
				continue
			}
			asnStart, err1 := strconv.ParseInt(rec.Start, 10, 64)
			asnCount, err2 := strconv.ParseInt(rec.Value, 10, 64)
			if err1 != nil || err2 != nil {
				continue
			}
			cc := strings.ToUpper(rec.CC)
			a := &domain.ScopeASN{
				DatasetID:   datasetID,
				ScopeType:   "country",
				ScopeValue:  cc,
				ASNStart:    asnStart,
				ASNCount:    asnCount,
				Status:      rec.Status,
				CC:          rec.CC,
				Date:        rec.Date,
				Registry:    registry,
				RawIdentity: rec.BlockRawIdentity(),
				CreatedAt:   time.Now(),
			}
			if err := s.Store.UpsertASN(a); err != nil {
				continue
			}
			asnsPersisted++
		}
	}

	dur := time.Since(start).Milliseconds()
	_ = s.Store.UpdateImportState(importID, domain.ImportStateImported, count, asnsPersisted, "")
	return &ImportResult{
		Status:          string(domain.ImportStateImported),
		ImportID:        importID,
		DatasetID:       datasetID,
		BlocksPersisted: count,
		AsnsPersisted:   asnsPersisted,
		DurationMs:      dur,
	}, nil
}

// Sync fetches the latest validated dataset per registry from dataset-service and imports each into scope-service.
// Returns a summary with one result per registry (imported, already_imported, or failed).
func (s *Service) Sync(ctx context.Context) (*SyncResponse, error) {
	latestPerRegistry, err := fetchLatestValidatedPerRegistry(ctx, s.DatasetBase, s.Client)
	if err != nil {
		return nil, err
	}
	registryOrder := []string{"ripencc", "arin", "apnic", "lacnic", "afrinic"}
	var results []SyncResultItem
	for _, reg := range registryOrder {
		datasetID, ok := latestPerRegistry[reg]
		if !ok {
			continue
		}
		imp, _ := s.Import(ctx, datasetID)
		item := SyncResultItem{
			Registry:        reg,
			DatasetID:       datasetID,
			Status:          imp.Status,
			BlocksPersisted: imp.BlocksPersisted,
			AsnsPersisted:   imp.AsnsPersisted,
			DurationMs:      imp.DurationMs,
			Error:           imp.Error,
		}
		results = append(results, item)
	}
	return &SyncResponse{Results: results}, nil
}

// fetchLatestValidatedPerRegistry returns map[registry]datasetID for the latest validated version per registry.
func fetchLatestValidatedPerRegistry(ctx context.Context, base string, client *http.Client) (map[string]uuid.UUID, error) {
	if client == nil || base == "" {
		return nil, fmt.Errorf("dataset service not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(base, "/")+"/v1/datasets", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dataset service returned %d", resp.StatusCode)
	}
	var payload struct {
		Datasets []struct {
			ID       string `json:"id"`
			Registry string `json:"registry"`
			Serial   int64  `json:"serial"`
			State    string `json:"state"`
		} `json:"datasets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	// Latest validated per registry: keep the one with highest serial per registry.
	byReg := make(map[string]struct {
		id     uuid.UUID
		serial int64
	})
	for i := range payload.Datasets {
		d := &payload.Datasets[i]
		if strings.ToLower(d.State) != "validated" {
			continue
		}
		reg := strings.ToLower(strings.TrimSpace(d.Registry))
		if reg == "" {
			continue
		}
		id, err := uuid.Parse(d.ID)
		if err != nil {
			continue
		}
		if cur, ok := byReg[reg]; !ok || d.Serial > cur.serial {
			byReg[reg] = struct {
				id     uuid.UUID
				serial int64
			}{id, d.Serial}
		}
	}
	out := make(map[string]uuid.UUID)
	for reg, v := range byReg {
		out[reg] = v.id
	}
	return out, nil
}

func fetchRegistryFromDatasetService(ctx context.Context, base string, client *http.Client, datasetID uuid.UUID) string {
	if client == nil || base == "" {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/datasets/%s", strings.TrimSuffix(base, "/"), datasetID.String()), nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	defer resp.Body.Close()
	var meta struct {
		Registry string `json:"registry"`
	}
	if json.NewDecoder(resp.Body).Decode(&meta) != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(meta.Registry))
}

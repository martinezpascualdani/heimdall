package fetch

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/dataset-service/domain"
	"github.com/martinezpascualdani/heimdall/internal/dataset-service/storage"
	"github.com/martinezpascualdani/heimdall/pkg/registry"
	"github.com/martinezpascualdani/heimdall/pkg/rirparser"
	"github.com/lib/pq"
)

// Service performs fetch and validation of RIR datasets.
type Service struct {
	Store    *storage.PostgresStore
	Artifact  *storage.ArtifactStore
	Fetcher   registry.Fetcher
}

// FetchResult is the result of a fetch operation.
type FetchResult struct {
	Status    string    `json:"status"` // "existing" | "created" | "rejected" | "failed"
	DatasetID uuid.UUID `json:"dataset_id,omitempty"`
	Registry  string    `json:"registry"`
	Serial    int64     `json:"serial"`
	State     string    `json:"state,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// FetchLatest downloads the latest delegated file for the registry, validates header, and persists.
// Option A: always register attempt (create DatasetVersion with state rejected/failed/validated).
func (s *Service) FetchLatest(ctx context.Context, cfg registry.Config, registryName string) (*FetchResult, error) {
	body, _, err := s.Fetcher.Fetch(ctx, cfg)
	if err != nil {
		id := uuid.New()
		now := time.Now()
		v := &domain.DatasetVersion{ID: id, Registry: registryName, Serial: 0, State: domain.StateFailed, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
		_ = s.Store.CreateVersion(v)
		return &FetchResult{Status: "failed", DatasetID: id, Registry: registryName, State: string(domain.StateFailed), Error: err.Error()}, nil
	}
	defer body.Close()

	tmpDir := filepath.Join(s.Artifact.BaseDir, "tmp")
	_ = os.MkdirAll(tmpDir, 0755)
	tmpFile, err := os.CreateTemp(tmpDir, "fetch-*.txt")
	if err != nil {
		id := uuid.New()
		now := time.Now()
		v := &domain.DatasetVersion{ID: id, Registry: registryName, Serial: 0, State: domain.StateFailed, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
		_ = s.Store.CreateVersion(v)
		return &FetchResult{Status: "failed", DatasetID: id, Registry: registryName, State: string(domain.StateFailed), Error: err.Error()}, nil
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, body); err != nil {
		id := uuid.New()
		now := time.Now()
		v := &domain.DatasetVersion{ID: id, Registry: registryName, Serial: 0, State: domain.StateFailed, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
		_ = s.Store.CreateVersion(v)
		return &FetchResult{Status: "failed", DatasetID: id, Registry: registryName, State: string(domain.StateFailed), Error: err.Error()}, nil
	}
	if _, err := tmpFile.Seek(0, 0); err != nil {
		id := uuid.New()
		now := time.Now()
		v := &domain.DatasetVersion{ID: id, Registry: registryName, Serial: 0, State: domain.StateFailed, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
		_ = s.Store.CreateVersion(v)
		return &FetchResult{Status: "failed", DatasetID: id, Registry: registryName, State: string(domain.StateFailed), Error: err.Error()}, nil
	}

	result, err := rirparser.ParseStream(tmpFile)
	if err != nil {
		id := uuid.New()
		now := time.Now()
		v := &domain.DatasetVersion{ID: id, Registry: registryName, Serial: 0, State: domain.StateRejected, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
		_ = s.Store.CreateVersion(v)
		return &FetchResult{Status: "rejected", DatasetID: id, Registry: registryName, State: string(domain.StateRejected), Error: err.Error()}, nil
	}
	header := result.Header
	regNorm := strings.ToLower(header.Registry)
	// Drain records and err channels
	go func() {
		for range result.Records {
		}
	}()
	for err := range result.Err {
		if err != nil {
			id := uuid.New()
			now := time.Now()
			v := &domain.DatasetVersion{ID: id, Registry: regNorm, Serial: header.Serial, State: domain.StateFailed, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
			_ = s.Store.CreateVersion(v)
			return &FetchResult{Status: "failed", DatasetID: id, Registry: regNorm, Serial: header.Serial, State: string(domain.StateFailed), Error: err.Error()}, nil
		}
	}
	if !rirparser.ValidRegistry(header.Registry) {
		id := uuid.New()
		now := time.Now()
		v := &domain.DatasetVersion{ID: id, Registry: registryName, Serial: header.Serial, State: domain.StateRejected, Error: "invalid registry in header", CreatedAt: now, UpdatedAt: now}
		_ = s.Store.CreateVersion(v)
		return &FetchResult{Status: "rejected", DatasetID: id, Registry: registryName, State: string(domain.StateRejected), Error: "invalid registry"}, nil
	}

	existing, _ := s.Store.GetByRegistrySerial(regNorm, header.Serial)
	if existing != nil && existing.State == domain.StateValidated {
		return &FetchResult{Status: "existing", DatasetID: existing.ID, Registry: existing.Registry, Serial: existing.Serial, State: string(existing.State)}, nil
	}

	id := uuid.New()
	now := time.Now()
	f, err := os.Open(tmpPath)
	if err != nil {
		v := &domain.DatasetVersion{ID: id, Registry: regNorm, Serial: header.Serial, State: domain.StateFailed, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
		_ = s.Store.CreateVersion(v)
		return &FetchResult{Status: "failed", DatasetID: id, Registry: regNorm, Serial: header.Serial, State: string(domain.StateFailed), Error: err.Error()}, nil
	}
	if _, err := f.Seek(0, 0); err != nil {
		f.Close()
		v := &domain.DatasetVersion{ID: id, Registry: regNorm, Serial: header.Serial, State: domain.StateFailed, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
		_ = s.Store.CreateVersion(v)
		return &FetchResult{Status: "failed", DatasetID: id, Registry: regNorm, Serial: header.Serial, State: string(domain.StateFailed), Error: err.Error()}, nil
	}
	finalPath, _, err := s.Artifact.Save(id, regNorm, header.Serial, f)
	f.Close()
	if err != nil {
		v := &domain.DatasetVersion{ID: id, Registry: regNorm, Serial: header.Serial, State: domain.StateFailed, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
		_ = s.Store.CreateVersion(v)
		return &FetchResult{Status: "failed", DatasetID: id, Registry: regNorm, Serial: header.Serial, State: string(domain.StateFailed), Error: err.Error()}, nil
	}

	v := &domain.DatasetVersion{
		ID:          id,
		Registry:    regNorm,
		Serial:      header.Serial,
		StartDate:   header.StartDate,
		EndDate:     header.EndDate,
		RecordCount: header.Records,
		State:       domain.StateValidated,
		StoragePath: finalPath,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.Store.CreateVersion(v); err != nil {
		os.Remove(finalPath)
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			existing, _ := s.Store.GetByRegistrySerial(regNorm, header.Serial)
			if existing != nil && existing.State == domain.StateValidated {
				return &FetchResult{Status: "existing", DatasetID: existing.ID, Registry: existing.Registry, Serial: existing.Serial, State: string(existing.State)}, nil
			}
		}
		return nil, err
	}
	return &FetchResult{Status: "created", DatasetID: id, Registry: regNorm, Serial: header.Serial, State: string(domain.StateValidated)}, nil
}


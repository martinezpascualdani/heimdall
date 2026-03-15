package fetch

import (
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/dataset-service/domain"
	"github.com/martinezpascualdani/heimdall/internal/dataset-service/storage"
	"github.com/martinezpascualdani/heimdall/pkg/caida"
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
		regNorm := strings.ToLower(registryName)
		v := &domain.DatasetVersion{ID: id, Registry: regNorm, Serial: 0, SourceType: domain.SourceTypeRIR, Source: regNorm, SourceVersion: "", State: domain.StateFailed, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
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
		regNorm := strings.ToLower(registryName)
		v := &domain.DatasetVersion{ID: id, Registry: regNorm, Serial: 0, SourceType: domain.SourceTypeRIR, Source: regNorm, SourceVersion: "", State: domain.StateFailed, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
		_ = s.Store.CreateVersion(v)
		return &FetchResult{Status: "failed", DatasetID: id, Registry: registryName, State: string(domain.StateFailed), Error: err.Error()}, nil
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, body); err != nil {
		id := uuid.New()
		now := time.Now()
		regNorm := strings.ToLower(registryName)
		v := &domain.DatasetVersion{ID: id, Registry: regNorm, Serial: 0, SourceType: domain.SourceTypeRIR, Source: regNorm, SourceVersion: "", State: domain.StateFailed, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
		_ = s.Store.CreateVersion(v)
		return &FetchResult{Status: "failed", DatasetID: id, Registry: registryName, State: string(domain.StateFailed), Error: err.Error()}, nil
	}
	if _, err := tmpFile.Seek(0, 0); err != nil {
		id := uuid.New()
		now := time.Now()
		regNorm := strings.ToLower(registryName)
		v := &domain.DatasetVersion{ID: id, Registry: regNorm, Serial: 0, SourceType: domain.SourceTypeRIR, Source: regNorm, SourceVersion: "", State: domain.StateFailed, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
		_ = s.Store.CreateVersion(v)
		return &FetchResult{Status: "failed", DatasetID: id, Registry: registryName, State: string(domain.StateFailed), Error: err.Error()}, nil
	}

	result, err := rirparser.ParseStream(tmpFile)
	if err != nil {
		id := uuid.New()
		now := time.Now()
		regNorm := strings.ToLower(registryName)
		v := &domain.DatasetVersion{ID: id, Registry: regNorm, Serial: 0, SourceType: domain.SourceTypeRIR, Source: regNorm, SourceVersion: "", State: domain.StateRejected, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
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
			v := &domain.DatasetVersion{ID: id, Registry: regNorm, Serial: header.Serial, SourceType: domain.SourceTypeRIR, Source: regNorm, SourceVersion: "", State: domain.StateFailed, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
			_ = s.Store.CreateVersion(v)
			return &FetchResult{Status: "failed", DatasetID: id, Registry: regNorm, Serial: header.Serial, State: string(domain.StateFailed), Error: err.Error()}, nil
		}
	}
	if !rirparser.ValidRegistry(header.Registry) {
		id := uuid.New()
		now := time.Now()
		regNorm := strings.ToLower(registryName)
		v := &domain.DatasetVersion{ID: id, Registry: regNorm, Serial: header.Serial, SourceType: domain.SourceTypeRIR, Source: regNorm, SourceVersion: "", State: domain.StateRejected, Error: "invalid registry in header", CreatedAt: now, UpdatedAt: now}
		_ = s.Store.CreateVersion(v)
		return &FetchResult{Status: "rejected", DatasetID: id, Registry: registryName, State: string(domain.StateRejected), Error: "invalid registry"}, nil
	}

	existing, _ := s.Store.GetByRegistrySerial(regNorm, header.Serial)
	if existing != nil && existing.State == domain.StateValidated {
		return &FetchResult{Status: "existing", DatasetID: existing.ID, Registry: existing.Registry, Serial: existing.Serial, State: string(existing.State)}, nil
	}

	id := uuid.New()
	now := time.Now()
	artifactName := fmt.Sprintf("delegated-%s-serial-%d-%s.txt", regNorm, header.Serial, id.String()[:8])
	f, err := os.Open(tmpPath)
	if err != nil {
		v := &domain.DatasetVersion{ID: id, Registry: regNorm, Serial: header.Serial, SourceType: domain.SourceTypeRIR, Source: regNorm, SourceVersion: "", State: domain.StateFailed, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
		_ = s.Store.CreateVersion(v)
		return &FetchResult{Status: "failed", DatasetID: id, Registry: regNorm, Serial: header.Serial, State: string(domain.StateFailed), Error: err.Error()}, nil
	}
	if _, err := f.Seek(0, 0); err != nil {
		f.Close()
		v := &domain.DatasetVersion{ID: id, Registry: regNorm, Serial: header.Serial, SourceType: domain.SourceTypeRIR, Source: regNorm, SourceVersion: "", State: domain.StateFailed, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
		_ = s.Store.CreateVersion(v)
		return &FetchResult{Status: "failed", DatasetID: id, Registry: regNorm, Serial: header.Serial, State: string(domain.StateFailed), Error: err.Error()}, nil
	}
	finalPath, _, err := s.Artifact.Save(id, regNorm, artifactName, f)
	f.Close()
	if err != nil {
		v := &domain.DatasetVersion{ID: id, Registry: regNorm, Serial: header.Serial, SourceType: domain.SourceTypeRIR, Source: regNorm, SourceVersion: "", State: domain.StateFailed, Error: err.Error(), CreatedAt: now, UpdatedAt: now}
		_ = s.Store.CreateVersion(v)
		return &FetchResult{Status: "failed", DatasetID: id, Registry: regNorm, Serial: header.Serial, State: string(domain.StateFailed), Error: err.Error()}, nil
	}

	v := &domain.DatasetVersion{
		ID:            id,
		Registry:      regNorm,
		Serial:        header.Serial,
		SourceType:    domain.SourceTypeRIR,
		Source:        regNorm,
		SourceVersion: "",
		StartDate:     header.StartDate,
		EndDate:       header.EndDate,
		RecordCount:   header.Records,
		State:         domain.StateValidated,
		StoragePath:   finalPath,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Store.CreateVersion(v); err != nil {
		os.Remove(filepath.Join(s.Artifact.BaseDir, finalPath))
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

// CAIDA source constants (must match dataset-service API and routing-service).
const (
	SourceCAIDAPfx2asIPv4 = "caida_pfx2as_ipv4"
	SourceCAIDAPfx2asIPv6 = "caida_pfx2as_ipv6"
	SourceCAIDAASOrg      = "caida_as_org"
)

// FetchCAIDA downloads the latest snapshot for a CAIDA source, validates it, and persists. Returns FetchResult with Registry set to source name and Serial 0.
func (s *Service) FetchCAIDA(ctx context.Context, source string) (*FetchResult, error) {
	var body io.ReadCloser
	var sourceVersion, artifactName string
	switch source {
	case SourceCAIDAPfx2asIPv4:
		res, err := caida.FetchPfx2asLatest(ctx, caida.DefaultPfx2asIPv4Base, nil)
		if err != nil {
			return &FetchResult{Status: "failed", Registry: source, Serial: 0, State: string(domain.StateFailed), Error: err.Error()}, nil
		}
		body = res.Body
		sourceVersion = res.SourceVersion
		artifactName = res.ArtifactName
	case SourceCAIDAPfx2asIPv6:
		res, err := caida.FetchPfx2asLatest(ctx, caida.DefaultPfx2asIPv6Base, nil)
		if err != nil {
			return &FetchResult{Status: "failed", Registry: source, Serial: 0, State: string(domain.StateFailed), Error: err.Error()}, nil
		}
		body = res.Body
		sourceVersion = res.SourceVersion
		artifactName = res.ArtifactName
	case SourceCAIDAASOrg:
		res, err := caida.FetchASOrgLatest(ctx, caida.DefaultASOrgLatestURL, nil)
		if err != nil {
			return &FetchResult{Status: "failed", Registry: source, Serial: 0, State: string(domain.StateFailed), Error: err.Error()}, nil
		}
		body = res.Body
		sourceVersion = res.SourceVersion
		artifactName = res.ArtifactName
	default:
		return &FetchResult{Status: "failed", Registry: source, Serial: 0, State: string(domain.StateFailed), Error: "unsupported CAIDA source"}, nil
	}
	defer body.Close()

	existing, _ := s.Store.GetBySourceVersion(source, sourceVersion)
	if existing != nil && existing.State == domain.StateValidated {
		return &FetchResult{Status: "existing", DatasetID: existing.ID, Registry: source, Serial: 0, State: string(existing.State)}, nil
	}

	tmpDir := filepath.Join(s.Artifact.BaseDir, "tmp")
	_ = os.MkdirAll(tmpDir, 0755)
	tmpFile, err := os.CreateTemp(tmpDir, "caida-*.gz")
	if err != nil {
		return &FetchResult{Status: "failed", Registry: source, Serial: 0, State: string(domain.StateFailed), Error: err.Error()}, nil
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, body); err != nil {
		return &FetchResult{Status: "failed", Registry: source, Serial: 0, State: string(domain.StateFailed), Error: err.Error()}, nil
	}
	if _, err := tmpFile.Seek(0, 0); err != nil {
		return &FetchResult{Status: "failed", Registry: source, Serial: 0, State: string(domain.StateFailed), Error: err.Error()}, nil
	}

	if source == SourceCAIDAPfx2asIPv4 || source == SourceCAIDAPfx2asIPv6 {
		f, _ := os.Open(tmpPath)
		err = caida.ValidatePfx2asGzip(f)
		f.Close()
		if err != nil {
			return &FetchResult{Status: "rejected", Registry: source, Serial: 0, State: string(domain.StateRejected), Error: err.Error()}, nil
		}
	}

	id := uuid.New()
	now := time.Now()
	f, err := os.Open(tmpPath)
	if err != nil {
		return &FetchResult{Status: "failed", DatasetID: id, Registry: source, Serial: 0, State: string(domain.StateFailed), Error: err.Error()}, nil
	}
	finalPath, _, err := s.Artifact.Save(id, source, artifactName, f)
	f.Close()
	if err != nil {
		return &FetchResult{Status: "failed", DatasetID: id, Registry: source, Serial: 0, State: string(domain.StateFailed), Error: err.Error()}, nil
	}

	v := &domain.DatasetVersion{
		ID:            id,
		Registry:      source,
		Serial:        0,
		SourceType:    domain.SourceTypeCAIDA,
		Source:        source,
		SourceVersion: sourceVersion,
		State:         domain.StateValidated,
		StoragePath:   finalPath,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Store.CreateVersion(v); err != nil {
		os.Remove(filepath.Join(s.Artifact.BaseDir, finalPath))
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			existing, _ := s.Store.GetBySourceVersion(source, sourceVersion)
			if existing != nil && existing.State == domain.StateValidated {
				return &FetchResult{Status: "existing", DatasetID: existing.ID, Registry: source, Serial: 0, State: string(existing.State)}, nil
			}
		}
		return nil, err
	}
	// Contar líneas en el .gz para rellenar record_count (descomprimimos al leer)
	if n := countLinesGzip(filepath.Join(s.Artifact.BaseDir, finalPath)); n >= 0 {
		_ = s.Store.UpdateVersionMeta(id, 0, "", "", n, finalPath)
	}
	return &FetchResult{Status: "created", DatasetID: id, Registry: source, Serial: 0, State: string(domain.StateValidated)}, nil
}

// countLinesGzip abre el .gz, descomprime y cuenta líneas; devuelve -1 si falla.
func countLinesGzip(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		return -1
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return -1
	}
	defer gz.Close()
	var n int64
	sc := bufio.NewScanner(gz)
	sc.Buffer(nil, 512*1024)
	for sc.Scan() {
		n++
	}
	if sc.Err() != nil {
		return -1
	}
	return n
}


package ipresolver

import (
	"context"
	"errors"
	"net"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/storage"
)

var (
	ErrInvalidIP  = errors.New("invalid ip")
	ErrNoDataset  = errors.New("no dataset available")
)

// ResolveResult is the result of resolving an IP to a scope (e.g. country).
type ResolveResult struct {
	IP         string    `json:"ip"`
	ScopeType  string    `json:"scope_type"`
	ScopeValue string    `json:"scope_value"`
	DatasetID  uuid.UUID `json:"dataset_id"`
}

// Store is the minimal storage interface needed for IP resolution.
type Store interface {
	GetLatestImportedDatasetIDsPerRegistry() ([]uuid.UUID, error)
	FindBlockByIP(ip net.IP, datasetID uuid.UUID) (*storage.IPBlockMatch, error)
	FindBlockByIPInLatestPerRegistry(ip net.IP) (*storage.IPBlockMatch, error)
}

// Service resolves an IP to a scope (country) using imported blocks.
type Service struct {
	Store Store
}

// Resolve returns the scope (e.g. country) for the given IP using the latest imported dataset or the given dataset_id.
// Returns (nil, nil) when the IP is not found in any block (caller should respond 404).
// Returns (nil, ErrInvalidIP) for invalid IP (caller should respond 400).
// Returns (nil, ErrNoDataset) when no dataset is available (caller should respond 503).
func (s *Service) Resolve(ctx context.Context, ipStr string, datasetID *uuid.UUID) (*ResolveResult, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, ErrInvalidIP
	}
	if datasetID != nil {
		// Client asked for a specific dataset: search only that one.
		match, err := s.Store.FindBlockByIP(ip, *datasetID)
		if err != nil {
			return nil, err
		}
		if match == nil {
			return nil, nil
		}
		return &ResolveResult{
			IP:         ipStr,
			ScopeType:  match.ScopeType,
			ScopeValue: match.ScopeValue,
			DatasetID:  match.DatasetID,
		}, nil
	}
	// No dataset_id: use latest import per registry only (one RIPE, one ARIN, etc.) for a consistent global snapshot.
	ids, err := s.Store.GetLatestImportedDatasetIDsPerRegistry()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, ErrNoDataset
	}
	match, err := s.Store.FindBlockByIPInLatestPerRegistry(ip)
	if err != nil {
		return nil, err
	}
	if match == nil {
		// No imports have registry set yet (e.g. old data): fall back to first of latest-per-registry set.
		match, err = s.Store.FindBlockByIP(ip, ids[0])
		if err != nil {
			return nil, err
		}
	}
	if match == nil {
		return nil, nil
	}
	return &ResolveResult{
		IP:         ipStr,
		ScopeType:  match.ScopeType,
		ScopeValue: match.ScopeValue,
		DatasetID:  match.DatasetID,
	}, nil
}

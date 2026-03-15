package materializer

import (
	"context"
	"encoding/json"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/target-service/domain"
	"github.com/martinezpascualdani/heimdall/pkg/iso3166"
)

// Store is the storage interface required by the materializer.
type Store interface {
	CreateMaterialization(*domain.TargetMaterialization) error
	UpdateMaterialization(*domain.TargetMaterialization) error
	GetMaterializationByID(uuid.UUID) (*domain.TargetMaterialization, error)
	InsertTargetEntries(uuid.UUID, []string) error
	DeleteEntriesForMaterialization(uuid.UUID) error
}

// Service runs materialization: resolve includes, apply exclusions, persist snapshot.
type Service struct {
	Store   Store
	Scope   *ScopeClient
	Routing *RoutingClient
}

// Run materializes the target with the given rules. Creates one materialization row (running), then resolves, then writes entries and updates to completed/failed.
func (s *Service) Run(ctx context.Context, targetID uuid.UUID, rules []domain.TargetRule) (materializationID uuid.UUID, err error) {
	m := &domain.TargetMaterialization{
		TargetID: targetID,
		Status:   domain.MaterializationStatusRunning,
	}
	if err := s.Store.CreateMaterialization(m); err != nil {
		return uuid.Nil, err
	}
	materializationID = m.ID
	defer func() {
		if err != nil {
			m.Status = domain.MaterializationStatusFailed
			m.ErrorMessage = err.Error()
			_ = s.Store.UpdateMaterialization(m)
			_ = s.Store.DeleteEntriesForMaterialization(materializationID)
		}
	}()

	// Collect scope and routing refs for traceability (minimal structure).
	var scopeRefs, routingRefs []domain.SnapshotRef
	resolvedAt := time.Now()

	// 1) Resolve all inclusions into a set (map[string]struct{} for dedup).
	prefixSet := make(map[string]struct{})
	for _, r := range rules {
		if r.Kind != "include" {
			continue
		}
		af := r.AddressFamily
		if af == "" {
			af = "all"
		}
		switch r.SelectorType {
		case "country":
			cidrs, dsIDs, t, err := s.Scope.FetchCountryBlocks(ctx, r.SelectorValue, af)
			if err != nil {
				return materializationID, err
			}
			scopeRefs = append(scopeRefs, domain.SnapshotRef{
				Service: "scope", Endpoint: "GET /v1/scopes/country/" + r.SelectorValue + "/blocks",
				DatasetIDs: dsIDs, ResolvedAt: t,
			})
			for _, c := range cidrs {
				prefixSet[c] = struct{}{}
			}
		case "asn":
			cidrs, dsIDs, t, err := s.Routing.FetchASNPrefixes(ctx, r.SelectorValue, af)
			if err != nil {
				return materializationID, err
			}
			routingRefs = append(routingRefs, domain.SnapshotRef{
				Service: "routing", Endpoint: "GET /v1/asn/prefixes/" + r.SelectorValue,
				DatasetIDs: dsIDs, ResolvedAt: t,
			})
			for _, c := range cidrs {
				prefixSet[c] = struct{}{}
			}
		case "prefix":
			c := normalizeCIDR(r.SelectorValue)
			if c != "" {
				prefixSet[c] = struct{}{}
			}
		case "world":
			for _, cc := range iso3166.AllAlpha2() {
				cidrs, dsIDs, t, err := s.Scope.FetchCountryBlocks(ctx, cc, af)
				if err != nil {
					return materializationID, err
				}
				if len(dsIDs) > 0 || len(cidrs) > 0 {
					scopeRefs = append(scopeRefs, domain.SnapshotRef{
						Service: "scope", Endpoint: "GET /v1/scopes/country/" + cc + "/blocks",
						DatasetIDs: dsIDs, ResolvedAt: t,
					})
				}
				for _, c := range cidrs {
					prefixSet[c] = struct{}{}
				}
			}
		}
	}
	_ = resolvedAt

	// 2) Apply exclusions: remove entire prefixes that are contained in exclusion CIDR, or that match country/ASN sets.
	for _, r := range rules {
		if r.Kind != "exclude" {
			continue
		}
		switch r.SelectorType {
		case "country":
			cidrs, _, _, err := s.Scope.FetchCountryBlocks(ctx, r.SelectorValue, r.AddressFamily)
			if err != nil {
				return materializationID, err
			}
			for _, c := range cidrs {
				delete(prefixSet, c)
			}
		case "asn":
			cidrs, _, _, err := s.Routing.FetchASNPrefixes(ctx, r.SelectorValue, r.AddressFamily)
			if err != nil {
				return materializationID, err
			}
			for _, c := range cidrs {
				delete(prefixSet, c)
			}
		case "prefix":
			// v1: containment only. Remove Q from set if Q is contained in P or Q == P. No CIDR subtraction.
			exclNet := parseCIDRNet(r.SelectorValue)
			if exclNet == nil {
				continue
			}
			for p := range prefixSet {
				qNet := parseCIDRNet(p)
				if qNet == nil {
					continue
				}
				if prefixContainedIn(qNet, exclNet) || p == r.SelectorValue {
					delete(prefixSet, p)
				}
			}
		}
	}

	// 3) Build slice and persist.
	prefixes := make([]string, 0, len(prefixSet))
	for p := range prefixSet {
		prefixes = append(prefixes, p)
	}
	if err := s.Store.InsertTargetEntries(materializationID, prefixes); err != nil {
		return materializationID, err
	}

	scopeRefJSON, _ := json.Marshal(scopeRefs)
	routingRefJSON, _ := json.Marshal(routingRefs)
	m.TotalPrefixCount = len(prefixes)
	m.Status = domain.MaterializationStatusCompleted
	m.ScopeSnapshotRef = scopeRefJSON
	m.RoutingSnapshotRef = routingRefJSON
	if err := s.Store.UpdateMaterialization(m); err != nil {
		return materializationID, err
	}
	return materializationID, nil
}

// prefixContainedIn returns true if a is contained in b (a is equal or more specific than b).
func prefixContainedIn(a, b *net.IPNet) bool {
	aLen, _ := a.Mask.Size()
	bLen, _ := b.Mask.Size()
	if aLen < bLen {
		return false
	}
	return b.Contains(a.IP)
}

func parseCIDRNet(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		return nil
	}
	return n
}

func normalizeCIDR(s string) string {
	ip, n, err := net.ParseCIDR(s)
	if err != nil {
		return ""
	}
	return (&net.IPNet{IP: ip, Mask: n.Mask}).String()
}


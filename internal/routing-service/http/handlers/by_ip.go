package handlers

import (
	"encoding/json"
	"net"
	"net/http"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/routing-service/domain"
)

// ByIPStore is the storage interface needed for GET /v1/asn/by-ip/{ip}.
type ByIPStore interface {
	GetLatestImportedDatasetIDBySource(source string) (*uuid.UUID, error)
	HasImportedRoutingDataset(datasetID uuid.UUID) (bool, error)
	LongestPrefixMatch(datasetID uuid.UUID, ipVersion, ip string) (*domain.BGPPrefixOrigin, error)
	GetASNMetadata(asn int64) (*domain.ASNMetadata, error)
}

// ByIPHandler handles GET /v1/asn/by-ip/{ip}. Returns observed routing state; primary_asn is operational simplification from asn_raw.
type ByIPHandler struct {
	Store ByIPStore
}

// ByIPResponse is the response for by-ip.
type ByIPResponse struct {
	IP              string     `json:"ip"`
	MatchedPrefix   string     `json:"matched_prefix"`
	PrefixLength    int        `json:"prefix_length"`
	ASNRaw          string     `json:"asn_raw"`
	PrimaryASN      *int64     `json:"primary_asn,omitempty"`
	ASNType         string     `json:"asn_type"`
	ASName          *string    `json:"as_name,omitempty"`
	OrgName         *string    `json:"org_name,omitempty"`
	RoutingDataset  *uuid.UUID `json:"routing_dataset,omitempty"`
	MetadataDataset *uuid.UUID `json:"metadata_dataset,omitempty"`
}

func (h *ByIPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ipStr := r.PathValue("ip")
	if ipStr == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_ip"})
		return
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_ip"})
		return
	}
	ipVersion := "ipv4"
	if ip.To4() == nil {
		ipVersion = "ipv6"
	}

	var routingDatasetID *uuid.UUID
	if idStr := r.URL.Query().Get("dataset_id"); idStr != "" {
		id, err := uuid.Parse(idStr)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid dataset_id"})
			return
		}
		ok, err := h.Store.HasImportedRoutingDataset(id)
		if err != nil || !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "dataset_not_imported"})
			return
		}
		routingDatasetID = &id
	} else {
		source := "caida_pfx2as_ipv4"
		if ipVersion == "ipv6" {
			source = "caida_pfx2as_ipv6"
		}
		routingDatasetID, _ = h.Store.GetLatestImportedDatasetIDBySource(source)
	}
	if routingDatasetID == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "no_routing_dataset"})
		return
	}

	prefix, err := h.Store.LongestPrefixMatch(*routingDatasetID, ipVersion, ipStr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if prefix == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "no_match"})
		return
	}

	metadataDatasetID, _ := h.Store.GetLatestImportedDatasetIDBySource("caida_as_org")

	out := ByIPResponse{
		IP:               ipStr,
		MatchedPrefix:    prefix.Prefix,
		PrefixLength:     prefix.PrefixLength,
		ASNRaw:           prefix.ASNRaw,
		PrimaryASN:       prefix.PrimaryASN,
		ASNType:          prefix.ASNType,
		RoutingDataset:   routingDatasetID,
		MetadataDataset:  metadataDatasetID,
	}
	if prefix.PrimaryASN != nil {
		meta, _ := h.Store.GetASNMetadata(*prefix.PrimaryASN)
		if meta != nil {
			out.ASName = meta.ASName
			out.OrgName = meta.OrgName
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

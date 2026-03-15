package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/routing-service/domain"
)

// ASNPrefixesStore is the storage interface for GET /v1/asn/prefixes/{asn} (only primary_asn = asn).
type ASNPrefixesStore interface {
	GetLatestImportedDatasetIDBySource(source string) (*uuid.UUID, error)
	HasImportedRoutingDataset(datasetID uuid.UUID) (bool, error)
	ListPrefixesByPrimaryASN(datasetID uuid.UUID, asn int64, limit, offset int) ([]*domain.BGPPrefixOrigin, int, error)
}

// ASNPrefixesHandler handles GET /v1/asn/prefixes/{asn}. Only prefixes where primary_asn = asn; order by prefix_length, prefix.
type ASNPrefixesHandler struct {
	Store ASNPrefixesStore
}

// PrefixItem is one prefix in the list.
type PrefixItem struct {
	Prefix       string `json:"prefix"`
	PrefixLength int    `json:"prefix_length"`
	ASNRaw       string `json:"asn_raw"`
	ASNType      string `json:"asn_type"`
}

// ASNPrefixesResponse is the response for GET /v1/asn/prefixes/{asn}.
type ASNPrefixesResponse struct {
	ASN       int64         `json:"asn"`
	Items    []PrefixItem   `json:"items"`
	Total    int            `json:"total"`
	Limit    int            `json:"limit"`
	Offset   int            `json:"offset"`
	HasMore  bool           `json:"has_more"`
	DatasetID *uuid.UUID    `json:"dataset_id,omitempty"`
}

func (h *ASNPrefixesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	asnStr := r.PathValue("asn")
	if asnStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid asn"})
		return
	}
	asn, err := strconv.ParseInt(asnStr, 10, 64)
	if err != nil || asn < 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid asn"})
		return
	}

	limit := 100
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, _ := strconv.Atoi(l); n >= 0 && n <= 1000 {
			limit = n
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if n, _ := strconv.Atoi(o); n >= 0 {
			offset = n
		}
	}

	var datasetID *uuid.UUID
	if idStr := r.URL.Query().Get("dataset_id"); idStr != "" {
		id, err := uuid.Parse(idStr)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid dataset_id"})
			return
		}
		ok, _ := h.Store.HasImportedRoutingDataset(id)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "dataset_not_imported"})
			return
		}
		datasetID = &id
	} else {
		datasetID, _ = h.Store.GetLatestImportedDatasetIDBySource("caida_pfx2as_ipv4")
		if datasetID == nil {
			datasetID, _ = h.Store.GetLatestImportedDatasetIDBySource("caida_pfx2as_ipv6")
		}
	}
	if datasetID == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "no_routing_dataset"})
		return
	}

	list, total, err := h.Store.ListPrefixesByPrimaryASN(*datasetID, asn, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	items := make([]PrefixItem, len(list))
	for i, o := range list {
		items[i] = PrefixItem{Prefix: o.Prefix, PrefixLength: o.PrefixLength, ASNRaw: o.ASNRaw, ASNType: o.ASNType}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ASNPrefixesResponse{
		ASN:        asn,
		Items:      items,
		Total:      total,
		Limit:      limit,
		Offset:     offset,
		HasMore:    offset+len(list) < total,
		DatasetID:  datasetID,
	})
}

package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/routing-service/domain"
)

// ASNMetaStore is the storage interface for GET /v1/asn/{asn} (metadata only).
type ASNMetaStore interface {
	GetASNMetadata(asn int64) (*domain.ASNMetadata, error)
}

// ASNMetaHandler handles GET /v1/asn/{asn}. Returns metadata only; 404 if no metadata for that ASN.
type ASNMetaHandler struct {
	Store ASNMetaStore
}

// ASNMetaResponse is the response for GET /v1/asn/{asn}.
type ASNMetaResponse struct {
	ASN             int64      `json:"asn"`
	ASName          *string    `json:"as_name,omitempty"`
	OrgName         *string    `json:"org_name,omitempty"`
	Source          string     `json:"source"`
	SourceDatasetID *uuid.UUID `json:"source_dataset_id,omitempty"`
}

func (h *ASNMetaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	meta, err := h.Store.GetASNMetadata(asn)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if meta == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not_found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ASNMetaResponse{
		ASN:             meta.ASN,
		ASName:          meta.ASName,
		OrgName:         meta.OrgName,
		Source:          meta.Source,
		SourceDatasetID: meta.SourceDatasetID,
	})
}

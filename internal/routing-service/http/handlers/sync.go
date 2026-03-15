package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/martinezpascualdani/heimdall/internal/routing-service/importsvc"
)

// SyncHandler handles POST /v1/imports/sync.
type SyncHandler struct {
	Sync *importsvc.Service
}

func (h *SyncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	resp, err := h.Sync.Sync(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

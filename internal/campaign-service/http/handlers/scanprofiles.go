package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/domain"
)

// ScanProfileStore is the storage interface for scan profiles.
type ScanProfileStore interface {
	CreateScanProfile(*domain.ScanProfile) error
	GetScanProfileByID(uuid.UUID) (*domain.ScanProfile, error)
	GetScanProfileBySlug(string) (*domain.ScanProfile, error)
	ListScanProfiles(limit, offset int) ([]*domain.ScanProfile, int, error)
	UpdateScanProfile(*domain.ScanProfile) error
	CountCampaignsByScanProfileID(uuid.UUID) (int, error)
	DeleteScanProfile(uuid.UUID) error
}

// ScanProfilesHandler handles scan profile CRUD.
type ScanProfilesHandler struct {
	Store ScanProfileStore
}

// CreateScanProfileRequest is the body for POST /v1/scan-profiles.
type CreateScanProfileRequest struct {
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description string          `json:"description"`
	Config      json.RawMessage `json:"config,omitempty"`
}

// Create handles POST /v1/scan-profiles.
func (h *ScanProfilesHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req CreateScanProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid body", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Slug = strings.TrimSpace(strings.ToLower(req.Slug))
	if req.Name == "" {
		writeJSONError(w, "name required", http.StatusBadRequest)
		return
	}
	if req.Slug == "" {
		writeJSONError(w, "slug required", http.StatusBadRequest)
		return
	}
	existing, _ := h.Store.GetScanProfileBySlug(req.Slug)
	if existing != nil {
		writeJSONError(w, "slug already exists", http.StatusConflict)
		return
	}
	p := &domain.ScanProfile{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: strings.TrimSpace(req.Description),
		Config:      req.Config,
	}
	if err := h.Store.CreateScanProfile(p); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, scanProfileToResp(p))
}

// List handles GET /v1/scan-profiles.
func (h *ScanProfilesHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	limit, offset := parseLimitOffset(r, defaultLimit, maxLimit)
	list, total, err := h.Store.ListScanProfiles(limit, offset)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]map[string]interface{}, 0, len(list))
	for _, p := range list {
		items = append(items, scanProfileToResp(p))
	}
	hasMore := offset+len(items) < total
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":    items,
		"count":    len(items),
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": hasMore,
	})
}

// Get handles GET /v1/scan-profiles/{id}.
func (h *ScanProfilesHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	p, err := h.Store.GetScanProfileByID(id)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if p == nil {
		writeJSONError(w, "scan_profile not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, scanProfileToResp(p))
}

// Update handles PUT /v1/scan-profiles/{id} (full replacement).
func (h *ScanProfilesHandler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	p, err := h.Store.GetScanProfileByID(id)
	if err != nil || p == nil {
		if p == nil {
			writeJSONError(w, "scan_profile not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var req CreateScanProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid body", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Slug = strings.TrimSpace(strings.ToLower(req.Slug))
	if req.Name == "" || req.Slug == "" {
		writeJSONError(w, "name and slug required", http.StatusBadRequest)
		return
	}
	existing, _ := h.Store.GetScanProfileBySlug(req.Slug)
	if existing != nil && existing.ID != id {
		writeJSONError(w, "slug already exists", http.StatusConflict)
		return
	}
	p.Name = req.Name
	p.Slug = req.Slug
	p.Description = strings.TrimSpace(req.Description)
	p.Config = req.Config
	if err := h.Store.UpdateScanProfile(p); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, scanProfileToResp(p))
}

// Delete handles DELETE /v1/scan-profiles/{id}. Rejects if any campaign uses it.
func (h *ScanProfilesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	n, err := h.Store.CountCampaignsByScanProfileID(id)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if n > 0 {
		writeJSONError(w, "cannot delete: campaigns use this profile", http.StatusConflict)
		return
	}
	if err := h.Store.DeleteScanProfile(id); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func scanProfileToResp(p *domain.ScanProfile) map[string]interface{} {
	return map[string]interface{}{
		"id":          p.ID,
		"name":        p.Name,
		"slug":        p.Slug,
		"description": p.Description,
		"config":      p.Config,
		"created_at":  p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":  p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
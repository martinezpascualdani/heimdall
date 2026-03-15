package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/domain"
)

// CampaignStore is the storage interface for campaigns.
type CampaignStore interface {
	CreateCampaign(*domain.Campaign) error
	GetCampaignByID(uuid.UUID) (*domain.Campaign, error)
	ListCampaigns(limit, offset int, activeOnly bool) ([]*domain.Campaign, int, error)
	UpdateCampaign(*domain.Campaign) error
	SoftDeleteCampaign(uuid.UUID) error
}

// TargetValidator validates that a target exists (e.g. via target-service).
type TargetValidator interface {
	TargetExists(ctx context.Context, targetID uuid.UUID) (bool, error)
}

// CreateCampaignRequest is the body for POST /v1/campaigns.
type CreateCampaignRequest struct {
	Name                 string          `json:"name"`
	Description          string          `json:"description"`
	TargetID             string          `json:"target_id"`
	ScanProfileID        string          `json:"scan_profile_id"`
	ScheduleType         string          `json:"schedule_type"`
	ScheduleConfig       json.RawMessage `json:"schedule_config,omitempty"`
	MaterializationPolicy string         `json:"materialization_policy"`
	NextRunAt            *string         `json:"next_run_at,omitempty"` // RFC3339; optional for once/interval
	ConcurrencyPolicy    string          `json:"concurrency_policy"`
}

// ScanProfileGetter is used for validation (campaign create/update).
type ScanProfileGetter interface {
	GetScanProfileByID(uuid.UUID) (*domain.ScanProfile, error)
}

// CampaignsHandler handles campaign CRUD.
type CampaignsHandler struct {
	Store         CampaignStore
	ProfileGetter ScanProfileGetter
	Validator     TargetValidator
}

// Create handles POST /v1/campaigns.
func (h *CampaignsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req CreateCampaignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid body", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeJSONError(w, "name required", http.StatusBadRequest)
		return
	}
	targetID, err := uuid.Parse(req.TargetID)
	if err != nil {
		writeJSONError(w, "invalid target_id", http.StatusBadRequest)
		return
	}
	scanProfileID, err := uuid.Parse(req.ScanProfileID)
	if err != nil {
		writeJSONError(w, "invalid scan_profile_id", http.StatusBadRequest)
		return
	}
	scheduleType := strings.TrimSpace(strings.ToLower(req.ScheduleType))
	if scheduleType != domain.ScheduleTypeManual && scheduleType != domain.ScheduleTypeOnce && scheduleType != domain.ScheduleTypeInterval {
		writeJSONError(w, "schedule_type must be manual, once, or interval", http.StatusBadRequest)
		return
	}
	matPolicy := strings.TrimSpace(strings.ToLower(req.MaterializationPolicy))
	if matPolicy != domain.MaterializationPolicyUseLatest && matPolicy != domain.MaterializationPolicyRematerialize {
		writeJSONError(w, "materialization_policy must be use_latest or rematerialize", http.StatusBadRequest)
		return
	}
	concurrency := strings.TrimSpace(strings.ToLower(req.ConcurrencyPolicy))
	if concurrency == "" {
		concurrency = domain.ConcurrencyPolicyAllow
	}
	if concurrency != domain.ConcurrencyPolicyAllow && concurrency != domain.ConcurrencyPolicyForbidIfActive {
		writeJSONError(w, "concurrency_policy must be allow or forbid_if_active", http.StatusBadRequest)
		return
	}
	// Validación puntual: target existe
	exists, err := h.Validator.TargetExists(r.Context(), targetID)
	if err != nil {
		writeJSONError(w, "target validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !exists {
		writeJSONError(w, "target not found", http.StatusBadRequest)
		return
	}
	// Scan profile must exist
	profile, err := h.ProfileGetter.GetScanProfileByID(scanProfileID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if profile == nil {
		writeJSONError(w, "scan_profile not found", http.StatusBadRequest)
		return
	}
	c := &domain.Campaign{
		Name:                 req.Name,
		Description:          strings.TrimSpace(req.Description),
		Active:               true,
		TargetID:             targetID,
		ScanProfileID:        scanProfileID,
		ScheduleType:         scheduleType,
		ScheduleConfig:       req.ScheduleConfig,
		MaterializationPolicy: matPolicy,
		RunOnceDone:          false,
		ConcurrencyPolicy:    concurrency,
	}
	// next_run_at: if not provided for once/interval, set to now
	if scheduleType == domain.ScheduleTypeOnce || scheduleType == domain.ScheduleTypeInterval {
		if req.NextRunAt != nil && strings.TrimSpace(*req.NextRunAt) != "" {
			t, err := parseTime(*req.NextRunAt)
			if err != nil {
				writeJSONError(w, "invalid next_run_at", http.StatusBadRequest)
				return
			}
			c.NextRunAt = &t
		} else {
			now := timeNow()
			c.NextRunAt = &now
		}
	}
	if err := h.Store.CreateCampaign(c); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, campaignToResp(c))
}

// List handles GET /v1/campaigns.
func (h *CampaignsHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	limit, offset := parseLimitOffset(r, defaultLimit, maxLimit)
	includeInactive := r.URL.Query().Get("include_inactive") == "true" || r.URL.Query().Get("include_inactive") == "1"
	activeOnly := !includeInactive
	list, total, err := h.Store.ListCampaigns(limit, offset, activeOnly)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]map[string]interface{}, 0, len(list))
	for _, c := range list {
		items = append(items, campaignToResp(c))
	}
	hasMore := offset+len(items) < total
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":   items,
		"count":   len(items),
		"total":   total,
		"limit":   limit,
		"offset":  offset,
		"has_more": hasMore,
	})
}

// Get handles GET /v1/campaigns/{id}.
func (h *CampaignsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	c, err := h.Store.GetCampaignByID(id)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if c == nil {
		writeJSONError(w, "campaign not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, campaignToResp(c))
}

// Update handles PUT /v1/campaigns/{id} (full replacement).
func (h *CampaignsHandler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	c, err := h.Store.GetCampaignByID(id)
	if err != nil || c == nil {
		if c == nil {
			writeJSONError(w, "campaign not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var req CreateCampaignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid body", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeJSONError(w, "name required", http.StatusBadRequest)
		return
	}
	targetID, err := uuid.Parse(req.TargetID)
	if err != nil {
		writeJSONError(w, "invalid target_id", http.StatusBadRequest)
		return
	}
	scanProfileID, err := uuid.Parse(req.ScanProfileID)
	if err != nil {
		writeJSONError(w, "invalid scan_profile_id", http.StatusBadRequest)
		return
	}
	scheduleType := strings.TrimSpace(strings.ToLower(req.ScheduleType))
	if scheduleType != domain.ScheduleTypeManual && scheduleType != domain.ScheduleTypeOnce && scheduleType != domain.ScheduleTypeInterval {
		writeJSONError(w, "schedule_type must be manual, once, or interval", http.StatusBadRequest)
		return
	}
	matPolicy := strings.TrimSpace(strings.ToLower(req.MaterializationPolicy))
	if matPolicy != domain.MaterializationPolicyUseLatest && matPolicy != domain.MaterializationPolicyRematerialize {
		writeJSONError(w, "materialization_policy must be use_latest or rematerialize", http.StatusBadRequest)
		return
	}
	concurrency := strings.TrimSpace(strings.ToLower(req.ConcurrencyPolicy))
	if concurrency == "" {
		concurrency = domain.ConcurrencyPolicyAllow
	}
	if concurrency != domain.ConcurrencyPolicyAllow && concurrency != domain.ConcurrencyPolicyForbidIfActive {
		writeJSONError(w, "concurrency_policy must be allow or forbid_if_active", http.StatusBadRequest)
		return
	}
	exists, err := h.Validator.TargetExists(r.Context(), targetID)
	if err != nil || !exists {
		if !exists {
			writeJSONError(w, "target not found", http.StatusBadRequest)
			return
		}
		writeJSONError(w, "target validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	profile, errProfile := h.ProfileGetter.GetScanProfileByID(scanProfileID)
	if errProfile != nil || profile == nil {
		if profile == nil {
			writeJSONError(w, "scan_profile not found", http.StatusBadRequest)
			return
		}
		writeJSONError(w, errProfile.Error(), http.StatusInternalServerError)
		return
	}
	c.Name = req.Name
	c.Description = strings.TrimSpace(req.Description)
	c.TargetID = targetID
	c.ScanProfileID = scanProfileID
	c.ScheduleType = scheduleType
	c.ScheduleConfig = req.ScheduleConfig
	c.MaterializationPolicy = matPolicy
	c.ConcurrencyPolicy = concurrency
	if req.NextRunAt != nil && strings.TrimSpace(*req.NextRunAt) != "" && (scheduleType == domain.ScheduleTypeOnce || scheduleType == domain.ScheduleTypeInterval) {
		t, err := parseTime(*req.NextRunAt)
		if err != nil {
			writeJSONError(w, "invalid next_run_at", http.StatusBadRequest)
			return
		}
		c.NextRunAt = &t
	}
	if err := h.Store.UpdateCampaign(c); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, campaignToResp(c))
}

// Delete handles DELETE /v1/campaigns/{id} (soft delete).
func (h *CampaignsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.Store.SoftDeleteCampaign(id); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func campaignToResp(c *domain.Campaign) map[string]interface{} {
	resp := map[string]interface{}{
		"id":                    c.ID,
		"name":                  c.Name,
		"description":           c.Description,
		"active":                c.Active,
		"target_id":             c.TargetID,
		"scan_profile_id":       c.ScanProfileID,
		"schedule_type":         c.ScheduleType,
		"schedule_config":       c.ScheduleConfig,
		"materialization_policy": c.MaterializationPolicy,
		"run_once_done":         c.RunOnceDone,
		"concurrency_policy":    c.ConcurrencyPolicy,
		"created_at":            c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":            c.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if c.NextRunAt != nil {
		resp["next_run_at"] = c.NextRunAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return resp
}

// timeNow is for testability.
var timeNow = func() time.Time { return time.Now() }

// parseTime parses RFC3339.
func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/domain"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/dispatch"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/targetclient"
)

// ActiveRunChecker returns whether a campaign has an active run (pending/dispatching/dispatched).
type ActiveRunChecker interface {
	HasActiveRun(campaignID uuid.UUID) (bool, error)
}

// LaunchHandler handles POST /v1/campaigns/{id}/launch.
type LaunchHandler struct {
	Store           CampaignStore
	ProfileStore    ScanProfileStore
	ActiveRunChecker ActiveRunChecker
	RunUpdater      RunUpdater
	CampaignUpdater CampaignUpdater
	Target          *targetclient.Client
	Dispatcher      dispatch.Dispatcher
}

// CampaignUpdater updates campaign fields (next_run_at, run_once_done).
type CampaignUpdater interface {
	UpdateCampaign(*domain.Campaign) error
}

// RunUpdater creates and updates runs.
type RunUpdater interface {
	CreateCampaignRun(*domain.CampaignRun) error
	UpdateCampaignRun(*domain.CampaignRun) error
}

// Launch creates a run, resolves materialization, dispatches (or marks completed if 0 prefixes).
func (h *LaunchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	campaignID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	campaign, err := h.Store.GetCampaignByID(campaignID)
	if err != nil || campaign == nil {
		if campaign == nil {
			writeJSONError(w, "campaign not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Concurrency: forbid_if_active and already has active run -> 409
	hasActive, err := h.ActiveRunChecker.HasActiveRun(campaignID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if campaign.ConcurrencyPolicy == domain.ConcurrencyPolicyForbidIfActive && hasActive {
		writeJSONError(w, "campaign has an active run", http.StatusConflict)
		return
	}
	profile, err := h.ProfileStore.GetScanProfileByID(campaign.ScanProfileID)
	if err != nil || profile == nil {
		if profile == nil {
			writeJSONError(w, "scan_profile not found", http.StatusInternalServerError)
			return
		}
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Resolve materialization
	var materializationID uuid.UUID
	var prefixCount int
	if campaign.MaterializationPolicy == domain.MaterializationPolicyUseLatest {
		resp, err := h.Target.ListMaterializations(r.Context(), campaign.TargetID, 1, 0)
		if err != nil {
			writeJSONError(w, "target-service: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(resp.Items) == 0 {
			writeJSONError(w, "no materializations for target (use_latest); materialize first or use rematerialize", http.StatusBadRequest)
			return
		}
		materializationID = resp.Items[0].ID
		prefixCount = resp.Items[0].TotalPrefixCount
	} else {
		matResp, err := h.Target.Materialize(r.Context(), campaign.TargetID)
		if err != nil {
			writeJSONError(w, "target-service materialize: "+err.Error(), http.StatusBadRequest)
			return
		}
		materializationID = matResp.MaterializationID
		prefixCount = matResp.TotalPrefixCount
	}

	now := time.Now()
	statsBytes, _ := json.Marshal(map[string]int{"prefix_count": prefixCount})
	stats := json.RawMessage(statsBytes)

	run := &domain.CampaignRun{
		CampaignID:                campaignID,
		TargetID:                  campaign.TargetID,
		TargetMaterializationID:   materializationID,
		ScanProfileID:            profile.ID,
		ScanProfileSlug:          profile.Slug,
		ScanProfileConfigSnapshot: profile.Config,
		Status:                   domain.StatusDispatching,
		StartedAt:                &now,
		Stats:                    stats,
	}
	if err := h.RunUpdater.CreateCampaignRun(run); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 0 prefixes: complete run, do not dispatch
	if prefixCount == 0 {
		run.Status = domain.StatusCompleted
		run.CompletedAt = &now
		_ = h.RunUpdater.UpdateCampaignRun(run)
		if campaign.ScheduleType == domain.ScheduleTypeOnce {
			campaign.RunOnceDone = true
			_ = h.CampaignUpdater.UpdateCampaign(campaign)
		}
		writeJSON(w, http.StatusCreated, runToResp(run))
		return
	}

	// Dispatch to Redis
	payload := &dispatch.Payload{
		RunID:                   run.ID.String(),
		CampaignID:              campaignID.String(),
		TargetID:                campaign.TargetID.String(),
		TargetMaterializationID: materializationID.String(),
		ScanProfileSlug:         profile.Slug,
		ScanProfileConfig:       profile.Config,
		CreatedAt:               run.CreatedAt,
		DispatchAttempt:         1,
	}
	streamID, err := h.Dispatcher.Dispatch(r.Context(), payload)
	if err != nil {
		run.Status = domain.StatusDispatchFailed
		run.ErrorMessage = err.Error()
		_ = h.RunUpdater.UpdateCampaignRun(run)
		writeJSONError(w, "dispatch failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	run.Status = domain.StatusDispatched
	run.DispatchRef = streamID
	run.DispatchedAt = &now
	if err := h.RunUpdater.UpdateCampaignRun(run); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if campaign.ScheduleType == domain.ScheduleTypeOnce {
		campaign.RunOnceDone = true
		_ = h.CampaignUpdater.UpdateCampaign(campaign)
	}
	writeJSON(w, http.StatusCreated, runToResp(run))
}

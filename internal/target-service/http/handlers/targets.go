package handlers

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/target-service/domain"
	"github.com/martinezpascualdani/heimdall/pkg/iso3166"
)

const defaultLimit = 100
const maxLimit = 10_000

var errInvalidRule = errors.New("invalid rule")

func parseCIDR(s string) (net.IP, *net.IPNet, error) {
	return net.ParseCIDR(s)
}

// TargetStore is the storage interface for target CRUD.
type TargetStore interface {
	CreateTarget(*domain.Target) error
	GetTargetByID(uuid.UUID) (*domain.Target, error)
	ListTargets(limit, offset int, activeOnly bool) ([]*domain.Target, error)
	UpdateTarget(*domain.Target) error
	SoftDeleteTarget(uuid.UUID) error
	InsertRules(uuid.UUID, []domain.TargetRule) error
	ListRulesByTargetID(uuid.UUID) ([]domain.TargetRule, error)
}

// TargetsHandler handles CRUD for /v1/targets.
type TargetsHandler struct {
	Store TargetStore
}

// CreateTargetRequest is the body for POST /v1/targets.
type CreateTargetRequest struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Rules       []CreateTargetRule   `json:"rules,omitempty"`
}

// CreateTargetRule is a rule in create/update request.
type CreateTargetRule struct {
	Kind          string `json:"kind"`           // include | exclude
	SelectorType  string `json:"selector_type"`   // country | asn | prefix | world
	SelectorValue string `json:"selector_value"`  // ISO, ASN string, CIDR, or empty for world
	AddressFamily string `json:"address_family,omitempty"`
	RuleOrder     int    `json:"rule_order"`
}

// TargetResponse is target with rules for GET responses.
type TargetResponse struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Active      bool              `json:"active"`
	Tags        []string          `json:"tags,omitempty"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
	Rules       []TargetRuleResp  `json:"rules"`
}

// TargetRuleResp is a rule in API response.
type TargetRuleResp struct {
	ID            uuid.UUID `json:"id"`
	Kind          string    `json:"kind"`
	SelectorType  string    `json:"selector_type"`
	SelectorValue string    `json:"selector_value"`
	AddressFamily string    `json:"address_family,omitempty"`
	RuleOrder     int       `json:"rule_order"`
}

func validateRule(r *CreateTargetRule) error {
	kind := strings.ToLower(strings.TrimSpace(r.Kind))
	if kind != "include" && kind != "exclude" {
		return errInvalidRule
	}
	sel := strings.ToLower(strings.TrimSpace(r.SelectorType))
	if sel != "country" && sel != "asn" && sel != "prefix" && sel != "world" {
		return errInvalidRule
	}
	if sel == "world" && kind == "exclude" {
		return errInvalidRule // world in exclude is rejected
	}
	if sel == "world" && strings.TrimSpace(r.SelectorValue) != "" {
		return errInvalidRule
	}
	if sel == "country" {
		cc := strings.TrimSpace(r.SelectorValue)
		if len(cc) != 2 {
			return errInvalidRule
		}
		if !iso3166.ValidAlpha2(cc) {
			return errInvalidRule
		}
	}
	if sel == "asn" {
		// must be numeric string
		v := strings.TrimSpace(r.SelectorValue)
		if v == "" {
			return errInvalidRule
		}
		for _, c := range v {
			if c < '0' || c > '9' {
				return errInvalidRule
			}
		}
	}
	if sel == "prefix" {
		v := strings.TrimSpace(r.SelectorValue)
		if v == "" {
			return errInvalidRule
		}
		if _, _, err := parseCIDR(v); err != nil {
			return errInvalidRule
		}
	}
	return nil
}

func toDomainRules(req []CreateTargetRule) ([]domain.TargetRule, error) {
	out := make([]domain.TargetRule, 0, len(req))
	for i := range req {
		if err := validateRule(&req[i]); err != nil {
			return nil, err
		}
		af := strings.ToLower(strings.TrimSpace(req[i].AddressFamily))
		if af == "" {
			af = "all"
		}
		r := domain.TargetRule{
			Kind:          strings.ToLower(strings.TrimSpace(req[i].Kind)),
			SelectorType:  strings.ToLower(strings.TrimSpace(req[i].SelectorType)),
			SelectorValue: strings.TrimSpace(req[i].SelectorValue),
			AddressFamily: af,
			RuleOrder:     req[i].RuleOrder,
		}
		out = append(out, r)
	}
	return out, nil
}

// POST /v1/targets
func (h *TargetsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req CreateTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid body", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeJSONError(w, "name required", http.StatusBadRequest)
		return
	}
	rules, err := toDomainRules(req.Rules)
	if err != nil {
		writeJSONError(w, "invalid rule", http.StatusBadRequest)
		return
	}
	t := &domain.Target{
		Name:        req.Name,
		Description: strings.TrimSpace(req.Description),
		Active:      true,
	}
	if err := h.Store.CreateTarget(t); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(rules) > 0 {
		if err := h.Store.InsertRules(t.ID, rules); err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toTargetResponse(t, rules))
}

// GET /v1/targets
func (h *TargetsHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	limit, offset := parseLimitOffset(r, defaultLimit, maxLimit)
	includeInactive := r.URL.Query().Get("include_inactive") == "true" || r.URL.Query().Get("include_inactive") == "1"
	activeOnly := !includeInactive
	targets, err := h.Store.ListTargets(limit, offset, activeOnly)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]TargetResponse, 0, len(targets))
	for _, t := range targets {
		rules, _ := h.Store.ListRulesByTargetID(t.ID)
		items = append(items, toTargetResponse(t, rules))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items": items,
		"count": len(items),
	})
}

// GET /v1/targets/{id}
func (h *TargetsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	t, err := h.Store.GetTargetByID(id)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if t == nil {
		writeJSONError(w, "target not found", http.StatusNotFound)
		return
	}
	rules, _ := h.Store.ListRulesByTargetID(t.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toTargetResponse(t, rules))
}

// PUT /v1/targets/{id} — full replacement
func (h *TargetsHandler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	t, err := h.Store.GetTargetByID(id)
	if err != nil || t == nil {
		writeJSONError(w, "target not found", http.StatusNotFound)
		return
	}
	var req CreateTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid body", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeJSONError(w, "name required", http.StatusBadRequest)
		return
	}
	rules, err := toDomainRules(req.Rules)
	if err != nil {
		writeJSONError(w, "invalid rule", http.StatusBadRequest)
		return
	}
	t.Name = req.Name
	t.Description = strings.TrimSpace(req.Description)
	if err := h.Store.UpdateTarget(t); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.Store.InsertRules(t.ID, rules); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toTargetResponse(t, rules))
}

// DELETE /v1/targets/{id} — soft delete, idempotent
func (h *TargetsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.Store.SoftDeleteTarget(id); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toTargetResponse(t *domain.Target, rules []domain.TargetRule) TargetResponse {
	resp := TargetResponse{
		ID:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		Active:      t.Active,
		Tags:        t.Tags,
		CreatedAt:   t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   t.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Rules:       make([]TargetRuleResp, 0, len(rules)),
	}
	for _, r := range rules {
		resp.Rules = append(resp.Rules, TargetRuleResp{
			ID: r.ID, Kind: r.Kind, SelectorType: r.SelectorType, SelectorValue: r.SelectorValue,
			AddressFamily: r.AddressFamily, RuleOrder: r.RuleOrder,
		})
	}
	return resp
}

func parseLimitOffset(r *http.Request, defaultL, maxL int) (limit, offset int) {
	limit = defaultL
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			limit = n
			if limit > maxL {
				limit = maxL
			}
		}
	}
	offset = 0
	if s := r.URL.Query().Get("offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

func writeJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

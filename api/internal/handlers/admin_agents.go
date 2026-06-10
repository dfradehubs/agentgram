package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/proxy"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/security"
	"go.uber.org/zap"
)

// AdminAgentsHandler handles admin CRUD for agents
type AdminAgentsHandler struct {
	agentRepo repository.AgentRepository
	auditRepo repository.AuditRepository
	groupRepo repository.GroupRepository
	registry  *agents.Registry
	logger    *zap.Logger
}

// NewAdminAgentsHandler creates a new admin agents handler
func NewAdminAgentsHandler(agentRepo repository.AgentRepository, auditRepo repository.AuditRepository, groupRepo repository.GroupRepository, registry *agents.Registry, logger *zap.Logger) *AdminAgentsHandler {
	return &AdminAgentsHandler{
		agentRepo: agentRepo,
		auditRepo: auditRepo,
		groupRepo: groupRepo,
		registry:  registry,
		logger:    logger,
	}
}

// AdminAgentRequest is the request body for creating/updating agents
type AdminAgentRequest struct {
	ID                   string                     `json:"id"`
	Name                 string                     `json:"name"`
	Description          string                     `json:"description"`
	Category             string                     `json:"category"`
	Protocol             string                     `json:"protocol"`
	Endpoint             string                     `json:"endpoint"`
	AgentCardPath        string                     `json:"agent_card_path"`
	ForwardAuthorization bool                       `json:"forward_authorization"`
	RequireGitHubToken   bool                       `json:"require_github_token"`
	PipelineFinalAgent   string                     `json:"pipeline_final_agent"`
	ADKAppName           string                     `json:"adk_app_name"`
	ADKUserID            string                     `json:"adk_user_id"`
	Headers              map[string]string          `json:"headers"`
	RateLimit            *models.RateLimitConfig    `json:"rate_limit"`
	HealthCheck          *models.HealthCheckConfig  `json:"health_check"`
	Polling              *models.PollingConfig      `json:"polling"`
	CustomFormat         *models.CustomFormatConfig `json:"custom_format"`
	MaxContextTokens     *int                       `json:"max_context_tokens"`
	SummarizeThreshold   *float64                   `json:"summarize_threshold"`
	AllowedUsers         []string                   `json:"allowed_users"`
	AllowedGroups        []string                   `json:"allowed_groups"`

	// Outbound auth. AuthType: "" | "none" | "forward" | "bearer" ("oauth2"
	// is reserved and rejected). Empty keeps the legacy ForwardAuthorization
	// semantics. APIKeyRules nil means "leave existing rules untouched" so
	// older API clients don't wipe them on update.
	AuthType       string                   `json:"auth_type"`
	BearerToken    string                   `json:"bearer_token"`
	AuthHeaderName string                   `json:"auth_header_name"`
	APIKeyRules    []models.AgentAPIKeyRule `json:"api_key_rules"`
}

// AdminAgentResponse includes agent data + permissions for admin view
type AdminAgentResponse struct {
	ID                   string                     `json:"id"`
	Name                 string                     `json:"name"`
	Description          string                     `json:"description"`
	Category             string                     `json:"category"`
	Protocol             string                     `json:"protocol"`
	Endpoint             string                     `json:"endpoint"`
	AgentCardPath        string                     `json:"agent_card_path,omitempty"`
	ForwardAuthorization bool                       `json:"forward_authorization"`
	RequireGitHubToken   bool                       `json:"require_github_token"`
	PipelineFinalAgent   string                     `json:"pipeline_final_agent,omitempty"`
	ADKAppName           string                     `json:"adk_app_name,omitempty"`
	ADKUserID            string                     `json:"adk_user_id,omitempty"`
	Headers              map[string]string          `json:"headers"`
	RateLimit            *models.RateLimitConfig    `json:"rate_limit,omitempty"`
	HealthCheck          *models.HealthCheckConfig  `json:"health_check,omitempty"`
	Polling              *models.PollingConfig      `json:"polling,omitempty"`
	CustomFormat         *models.CustomFormatConfig `json:"custom_format,omitempty"`
	MaxContextTokens     int                        `json:"max_context_tokens"`
	SummarizeThreshold   float64                    `json:"summarize_threshold"`
	AllowedUsers         []string                   `json:"allowed_users"`
	AllowedGroups        []string                   `json:"allowed_groups"`
	InheritedPermissions *models.InheritedPerms     `json:"inherited_permissions,omitempty"`
	Status               string                     `json:"status"`
	AuthType             string                     `json:"auth_type"`
	BearerToken          string                     `json:"bearer_token,omitempty"`
	AuthHeaderName       string                     `json:"auth_header_name,omitempty"`
	APIKeyRules          []models.AgentAPIKeyRule   `json:"api_key_rules,omitempty"`
}

func agentToAdminResponse(a *models.Agent, users, groups []string) AdminAgentResponse {
	return AdminAgentResponse{
		ID:                   a.ID,
		Name:                 a.Name,
		Description:          a.Description,
		Category:             a.Category,
		Protocol:             a.Protocol,
		Endpoint:             a.Endpoint,
		AgentCardPath:        a.AgentCardPath,
		ForwardAuthorization: a.ForwardAuthorization,
		RequireGitHubToken:   a.RequireGitHubToken,
		PipelineFinalAgent:   a.PipelineFinalAgent,
		ADKAppName:           a.ADKAppName,
		ADKUserID:            a.ADKUserID,
		Headers:              a.Headers,
		RateLimit:            a.RateLimit,
		HealthCheck:          a.HealthCheck,
		Polling:              a.Polling,
		CustomFormat:         a.CustomFormat,
		MaxContextTokens:     a.MaxContextTokens,
		SummarizeThreshold:   a.SummarizeThreshold,
		AllowedUsers:         users,
		AllowedGroups:        groups,
		Status:               a.Status,
		AuthType:             a.GetAuthType(),
		BearerToken:          a.BearerToken,
		AuthHeaderName:       a.AuthHeaderName,
		APIKeyRules:          a.APIKeyRules,
	}
}

// validateAgentAuth validates the outbound-auth fields shared by Create/Update.
// Returns a client-facing error message, or "" when valid.
func validateAgentAuth(req *AdminAgentRequest) string {
	switch req.AuthType {
	case "", models.AgentAuthNone, models.AgentAuthForward, models.AgentAuthBearer:
	case models.AgentAuthOAuth2:
		return "auth_type oauth2 is not yet supported for agents"
	default:
		return fmt.Sprintf("invalid auth_type: %s", req.AuthType)
	}

	if req.AuthHeaderName != "" {
		if err := security.ValidateHeaders(map[string]string{req.AuthHeaderName: "x"}); err != nil {
			return fmt.Sprintf("invalid auth_header_name: %s", err.Error())
		}
		// Headers that would corrupt the outbound HTTP request itself
		// (not covered by the SSRF blocklist, which targets metadata APIs).
		switch strings.ToLower(req.AuthHeaderName) {
		case "host", "content-length", "content-type", "transfer-encoding", "connection":
			return fmt.Sprintf("invalid auth_header_name: %s", req.AuthHeaderName)
		}
	}

	seen := make(map[string]struct{}, len(req.APIKeyRules))
	for _, rule := range req.APIKeyRules {
		if rule.SubjectType != "user" && rule.SubjectType != "group" {
			return fmt.Sprintf("invalid api_key_rules subject_type: %s", rule.SubjectType)
		}
		if rule.Subject == "" || rule.APIKey == "" {
			return "api_key_rules entries require subject and api_key"
		}
		key := rule.SubjectType + "\x00" + rule.Subject
		if _, dup := seen[key]; dup {
			return fmt.Sprintf("duplicate api_key_rules entry: %s %s", rule.SubjectType, rule.Subject)
		}
		seen[key] = struct{}{}
	}
	return ""
}

// ListAgents handles GET /api/admin/agents
// @Summary List all agents (admin)
// @Description Returns all agents with full configuration including endpoints, permissions, and health status. Requires admin role.
// @Tags admin-agents
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Success 200 {object} map[string][]AdminAgentResponse "agents array"
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/admin/agents [get]
func (h *AdminAgentsHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := h.agentRepo.List(r.Context())
	if err != nil {
		h.logger.Error("list agents failed", zap.Error(err))
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	responses := make([]AdminAgentResponse, 0, len(agents))
	for _, a := range agents {
		// Get current status from registry
		if cached, err := h.registry.Get(a.ID); err == nil {
			a.Status = cached.Status
		}
		responses = append(responses, agentToAdminResponse(a, a.AllowedUsers, a.AllowedGroups))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"agents": responses})
}

// GetAgent handles GET /api/admin/agents/{id}
// @Summary Get agent details (admin)
// @Description Returns full agent configuration including endpoint, headers, permissions, and inherited permissions. Requires admin role.
// @Tags admin-agents
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param id path string true "Agent ID"
// @Success 200 {object} AdminAgentResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/admin/agents/{id} [get]
func (h *AdminAgentsHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	agent, users, groups, err := h.agentRepo.Get(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}

	// Get current status from registry
	if cached, err := h.registry.Get(id); err == nil {
		agent.Status = cached.Status
	}

	resp := agentToAdminResponse(agent, users, groups)

	// Load inherited permissions from agent groups
	inheritedMap, err := h.groupRepo.GetAllInheritedPermissions(r.Context())
	if err == nil {
		if perms, ok := inheritedMap[id]; ok {
			resp.InheritedPermissions = perms
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// CreateAgent handles POST /api/admin/agents
// @Summary Create a new agent (admin)
// @Description Creates a new agent configuration. Requires admin role. Fields id, name, protocol, and endpoint are required.
// @Tags admin-agents
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param request body AdminAgentRequest true "Agent configuration"
// @Success 201 {object} AdminAgentResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/admin/agents [post]
func (h *AdminAgentsHandler) CreateAgent(w http.ResponseWriter, r *http.Request) {
	var req AdminAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.ID == "" || req.Name == "" || req.Protocol == "" || req.Endpoint == "" {
		http.Error(w, `{"error":"id, name, protocol, and endpoint are required"}`, http.StatusBadRequest)
		return
	}

	// Validate endpoint URL against SSRF
	if err := security.ValidateEndpointURL(req.Endpoint); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid endpoint: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	// Validate headers against blocked list
	if err := security.ValidateHeaders(req.Headers); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid headers: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	// Validate health check URL if provided
	if req.HealthCheck != nil && req.HealthCheck.URL != "" {
		if err := security.ValidateEndpointURL(req.HealthCheck.URL); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid health check URL: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}
	}

	// Validate custom format template if provided
	if req.Protocol == "custom" && req.CustomFormat != nil && req.CustomFormat.RequestTemplate != "" {
		if err := proxy.ValidateRequestTemplate(req.CustomFormat.RequestTemplate); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid request template: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}
	}

	if msg := validateAgentAuth(&req); msg != "" {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, msg), http.StatusBadRequest)
		return
	}

	agent := &models.Agent{
		ID:                   req.ID,
		Name:                 req.Name,
		Description:          req.Description,
		Category:             req.Category,
		Protocol:             req.Protocol,
		Endpoint:             req.Endpoint,
		AgentCardPath:        req.AgentCardPath,
		ForwardAuthorization: req.ForwardAuthorization,
		RequireGitHubToken:   req.RequireGitHubToken,
		PipelineFinalAgent:   req.PipelineFinalAgent,
		ADKAppName:           req.ADKAppName,
		ADKUserID:            req.ADKUserID,
		Headers:              req.Headers,
		RateLimit:            req.RateLimit,
		HealthCheck:          req.HealthCheck,
		Polling:              req.Polling,
		CustomFormat:         req.CustomFormat,
		MaxContextTokens:     derefInt(req.MaxContextTokens, 200000),
		SummarizeThreshold:   derefFloat(req.SummarizeThreshold, 0.8),
		AuthType:             req.AuthType,
		BearerToken:          req.BearerToken,
		AuthHeaderName:       req.AuthHeaderName,
		APIKeyRules:          req.APIKeyRules,
	}
	// Keep the legacy flag in sync for old readers of forward_authorization.
	agent.ForwardAuthorization = req.ForwardAuthorization || req.AuthType == models.AgentAuthForward

	if err := h.agentRepo.Create(r.Context(), agent, req.AllowedUsers, req.AllowedGroups); err != nil {
		h.logger.Error("create agent failed", zap.Error(err))
		http.Error(w, `{"error":"failed to create agent"}`, http.StatusInternalServerError)
		return
	}

	// Audit
	claims := middleware.GetUserFromContext(r.Context())
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       "create",
		ResourceType: "agent",
		ResourceID:   req.ID,
	})

	// Refresh registry
	h.registry.Refresh(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(agentToAdminResponse(agent, req.AllowedUsers, req.AllowedGroups))
}

// UpdateAgent handles PUT /api/admin/agents/{id}
// @Summary Update an agent (admin)
// @Description Updates an existing agent configuration. Requires admin role.
// @Tags admin-agents
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param id path string true "Agent ID"
// @Param request body AdminAgentRequest true "Updated agent configuration"
// @Success 200 {object} AdminAgentResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/admin/agents/{id} [put]
func (h *AdminAgentsHandler) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req AdminAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Validate endpoint URL against SSRF
	if req.Endpoint != "" {
		if err := security.ValidateEndpointURL(req.Endpoint); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid endpoint: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}
	}

	// Validate headers against blocked list
	if err := security.ValidateHeaders(req.Headers); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid headers: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	// Validate health check URL if provided
	if req.HealthCheck != nil && req.HealthCheck.URL != "" {
		if err := security.ValidateEndpointURL(req.HealthCheck.URL); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid health check URL: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}
	}

	// Validate custom format template if provided
	if req.Protocol == "custom" && req.CustomFormat != nil && req.CustomFormat.RequestTemplate != "" {
		if err := proxy.ValidateRequestTemplate(req.CustomFormat.RequestTemplate); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid request template: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}
	}

	if msg := validateAgentAuth(&req); msg != "" {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, msg), http.StatusBadRequest)
		return
	}

	agent := &models.Agent{
		ID:                   id,
		Name:                 req.Name,
		Description:          req.Description,
		Category:             req.Category,
		Protocol:             req.Protocol,
		Endpoint:             req.Endpoint,
		AgentCardPath:        req.AgentCardPath,
		ForwardAuthorization: req.ForwardAuthorization,
		RequireGitHubToken:   req.RequireGitHubToken,
		PipelineFinalAgent:   req.PipelineFinalAgent,
		ADKAppName:           req.ADKAppName,
		ADKUserID:            req.ADKUserID,
		Headers:              req.Headers,
		RateLimit:            req.RateLimit,
		HealthCheck:          req.HealthCheck,
		Polling:              req.Polling,
		CustomFormat:         req.CustomFormat,
		MaxContextTokens:     derefInt(req.MaxContextTokens, 200000),
		SummarizeThreshold:   derefFloat(req.SummarizeThreshold, 0.8),
		AuthType:             req.AuthType,
		BearerToken:          req.BearerToken,
		AuthHeaderName:       req.AuthHeaderName,
	}
	// Keep the legacy flag in sync for old readers of forward_authorization.
	agent.ForwardAuthorization = req.ForwardAuthorization || req.AuthType == models.AgentAuthForward

	if err := h.agentRepo.Update(r.Context(), agent); err != nil {
		h.logger.Error("update agent failed", zap.Error(err))
		http.Error(w, `{"error":"failed to update agent"}`, http.StatusInternalServerError)
		return
	}

	// Replace API key rules only when the field is present in the request,
	// so older API clients that omit it don't wipe existing rules.
	if req.APIKeyRules != nil {
		if err := h.agentRepo.ReplaceAPIKeyRules(r.Context(), id, req.APIKeyRules); err != nil {
			h.logger.Error("replace api key rules failed", zap.Error(err))
			http.Error(w, `{"error":"failed to update api key rules"}`, http.StatusInternalServerError)
			return
		}
		agent.APIKeyRules = req.APIKeyRules
	} else {
		agent.APIKeyRules, _ = h.agentRepo.ListAPIKeyRules(r.Context(), id)
	}

	// Audit
	claims := middleware.GetUserFromContext(r.Context())
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       "update",
		ResourceType: "agent",
		ResourceID:   id,
	})

	// Refresh registry
	h.registry.Refresh(r.Context())

	users, groups, _ := h.agentRepo.GetPermissions(r.Context(), id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agentToAdminResponse(agent, users, groups))
}

// DeleteAgent handles DELETE /api/admin/agents/{id}
// @Summary Delete an agent (admin)
// @Description Deletes an agent configuration. Requires admin role.
// @Tags admin-agents
// @Security BearerAuth
// @Security CookieAuth
// @Param id path string true "Agent ID"
// @Success 204 "Agent deleted"
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/admin/agents/{id} [delete]
func (h *AdminAgentsHandler) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.agentRepo.Delete(r.Context(), id); err != nil {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}

	// Audit
	claims := middleware.GetUserFromContext(r.Context())
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       "delete",
		ResourceType: "agent",
		ResourceID:   id,
	})

	// Refresh registry
	h.registry.Refresh(r.Context())

	w.WriteHeader(http.StatusNoContent)
}

func derefInt(p *int, fallback int) int {
	if p != nil {
		return *p
	}
	return fallback
}

func derefFloat(p *float64, fallback float64) float64 {
	if p != nil {
		return *p
	}
	return fallback
}

// PermissionsRequest is the request body for updating permissions
type PermissionsRequest struct {
	AllowedUsers  []string `json:"allowed_users"`
	AllowedGroups []string `json:"allowed_groups"`
}

// UpdatePermissions handles PUT /api/admin/agents/{id}/permissions
func (h *AdminAgentsHandler) UpdatePermissions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req PermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if err := h.agentRepo.UpdatePermissions(r.Context(), id, req.AllowedUsers, req.AllowedGroups); err != nil {
		h.logger.Error("update permissions failed", zap.Error(err))
		http.Error(w, `{"error":"failed to update permissions"}`, http.StatusInternalServerError)
		return
	}

	// Audit
	claims := middleware.GetUserFromContext(r.Context())
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       "update_permissions",
		ResourceType: "agent",
		ResourceID:   id,
	})

	// Refresh registry
	h.registry.Refresh(r.Context())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"allowed_users":  req.AllowedUsers,
		"allowed_groups": req.AllowedGroups,
	})
}

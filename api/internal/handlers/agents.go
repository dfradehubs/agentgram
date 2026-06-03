package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/service"
	"go.uber.org/zap"
)

// AgentsHandler handles agent endpoints
type AgentsHandler struct {
	registry    *agents.Registry
	userService *service.UserService
	groupRepo   repository.GroupRepository
	logger      *zap.Logger
}

// NewAgentsHandler creates a new agents handler
func NewAgentsHandler(registry *agents.Registry, userService *service.UserService, groupRepo repository.GroupRepository, logger *zap.Logger) *AgentsHandler {
	return &AgentsHandler{
		registry:    registry,
		userService: userService,
		groupRepo:   groupRepo,
		logger:      logger,
	}
}

// ListAgents handles GET /api/agents
// Returns ONLY the agents the user has access to
// @Summary List agents
// @Description Returns only the agents the authenticated user has access to. Admins see all agents.
// @Tags agents
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Success 200 {object} models.AgentListResponse
// @Failure 401 {object} models.ErrorResponse
// @Router /api/agents [get]
func (h *AgentsHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get user claims from context
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	userEmail := claims.GetEmail()
	userGroups := claims.GetGroups()

	h.logger.Debug("listing agents for user",
		zap.String("email", userEmail),
		zap.Strings("groups", userGroups))

	// Get all agents
	allAgents := h.registry.List()

	// Admins see all agents; regular users see only permitted ones (including inherited group permissions)
	var allowedAgents []*models.Agent
	isAdmin, _ := h.userService.IsAdmin(r.Context(), userEmail, userGroups)
	if isAdmin {
		allowedAgents = allAgents
	} else {
		// Load inherited permissions from agent groups
		inheritedMap, err := h.groupRepo.GetAllInheritedPermissions(r.Context())
		if err != nil {
			h.logger.Warn("failed to load inherited permissions", zap.Error(err))
			inheritedMap = nil
		}
		if inheritedMap != nil {
			allowedAgents = agents.FilterAgentsByAccessWithInherited(allAgents, userEmail, userGroups, inheritedMap)
		} else {
			allowedAgents = agents.FilterAgentsByAccess(allAgents, userEmail, userGroups)
		}
	}

	// Convert to response (without sensitive data)
	agentResponses := make([]models.AgentResponse, 0, len(allowedAgents))
	for _, agent := range allowedAgents {
		agentResponses = append(agentResponses, agent.ToResponse())
	}

	response := models.AgentListResponse{
		Agents: agentResponses,
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// GetAgent handles GET /api/agents/:agentId
// @Summary Get agent details
// @Description Returns a single agent by ID. Requires access permission or admin role.
// @Tags agents
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param agentId path string true "Agent ID"
// @Success 200 {object} models.AgentResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/agents/{agentId} [get]
func (h *AgentsHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	agentID := chi.URLParam(r, "agentId")
	if agentID == "" {
		http.Error(w, `{"error":"agent id required"}`, http.StatusBadRequest)
		return
	}

	// Get user claims
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Find the agent
	agent, err := h.registry.Get(agentID)
	if err != nil {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}

	// Admins can access all agents; regular users need explicit or inherited permission
	isAdmin, _ := h.userService.IsAdmin(r.Context(), claims.GetEmail(), claims.GetGroups())
	if !isAdmin {
		inheritedMap, _ := h.groupRepo.GetAllInheritedPermissions(r.Context())
		inherited := inheritedMap[agentID]
		if !agents.HasAccessWithInherited(agent, claims.GetEmail(), claims.GetGroups(), inherited) {
			http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(agent.ToResponse())
}

// GetAgentHealth handles GET /api/agents/:agentId/health
// @Summary Get agent health status
// @Description Returns the health status of a specific agent
// @Tags agents
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param agentId path string true "Agent ID"
// @Success 200 {object} models.HealthResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/agents/{agentId}/health [get]
func (h *AgentsHandler) GetAgentHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	agentID := chi.URLParam(r, "agentId")
	if agentID == "" {
		http.Error(w, `{"error":"agent id required"}`, http.StatusBadRequest)
		return
	}

	// Get user claims
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Find the agent
	agent, err := h.registry.Get(agentID)
	if err != nil {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}

	// Admins can access all agents; regular users need explicit or inherited permission
	isAdmin, _ := h.userService.IsAdmin(r.Context(), claims.GetEmail(), claims.GetGroups())
	if !isAdmin {
		inheritedMap, _ := h.groupRepo.GetAllInheritedPermissions(r.Context())
		inherited := inheritedMap[agentID]
		if !agents.HasAccessWithInherited(agent, claims.GetEmail(), claims.GetGroups(), inherited) {
			http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
			return
		}
	}

	response := models.HealthResponse{
		Status: agent.Status,
		Details: map[string]interface{}{
			"agent_id": agent.ID,
			"name":     agent.Name,
			"protocol": agent.Protocol,
		},
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

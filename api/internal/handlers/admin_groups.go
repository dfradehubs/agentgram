package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

// AdminGroupsHandler handles admin CRUD for agent groups
type AdminGroupsHandler struct {
	groupRepo repository.GroupRepository
	auditRepo repository.AuditRepository
	logger    *zap.Logger
}

// NewAdminGroupsHandler creates a new admin groups handler
func NewAdminGroupsHandler(groupRepo repository.GroupRepository, auditRepo repository.AuditRepository, logger *zap.Logger) *AdminGroupsHandler {
	return &AdminGroupsHandler{
		groupRepo: groupRepo,
		auditRepo: auditRepo,
		logger:    logger,
	}
}

// AdminGroupRequest is the request body for creating/updating groups
type AdminGroupRequest struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	AgentIDs      []string `json:"agent_ids"`
	AllowedUsers  []string `json:"allowed_users"`
	AllowedGroups []string `json:"allowed_groups"`
}

// AdminGroupResponse is the response for admin group views
type AdminGroupResponse struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	AgentIDs      []string `json:"agent_ids"`
	CreatedBy     string   `json:"created_by"`
	AllowedUsers  []string `json:"allowed_users"`
	AllowedGroups []string `json:"allowed_groups"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

func groupToAdminResponse(g *models.AgentGroup) AdminGroupResponse {
	return AdminGroupResponse{
		ID:            g.ID,
		Name:          g.Name,
		AgentIDs:      g.AgentIDs,
		CreatedBy:     g.CreatedBy,
		AllowedUsers:  g.AllowedUsers,
		AllowedGroups: g.AllowedGroups,
		CreatedAt:     g.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     g.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// ListGroups handles GET /api/admin/groups
func (h *AdminGroupsHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.groupRepo.List(r.Context())
	if err != nil {
		h.logger.Error("list groups failed", zap.Error(err))
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	responses := make([]AdminGroupResponse, 0, len(groups))
	for _, g := range groups {
		responses = append(responses, groupToAdminResponse(g))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"groups": responses})
}

// GetGroup handles GET /api/admin/groups/{id}
func (h *AdminGroupsHandler) GetGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	group, err := h.groupRepo.Get(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"group not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(groupToAdminResponse(group))
}

// CreateGroup handles POST /api/admin/groups
func (h *AdminGroupsHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var req AdminGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.ID == "" || req.Name == "" || len(req.AgentIDs) < 2 {
		http.Error(w, `{"error":"id, name, and at least 2 agent_ids are required"}`, http.StatusBadRequest)
		return
	}

	claims := middleware.GetUserFromContext(r.Context())

	group := &models.AgentGroup{
		ID:       req.ID,
		Name:     req.Name,
		AgentIDs: req.AgentIDs,
		CreatedBy: claims.GetEmail(),
	}

	if err := h.groupRepo.Create(r.Context(), group, req.AllowedUsers, req.AllowedGroups); err != nil {
		h.logger.Error("create group failed", zap.Error(err))
		http.Error(w, `{"error":"failed to create group"}`, http.StatusInternalServerError)
		return
	}

	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       "create",
		ResourceType: "group",
		ResourceID:   req.ID,
	})

	// Re-fetch to get full data with permissions
	created, err := h.groupRepo.Get(r.Context(), req.ID)
	if err != nil {
		// Fallback: return the input
		group.AllowedUsers = req.AllowedUsers
		group.AllowedGroups = req.AllowedGroups
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(groupToAdminResponse(group))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(groupToAdminResponse(created))
}

// UpdateGroup handles PUT /api/admin/groups/{id}
func (h *AdminGroupsHandler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req AdminGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	group := &models.AgentGroup{
		ID:       id,
		Name:     req.Name,
		AgentIDs: req.AgentIDs,
	}

	if err := h.groupRepo.Update(r.Context(), group); err != nil {
		h.logger.Error("update group failed", zap.Error(err))
		http.Error(w, `{"error":"failed to update group"}`, http.StatusInternalServerError)
		return
	}

	claims := middleware.GetUserFromContext(r.Context())
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       "update",
		ResourceType: "group",
		ResourceID:   id,
	})

	updated, err := h.groupRepo.Get(r.Context(), id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(groupToAdminResponse(group))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(groupToAdminResponse(updated))
}

// DeleteGroup handles DELETE /api/admin/groups/{id}
func (h *AdminGroupsHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.groupRepo.Delete(r.Context(), id); err != nil {
		http.Error(w, `{"error":"group not found"}`, http.StatusNotFound)
		return
	}

	claims := middleware.GetUserFromContext(r.Context())
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       "delete",
		ResourceType: "group",
		ResourceID:   id,
	})

	w.WriteHeader(http.StatusNoContent)
}

// UpdateGroupPermissions handles PUT /api/admin/groups/{id}/permissions
func (h *AdminGroupsHandler) UpdateGroupPermissions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req PermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if err := h.groupRepo.UpdatePermissions(r.Context(), id, req.AllowedUsers, req.AllowedGroups); err != nil {
		h.logger.Error("update group permissions failed", zap.Error(err))
		http.Error(w, `{"error":"failed to update permissions"}`, http.StatusInternalServerError)
		return
	}

	claims := middleware.GetUserFromContext(r.Context())
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       "update_permissions",
		ResourceType: "group",
		ResourceID:   id,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"allowed_users":  req.AllowedUsers,
		"allowed_groups": req.AllowedGroups,
	})
}

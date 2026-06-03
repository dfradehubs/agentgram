package handlers

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/metrics"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/service"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"go.uber.org/zap"
)

// GroupsHandler handles user-facing group endpoints
type GroupsHandler struct {
	groupRepo    repository.GroupRepository
	sessionStore store.SessionStore
	userService  *service.UserService
	registry     *agents.Registry
	logger       *zap.Logger
}

// NewGroupsHandler creates a new groups handler
func NewGroupsHandler(groupRepo repository.GroupRepository, sessionStore store.SessionStore, userService *service.UserService, registry *agents.Registry, logger *zap.Logger) *GroupsHandler {
	return &GroupsHandler{
		groupRepo:    groupRepo,
		sessionStore: sessionStore,
		userService:  userService,
		registry:     registry,
		logger:       logger,
	}
}

// UserGroupResponse is the response for user-facing group views
type UserGroupResponse struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	AgentIDs      []string `json:"agentIds"`
	AllowedUsers  []string `json:"allowed_users,omitempty"`
	AllowedGroups []string `json:"allowed_groups,omitempty"`
	CreatedAt     string   `json:"created_at"`
}

func groupToUserResponse(g *models.AgentGroup) UserGroupResponse {
	return UserGroupResponse{
		ID:            g.ID,
		Name:          g.Name,
		AgentIDs:      g.AgentIDs,
		AllowedUsers:  g.AllowedUsers,
		AllowedGroups: g.AllowedGroups,
		CreatedAt:     g.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// CreateGroupRequest is the request body for user group creation
type CreateGroupRequest struct {
	Name          string   `json:"name"`
	AgentIDs      []string `json:"agentIds"`
	AllowedUsers  []string `json:"allowed_users,omitempty"`
	AllowedGroups []string `json:"allowed_groups,omitempty"`
}

// UpdateGroupRequest is the request body for user group update
type UpdateGroupRequest struct {
	Name          string   `json:"name,omitempty"`
	AgentIDs      []string `json:"agentIds,omitempty"`
	AllowedUsers  []string `json:"allowed_users,omitempty"`
	AllowedGroups []string `json:"allowed_groups,omitempty"`
}

// ListGroups handles GET /api/groups
func (h *GroupsHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	email := claims.GetEmail()
	userGroups := claims.GetGroups()

	// Only show groups where the user is an actual participant (no admin bypass)
	var groups []*models.AgentGroup
	var err error
	groups, err = h.groupRepo.ListAccessible(r.Context(), email, userGroups)

	if err != nil {
		h.logger.Error("list groups failed", zap.Error(err))
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	responses := make([]UserGroupResponse, 0, len(groups))
	for _, g := range groups {
		responses = append(responses, groupToUserResponse(g))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"groups": responses})
}

// CreateGroup handles POST /api/groups
func (h *GroupsHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Name == "" || len(req.AgentIDs) < 2 {
		http.Error(w, `{"error":"name and at least 2 agentIds are required"}`, http.StatusBadRequest)
		return
	}

	// Verify the user has access to every requested agent
	email := claims.GetEmail()
	userGroups := claims.GetGroups()
	for _, agentID := range req.AgentIDs {
		agent, err := h.registry.Get(agentID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"agent not found: %s"}`, agentID), http.StatusBadRequest)
			return
		}
		if !agents.HasAccess(agent, email, userGroups) {
			http.Error(w, fmt.Sprintf(`{"error":"access denied for agent: %s"}`, agentID), http.StatusForbidden)
			return
		}
	}

	id := generateGroupID()

	group := &models.AgentGroup{
		ID:        id,
		Name:      req.Name,
		AgentIDs:  req.AgentIDs,
		CreatedBy: claims.GetEmail(),
	}

	// Merge creator + additional allowed users
	allowedUsers := []string{claims.GetEmail()}
	for _, u := range req.AllowedUsers {
		u = strings.TrimSpace(u)
		if u != "" && !strings.EqualFold(u, claims.GetEmail()) {
			allowedUsers = append(allowedUsers, u)
		}
	}

	if err := h.groupRepo.Create(r.Context(), group, allowedUsers, req.AllowedGroups); err != nil {
		h.logger.Error("create group failed", zap.Error(err))
		http.Error(w, `{"error":"failed to create group"}`, http.StatusInternalServerError)
		return
	}

	if metrics.IsEnabled() {
		metrics.GroupsCreatedTotal.Inc()
	}

	// Re-fetch to get timestamps
	created, err := h.groupRepo.Get(r.Context(), id)
	if err != nil {
		created = group
		created.CreatedAt = time.Now()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(groupToUserResponse(created))
}

// UpdateGroup handles PUT /api/groups/{groupId} (user-facing edit)
func (h *GroupsHandler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupId")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Only the group owner (or admin) can update a group
	if !IsGroupOwner(r.Context(), claims, groupID, h.groupRepo, h.userService) {
		http.Error(w, `{"error":"only the group owner can update this group"}`, http.StatusForbidden)
		return
	}

	var req UpdateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Fetch current group
	existing, err := h.groupRepo.Get(r.Context(), groupID)
	if err != nil {
		http.Error(w, `{"error":"group not found"}`, http.StatusNotFound)
		return
	}

	// Verify the user has access to any new agents being added
	if len(req.AgentIDs) >= 2 {
		email := claims.GetEmail()
		userGroups := claims.GetGroups()
		for _, agentID := range req.AgentIDs {
			agent, err := h.registry.Get(agentID)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"agent not found: %s"}`, agentID), http.StatusBadRequest)
				return
			}
			if !agents.HasAccess(agent, email, userGroups) {
				http.Error(w, fmt.Sprintf(`{"error":"access denied for agent: %s"}`, agentID), http.StatusForbidden)
				return
			}
		}
	}

	// Apply updates
	if req.Name != "" {
		existing.Name = req.Name
	}
	if len(req.AgentIDs) >= 2 {
		existing.AgentIDs = req.AgentIDs
	}

	if err := h.groupRepo.Update(r.Context(), existing); err != nil {
		h.logger.Error("update group failed", zap.Error(err))
		http.Error(w, `{"error":"failed to update group"}`, http.StatusInternalServerError)
		return
	}

	// Update permissions if provided
	if req.AllowedUsers != nil || req.AllowedGroups != nil {
		users := req.AllowedUsers
		if users == nil {
			users = existing.AllowedUsers
		}
		// Ensure creator stays in the list
		creatorIncluded := false
		for _, u := range users {
			if strings.EqualFold(u, existing.CreatedBy) {
				creatorIncluded = true
				break
			}
		}
		if !creatorIncluded && existing.CreatedBy != "" {
			users = append(users, existing.CreatedBy)
		}
		groups := req.AllowedGroups
		if groups == nil {
			groups = existing.AllowedGroups
		}
		if err := h.groupRepo.UpdatePermissions(r.Context(), groupID, users, groups); err != nil {
			h.logger.Error("update group permissions failed", zap.Error(err))
			http.Error(w, `{"error":"failed to update group permissions"}`, http.StatusInternalServerError)
			return
		}
	}

	updated, err := h.groupRepo.Get(r.Context(), groupID)
	if err != nil {
		updated = existing
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(groupToUserResponse(updated))
}

// DeleteGroup handles DELETE /api/groups/{groupId} (user-facing)
func (h *GroupsHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupId")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Only the group owner (or admin) can delete a group
	if !IsGroupOwner(r.Context(), claims, groupID, h.groupRepo, h.userService) {
		http.Error(w, `{"error":"only the group owner can delete this group"}`, http.StatusForbidden)
		return
	}

	if err := h.groupRepo.Delete(r.Context(), groupID); err != nil {
		http.Error(w, `{"error":"group not found"}`, http.StatusNotFound)
		return
	}

	if metrics.IsEnabled() {
		metrics.GroupsDeletedTotal.Inc()
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListGroupSessions handles GET /api/groups/{groupId}/sessions
func (h *GroupsHandler) ListGroupSessions(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupId")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Admins can view group metadata but not sessions — must be actual participant
	if !CanParticipateInGroup(r.Context(), claims, groupID, h.groupRepo) {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}

	sessionIDs, err := h.groupRepo.ListSessions(r.Context(), groupID)
	if err != nil {
		h.logger.Error("list group sessions failed", zap.Error(err))
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Fetch session details from Redis
	email := claims.GetEmail()
	sessions := make([]models.Session, 0, len(sessionIDs))
	for _, sid := range sessionIDs {
		s, err := h.sessionStore.GetSession(r.Context(), sid)
		if err != nil || s == nil {
			continue
		}
		// For Slack sessions: only show sessions the user participated in
		if s.Source == "slack" && s.UserID != email {
			// Check if this user is registered as participant via user_sessions
			if !h.sessionStore.IsParticipant(r.Context(), sid, email) {
				continue
			}
		}
		// Exclude messages for list view
		s.Messages = nil
		sessions = append(sessions, *s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.SessionListResponse{Sessions: sessions})
}

// AddGroupSession handles POST /api/groups/{groupId}/sessions
func (h *GroupsHandler) AddGroupSession(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupId")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	if !CanAccessGroup(r.Context(), claims, groupID, h.groupRepo, h.userService) {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionID == "" {
		http.Error(w, `{"error":"session_id required"}`, http.StatusBadRequest)
		return
	}

	// Verify the session belongs to the authenticated user
	session, err := h.sessionStore.GetSession(r.Context(), req.SessionID)
	if err != nil || session == nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}
	if session.UserID != claims.GetEmail() {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}

	if err := h.groupRepo.AddSession(r.Context(), groupID, req.SessionID); err != nil {
		h.logger.Error("add group session failed", zap.Error(err))
		http.Error(w, `{"error":"failed to add session"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
}

// RemoveGroupSession handles DELETE /api/groups/{groupId}/sessions/{sessionId}
func (h *GroupsHandler) RemoveGroupSession(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupId")
	sessionID := chi.URLParam(r, "sessionId")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	if !CanAccessGroup(r.Context(), claims, groupID, h.groupRepo, h.userService) {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}

	if err := h.groupRepo.RemoveSession(r.Context(), groupID, sessionID); err != nil {
		h.logger.Error("remove group session failed", zap.Error(err))
		http.Error(w, `{"error":"failed to remove session"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}


func generateGroupID() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return fmt.Sprintf("group-%d-%s", time.Now().UnixMilli(), string(b))
}

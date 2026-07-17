package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"go.uber.org/zap"
)

// GroupsHandler serves the user-facing, read-only view of agent groups.
// Group creation/editing/deletion is admin-only (see AdminGroupsHandler);
// group sessions are personal — each user only sees their own.
type GroupsHandler struct {
	groupRepo    repository.GroupRepository
	sessionStore store.SessionStore
	logger       *zap.Logger
}

// NewGroupsHandler creates a new groups handler
func NewGroupsHandler(groupRepo repository.GroupRepository, sessionStore store.SessionStore, logger *zap.Logger) *GroupsHandler {
	return &GroupsHandler{
		groupRepo:    groupRepo,
		sessionStore: sessionStore,
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

// ListGroups handles GET /api/groups — the groups this user may chat with.
func (h *GroupsHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Only show groups where the user is an actual participant (no admin bypass)
	groups, err := h.groupRepo.ListAccessible(r.Context(), claims.GetEmail(), claims.GetGroups())
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

// ListGroupSessions handles GET /api/groups/{groupId}/sessions.
// Group sessions are personal: a user only ever sees the sessions they own.
func (h *GroupsHandler) ListGroupSessions(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupId")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

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

	email := claims.GetEmail()
	sessions := make([]models.Session, 0, len(sessionIDs))
	for _, sid := range sessionIDs {
		s, err := h.sessionStore.GetSession(r.Context(), sid)
		if err != nil || s == nil {
			continue
		}
		// Personal sessions: only the owner's own sessions are listed.
		if s.UserID != email {
			continue
		}
		s.Messages = nil // exclude messages for the list view
		sessions = append(sessions, *s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.SessionListResponse{Sessions: sessions})
}

package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/audit"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/service"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"go.uber.org/zap"
)

// SessionsHandler handles session requests (backed by Redis)
type SessionsHandler struct {
	registry    *agents.Registry
	store       store.SessionStore
	groupRepo   repository.GroupRepository
	userService *service.UserService
	audit       *audit.Logger
	logger      *zap.Logger
}

// NewSessionsHandler creates a new sessions handler
func NewSessionsHandler(registry *agents.Registry, sessionStore store.SessionStore, groupRepo repository.GroupRepository, userService *service.UserService, auditLogger *audit.Logger, logger *zap.Logger) *SessionsHandler {
	return &SessionsHandler{
		registry:    registry,
		store:       sessionStore,
		groupRepo:   groupRepo,
		userService: userService,
		audit:       auditLogger,
		logger:      logger,
	}
}

// ListSessions handles GET /api/agents/{agentId}/sessions
// @Summary List sessions for an agent
// @Description Returns all sessions for the authenticated user and the specified agent
// @Tags sessions
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param agentId path string true "Agent ID"
// @Success 200 {object} models.SessionListResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/agents/{agentId}/sessions [get]
func (h *SessionsHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	if agentID == "" {
		writeJSONError(w, "agent id required", http.StatusBadRequest)
		return
	}

	// Get user claims
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify agent exists
	_, err := h.registry.Get(agentID)
	if err != nil {
		writeJSONError(w, "agent not found", http.StatusNotFound)
		return
	}

	sessions, err := h.store.ListSessions(r.Context(), claims.GetEmail(), agentID)
	if err != nil {
		h.logger.Error("failed to list sessions",
			zap.String("agent_id", agentID),
			zap.Error(err))
		writeJSONError(w, "failed to list sessions", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.SessionListResponse{Sessions: sessions})
}

// GetSession handles GET /api/agents/{agentId}/sessions/{sessionId}
// Supports pagination via query params:
//   - limit: number of messages to return (default: 0 = all, for backward compatibility)
//   - before: cursor (message index) to load messages before (for loading older messages)
//
// @Summary Get session with messages
// @Description Returns a session and its messages. Supports cursor-based pagination. When limit=0 (default), returns all messages in legacy format for backward compatibility.
// @Tags sessions
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param agentId path string true "Agent ID"
// @Param sessionId path string true "Session ID (UUID)"
// @Param limit query int false "Number of messages to return (0 = all)" default(0)
// @Param before query int false "Cursor: message index to load messages before"
// @Success 200 {object} models.SessionGetResponse "Paginated response (when limit > 0)"
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/agents/{agentId}/sessions/{sessionId} [get]
func (h *SessionsHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	sessionID := chi.URLParam(r, "sessionId")

	if agentID == "" || sessionID == "" {
		writeJSONError(w, "agent id and session id required", http.StatusBadRequest)
		return
	}

	// Get user claims
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse pagination params
	limit := 0 // 0 = all messages (backward compatible)
	before := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("before"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			before = n
		}
	}

	resp, err := h.store.GetSessionPaginated(r.Context(), sessionID, limit, before)
	if err != nil {
		h.logger.Error("failed to get session",
			zap.String("session_id", sessionID),
			zap.Error(err))
		writeJSONError(w, "failed to get session", http.StatusInternalServerError)
		return
	}

	if resp == nil {
		writeJSONError(w, "session not found", http.StatusNotFound)
		return
	}

	// Verify ownership, group membership, or Slack participation
	if resp.Session.UserID != claims.GetEmail() {
		allowed := false
		if resp.Session.GroupID != "" {
			allowed = CanParticipateInGroup(r.Context(), claims, resp.Session.GroupID, h.groupRepo)
		}
		if !allowed && resp.Session.Source == "slack" {
			allowed = h.store.IsParticipant(r.Context(), sessionID, claims.GetEmail())
		}
		if !allowed {
			writeJSONError(w, "access denied", http.StatusForbidden)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")

	// When no pagination is requested (limit=0), return the legacy format
	// to maintain backward compatibility with existing clients
	if limit == 0 {
		legacy := resp.Session
		legacy.Messages = resp.Messages
		json.NewEncoder(w).Encode(legacy)
		return
	}

	json.NewEncoder(w).Encode(resp)
}

// RenameSession handles PATCH /api/agents/{agentId}/sessions/{sessionId}
// @Summary Rename a session
// @Description Updates the display name of a session. Only the session owner can rename it.
// @Tags sessions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param agentId path string true "Agent ID"
// @Param sessionId path string true "Session ID (UUID)"
// @Param request body models.SessionRenameRequest true "New session name"
// @Success 200 {object} models.Session
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/agents/{agentId}/sessions/{sessionId} [patch]
func (h *SessionsHandler) RenameSession(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	sessionID := chi.URLParam(r, "sessionId")

	if agentID == "" || sessionID == "" {
		writeJSONError(w, "agent id and session id required", http.StatusBadRequest)
		return
	}

	// Get user claims
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse body
	var req models.SessionRenameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionName == "" {
		writeJSONError(w, "session_name required", http.StatusBadRequest)
		return
	}

	// Verify ownership first
	existing, err := h.store.GetSession(r.Context(), sessionID)
	if err != nil {
		writeJSONError(w, "failed to get session", http.StatusInternalServerError)
		return
	}
	if existing == nil {
		writeJSONError(w, "session not found", http.StatusNotFound)
		return
	}
	if existing.UserID != claims.GetEmail() {
		writeJSONError(w, "access denied", http.StatusForbidden)
		return
	}

	session, err := h.store.RenameSession(r.Context(), sessionID, req.SessionName)
	if err != nil {
		h.logger.Error("failed to rename session",
			zap.String("session_id", sessionID),
			zap.Error(err))
		writeJSONError(w, "failed to rename session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessionMetadataResponse(session))
}

// DeleteSession handles DELETE /api/agents/{agentId}/sessions/{sessionId}
// @Summary Delete a session
// @Description Permanently deletes a session and all its messages. Only the session owner can delete it.
// @Tags sessions
// @Security BearerAuth
// @Security CookieAuth
// @Param agentId path string true "Agent ID"
// @Param sessionId path string true "Session ID (UUID)"
// @Success 204 "Session deleted"
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/agents/{agentId}/sessions/{sessionId} [delete]
func (h *SessionsHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	sessionID := chi.URLParam(r, "sessionId")

	if agentID == "" || sessionID == "" {
		writeJSONError(w, "agent id and session id required", http.StatusBadRequest)
		return
	}

	// Get user claims
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify ownership first
	existing, err := h.store.GetSession(r.Context(), sessionID)
	if err != nil {
		writeJSONError(w, "failed to get session", http.StatusInternalServerError)
		return
	}
	if existing == nil {
		writeJSONError(w, "session not found", http.StatusNotFound)
		return
	}
	if existing.UserID != claims.GetEmail() {
		writeJSONError(w, "access denied", http.StatusForbidden)
		return
	}

	if err := h.store.DeleteSession(r.Context(), sessionID, claims.GetEmail(), agentID); err != nil {
		h.logger.Error("failed to delete session",
			zap.String("session_id", sessionID),
			zap.Error(err))
		writeJSONError(w, "failed to delete session", http.StatusInternalServerError)
		return
	}

	h.audit.Log(claims.GetEmail(), audit.ActionSessionDelete,
		zap.String("agent_id", agentID),
		zap.String("session_id", sessionID))

	w.WriteHeader(http.StatusNoContent)
}

// PatchCharts handles POST /api/agents/{agentId}/sessions/{sessionId}/charts
// @Summary Append charts to the last assistant message
// @Description Persists extracted chart data as content_parts on the last assistant message in the session
// @Tags sessions
// @Accept json
// @Security BearerAuth
// @Security CookieAuth
// @Param agentId path string true "Agent ID"
// @Param sessionId path string true "Session ID (UUID)"
// @Param request body object true "Charts payload" SchemaExample({"charts":[{"chartType":"bar","labels":["A","B"],"datasets":[{"label":"v","data":[1,2]}]}]})
// @Success 204 "Charts appended"
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/agents/{agentId}/sessions/{sessionId}/charts [post]
func (h *SessionsHandler) PatchCharts(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	sessionID := chi.URLParam(r, "sessionId")

	if agentID == "" || sessionID == "" {
		writeJSONError(w, "agent id and session id required", http.StatusBadRequest)
		return
	}

	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse body
	var req struct {
		Charts          []map[string]interface{} `json:"charts"`
		AssistantOffset int                      `json:"assistant_offset"` // 0 = last assistant, 1 = second-to-last, etc.
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Charts) == 0 {
		writeJSONError(w, "charts must not be empty", http.StatusBadRequest)
		return
	}

	// Verify session exists and check ownership
	existing, err := h.store.GetSession(r.Context(), sessionID)
	if err != nil {
		writeJSONError(w, "failed to get session", http.StatusInternalServerError)
		return
	}
	if existing == nil {
		writeJSONError(w, "session not found", http.StatusNotFound)
		return
	}
	if existing.UserID != claims.GetEmail() {
		allowed := false
		if existing.GroupID != "" {
			allowed = CanParticipateInGroup(r.Context(), claims, existing.GroupID, h.groupRepo)
		}
		if !allowed && existing.Source == "slack" {
			allowed = h.store.IsParticipant(r.Context(), sessionID, claims.GetEmail())
		}
		if !allowed {
			writeJSONError(w, "access denied", http.StatusForbidden)
			return
		}
	}

	if err := h.store.AppendChartsToAssistant(r.Context(), sessionID, req.AssistantOffset, req.Charts); err != nil {
		h.logger.Error("failed to append charts",
			zap.String("session_id", sessionID),
			zap.Error(err))
		writeJSONError(w, "failed to append charts", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// sessionMetadataResponse returns only session metadata without messages.
// Used in rename responses to avoid leaking conversation history.
func sessionMetadataResponse(s *models.Session) map[string]interface{} {
	return map[string]interface{}{
		"session_id":    s.SessionID,
		"session_name":  s.SessionName,
		"last_activity": s.LastActivity,
	}
}

// writeJSONError writes a JSON error response
func writeJSONError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(models.ErrorResponse{Error: message})
}

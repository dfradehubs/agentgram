package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/audit"
	"github.com/dfradehubs/agentgram-api/internal/metrics"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"go.uber.org/zap"
)

const (
	defaultShareHours = 168 // 7 days
	maxShareHours     = 168
)

// ShareHandler handles session sharing endpoints
type ShareHandler struct {
	registry  *agents.Registry
	store     store.SessionStore
	shareRepo repository.SharedSessionRepository
	groupRepo repository.GroupRepository
	audit     *audit.Logger
	logger    *zap.Logger
}

// NewShareHandler creates a new share handler
func NewShareHandler(
	registry *agents.Registry,
	sessionStore store.SessionStore,
	shareRepo repository.SharedSessionRepository,
	groupRepo repository.GroupRepository,
	auditLogger *audit.Logger,
	logger *zap.Logger,
) *ShareHandler {
	return &ShareHandler{
		registry:  registry,
		store:     sessionStore,
		shareRepo: shareRepo,
		groupRepo: groupRepo,
		audit:     auditLogger,
		logger:    logger,
	}
}

// CreateShare handles POST /api/agents/{agentId}/sessions/{sessionId}/share
func (h *ShareHandler) CreateShare(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	sessionID := chi.URLParam(r, "sessionId")

	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify session exists and belongs to the user
	session, err := h.store.GetSession(r.Context(), sessionID)
	if err != nil {
		h.logger.Error("failed to get session", zap.Error(err))
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if session == nil {
		writeJSONError(w, "session not found", http.StatusNotFound)
		return
	}
	if session.UserID != claims.GetEmail() {
		writeJSONError(w, "access denied", http.StatusForbidden)
		return
	}

	// Check if there's already an active share for this session
	existing, err := h.shareRepo.GetBySessionID(r.Context(), sessionID)
	if err != nil {
		h.logger.Error("failed to check existing share", zap.Error(err))
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if existing != nil {
		shareURL := fmt.Sprintf("/shared/%s", existing.Token)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models.ShareResponse{
			Token:     existing.Token,
			URL:       shareURL,
			ExpiresAt: existing.ExpiresAt,
		})
		return
	}

	// Parse request body
	var req models.ShareRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	hours := req.ExpiresInHours
	if hours <= 0 {
		hours = defaultShareHours
	}
	if hours > maxShareHours {
		hours = maxShareHours
	}

	expiresAt := time.Now().Add(time.Duration(hours) * time.Hour)

	shared, err := h.shareRepo.Create(r.Context(), sessionID, agentID, claims.GetEmail(), expiresAt)
	if err != nil {
		h.logger.Error("failed to create share", zap.Error(err))
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}

	if metrics.IsEnabled() {
		metrics.SharesCreatedTotal.WithLabelValues(agentID).Inc()
	}

	h.audit.Log(claims.GetEmail(), audit.ActionShareSession,
		zap.String("agent_id", agentID),
		zap.String("session_id", sessionID),
		zap.String("token", shared.Token))

	shareURL := fmt.Sprintf("/shared/%s", shared.Token)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.ShareResponse{
		Token:     shared.Token,
		URL:       shareURL,
		ExpiresAt: shared.ExpiresAt,
	})
}

// RevokeShare handles DELETE /api/agents/{agentId}/sessions/{sessionId}/share
func (h *ShareHandler) RevokeShare(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")

	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify session belongs to the user
	session, err := h.store.GetSession(r.Context(), sessionID)
	if err != nil {
		h.logger.Error("failed to get session", zap.Error(err))
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if session == nil {
		writeJSONError(w, "session not found", http.StatusNotFound)
		return
	}
	if session.UserID != claims.GetEmail() {
		writeJSONError(w, "access denied", http.StatusForbidden)
		return
	}

	if err := h.shareRepo.Revoke(r.Context(), sessionID, claims.GetEmail()); err != nil {
		writeJSONError(w, "no active share found", http.StatusNotFound)
		return
	}

	h.audit.Log(claims.GetEmail(), audit.ActionRevokeShare,
		zap.String("session_id", sessionID))

	w.WriteHeader(http.StatusNoContent)
}

// GetSharedSession handles GET /api/shared/{token}
func (h *ShareHandler) GetSharedSession(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	shared, err := h.shareRepo.GetByToken(r.Context(), token)
	if err != nil {
		h.logger.Error("failed to get shared session", zap.Error(err))
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if shared == nil {
		writeJSONError(w, "shared session not found or expired", http.StatusNotFound)
		return
	}

	// Verify user has access to the agent
	agent, err := h.registry.Get(shared.AgentID)
	if err != nil {
		writeJSONError(w, "agent not found", http.StatusNotFound)
		return
	}

	inheritedMap, _ := h.groupRepo.GetAllInheritedPermissions(r.Context())
	inherited := inheritedMap[shared.AgentID]
	if !agents.HasAccessWithInherited(agent, claims.GetEmail(), claims.GetGroups(), inherited) {
		writeJSONError(w, "access denied", http.StatusForbidden)
		return
	}

	// Get session for metadata
	session, err := h.store.GetSession(r.Context(), shared.SessionID)
	if err != nil {
		h.logger.Error("failed to get session", zap.Error(err))
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if session == nil {
		writeJSONError(w, "session expired", http.StatusGone)
		return
	}

	if metrics.IsEnabled() {
		metrics.SharesAccessedTotal.WithLabelValues(shared.AgentID).Inc()
	}

	info := models.SharedSessionInfo{
		Token:        shared.Token,
		AgentID:      shared.AgentID,
		AgentName:    agent.Name,
		SessionName:  session.SessionName,
		SharedBy:     shared.SharedBy,
		MessageCount: session.MessageCount,
		ExpiresAt:    shared.ExpiresAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// CloneSharedSession handles POST /api/shared/{token}/clone
func (h *ShareHandler) CloneSharedSession(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	shared, err := h.shareRepo.GetByToken(r.Context(), token)
	if err != nil {
		h.logger.Error("failed to get shared session", zap.Error(err))
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if shared == nil {
		writeJSONError(w, "shared session not found or expired", http.StatusNotFound)
		return
	}

	// Verify user has access to the agent
	agent, err := h.registry.Get(shared.AgentID)
	if err != nil {
		writeJSONError(w, "agent not found", http.StatusNotFound)
		return
	}

	inheritedMap, _ := h.groupRepo.GetAllInheritedPermissions(r.Context())
	inherited := inheritedMap[shared.AgentID]
	if !agents.HasAccessWithInherited(agent, claims.GetEmail(), claims.GetGroups(), inherited) {
		writeJSONError(w, "access denied", http.StatusForbidden)
		return
	}

	// Get source session to verify it still exists
	sourceSession, err := h.store.GetSession(r.Context(), shared.SessionID)
	if err != nil {
		h.logger.Error("failed to get source session", zap.Error(err))
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if sourceSession == nil {
		writeJSONError(w, "session expired", http.StatusGone)
		return
	}

	cloneName := "[Compartido] " + sourceSession.SessionName
	cloned, err := h.store.CloneSession(r.Context(), shared.SessionID, claims.GetEmail(), shared.AgentID, cloneName)
	if err != nil {
		h.logger.Error("failed to clone session", zap.Error(err))
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}

	if metrics.IsEnabled() {
		metrics.SessionsClonesTotal.WithLabelValues(shared.AgentID).Inc()
	}

	h.audit.Log(claims.GetEmail(), audit.ActionCloneShare,
		zap.String("session_id", cloned.SessionID),
		zap.String("source_session_id", shared.SessionID),
		zap.String("agent_id", shared.AgentID),
		zap.String("share_token", shared.Token))

	// Return cloned session without messages (like list view)
	cloned.Messages = nil
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"session": cloned,
	})
}

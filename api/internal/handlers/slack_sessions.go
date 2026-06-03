package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"go.uber.org/zap"
)

// SlackSessionsHandler handles Slack session listing.
type SlackSessionsHandler struct {
	store  store.SessionStore
	logger *zap.Logger
}

// NewSlackSessionsHandler creates a new handler.
func NewSlackSessionsHandler(store store.SessionStore, logger *zap.Logger) *SlackSessionsHandler {
	return &SlackSessionsHandler{store: store, logger: logger}
}

// ListSlackSessions returns Slack sessions where the authenticated user is a participant.
// GET /api/slack/sessions
func (h *SlackSessionsHandler) ListSlackSessions(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	email := claims.GetEmail()
	sessionIDs, err := h.store.ListSlackSessions(r.Context(), email)
	if err != nil {
		h.logger.Error("failed to list slack sessions", zap.Error(err))
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	sessions := make([]models.Session, 0, len(sessionIDs))
	for _, sid := range sessionIDs {
		s, err := h.store.GetSession(r.Context(), sid)
		if err != nil || s == nil {
			continue
		}
		s.Messages = nil // Exclude messages for list view
		sessions = append(sessions, *s)
	}

	// Sort by last activity (newest first)
	for i := 0; i < len(sessions); i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[j].LastActivity > sessions[i].LastActivity {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.SessionListResponse{Sessions: sessions})
}

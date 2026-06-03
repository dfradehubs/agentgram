package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/pubsub"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/service"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"go.uber.org/zap"
)

// SubscribeHandler handles SSE subscriptions for real-time collaborative sessions
type SubscribeHandler struct {
	hub         *pubsub.Hub
	store       store.SessionStore
	groupRepo   repository.GroupRepository
	userService *service.UserService
	logger      *zap.Logger
}

// NewSubscribeHandler creates a new subscribe handler
func NewSubscribeHandler(hub *pubsub.Hub, sessionStore store.SessionStore, groupRepo repository.GroupRepository, userService *service.UserService, logger *zap.Logger) *SubscribeHandler {
	return &SubscribeHandler{
		hub:         hub,
		store:       sessionStore,
		groupRepo:   groupRepo,
		userService: userService,
		logger:      logger,
	}
}

// Subscribe handles GET /api/sessions/{sessionId}/subscribe
// It opens an SSE connection and forwards events from Redis Pub/Sub.
func (h *SubscribeHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	if sessionID == "" {
		http.Error(w, `{"error":"session id required"}`, http.StatusBadRequest)
		return
	}

	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Get session and verify it's a group session
	session, err := h.store.GetSession(r.Context(), sessionID)
	if err != nil || session == nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}

	if session.GroupID == "" {
		http.Error(w, `{"error":"not a group session"}`, http.StatusBadRequest)
		return
	}

	// Verify user has access to the group
	if !CanAccessGroup(r.Context(), claims, session.GroupID, h.groupRepo, h.userService) {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}

	// Set SSE headers
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Subscribe to Pub/Sub channel
	ch, cancel, err := h.hub.Subscribe(r.Context(), sessionID)
	if err != nil {
		h.logger.Error("failed to subscribe to session events",
			zap.String("session_id", sessionID),
			zap.Error(err))
		http.Error(w, `{"error":"failed to subscribe"}`, http.StatusInternalServerError)
		return
	}
	defer cancel()

	h.logger.Debug("client subscribed to session events",
		zap.String("session_id", sessionID),
		zap.String("user", claims.GetEmail()))

	// Keep-alive ticker
	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			h.logger.Debug("subscriber disconnected",
				zap.String("session_id", sessionID),
				zap.String("user", claims.GetEmail()))
			return

		case data, ok := <-ch:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()

		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}


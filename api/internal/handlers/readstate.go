package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/pubsub"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"go.uber.org/zap"
)

// ReadStateHandler handles read/unread state for sessions
type ReadStateHandler struct {
	store  store.SessionStore
	hub    *pubsub.Hub
	logger *zap.Logger
}

// NewReadStateHandler creates a new read state handler
func NewReadStateHandler(sessionStore store.SessionStore, hub *pubsub.Hub, logger *zap.Logger) *ReadStateHandler {
	return &ReadStateHandler{
		store:  sessionStore,
		hub:    hub,
		logger: logger,
	}
}

// GetReadState handles GET /api/read-state
func (h *ReadStateHandler) GetReadState(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	state, err := h.store.GetReadState(r.Context(), claims.GetEmail())
	if err != nil {
		h.logger.Error("failed to get read state", zap.Error(err))
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

// markReadRequest is the body for PUT /api/read-state/{sessionId}
type markReadRequest struct {
	Count int `json:"count"`
}

// MarkRead handles PUT /api/read-state/{sessionId}
func (h *ReadStateHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID := chi.URLParam(r, "sessionId")
	if sessionID == "" {
		writeJSONError(w, "session id required", http.StatusBadRequest)
		return
	}

	var req markReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.store.SetReadCount(r.Context(), claims.GetEmail(), sessionID, req.Count); err != nil {
		h.logger.Error("failed to set read count", zap.Error(err))
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Publish read state change to other connected clients
	if h.hub != nil {
		if err := h.hub.PublishReadState(r.Context(), claims.GetEmail(), sessionID, req.Count); err != nil {
			h.logger.Warn("failed to publish read state update",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// BatchUpdate handles PUT /api/read-state (batch update for migration)
func (h *ReadStateHandler) BatchUpdate(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var state map[string]int
	if err := json.NewDecoder(r.Body).Decode(&state); err != nil {
		writeJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.store.SetReadStateBatch(r.Context(), claims.GetEmail(), state); err != nil {
		h.logger.Error("failed to batch update read state", zap.Error(err))
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Publish each read state change to other connected clients
	if h.hub != nil {
		for sessionID, count := range state {
			if err := h.hub.PublishReadState(r.Context(), claims.GetEmail(), sessionID, count); err != nil {
				h.logger.Warn("failed to publish read state update",
					zap.String("session_id", sessionID),
					zap.Error(err))
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// Subscribe handles GET /api/read-state/subscribe
// It opens an SSE connection and forwards read state changes from Redis Pub/Sub.
func (h *ReadStateHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	if h.hub == nil {
		http.Error(w, `{"error":"read state subscription not available"}`, http.StatusServiceUnavailable)
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

	// Subscribe to user's read state channel
	ch, cancel, err := h.hub.SubscribeReadState(r.Context(), claims.GetEmail())
	if err != nil {
		h.logger.Error("failed to subscribe to read state events",
			zap.String("user", claims.GetEmail()),
			zap.Error(err))
		http.Error(w, `{"error":"failed to subscribe"}`, http.StatusInternalServerError)
		return
	}
	defer cancel()

	h.logger.Debug("client subscribed to read state events",
		zap.String("user", claims.GetEmail()))

	// Keep-alive ticker
	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			h.logger.Debug("read state subscriber disconnected",
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

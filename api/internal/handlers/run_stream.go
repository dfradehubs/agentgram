package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/service"
	"github.com/dfradehubs/agentgram-api/internal/store"
)

// runStreamBlock is how long XREAD blocks waiting for new events before
// re-checking whether the run is still active.
const runStreamBlock = 15 * time.Second

// runStreamMaxDuration caps a reconnect SSE slightly above the agent run
// context timeout, so a stale active-run flag can never hang the connection.
const runStreamMaxDuration = 11 * time.Minute

// RunStreamHandler lets a client reconnect to an in-flight run after a reload.
// It replays the buffered AG-UI events for the session and then streams new
// ones live until the run finishes. Reuses the same AG-UI SSE format as the
// chat endpoint, so the frontend processes it with its normal stream pipeline.
type RunStreamHandler struct {
	rdb         *redis.Client
	store       store.SessionStore
	registry    *agents.Registry
	groupRepo   repository.GroupRepository
	userService *service.UserService
	logger      *zap.Logger
}

// NewRunStreamHandler creates a new run stream handler.
func NewRunStreamHandler(rdb *redis.Client, sessionStore store.SessionStore, registry *agents.Registry, groupRepo repository.GroupRepository, userService *service.UserService, logger *zap.Logger) *RunStreamHandler {
	return &RunStreamHandler{
		rdb:         rdb,
		store:       sessionStore,
		registry:    registry,
		groupRepo:   groupRepo,
		userService: userService,
		logger:      logger,
	}
}

// Stream handles GET /api/agents/{agentId}/sessions/{sessionId}/stream
func (h *RunStreamHandler) Stream(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	sessionID := chi.URLParam(r, "sessionId")
	if agentID == "" || sessionID == "" {
		http.Error(w, `{"error":"agent id and session id required"}`, http.StatusBadRequest)
		return
	}

	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Verify access to the agent (same rule as the chat endpoint).
	agent, err := h.registry.Get(agentID)
	if err != nil {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}
	isAdmin, _ := h.userService.IsAdmin(r.Context(), claims.GetEmail(), claims.GetGroups())
	if !isAdmin {
		inheritedMap, _ := h.groupRepo.GetAllInheritedPermissions(r.Context())
		if !agents.HasAccessWithInherited(agent, claims.GetEmail(), claims.GetGroups(), inheritedMap[agentID]) {
			http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
			return
		}
	}

	// Verify the session exists and the user may access it.
	session, err := h.store.GetSession(r.Context(), sessionID)
	if err != nil || session == nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}
	if session.UserID != claims.GetEmail() {
		allowed := false
		if session.GroupID != "" {
			allowed = CanAccessGroup(r.Context(), claims, session.GroupID, h.groupRepo, h.userService)
		}
		if !allowed && session.Source == "slack" {
			allowed = h.store.IsParticipant(r.Context(), sessionID, claims.GetEmail())
		}
		if !allowed {
			http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
			return
		}
	}

	// Nothing buffered and no active run: tell the client to fall back to
	// loading the persisted session. Must be decided before opening the SSE.
	streamKey := store.RunEventsKey(sessionID)
	length, _ := h.rdb.XLen(r.Context(), streamKey).Result()
	if length == 0 && !h.store.HasActiveRun(r.Context(), sessionID) {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	ctx, cancel := context.WithTimeout(r.Context(), runStreamMaxDuration)
	defer cancel()

	h.logger.Debug("client reconnected to run stream",
		zap.String("session_id", sessionID),
		zap.String("user", claims.GetEmail()))

	// Single XREAD loop starting at "0": Redis returns all buffered events
	// first (replay), then blocks for new ones (live). IDs are monotonic and
	// exclusive, so there is no gap or duplication.
	lastID := "0"
	for {
		res, err := h.rdb.XRead(ctx, &redis.XReadArgs{
			Streams: []string{streamKey, lastID},
			Block:   runStreamBlock,
			Count:   100,
		}).Result()
		if err == redis.Nil {
			// Block window expired with no new events. If the run is no longer
			// active, it ended without a terminal event (crash/timeout): stop.
			if !h.store.HasActiveRun(ctx, sessionID) {
				return
			}
			continue
		}
		if err != nil {
			return // context cancelled (client gone) or Redis error
		}

		for _, stream := range res {
			for _, msg := range stream.Messages {
				lastID = msg.ID
				raw, _ := msg.Values["e"].(string)
				if raw == "" {
					continue
				}
				if _, werr := fmt.Fprintf(w, "data: %s\n\n", raw); werr != nil {
					return // client disconnected
				}
				flusher.Flush()
				// Primary close criterion: a terminal event in the stream.
				if t := aguiEventType(raw); t == "RUN_FINISHED" || t == "RUN_ERROR" {
					return
				}
			}
		}
	}
}

// aguiEventType extracts the "type" field from a serialized AG-UI event.
func aguiEventType(raw string) string {
	var e struct {
		Type string `json:"type"`
	}
	if json.Unmarshal([]byte(raw), &e) != nil {
		return ""
	}
	return e.Type
}

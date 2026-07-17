package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/audit"
	"github.com/dfradehubs/agentgram-api/internal/auth"
	lf "github.com/dfradehubs/agentgram-api/internal/langfuse"
	"github.com/dfradehubs/agentgram-api/internal/metrics"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/orchestrator"
	"github.com/dfradehubs/agentgram-api/internal/proxy"
	appsettings "github.com/dfradehubs/agentgram-api/internal/settings"
	"go.uber.org/zap"
)

// GroupChat handles POST /api/groups/{groupId}/chat
// @Summary Chat with an agent group (moderated debate)
// @Description Sends a message to a group. An LLM moderator picks which agents respond,
// @Description in sequence, each seeing the previous agents' replies. The response is a single
// @Description SSE stream (one RUN_STARTED/RUN_FINISHED pair) with TEXT_MESSAGE_* events tagged
// @Description per agent via the agentId field.
// @Tags chat
// @Accept json
// @Produce text/event-stream
// @Security BearerAuth
// @Security CookieAuth
// @Param groupId path string true "Group ID"
// @Param request body models.ChatRequest true "Chat request with messages, optional session_id and optional agent_ids roster override"
// @Success 200 {string} string "SSE stream with AG-UI events"
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 503 {object} models.ErrorResponse
// @Router /api/groups/{groupId}/chat [post]
func (h *ProxyHandler) GroupChat(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "groupId")
	if groupID == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"group id required"}`, http.StatusBadRequest)
		return
	}

	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	if h.moderator == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"no moderator LLM configured (admin: add an LLM model with role 'moderator')"}`, http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	if !CanAccessGroup(ctx, claims, groupID, h.groupRepo, h.userService) {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"access denied to group"}`, http.StatusForbidden)
		return
	}

	group, err := h.groupRepo.Get(ctx, groupID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"group not found"}`, http.StatusNotFound)
		return
	}

	var chatReq models.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if errMsg := validateChatRequest(&chatReq); errMsg != "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error":%q}`, errMsg), http.StatusBadRequest)
		return
	}

	userEmail := claims.GetEmail()
	userGroups := claims.GetGroups()
	isAdmin, _ := h.userService.IsAdmin(ctx, userEmail, userGroups)

	// Build the debate roster: group agents the user can access, optionally
	// restricted by the agent_ids override (@mention semantics). Agents that
	// require a GitHub token are excluded when the caller has none, so the
	// moderator never picks an agent that would fail mid-debate.
	hasGitHubToken := middleware.GetGitHubTokenFromContext(r.Context()) != ""
	roster, agentsByID := h.buildRoster(ctx, group.AgentIDs, chatReq.AgentIDs, userEmail, userGroups, isAdmin, hasGitHubToken)
	if len(roster) == 0 {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"no accessible agents in this group"}`, http.StatusForbidden)
		return
	}

	rosterIDs := make([]string, 0, len(roster))
	for _, b := range roster {
		rosterIDs = append(rosterIDs, b.ID)
	}

	// Get or create the group session (multi-agent, group-linked).
	// ponytail: the debate works on an in-memory snapshot of session.Messages;
	// two members debating concurrently on the same session won't see each
	// other's turns until persisted. Upgrade path: per-session Redis lock or
	// re-read before each turn if this bites in practice.
	session, err := h.getOrCreateGroupSession(ctx, claims, groupID, rosterIDs, &chatReq)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		status := http.StatusForbidden
		switch err {
		case errSessionNotFound:
			status = http.StatusNotFound
		case errSessionStore:
			status = http.StatusInternalServerError
		}
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), status)
		return
	}

	// Save the user message. BroadcastAgentIDs marks the roster so the context
	// delta excludes it for agents that receive it directly as the new message.
	userMsg := chatReq.Messages[len(chatReq.Messages)-1]
	userMsg.UserName = claims.GetDisplayName()
	userMsg.UserEmail = userEmail
	userMsg.IsAdmin = isAdmin
	userMsg.BroadcastAgentIDs = rosterIDs
	if err := h.store.AddMessage(ctx, session.SessionID, userMsg); err != nil {
		h.logger.Error("failed to save user message", zap.Error(err))
	}
	session.Messages = append(session.Messages, userMsg)

	requestID := chiMiddleware.GetReqID(ctx)
	authHeader := middleware.GetAuthHeaderFromContext(ctx)
	locale := localeFromRequest(r)

	// Langfuse trace for the whole debate (one trace, one span per agent turn)
	var lfTrace *lf.Trace
	if h.langfuseTracer != nil && h.langfuseTracer.Enabled() {
		lfTrace = h.langfuseTracer.StartTrace(ctx, "group-chat", userEmail, session.SessionID, map[string]interface{}{
			"group_id":   groupID,
			"group_name": group.Name,
			"request_id": requestID,
		})
		lfTrace.SetInput(truncate(userMsg.Content, 1000))
		ctx = lf.ContextWithTrace(ctx, lfTrace)
	}

	// onEvent: buffer for reconnect + pub/sub for live multi-user (same as Chat)
	sessionID := session.SessionID
	onEvent := func(event interface{}) {
		data, err := json.Marshal(event)
		if err != nil {
			return
		}
		if err := h.store.AppendRunEvent(context.Background(), sessionID, data); err != nil {
			h.logger.Debug("failed to buffer run event", zap.String("session_id", sessionID), zap.Error(err))
		}
		if h.hub != nil {
			if err := h.hub.Publish(context.Background(), sessionID, event); err != nil {
				h.logger.Debug("failed to publish event to pub/sub", zap.String("session_id", sessionID), zap.Error(err))
			}
		}
	}

	// Outer SSE lifecycle: exactly one RUN_STARTED/RUN_FINISHED for the whole debate.
	sse, err := proxy.NewSSEWriter(w)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}
	sse.Apply(proxy.SSEConfig{
		ThreadID:    session.SessionID,
		SessionName: session.SessionName,
		OnEvent:     onEvent,
	})

	// Mark the run in-flight BEFORE the first buffered event: SetActiveRun
	// resets the run-event stream, so emitting RUN_STARTED first would wipe it
	// from the reconnect replay (same ordering as the single-agent handler).
	_ = h.store.SetActiveRun(ctx, session.SessionID, requestID)
	defer func() {
		clearCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.store.ClearActiveRun(clearCtx, session.SessionID, requestID)
	}()

	if err := sse.SendRunStarted(); err != nil {
		return
	}

	// Streaming TurnRunner: one agent turn = context prep → proxied SSE
	// (lifecycle suppressed, events tagged with agentId) → persistence.
	run := func(turnCtx context.Context, agentID string) (string, error) {
		agent := agentsByID[agentID]
		_ = sse.SendCustomEvent("moderator.select", map[string]interface{}{"agentId": agentID})

		agentSessionID, _ := h.store.GetAgentSessionID(turnCtx, session.SessionID, agentID)
		hasAgentSession := agentSessionID != ""

		prep := proxy.PrepareMessagesForMultiAgent(session, agentID, userMsg, hasAgentSession, true, agent.MaxContextTokens, agent.SummarizeThreshold, h.summarizer, turnCtx)
		messagesToSend := prep.Messages

		// Prefix [DisplayName] on the last user message so agents know who is asking
		if lastIdx := len(messagesToSend) - 1; lastIdx >= 0 && messagesToSend[lastIdx].Role == "user" && messagesToSend[lastIdx].UserName != "" {
			messagesToSend[lastIdx].Content = fmt.Sprintf("[%s]: %s", messagesToSend[lastIdx].UserName, messagesToSend[lastIdx].Content)
		}

		// Process file attachments for custom-protocol agents (A2A/ADK pass them natively)
		if agent.Protocol == "custom" && h.fileProcessor != nil && len(messagesToSend) > 0 {
			lastMsg := &messagesToSend[len(messagesToSend)-1]
			if len(lastMsg.Attachments) > 0 {
				if err := h.fileProcessor.ProcessAttachments(turnCtx, lastMsg); err != nil {
					h.logger.Error("failed to process attachments", zap.Error(err))
				}
			}
		}

		var agentSpan *lf.Span
		if lfTrace != nil && lfTrace.IsEnabled() {
			agentSpan = lfTrace.StartToolCall(fmt.Sprintf("proxy:%s", agentID), map[string]interface{}{
				"agent_name":     agent.Name,
				"agent_protocol": agent.Protocol,
			})
		}

		if metrics.IsEnabled() {
			metrics.ActiveStreams.WithLabelValues("agent", agentID).Inc()
		}
		turnStart := time.Now()

		result, err := h.proxy.Handle(turnCtx, w, agent, &models.ChatRequest{
			Messages:  messagesToSend,
			SessionID: agentSessionID,
		}, authHeader, proxy.HandleOptions{
			ThreadID:          session.SessionID,
			SessionName:       session.SessionName,
			Locale:            locale,
			RequestID:         requestID,
			UserEmail:         userEmail,
			UserGroups:        userGroups,
			OnEvent:           onEvent,
			AgentID:           agentID,
			SuppressLifecycle: true,
		})

		if metrics.IsEnabled() {
			metrics.ActiveStreams.WithLabelValues("agent", agentID).Dec()
		}
		if agentSpan != nil {
			if err != nil {
				agentSpan.EndWithError(err)
			} else if result != nil {
				agentSpan.End(truncate(result.AssistantText, 2000))
			} else {
				agentSpan.End(nil)
			}
		}

		// Observability + audit per turn (same shape as single-agent chat)
		h.recordGroupTurnEvent(agent, session.SessionID, userEmail, result, err, int(time.Since(turnStart).Milliseconds()), len(messagesToSend))
		h.audit.Log(userEmail, audit.ActionChat,
			zap.String("agent_id", agentID),
			zap.String("group_id", groupID),
			zap.String("session_id", session.SessionID),
			zap.Bool("is_multi_agent", true))

		// Persist and grow the in-memory session so the next turn's context
		// delta (and the moderator transcript) includes this reply.
		if msg := h.persistTurnResult(session, agent, result, agentSessionID, hasAgentSession, locale); msg != nil {
			session.Messages = append(session.Messages, *msg)
		}

		if err != nil {
			return "", err
		}
		if result != nil && result.Error != "" {
			return "", fmt.Errorf("%s", result.Error)
		}
		if result == nil {
			return "", nil
		}
		return result.AssistantText, nil
	}

	maxTurns := h.settings.Int(appsettings.KeyGroupMaxTurnsAPI)
	if group.MaxTurns > 0 {
		maxTurns = group.MaxTurns
	}
	transcript := orchestrator.RenderTranscript(session.Messages)
	results, debateErr := h.moderator.Debate(ctx, roster, transcript, run, maxTurns)

	if debateErr != nil && len(results) == 0 {
		// Moderator failed before anyone spoke: surface as a run error.
		h.logger.Error("group debate failed", zap.String("group_id", groupID), zap.Error(debateErr))
		_ = sse.SendRunError("moderator failed")
		if lfTrace != nil {
			lfTrace.End(false, debateErr.Error())
		}
		return
	}
	if debateErr != nil {
		// Some agent replied but the debate ended abnormally (moderator error or
		// deadline). Tell the user the answer may be incomplete instead of
		// finishing silently.
		h.logger.Warn("group debate ended early", zap.String("group_id", groupID), zap.Error(debateErr))
		note := "⚠️ The debate ended early (moderator error or timeout); the replies above may be incomplete."
		if debateErr == context.DeadlineExceeded {
			note = "⚠️ The debate hit the time limit before finishing; the replies above may be incomplete."
		}
		h.streamModeratorMessage(sse, "moderator", note)
		h.persistModeratorMessage(session.SessionID, "moderator", note)
	}

	if len(results) == 0 {
		// Moderator decided nobody applies: tell the user instead of silence.
		noAgentMsg := "No agent in this group can help with that request."
		if locale == "es" {
			noAgentMsg = "Ningún agente de este grupo puede ayudar con esa petición."
		}
		h.streamModeratorMessage(sse, "moderator", noAgentMsg)
		h.persistModeratorMessage(session.SessionID, "moderator", noAgentMsg)
	} else if speakers := distinctSuccessfulSpeakers(results); speakers >= 2 {
		// Optional synthesis when several agents contributed
		synthesis, err := h.moderator.Synthesize(ctx, orchestrator.RenderTranscript(session.Messages))
		if err != nil {
			h.logger.Warn("group synthesis failed", zap.String("group_id", groupID), zap.Error(err))
		} else if synthesis != "" {
			h.streamModeratorMessage(sse, "moderator", synthesis)
			h.persistModeratorMessage(session.SessionID, "moderator", synthesis)
		}
	}

	if lfTrace != nil {
		lfTrace.End(true, truncate(orchestrator.RenderTranscript(session.Messages), 2000))
	}

	_ = sse.SendRunFinished()
}

// buildRoster returns the accessible agents of the group as moderator briefs
// plus a lookup map. An optional override list restricts the roster further.
func (h *ProxyHandler) buildRoster(ctx context.Context, groupAgentIDs, override []string, userEmail string, userGroups []string, isAdmin, hasGitHubToken bool) ([]orchestrator.AgentBrief, map[string]*models.Agent) {
	overrideSet := make(map[string]bool, len(override))
	for _, id := range override {
		overrideSet[id] = true
	}

	var inheritedMap map[string]*models.InheritedPerms
	if !isAdmin {
		inheritedMap, _ = h.groupRepo.GetAllInheritedPermissions(ctx)
	}

	var roster []orchestrator.AgentBrief
	agentsByID := make(map[string]*models.Agent)
	for _, agentID := range groupAgentIDs {
		if len(overrideSet) > 0 && !overrideSet[agentID] {
			continue
		}
		agent, err := h.registry.Get(agentID)
		if err != nil {
			continue
		}
		if !isAdmin && !agents.HasAccessWithInherited(agent, userEmail, userGroups, inheritedMap[agentID]) {
			continue
		}
		// Skip agents that need a GitHub token the caller doesn't have.
		if agent.RequireGitHubToken && !hasGitHubToken {
			continue
		}
		roster = append(roster, orchestrator.AgentBrief{
			ID:          agent.ID,
			Name:        agent.Name,
			Description: agent.Description,
		})
		agentsByID[agent.ID] = agent
	}
	return roster, agentsByID
}

// errSessionNotFound signals a resume request for a session_id that doesn't exist.
var errSessionNotFound = fmt.Errorf("session not found")

// errSessionStore signals a transient store failure (distinct from "not found").
var errSessionStore = fmt.Errorf("session store error")

// getOrCreateGroupSession resolves the session for a group chat request.
// Group sessions are PERSONAL: resuming a session_id requires it to exist,
// belong to this group, and be owned by the caller — a transient store error
// or someone else's session must never silently fork a new conversation.
// Only an empty session_id creates a fresh session.
func (h *ProxyHandler) getOrCreateGroupSession(ctx context.Context, claims *auth.Claims, groupID string, rosterIDs []string, chatReq *models.ChatRequest) (*models.Session, error) {
	userEmail := claims.GetEmail()

	if chatReq.SessionID != "" {
		session, err := h.store.GetSession(ctx, chatReq.SessionID)
		if err != nil {
			// Transient store failure — surface as 5xx, don't fork a new session.
			h.logger.Error("failed to get group session", zap.String("session_id", chatReq.SessionID), zap.Error(err))
			return nil, errSessionStore
		}
		if session == nil {
			return nil, errSessionNotFound
		}
		if session.GroupID != groupID {
			return nil, fmt.Errorf("session does not belong to this group")
		}
		// Personal sessions: only the owner may resume, no membership/admin bypass.
		if session.UserID != userEmail {
			return nil, fmt.Errorf("access denied")
		}
		return session, nil
	}

	sessionName := chatReq.Messages[len(chatReq.Messages)-1].Content
	if len(sessionName) > 50 {
		sessionName = sessionName[:50]
	}
	firstAgentID := ""
	if len(rosterIDs) > 0 {
		firstAgentID = rosterIDs[0]
	}
	session, err := h.store.CreateSession(ctx, userEmail, firstAgentID, sessionName)
	if err != nil {
		return nil, fmt.Errorf("failed to create session")
	}
	session.IsMultiAgent = true
	session.GroupID = groupID
	session.AgentIDs = rosterIDs
	// The group flags are load-bearing (they make this a personal group session);
	// if they don't persist, fail rather than return a mislabeled session.
	if err := h.store.SaveSession(ctx, session); err != nil {
		h.logger.Error("failed to save group session flags", zap.String("session_id", session.SessionID), zap.Error(err))
		return nil, errSessionStore
	}
	// Group↔session index is best-effort (drives the sidebar list, not access).
	if err := h.groupRepo.AddSession(ctx, groupID, session.SessionID); err != nil {
		h.logger.Error("failed to add session to group", zap.Error(err),
			zap.String("group_id", groupID), zap.String("session_id", session.SessionID))
	}
	return session, nil
}

// streamModeratorMessage streams a synthetic (non-proxied) assistant message,
// optionally attributed to the special "moderator" speaker.
func (h *ProxyHandler) streamModeratorMessage(sse *proxy.SSEWriter, agentID, text string) {
	messageID := uuid.New().String()
	_ = sse.SendAGUIEvent(models.NewAGUITextMessageStartEventFull(messageID, agentID, false))
	_ = sse.SendAGUIEvent(models.NewAGUITextMessageContentEventWithAgent(messageID, text, agentID))
	_ = sse.SendAGUIEvent(models.NewAGUITextMessageEndEventWithAgent(messageID, agentID))
}

// persistModeratorMessage saves a synthetic assistant message to the session.
func (h *ProxyHandler) persistModeratorMessage(sessionID, agentID, text string) {
	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	msg := models.ChatMessage{Role: "assistant", Content: text, AgentID: agentID}
	if err := h.store.AddMessage(saveCtx, sessionID, msg); err != nil {
		h.logger.Error("failed to save moderator message", zap.Error(err))
	}
}

// recordGroupTurnEvent records one debate turn as a chat event for observability.
func (h *ProxyHandler) recordGroupTurnEvent(agent *models.Agent, sessionID, userEmail string, result *proxy.ProxyResult, err error, durationMs, messageCount int) {
	status := "ok"
	var errType, errMsg string
	if err != nil {
		status = "error"
		errMsg = err.Error()
		errType = classifyError(errMsg)
	} else if result != nil && result.Error != "" {
		status = "error"
		errMsg = result.Error
		errType = classifyError(errMsg)
	}
	var toolCallInfos []models.ToolCallInfo
	if result != nil {
		for _, tc := range result.ToolCalls {
			toolCallInfos = append(toolCallInfos, models.ToolCallInfo{Name: tc.Name})
		}
	}
	recordChatEvent(h.chatEventRepo, &models.ChatEvent{
		ResourceType: "agent",
		ResourceID:   agent.ID,
		ResourceName: agent.Name,
		Protocol:     agent.Protocol,
		UserEmail:    userEmail,
		SessionID:    sessionID,
		Status:       status,
		ErrorType:    errType,
		ErrorMsg:     errMsg,
		DurationMs:   durationMs,
		MessageCount: messageCount,
		ToolCalls:    toolCallInfos,
	}, h.logger)
}

// distinctSuccessfulSpeakers counts how many different agents replied successfully.
func distinctSuccessfulSpeakers(results []orchestrator.TurnResult) int {
	seen := make(map[string]bool)
	for _, r := range results {
		if r.Err == nil && r.Text != "" {
			seen[r.AgentID] = true
		}
	}
	return len(seen)
}

// localeFromRequest extracts "es"/"en" from the Accept-Language header
// (e.g. "es", "es-ES,es;q=0.9").
func localeFromRequest(r *http.Request) string {
	lang := strings.SplitN(r.Header.Get("Accept-Language"), ",", 2)[0]
	lang = strings.SplitN(lang, "-", 2)[0]
	lang = strings.SplitN(lang, ";", 2)[0]
	if strings.TrimSpace(lang) == "es" {
		return "es"
	}
	return "en"
}

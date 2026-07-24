package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	lf "github.com/dfradehubs/agentgram-api/internal/langfuse"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/orchestrator"
	"github.com/dfradehubs/agentgram-api/internal/proxy"
	appsettings "github.com/dfradehubs/agentgram-api/internal/settings"
	"go.uber.org/zap"
)

// handleGroupToolCall handles a tools/call for an agent group (group__<groupID>).
// It runs a moderated debate over the group's agents and returns the collected
// replies as a single text result, with progress notifications to keep the MCP
// client's timeout alive.
func (h *Handler) handleGroupToolCall(w http.ResponseWriter, r *http.Request, req jsonRPCRequest, groupID string, arguments json.RawMessage, userEmail string, userGroups []string, mcpSessionID string) {
	var args struct {
		Question  string `json:"question"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		h.writeJSON(w, http.StatusOK, h.server.MarshalError(req.ID, errCodeInvalidParams, "invalid arguments"))
		return
	}
	if args.Question == "" {
		h.writeJSON(w, http.StatusOK, h.server.MarshalError(req.ID, errCodeInvalidParams, "question is required"))
		return
	}

	// Start the tool-call budget before authorization/roster/session setup. The
	// execution detaches from client cancellation later, but preserves this
	// absolute deadline so preflight work cannot restart the timeout.
	toolCtx, toolCancel := context.WithTimeout(r.Context(), h.settings.Duration(appsettings.KeyMCPToolCallTimeout))
	defer toolCancel()

	// Access check: the group must be in the user's accessible list
	var accessible bool
	for _, g := range h.server.AccessibleGroupsContext(toolCtx, userEmail, userGroups) {
		if g.ID == groupID {
			accessible = true
			break
		}
	}
	if !accessible {
		h.writeJSON(w, http.StatusOK, h.server.MarshalToolResult(req.ID, "Access denied to this group", true))
		return
	}
	// Re-fetch via Get: AccessibleGroups (ListAccessible) doesn't select all
	// fields (e.g. max_turns), so use the full record for the debate.
	group, err := h.groupRepo.Get(toolCtx, groupID)
	if err != nil {
		h.writeJSON(w, http.StatusOK, h.server.MarshalToolResult(req.ID, "Group not found", true))
		return
	}

	// Resolve the moderator after authorization, for every tool call. This keeps
	// MCP behavior in sync with admin configuration without a process restart.
	moderator := h.moderator
	if h.moderatorResolver != nil {
		resolvedModerator, resolveErr := h.moderatorResolver.Resolve(toolCtx)
		if resolveErr != nil {
			if errors.Is(resolveErr, orchestrator.ErrModeratorNotConfigured) {
				h.writeJSON(w, http.StatusOK, h.server.MarshalToolResult(req.ID, "No moderator LLM configured (admin: add an enabled LLM model with role 'moderator')", true))
				return
			}
			h.logger.Error("MCP moderator is unavailable", zap.Error(resolveErr))
			h.writeJSON(w, http.StatusOK, h.server.MarshalToolResult(req.ID, "Moderator LLM is temporarily unavailable", true))
			return
		}
		moderator = resolvedModerator
	}
	if moderator == nil {
		h.writeJSON(w, http.StatusOK, h.server.MarshalToolResult(req.ID, "No moderator LLM configured (admin: add an enabled LLM model with role 'moderator')", true))
		return
	}

	// Roster: group agents the user can access
	roster, agentsByID := h.buildGroupRoster(toolCtx, group, userEmail, userGroups)
	if len(roster) == 0 {
		h.writeJSON(w, http.StatusOK, h.server.MarshalToolResult(req.ID, "No accessible agents in this group", true))
		return
	}

	// Resolve session continuity: explicit arg or MCP session mapping
	// (group sessions reuse the agent-session map with a pseudo agent key).
	groupSessionKey := "group:" + groupID
	sessionID := args.SessionID
	if sessionID == "" && mcpSessionID != "" {
		if sid, ok := h.server.sessions.GetAgentSession(mcpSessionID, groupSessionKey); ok {
			sessionID = sid
		}
	}

	// Langfuse trace for the debate
	var lfTrace *lf.Trace
	if h.langfuseTracer != nil && h.langfuseTracer.Enabled() {
		lfTrace = h.langfuseTracer.StartTrace(toolCtx, "mcp:group-chat", userEmail, sessionID, map[string]interface{}{
			"group_id":   groupID,
			"group_name": group.Name,
			"source":     "mcp",
		})
		lfTrace.SetInput(truncateString(args.Question, 1000))
	}

	// SSE streaming response with progress notifications (same pattern as agents)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	flushSSE := func(data []byte) {
		fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}

	type callResult struct {
		text      string
		sessionID string
		isError   bool
		err       error
	}
	done := make(chan callResult, 1)
	go func() {
		text, resultSessionID, isError, err := h.callGroupWithModerator(toolCtx, moderator, group, roster, agentsByID, args.Question, sessionID, userEmail, userGroups, lfTrace)
		done <- callResult{text: text, sessionID: resultSessionID, isError: isError, err: err}
	}()

	progressTicker := time.NewTicker(15 * time.Second)
	defer progressTicker.Stop()
	elapsedSeconds := 0

	var cr callResult
	waiting := true
	for waiting {
		select {
		case cr = <-done:
			waiting = false
		case <-progressTicker.C:
			elapsedSeconds += 15
			progressNotification := fmt.Sprintf(
				`{"jsonrpc":"2.0","method":"notifications/progress","params":{"progressToken":%s,"progress":%d,"message":"Group %s is debating... %ds elapsed"}}`,
				string(req.ID), elapsedSeconds, groupID, elapsedSeconds)
			flushSSE([]byte(progressNotification))
		case <-r.Context().Done():
			h.logger.Warn("MCP client disconnected during group call", zap.String("group_id", groupID))
			if lfTrace != nil {
				lfTrace.End(false, "client disconnected")
			}
			return
		}
	}

	if cr.err != nil {
		h.logger.Error("MCP group tools/call error",
			zap.String("group_id", groupID),
			zap.String("user_email", userEmail),
			zap.Error(cr.err))
		if lfTrace != nil {
			lfTrace.End(false, cr.err.Error())
		}
		flushSSE(h.server.MarshalToolResult(req.ID, "Group call failed. Please try again.", true))
		return
	}

	// Store the group session mapping for future calls
	if mcpSessionID != "" && cr.sessionID != "" {
		h.server.sessions.SetAgentSession(mcpSessionID, groupSessionKey, cr.sessionID)
	}

	responseText := cr.text
	if cr.sessionID != "" {
		responseText += fmt.Sprintf("\n\n---\n[session_id: %s]", cr.sessionID)
	}

	if lfTrace != nil {
		lfTrace.End(!cr.isError, truncateString(responseText, 2000))
	}

	// isError=true for incomplete outcomes as well as total agent failure.
	groupAuditStatus := "ok"
	if cr.isError {
		groupAuditStatus = "error"
	}
	h.recordAudit(&models.AuditEvent{
		UserEmail: userEmail, UserGroups: userGroups,
		ResourceType: models.AuditResourceGroup, ResourceID: groupID, ResourceName: group.Name,
		Source: models.AuditSourceMCP, Client: r.UserAgent(), Action: models.AuditActionGroupDebate,
		Prompt: args.Question, Response: responseText, Status: groupAuditStatus,
		DurationMs: elapsedSeconds * 1000,
	})
	flushSSE(h.server.MarshalToolResult(req.ID, responseText, cr.isError))
}

// buildGroupRoster returns the group's agents the user can access as moderator
// briefs plus a lookup map.
func (h *Handler) buildGroupRoster(ctx context.Context, group *models.AgentGroup, userEmail string, userGroups []string) ([]orchestrator.AgentBrief, map[string]*models.Agent) {
	isAdmin, _ := h.userService.IsAdmin(ctx, userEmail, userGroups)
	var inheritedMap map[string]*models.InheritedPerms
	if !isAdmin {
		inheritedMap, _ = h.groupRepo.GetAllInheritedPermissions(ctx)
	}
	// The MCP surface has no interactive GitHub-connect flow, so an agent that
	// needs a GitHub token only works if one was forwarded on this request.
	hasGitHubToken := middleware.GetGitHubTokenFromContext(ctx) != ""

	var roster []orchestrator.AgentBrief
	agentsByID := make(map[string]*models.Agent)
	for _, agentID := range group.AgentIDs {
		agent, err := h.registry.Get(agentID)
		if err != nil {
			continue
		}
		if !isAdmin && !agents.HasAccessWithInherited(agent, userEmail, userGroups, inheritedMap[agentID]) {
			continue
		}
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

// callGroup runs the moderated debate and returns the collected replies as a
// single markdown text, plus the Agentgram session ID for continuity.
// callGroup returns the collected debate text, the session ID, an isError flag
// (true for an incomplete outcome or when no agent responded), and a transport error.
func (h *Handler) callGroup(ctx context.Context, group *models.AgentGroup, roster []orchestrator.AgentBrief, agentsByID map[string]*models.Agent, question, sessionID, userEmail string, userGroups []string, lfTrace *lf.Trace) (string, string, bool, error) {
	return h.callGroupWithModerator(ctx, h.moderator, group, roster, agentsByID, question, sessionID, userEmail, userGroups, lfTrace)
}

func (h *Handler) callGroupWithModerator(ctx context.Context, moderator *orchestrator.Moderator, group *models.AgentGroup, roster []orchestrator.AgentBrief, agentsByID map[string]*models.Agent, question, sessionID, userEmail string, userGroups []string, lfTrace *lf.Trace) (string, string, bool, error) {
	if moderator == nil {
		return "", sessionID, true, orchestrator.ErrModeratorNotConfigured
	}

	// The MCP user's JWT, forwarded only when the agent's auth method is "forward"
	authHeader := middleware.GetAuthHeaderFromContext(ctx)

	// Absolute deadline for the WHOLE tool call, started up-front so session
	// resolution + store ops also count against the budget (and can't restart
	// the timeout later). Detached from client cancellation (survives a client
	// disconnect); carries the GitHub token and identity claims for downstream.
	deadline := time.Now().Add(h.settings.Duration(appsettings.KeyMCPToolCallTimeout))
	if parentDeadline, ok := ctx.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
	}
	callCtx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	if tok := middleware.GetGitHubTokenFromContext(ctx); tok != "" {
		callCtx = context.WithValue(callCtx, middleware.GitHubTokenContextKey, tok)
	}
	if claims := middleware.GetUserFromContext(ctx); claims != nil {
		callCtx = context.WithValue(callCtx, middleware.UserContextKey, claims)
	}
	if lfTrace != nil {
		callCtx = lf.ContextWithTrace(callCtx, lfTrace)
	}

	// Resolve or create the group session (against callCtx so it counts against
	// the deadline). Personal sessions: a resume must exist, belong to this
	// group, and be owned by the caller — a transient error or another user's
	// session must not silently fork a new session.
	var session *models.Session
	createdSession := false
	if sessionID != "" {
		s, err := h.sessionStore.GetSession(callCtx, sessionID)
		if err != nil {
			return "", "", true, fmt.Errorf("session store error: %w", err)
		}
		if s == nil {
			return "", "", true, fmt.Errorf("session not found")
		}
		if s.GroupID != group.ID || s.UserID != userEmail {
			return "", "", true, fmt.Errorf("access denied to session")
		}
		session = s
	}
	if session == nil {
		s, err := h.sessionStore.CreateSession(callCtx, userEmail, roster[0].ID, truncateString(question, 50))
		if err != nil {
			return "", "", true, fmt.Errorf("failed to create session: %w", err)
		}
		s.IsMultiAgent = true
		s.GroupID = group.ID
		s.AgentIDs = make([]string, 0, len(roster))
		for _, b := range roster {
			s.AgentIDs = append(s.AgentIDs, b.ID)
		}
		// The group flags are load-bearing (they make this a resumable personal
		// group session); if they don't persist, delete the half-created session
		// and fail rather than leave an orphan or hand back an unusable id.
		createdSession = true
		if err := h.sessionStore.SaveSession(callCtx, s); err != nil {
			h.logger.Error("failed to save group session flags", zap.String("session_id", s.SessionID), zap.Error(err))
			h.cleanupOrphanSession(callCtx, group.ID, s.SessionID, userEmail, roster[0].ID)
			return "", "", true, fmt.Errorf("failed to persist session: %w", err)
		}
		// Index the session for the sidebar. This is load-bearing for listing —
		// if it fails, the session would vanish from the user's view — so fail.
		if err := h.groupRepo.AddSession(callCtx, group.ID, s.SessionID); err != nil {
			h.logger.Error("failed to index group session", zap.String("session_id", s.SessionID), zap.Error(err))
			h.cleanupOrphanSession(callCtx, group.ID, s.SessionID, userEmail, roster[0].ID)
			return "", "", true, fmt.Errorf("failed to index session: %w", err)
		}
		session = s
	}

	// Save the user message (BroadcastAgentIDs = roster, see group_chat.go)
	userMsg := models.ChatMessage{
		Role:      "user",
		Content:   question,
		UserEmail: userEmail,
	}
	for _, b := range roster {
		userMsg.BroadcastAgentIDs = append(userMsg.BroadcastAgentIDs, b.ID)
	}
	if err := h.sessionStore.AddMessage(callCtx, session.SessionID, userMsg); err != nil {
		h.logger.Error("failed to persist user message", zap.Error(err))
		if createdSession {
			h.cleanupOrphanSession(callCtx, group.ID, session.SessionID, userEmail, roster[0].ID)
		}
		return "", session.SessionID, true, fmt.Errorf("failed to persist user message: %w", err)
	}
	session.Messages = append(session.Messages, userMsg)

	reqIDSuffix := session.SessionID
	if len(reqIDSuffix) > 8 {
		reqIDSuffix = reqIDSuffix[:8]
	}

	// Collecting TurnRunner: run the agent against a buffer, persist, return text
	run := func(turnCtx context.Context, agentID string) (string, error) {
		agent := agentsByID[agentID]

		// Each turn is bounded by the time LEFT in the shared debate deadline,
		// never a fresh full timeout. (The proxy also clamps to callCtx's
		// deadline, so this is belt-and-suspenders.)
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return "", context.DeadlineExceeded
		}

		agentSessionID, mappingErr := h.sessionStore.GetAgentSessionID(turnCtx, session.SessionID, agentID)
		if mappingErr != nil {
			return "", orchestrator.NewTurnError(orchestrator.TurnFailureMapping, fmt.Errorf("load agent session mapping: %w", mappingErr))
		}
		hasAgentSession := agentSessionID != ""

		// No summarizer on this surface — context is capped by maxContextMessages anyway
		prep := proxy.PrepareMessagesForMultiAgent(session, agentID, userMsg, hasAgentSession, true, 0, 0, nil, turnCtx)

		var agentSpan *lf.Span
		if lfTrace != nil && lfTrace.IsEnabled() {
			agentSpan = lfTrace.StartToolCall(fmt.Sprintf("proxy:%s", agentID), map[string]interface{}{
				"agent_name":     agent.Name,
				"agent_protocol": agent.Protocol,
			})
		}

		buf := &sseCapture{}
		result, err := h.proxy.Handle(turnCtx, buf, agent, &models.ChatRequest{
			Messages:  prep.Messages,
			SessionID: agentSessionID,
		}, authHeader, proxy.HandleOptions{
			ThreadID:     session.SessionID,
			RequestID:    fmt.Sprintf("mcp-group-%s", reqIDSuffix),
			UserEmail:    userEmail,
			UserGroups:   userGroups,
			AgentTimeout: remaining,
		})

		if agentSpan != nil {
			if err != nil {
				agentSpan.EndWithError(err)
			} else if result != nil {
				agentSpan.End(truncateString(result.AssistantText, 2000))
			} else {
				agentSpan.End(nil)
			}
		}
		turnErr := orchestrator.NewTurnError(orchestrator.TurnFailureAgent, err)
		if result == nil {
			return "", turnErr
		}
		if result.Error != "" && turnErr == nil {
			turnErr = orchestrator.NewTurnError(orchestrator.TurnFailureAgent, errors.New(result.Error))
		}

		// Finalization is detached from the execution deadline but independently
		// bounded, so a result completed at the deadline is not silently lost.
		saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(callCtx), 5*time.Second)
		defer saveCancel()
		assistantMsg := result.ToChatMessage(agentID, proxy.PublicErrorMessage(turnErr))
		assistantPersisted := true
		if err := h.sessionStore.AddMessage(saveCtx, session.SessionID, assistantMsg); err != nil {
			h.logger.Error("failed to persist assistant message", zap.Error(err))
			turnErr = errors.Join(turnErr, orchestrator.NewTurnError(orchestrator.TurnFailurePersistence, fmt.Errorf("reply persistence failed: %w", err)))
			assistantPersisted = false
		} else {
			session.Messages = append(session.Messages, assistantMsg)
		}

		if turnErr == nil && assistantPersisted {
			mappingID := ""
			if result.AgentSessionID != "" && result.AgentSessionID != agentSessionID {
				mappingID = result.AgentSessionID
			} else if !hasAgentSession {
				mappingID = session.SessionID
			}
			if mappingID != "" {
				if err := h.sessionStore.SetAgentSessionID(saveCtx, session.SessionID, agentID, mappingID); err != nil {
					h.logger.Error("failed to persist agent session mapping", zap.Error(err))
					turnErr = orchestrator.NewTurnError(orchestrator.TurnFailureMapping, fmt.Errorf("session mapping persistence failed: %w", err))
				}
			}
		}

		return proxy.TranscriptText(result), turnErr
	}

	maxTurns := h.settings.Int(appsettings.KeyGroupMaxTurnsMCP)
	if group.MaxTurns > 0 {
		maxTurns = group.MaxTurns
	}
	transcript := orchestrator.RenderTranscript(session.Messages)
	results, debateErr := moderator.Debate(callCtx, roster, transcript, run, maxTurns)
	if debateErr != nil && len(results) == 0 {
		return "", session.SessionID, true, debateErr
	}

	if len(results) == 0 {
		// Nobody applied — informational, not an error.
		return "No agent in this group can help with that request.", session.SessionID, false, nil
	}

	// Collect per-agent contributions as markdown sections
	var parts []string
	for _, res := range results {
		name := res.AgentID
		if agent, ok := agentsByID[res.AgentID]; ok && agent.Name != "" {
			name = agent.Name
		}
		if res.Err != nil {
			publicTurnError := "agent response could not be completed"
			if res.HasFailure(orchestrator.TurnFailurePersistence) {
				publicTurnError = "reply could not be saved"
			} else if res.HasFailure(orchestrator.TurnFailureMapping) {
				publicTurnError = "agent session continuity could not be saved"
			}
			if res.Text != "" {
				parts = append(parts, fmt.Sprintf("**[%s]**\n\n%s\n\n_error: %s_", name, res.Text, publicTurnError))
			} else {
				parts = append(parts, fmt.Sprintf("**[%s]**\n\n_error: %s_", name, publicTurnError))
			}
			continue
		}
		parts = append(parts, fmt.Sprintf("**[%s]**\n\n%s", name, res.Text))
	}

	// A debate that started (some agent replied) but ended abnormally — the
	// moderator failed or the deadline hit before FINISH — is incomplete, not a
	// clean success. Tell the client rather than presenting a partial result.
	if debateErr != nil {
		note := "the debate ended early because the moderator failed"
		if errors.Is(debateErr, context.DeadlineExceeded) {
			note = "the debate hit the tool-call timeout before finishing"
		} else if errors.Is(debateErr, orchestrator.ErrMaxTurnsReached) {
			note = "the debate reached its turn limit"
		}
		parts = append(parts, fmt.Sprintf("_⚠️ Note: %s; the answer above may be incomplete._", note))
	}

	resultIncomplete := debateErr != nil
	for _, result := range results {
		if result.HasFailure(orchestrator.TurnFailurePersistence) || result.HasFailure(orchestrator.TurnFailureMapping) {
			resultIncomplete = true
		}
	}
	// Optional synthesis when several agents contributed
	if !resultIncomplete && distinctSpeakers(results) >= 2 {
		synthesis, err := moderator.Synthesize(callCtx, orchestrator.RenderTranscript(session.Messages))
		if err != nil {
			h.logger.Warn("group synthesis failed", zap.String("group_id", group.ID), zap.Error(err))
		} else if synthesis != "" {
			parts = append(parts, fmt.Sprintf("**[Moderator]**\n\n%s", synthesis))
			saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(callCtx), 5*time.Second)
			if err := h.sessionStore.AddMessage(saveCtx, session.SessionID, models.ChatMessage{
				Role:    "assistant",
				Content: synthesis,
				AgentID: "moderator",
			}); err != nil {
				h.logger.Error("failed to persist moderator synthesis", zap.Error(err))
				parts = append(parts, "_error: synthesis could not be saved_")
				resultIncomplete = true
			}
			saveCancel()
		}
	}

	// isError when no agent replied successfully OR the debate ended abnormally
	// (moderator error / deadline) — either way the client shouldn't treat the
	// result as a complete success.
	isError := distinctSpeakers(results) == 0 || resultIncomplete
	return strings.Join(parts, "\n\n---\n\n"), session.SessionID, isError, nil
}

// cleanupOrphanSession reconciles both the PostgreSQL group index and Redis.
// When the absolute call deadline is too close, cleanup continues asynchronously
// so compensation cannot extend the user-visible tool-call budget.
func (h *Handler) cleanupOrphanSession(callCtx context.Context, groupID, sessionID, userEmail, agentID string) {
	cleanup := func() {
		h.deleteOrphanSession(groupID, sessionID, userEmail, agentID)
	}
	if deadline, ok := callCtx.Deadline(); ok && time.Until(deadline) <= 5*time.Second {
		go cleanup()
		return
	}
	cleanup()
}

func (h *Handler) deleteOrphanSession(groupID, sessionID, userEmail, agentID string) {
	delCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.groupRepo.RemoveSession(delCtx, groupID, sessionID); err != nil {
		h.logger.Warn("failed to remove orphan group-session index; manual reconciliation may be needed",
			zap.String("group_id", groupID), zap.String("session_id", sessionID), zap.Error(err))
	}
	if err := h.sessionStore.DeleteSession(delCtx, sessionID, userEmail, agentID); err != nil {
		h.logger.Warn("failed to clean up orphan group session; manual reconciliation may be needed",
			zap.String("session_id", sessionID), zap.Error(err))
	}
}

// distinctSpeakers counts agents that returned content; persistence failures
// still count as responses, while agent execution failures do not.
func distinctSpeakers(results []orchestrator.TurnResult) int {
	seen := make(map[string]bool)
	for _, r := range results {
		if r.Text != "" && !r.HasFailure(orchestrator.TurnFailureAgent) {
			seen[r.AgentID] = true
		}
	}
	return len(seen)
}

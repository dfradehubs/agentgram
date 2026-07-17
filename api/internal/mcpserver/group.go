package mcpserver

import (
	"context"
	"encoding/json"
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
	"go.uber.org/zap"
)

// mcpGroupMaxTurns caps the moderated debate for the synchronous MCP surface.
// ponytail: lower than the SSE endpoint (6) — the tool call blocks until the
// debate ends; next step if too slow is partial response + session_id continue.
const mcpGroupMaxTurns = 3

// groupExists reports whether an agent group with the given ID exists,
// regardless of the caller's access (authorization happens in the handler).
func (h *Handler) groupExists(ctx context.Context, groupID string) bool {
	if h.groupRepo == nil {
		return false
	}
	group, err := h.groupRepo.Get(ctx, groupID)
	return err == nil && group != nil
}

// handleGroupToolCall handles a tools/call for an agent group (ask_group_{groupID}).
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

	if h.moderator == nil {
		h.writeJSON(w, http.StatusOK, h.server.MarshalToolResult(req.ID, "No moderator LLM configured (admin: add an LLM model with role 'moderator')", true))
		return
	}

	// Access check: the group must be in the user's accessible list
	var group *models.AgentGroup
	for _, g := range h.server.AccessibleGroups(userEmail, userGroups) {
		if g.ID == groupID {
			group = g
			break
		}
	}
	if group == nil {
		h.writeJSON(w, http.StatusOK, h.server.MarshalToolResult(req.ID, "Access denied to this group", true))
		return
	}

	// Roster: group agents the user can access
	roster, agentsByID := h.buildGroupRoster(r.Context(), group, userEmail, userGroups)
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
		lfTrace = h.langfuseTracer.StartTrace(r.Context(), "mcp:group-chat", userEmail, sessionID, map[string]interface{}{
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
		err       error
	}
	done := make(chan callResult, 1)
	go func() {
		text, resultSessionID, err := h.callGroup(r.Context(), group, roster, agentsByID, args.Question, sessionID, userEmail, userGroups, lfTrace)
		done <- callResult{text: text, sessionID: resultSessionID, err: err}
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
		lfTrace.End(true, truncateString(responseText, 2000))
	}

	flushSSE(h.server.MarshalToolResult(req.ID, responseText, false))
}

// buildGroupRoster returns the group's agents the user can access as moderator
// briefs plus a lookup map.
func (h *Handler) buildGroupRoster(ctx context.Context, group *models.AgentGroup, userEmail string, userGroups []string) ([]orchestrator.AgentBrief, map[string]*models.Agent) {
	isAdmin, _ := h.userService.IsAdmin(ctx, userEmail, userGroups)
	var inheritedMap map[string]*models.InheritedPerms
	if !isAdmin {
		inheritedMap, _ = h.groupRepo.GetAllInheritedPermissions(ctx)
	}

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
func (h *Handler) callGroup(ctx context.Context, group *models.AgentGroup, roster []orchestrator.AgentBrief, agentsByID map[string]*models.Agent, question, sessionID, userEmail string, userGroups []string, lfTrace *lf.Trace) (string, string, error) {
	// Resolve or create the group session
	var session *models.Session
	if sessionID != "" {
		if s, err := h.sessionStore.GetSession(ctx, sessionID); err == nil && s != nil && s.GroupID == group.ID {
			session = s
		}
	}
	if session == nil {
		s, err := h.sessionStore.CreateSession(ctx, userEmail, roster[0].ID, truncateString(question, 50))
		if err != nil {
			return "", "", fmt.Errorf("failed to create session: %w", err)
		}
		s.IsMultiAgent = true
		s.GroupID = group.ID
		s.AgentIDs = make([]string, 0, len(roster))
		for _, b := range roster {
			s.AgentIDs = append(s.AgentIDs, b.ID)
		}
		if err := h.sessionStore.SaveSession(ctx, s); err != nil {
			h.logger.Error("failed to save session flags", zap.Error(err))
		}
		if err := h.groupRepo.AddSession(ctx, group.ID, s.SessionID); err != nil {
			h.logger.Error("failed to add session to group", zap.Error(err))
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
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer saveCancel()
	if err := h.sessionStore.AddMessage(saveCtx, session.SessionID, userMsg); err != nil {
		h.logger.Error("failed to persist user message", zap.Error(err))
	}
	session.Messages = append(session.Messages, userMsg)

	// The MCP user's JWT, forwarded only when the agent's auth method is "forward"
	authHeader := middleware.GetAuthHeaderFromContext(ctx)

	// Detached context so the debate survives client disconnects/gateway timeouts
	callCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if lfTrace != nil {
		callCtx = lf.ContextWithTrace(callCtx, lfTrace)
	}

	reqIDSuffix := session.SessionID
	if len(reqIDSuffix) > 8 {
		reqIDSuffix = reqIDSuffix[:8]
	}

	// Collecting TurnRunner: run the agent against a buffer, persist, return text
	run := func(turnCtx context.Context, agentID string) (string, error) {
		agent := agentsByID[agentID]

		agentSessionID, _ := h.sessionStore.GetAgentSessionID(turnCtx, session.SessionID, agentID)
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
			ThreadID:   session.SessionID,
			RequestID:  fmt.Sprintf("mcp-group-%s", reqIDSuffix),
			UserEmail:  userEmail,
			UserGroups: userGroups,
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
		if err != nil {
			return "", err
		}
		if result == nil {
			return "", nil
		}
		if result.Error != "" {
			return "", fmt.Errorf("%s", result.Error)
		}

		// Persist the reply and grow the in-memory session for the next turn
		turnSaveCtx, turnSaveCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer turnSaveCancel()
		assistantMsg := models.ChatMessage{
			Role:    "assistant",
			Content: result.AssistantText,
			AgentID: agentID,
		}
		if err := h.sessionStore.AddMessage(turnSaveCtx, session.SessionID, assistantMsg); err != nil {
			h.logger.Error("failed to persist assistant message", zap.Error(err))
		}
		session.Messages = append(session.Messages, assistantMsg)

		if result.AgentSessionID != "" && result.AgentSessionID != agentSessionID {
			_ = h.sessionStore.SetAgentSessionID(turnSaveCtx, session.SessionID, agentID, result.AgentSessionID)
		} else if !hasAgentSession {
			_ = h.sessionStore.SetAgentSessionID(turnSaveCtx, session.SessionID, agentID, session.SessionID)
		}

		return result.AssistantText, nil
	}

	transcript := orchestrator.RenderTranscript(session.Messages)
	results, debateErr := h.moderator.Debate(callCtx, roster, transcript, run, mcpGroupMaxTurns)
	if debateErr != nil && len(results) == 0 {
		return "", session.SessionID, debateErr
	}

	if len(results) == 0 {
		return "No agent in this group can help with that request.", session.SessionID, nil
	}

	// Collect per-agent contributions as markdown sections
	var parts []string
	for _, res := range results {
		name := res.AgentID
		if agent, ok := agentsByID[res.AgentID]; ok && agent.Name != "" {
			name = agent.Name
		}
		if res.Err != nil {
			parts = append(parts, fmt.Sprintf("**[%s]**\n\n_error: %v_", name, res.Err))
			continue
		}
		parts = append(parts, fmt.Sprintf("**[%s]**\n\n%s", name, res.Text))
	}

	// Optional synthesis when several agents contributed
	if distinctSpeakers(results) >= 2 {
		if synthesis, err := h.moderator.Synthesize(callCtx, orchestrator.RenderTranscript(session.Messages)); err == nil && synthesis != "" {
			parts = append(parts, fmt.Sprintf("**[Moderator]**\n\n%s", synthesis))
			modSaveCtx, modSaveCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer modSaveCancel()
			if err := h.sessionStore.AddMessage(modSaveCtx, session.SessionID, models.ChatMessage{
				Role:    "assistant",
				Content: synthesis,
				AgentID: "moderator",
			}); err != nil {
				h.logger.Error("failed to persist moderator synthesis", zap.Error(err))
			}
		}
	}

	return strings.Join(parts, "\n\n---\n\n"), session.SessionID, nil
}

// distinctSpeakers counts how many different agents replied successfully.
func distinctSpeakers(results []orchestrator.TurnResult) int {
	seen := make(map[string]bool)
	for _, r := range results {
		if r.Err == nil && r.Text != "" {
			seen[r.AgentID] = true
		}
	}
	return len(seen)
}

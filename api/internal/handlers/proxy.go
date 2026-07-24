package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/audit"
	"github.com/dfradehubs/agentgram-api/internal/fileprocessor"
	lf "github.com/dfradehubs/agentgram-api/internal/langfuse"
	"github.com/dfradehubs/agentgram-api/internal/llmresolver"
	"github.com/dfradehubs/agentgram-api/internal/metrics"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/orchestrator"
	"github.com/dfradehubs/agentgram-api/internal/proxy"
	"github.com/dfradehubs/agentgram-api/internal/pubsub"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/service"
	"github.com/dfradehubs/agentgram-api/internal/sessionnamer"
	appsettings "github.com/dfradehubs/agentgram-api/internal/settings"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"github.com/dfradehubs/agentgram-api/internal/summarizer"
	"github.com/dfradehubs/agentgram-api/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// ProxyHandler handles chat requests to agents
type ProxyHandler struct {
	registry          *agents.Registry
	userService       *service.UserService
	groupRepo         repository.GroupRepository
	proxy             *proxy.Proxy
	store             store.SessionStore
	hub               *pubsub.Hub
	summarizer        *summarizer.Summarizer
	sessionNamer      *sessionnamer.Namer
	fileProcessor     *fileprocessor.Processor
	llmResolver       *llmresolver.Resolver
	moderator         *orchestrator.Moderator
	moderatorResolver *orchestrator.ModeratorResolver
	settings          *appsettings.Service
	audit             *audit.Logger
	chatEventRepo     repository.ChatEventRepository
	auditRepo         repository.AuditEventRepository
	langfuseTracer    *lf.Tracer
	logger            *zap.Logger
}

// SetAuditRepo wires the audit-event repository (optional; auditing is a no-op
// without it). Uses the handler's existing settings service for the truncation
// limit.
func (h *ProxyHandler) SetAuditRepo(repo repository.AuditEventRepository) {
	h.auditRepo = repo
}

// NewProxyHandler creates a new proxy handler
func NewProxyHandler(llmRepo repository.LLMModelRepository, registry *agents.Registry, userService *service.UserService, groupRepo repository.GroupRepository, sessionStore store.SessionStore, hub *pubsub.Hub, auditLogger *audit.Logger, logger *zap.Logger, settingsService *appsettings.Service, lfTracer *lf.Tracer, chatEventRepo ...repository.ChatEventRepository) *ProxyHandler {
	h := &ProxyHandler{
		registry:          registry,
		userService:       userService,
		groupRepo:         groupRepo,
		proxy:             proxy.NewProxy(logger),
		store:             sessionStore,
		hub:               hub,
		llmResolver:       llmresolver.New(llmRepo, lfTracer, logger),
		moderatorResolver: orchestrator.NewModeratorResolver(llmRepo, lfTracer, logger),
		settings:          settingsService,
		audit:             auditLogger,
		langfuseTracer:    lfTracer,
		logger:            logger,
	}
	if len(chatEventRepo) > 0 {
		h.chatEventRepo = chatEventRepo[0]
	}
	return h
}

func (h *ProxyHandler) currentSummarizer(ctx context.Context) *summarizer.Summarizer {
	if h.llmResolver == nil {
		return h.summarizer
	}
	_, provider, err := h.llmResolver.ResolveRole(ctx, "summarizer", "summarizer")
	if err != nil {
		return nil
	}
	return summarizer.NewWithProvider(provider, h.logger)
}

func (h *ProxyHandler) currentFileProcessor(ctx context.Context) *fileprocessor.Processor {
	if h.llmResolver == nil {
		return h.fileProcessor
	}
	model, provider, err := h.llmResolver.ResolveRole(ctx, "file_processor", "file-processor")
	if err != nil {
		return nil
	}
	return fileprocessor.NewWithProvider(provider, model, h.logger)
}

func (h *ProxyHandler) currentSessionNamer(ctx context.Context) *sessionnamer.Namer {
	if h.llmResolver == nil {
		return h.sessionNamer
	}
	_, provider, err := h.llmResolver.ResolveRole(ctx, "session_namer", "session-namer")
	if err != nil {
		return nil
	}
	return sessionnamer.NewWithProvider(provider, h.logger)
}

// Chat handles POST /api/agents/:agentId/chat
// @Summary Chat with an agent
// @Description Sends a chat request to an agent and returns a streaming SSE response with AG-UI protocol events.
// @Description The response is a Server-Sent Events stream (text/event-stream) containing AG-UI events:
// @Description RUN_STARTED, TEXT_MESSAGE_START, TEXT_MESSAGE_CONTENT, TEXT_MESSAGE_END, TOOL_CALL_START, TOOL_CALL_ARGS, TOOL_CALL_END, RUN_FINISHED, RUN_ERROR.
// @Tags chat
// @Accept json
// @Produce text/event-stream
// @Security BearerAuth
// @Security CookieAuth
// @Param agentId path string true "Agent ID"
// @Param request body models.ChatRequest true "Chat request with messages and optional session_id"
// @Success 200 {string} string "SSE stream with AG-UI events (e.g. data: {\"type\":\"TEXT_MESSAGE_CONTENT\",\"messageId\":\"...\",\"delta\":\"Hello\"})"
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/agents/{agentId}/chat [post]
func (h *ProxyHandler) Chat(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	if agentID == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"agent id required"}`, http.StatusBadRequest)
		return
	}

	// Get user claims
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Find the agent
	agent, err := h.registry.Get(agentID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}

	// Verify permissions (admins can access all agents; includes inherited group permissions)
	isAdmin, _ := h.userService.IsAdmin(r.Context(), claims.GetEmail(), claims.GetGroups())
	if !isAdmin {
		inheritedMap, _ := h.groupRepo.GetAllInheritedPermissions(r.Context())
		inherited := inheritedMap[agentID]
		if !agents.HasAccessWithInherited(agent, claims.GetEmail(), claims.GetGroups(), inherited) {
			h.logger.Warn("access denied to agent",
				zap.String("agent_id", agentID),
				zap.String("user_email", claims.GetEmail()),
				zap.Strings("user_groups", claims.GetGroups()),
				zap.Strings("agent_allowed_users", agent.AllowedUsers),
				zap.Strings("agent_allowed_groups", agent.AllowedGroups))
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
			return
		}
	}

	// Require GitHub token for agents that need it (e.g. coding-agent)
	if agent.RequireGitHubToken && middleware.GetGitHubTokenFromContext(r.Context()) == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"github_token_required","message":"Connect your GitHub account to use this agent"}`, http.StatusForbidden)
		return
	}

	// Parse body
	var chatReq models.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
		h.logger.Error("failed to decode chat request",
			zap.String("agent_id", agentID),
			zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Validate the chat request payload
	if errMsg := validateChatRequest(&chatReq); errMsg != "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error":%q}`, errMsg), http.StatusBadRequest)
		return
	}
	if chatReq.GroupID != "" {
		http.Error(w, `{"error":"group sessions must use /api/groups/{groupId}/chat"}`, http.StatusBadRequest)
		return
	}

	userEmail := claims.GetEmail()

	// 1. Get or create session in Redis
	ctx := r.Context()
	var session *models.Session
	isNewSession := false

	if chatReq.SessionID != "" {
		session, err = h.store.GetSession(ctx, chatReq.SessionID)
		if err != nil {
			h.logger.Error("failed to get session", zap.Error(err))
			http.Error(w, `{"error":"failed to get session"}`, http.StatusInternalServerError)
			return
		}
		if session == nil {
			http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
			return
		}
		if session.GroupID != "" {
			http.Error(w, `{"error":"group sessions must use /api/groups/{groupId}/chat"}`, http.StatusBadRequest)
			return
		}
		// Sessions are personal — only the owner may continue them (group
		// membership does NOT grant access to another member's session). Slack
		// threads keep their multi-user participant check.
		if session != nil && session.UserID != userEmail {
			allowed := session.Source == "slack" && h.store.IsParticipant(r.Context(), chatReq.SessionID, userEmail)
			if !allowed {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
				return
			}
		}
		// Upgrade to multi-agent if send_context is set on a single-agent session
		if session != nil && !session.IsMultiAgent && chatReq.SendContext != nil && *chatReq.SendContext {
			session.IsMultiAgent = true
			if err := h.store.SaveSession(ctx, session); err != nil {
				h.logger.Error("failed to upgrade session to multi-agent", zap.Error(err))
			}
		}
	}

	if session == nil {
		// Create new session - name from first 50 chars of user message (LLM rename happens async)
		lastUserMsg := chatReq.Messages[len(chatReq.Messages)-1].Content
		sessionName := lastUserMsg
		if len(sessionName) > 50 {
			sessionName = sessionName[:50]
		}
		isNewSession = true
		session, err = h.store.CreateSession(ctx, userEmail, agentID, sessionName)
		if err != nil {
			h.logger.Error("failed to create session", zap.Error(err))
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"failed to create session"}`, http.StatusInternalServerError)
			return
		}
		// If send_context is set, mark as multi-agent session
		if chatReq.SendContext != nil && *chatReq.SendContext {
			session.IsMultiAgent = true
		}
		if session.IsMultiAgent {
			if err := h.store.SaveSession(ctx, session); err != nil {
				h.logger.Error("failed to save session flags", zap.Error(err))
			}
		}
	}

	// 2. Save user message to Redis
	// Deduplication is handled atomically in the Redis Lua script — if the last
	// message is already an identical user message, the insert is skipped.
	// Note: user messages in multi-agent sessions intentionally have NO AgentID
	// so that calculateDelta treats them as shared context for all agents.
	userMsg := chatReq.Messages[len(chatReq.Messages)-1]
	// Tag user message with display name and admin status for multi-user group sessions
	if userMsg.Role == "user" {
		userMsg.UserName = claims.GetDisplayName()
		userMsg.UserEmail = claims.GetEmail()
		userMsg.IsAdmin = isAdmin
	}
	if err := h.store.AddMessage(ctx, session.SessionID, userMsg); err != nil {
		h.logger.Error("failed to save user message", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"failed to persist user message"}`, http.StatusInternalServerError)
		return
	}

	// Extract request ID from chi middleware for end-to-end correlation
	requestID := chiMiddleware.GetReqID(r.Context())

	// 3. Start Langfuse trace BEFORE any internal LLM calls (summarizer, file processor)
	var lfTrace *lf.Trace
	var proxyErr error
	if h.langfuseTracer != nil && h.langfuseTracer.Enabled() {
		lfTrace = h.langfuseTracer.StartTrace(ctx, "chat", userEmail, session.SessionID, map[string]interface{}{
			"agent_id":       agentID,
			"agent_name":     agent.Name,
			"agent_protocol": agent.Protocol,
			"is_multi_agent": session.IsMultiAgent,
			"request_id":     requestID,
		})
		// Set trace input to the last user message
		if lastMsg := chatReq.Messages[len(chatReq.Messages)-1]; lastMsg.Role == "user" {
			lfTrace.SetInput(truncate(lastMsg.Content, 1000))
		}
		ctx = lf.ContextWithTrace(ctx, lfTrace)
	}

	// 4. Prepare messages for the agent (may call summarizer LLM)
	agentSessionID, _ := h.store.GetAgentSessionID(ctx, session.SessionID, agentID)
	hasAgentSession := agentSessionID != ""

	var messagesToSend []models.ChatMessage
	var contextSent bool
	if session.IsMultiAgent {
		sendContext := chatReq.SendContext == nil || *chatReq.SendContext
		result := proxy.PrepareMessagesForMultiAgent(session, agentID, userMsg, hasAgentSession, sendContext, agent.MaxContextTokens, agent.SummarizeThreshold, h.currentSummarizer(ctx), ctx)
		messagesToSend = result.Messages
		contextSent = result.ContextSent
	} else {
		messagesToSend = proxy.PrepareMessagesForAgent(session, userMsg, hasAgentSession)
	}

	// Prefix [DisplayName] on the last user message so agents know who is asking
	lastIdx := len(messagesToSend) - 1
	if lastIdx >= 0 && messagesToSend[lastIdx].Role == "user" && messagesToSend[lastIdx].UserName != "" {
		messagesToSend[lastIdx].Content = fmt.Sprintf("[%s]: %s", messagesToSend[lastIdx].UserName, messagesToSend[lastIdx].Content)
	}

	// Process file attachments (convert to text via LLM) — only for "custom" protocol.
	// A2A and ADK support native file parts, so attachments are passed through as-is.
	fileProcessor := h.currentFileProcessor(ctx)
	if agent.Protocol == "custom" && fileProcessor != nil && len(messagesToSend) > 0 {
		lastMsg := &messagesToSend[len(messagesToSend)-1]
		if len(lastMsg.Attachments) > 0 {
			if err := fileProcessor.ProcessAttachments(ctx, lastMsg); err != nil {
				h.logger.Error("failed to process attachments", zap.Error(err))
			}
		}
	}

	// Start tracing span for the chat request
	ctx, span := tracing.Tracer().Start(ctx, "chat",
		trace.WithAttributes(
			attribute.String("agent.id", agentID),
			attribute.String("agent.protocol", agent.Protocol),
			attribute.String("user.email", userEmail),
			attribute.String("session.id", session.SessionID),
			attribute.String("request.id", requestID),
			attribute.Int("message.count", len(messagesToSend)),
		),
	)
	defer span.End()

	// Record the full message payload as a span event
	if bodyJSON, err := json.Marshal(messagesToSend); err == nil {
		span.AddEvent("request.messages", trace.WithAttributes(
			attribute.String("body", string(bodyJSON)),
		))
	}

	// Build the request for the agent
	// Only send session ID if we have a mapped agent session.
	// Don't send our internal session ID - let the agent create its own.
	agentReq := &models.ChatRequest{
		Messages:  messagesToSend,
		SessionID: agentSessionID,
	}

	h.logger.Info("proxying chat request",
		zap.String("agent_id", agentID),
		zap.String("user", userEmail),
		zap.String("session_id", session.SessionID),
		zap.String("request_id", requestID),
		zap.Bool("is_multi_agent", session.IsMultiAgent),
		zap.Bool("has_agent_session", hasAgentSession),
		zap.Bool("context_sent", contextSent),
		zap.Int("session_messages", len(session.Messages)),
		zap.Int("message_count", len(messagesToSend)))

	// Debug: log messages being sent to agent
	for i, msg := range messagesToSend {
		h.logger.Debug("message to agent",
			zap.Int("index", i),
			zap.String("role", msg.Role),
			zap.String("agent_id_field", msg.AgentID),
			zap.Int("content_len", len(msg.Content)),
			zap.String("content_preview", truncate(msg.Content, 200)))
	}

	// Get auth header for forwarding
	authHeader := middleware.GetAuthHeaderFromContext(r.Context())

	// Get locale from Accept-Language header (e.g. "es", "en")
	locale := localeFromRequest(r)

	// 4. Proxy the request (always responds with SSE)
	// Pass session ID as thread ID so frontend receives it in RUN_STARTED
	requestStart := time.Now()
	if metrics.IsEnabled() {
		metrics.ActiveStreams.WithLabelValues("agent", agentID).Inc()
	}

	// Build OnEvent callback. Every AG-UI event is buffered into the session's
	// run stream so a client that reloads can reconnect and replay the run live.
	// Group sessions additionally broadcast to Redis Pub/Sub so the owner's open
	// clients stay in sync. Both are fire-and-forget: a buffering failure must not break
	// the run or the original client's SSE.
	sessionID := session.SessionID
	onEvent := func(event interface{}) {
		data, err := json.Marshal(event)
		if err != nil {
			return
		}
		if err := h.store.AppendRunEvent(context.Background(), sessionID, data); err != nil {
			h.logger.Debug("failed to buffer run event",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
		if session.GroupID != "" && h.hub != nil {
			if err := h.hub.Publish(context.Background(), sessionID, event); err != nil {
				h.logger.Debug("failed to publish event to pub/sub",
					zap.String("session_id", sessionID),
					zap.Error(err))
			}
		}
	}

	// Start Langfuse span for the agent proxy call
	var agentSpan *lf.Span
	if lfTrace != nil && lfTrace.IsEnabled() {
		// Build input with all messages sent to the agent (role + truncated content)
		msgs := make([]map[string]string, 0, len(messagesToSend))
		for _, m := range messagesToSend {
			msgs = append(msgs, map[string]string{
				"role":    m.Role,
				"content": truncate(m.Content, 1000),
			})
		}
		spanInput := map[string]interface{}{
			"agent_name":     agent.Name,
			"agent_protocol": agent.Protocol,
			"messages":       msgs,
		}
		agentSpan = lfTrace.StartToolCall(fmt.Sprintf("proxy:%s", agentID), spanInput)
	}

	// Mark this run as in-flight so a client that reloads can reconnect to the
	// live stream. Cleared once the run is done — crucially AFTER the assistant
	// reply is persisted (see post-proxy block), so the invariant holds: if no
	// active run exists, the full reply is already saved. Uses a background
	// context so cleanup runs even if the original client disconnected.
	sse, sseErr := proxy.NewSSEWriter(w)
	if sseErr != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}
	sse.Apply(proxy.SSEConfig{ThreadID: session.SessionID, SessionName: session.SessionName, OnEvent: onEvent})

	_ = h.store.SetActiveRun(ctx, session.SessionID, requestID)
	defer func() {
		clearCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.store.ClearActiveRun(clearCtx, session.SessionID, requestID)
	}()
	if err := sse.SendRunStarted(); err != nil {
		return
	}

	result, err := h.proxy.Handle(ctx, w, agent, agentReq, authHeader, proxy.HandleOptions{
		ThreadID:       session.SessionID,
		SessionName:    session.SessionName,
		Locale:         locale,
		RequestID:      requestID,
		UserEmail:      claims.GetEmail(),
		UserGroups:     claims.GetGroups(),
		OnEvent:        onEvent,
		DeferLifecycle: true,
		SSEWriter:      sse,
	})
	proxyErr = err

	// End agent proxy span with actual response text
	if agentSpan != nil {
		if proxyErr != nil {
			agentSpan.EndWithError(proxyErr)
		} else if result != nil {
			agentSpan.End(truncate(result.AssistantText, 2000))
		} else {
			agentSpan.End(nil)
		}
	}

	// End Langfuse trace with output
	if lfTrace != nil {
		success := proxyErr == nil
		var output interface{}
		if proxyErr != nil {
			output = proxyErr.Error()
		} else if result != nil && result.AssistantText != "" {
			output = truncate(result.AssistantText, 2000)
		}
		lfTrace.End(success, output)
	}

	durationMs := int(time.Since(requestStart).Milliseconds())
	if metrics.IsEnabled() {
		metrics.ActiveStreams.WithLabelValues("agent", agentID).Dec()
	}

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.logger.Error("proxy error",
			zap.String("agent_id", agentID),
			zap.Error(err))
		// Error already sent as SSE event from proxy
	}

	// Record chat event for observability
	{
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
		sessionRotated := result != nil && result.SessionRotated
		event := &models.ChatEvent{
			ResourceType:   "agent",
			ResourceID:     agentID,
			ResourceName:   agent.Name,
			Protocol:       agent.Protocol,
			UserEmail:      userEmail,
			SessionID:      session.SessionID,
			Status:         status,
			ErrorType:      errType,
			ErrorMsg:       errMsg,
			DurationMs:     durationMs,
			MessageCount:   len(messagesToSend),
			ToolCalls:      toolCallInfos,
			SessionRotated: sessionRotated,
		}
		recordChatEvent(h.chatEventRepo, event, h.logger)

		var auditResp string
		if result != nil {
			auditResp = proxy.TranscriptText(result)
		}
		recordAuditEvent(h.auditRepo, h.settings, &models.AuditEvent{
			UserEmail:    userEmail,
			ResourceType: models.AuditResourceAgent,
			ResourceID:   agentID,
			ResourceName: agent.Name,
			Source:       models.AuditSourceWeb,
			Action:       models.AuditActionChat,
			Prompt:       lastMessageContent(messagesToSend),
			Response:     auditResp,
			Status:       status,
			ErrorType:    errType,
			ErrorMsg:     errMsg,
			DurationMs:   durationMs,
		}, h.logger)
	}

	// Audit log
	h.audit.Log(userEmail, audit.ActionChat,
		zap.String("agent_id", agentID),
		zap.String("session_id", session.SessionID),
		zap.Bool("is_multi_agent", session.IsMultiAgent))

	// Log session rotation if it occurred
	if result != nil && result.SessionRotated {
		h.logger.Info("ADK session rotated due to context-limit",
			zap.String("agent_id", agentID),
			zap.String("session_id", session.SessionID),
			zap.String("new_agent_session_id", result.AgentSessionID))
	}

	// 5. Post-proxy: save assistant response and agent session mapping
	_, persistErr := h.persistTurnResult(context.Background(), session, agent, result, agentSessionID, hasAgentSession, locale, err)
	if persistErr != nil {
		h.logger.Error("failed to persist turn result", zap.Error(persistErr))
	}
	terminalErr := errors.Join(err, persistErr)
	if result != nil && result.Error != "" {
		terminalErr = errors.Join(terminalErr, errors.New(result.Error))
	}
	sse.Apply(proxy.SSEConfig{DeferLifecycle: false})
	if terminalErr != nil {
		_ = sse.SendRunError(proxy.PublicErrorMessage(terminalErr))
	} else {
		_ = sse.SendRunFinished()
	}

	// Async: generate a short LLM-based session name for new sessions
	sessionNamer := h.currentSessionNamer(ctx)
	if isNewSession && sessionNamer != nil && result != nil && result.AssistantText != "" {
		go func() {
			namerCtx := lf.ContextWithTrace(context.Background(), lfTrace)
			namerCtx, cancel := context.WithTimeout(namerCtx, 10*time.Second)
			defer cancel()
			name, err := sessionNamer.GenerateName(namerCtx, userMsg.Content, result.AssistantText)
			if err != nil {
				h.logger.Warn("session namer failed", zap.String("session_id", session.SessionID), zap.Error(err))
				return
			}
			if name != "" {
				if _, err := h.store.RenameSession(namerCtx, session.SessionID, name); err != nil {
					h.logger.Error("failed to rename session", zap.String("session_id", session.SessionID), zap.Error(err))
				}
			}
		}()
	}
}

// persistTurnResult saves the assistant response of one agent turn: the
// optional session-rotation info message, the assistant message (content
// parts, tool calls) and the agent session mapping. The caller chooses the
// parent context: callers detach it from client cancellation so finalization
// survives disconnects and apply the bounded timeout below. Returns the
// persisted assistant message plus any persistence/mapping error.
func (h *ProxyHandler) persistTurnResult(ctx context.Context, session *models.Session, agent *models.Agent, result *proxy.ProxyResult, agentSessionID string, hasAgentSession bool, locale string, proxyErr error) (*models.ChatMessage, error) {
	if !result.HasPersistableContent() {
		return nil, nil
	}
	agentID := agent.ID
	{
		saveCtx, saveCancel := context.WithTimeout(ctx, 5*time.Second)
		defer saveCancel()
		var persistErr error

		// Persist session rotation info message so it survives page refresh
		if result.SessionRotated {
			agentName := agent.Name
			if agentName == "" {
				agentName = agentID
			}
			var rotationMsg string
			if locale == "es" {
				rotationMsg = fmt.Sprintf("\u23f3 Session limit reached for agent %s. Resuming with context...", agentName)
			} else {
				rotationMsg = fmt.Sprintf("\u23f3 Session limit reached for agent %s. Resuming with context...", agentName)
			}
			infoMsg := models.ChatMessage{
				Role:    "assistant",
				Content: rotationMsg,
			}
			if session.IsMultiAgent {
				infoMsg.AgentID = agentID
			}
			if err := h.store.AddMessage(saveCtx, session.SessionID, infoMsg); err != nil {
				h.logger.Error("failed to save session rotation info message", zap.Error(err))
				persistErr = errors.Join(persistErr, orchestrator.NewTurnError(orchestrator.TurnFailurePersistence, fmt.Errorf("rotation message persistence failed: %w", err)))
			}
		}

		rawResultErr := proxyErr
		if result.Error != "" {
			rawResultErr = errors.Join(rawResultErr, errors.New(result.Error))
		}
		assistantMsg := result.ToChatMessage("", proxy.PublicErrorMessage(rawResultErr))
		if session.IsMultiAgent {
			assistantMsg.AgentID = agentID
		}
		assistantPersisted := true
		if err := h.store.AddMessage(saveCtx, session.SessionID, assistantMsg); err != nil {
			h.logger.Error("failed to save assistant message", zap.Error(err))
			persistErr = errors.Join(persistErr, orchestrator.NewTurnError(orchestrator.TurnFailurePersistence, fmt.Errorf("reply persistence failed: %w", err)))
			assistantPersisted = false
		}

		// Save agent session mapping only on success — don't map sessions for
		// failed requests so retries start fresh without a stale session ID.
		if rawResultErr == nil && assistantPersisted {
			if result.AgentSessionID != "" && result.AgentSessionID != agentSessionID {
				if err := h.store.SetAgentSessionID(saveCtx, session.SessionID, agentID, result.AgentSessionID); err != nil {
					h.logger.Error("failed to save agent session mapping", zap.Error(err))
					persistErr = errors.Join(persistErr, orchestrator.NewTurnError(orchestrator.TurnFailureMapping, fmt.Errorf("session mapping persistence failed: %w", err)))
				}
			} else if !hasAgentSession {
				// No agent session ID returned - map our session ID
				if err := h.store.SetAgentSessionID(saveCtx, session.SessionID, agentID, session.SessionID); err != nil {
					h.logger.Error("failed to save agent session mapping", zap.Error(err))
					persistErr = errors.Join(persistErr, orchestrator.NewTurnError(orchestrator.TurnFailureMapping, fmt.Errorf("session mapping persistence failed: %w", err)))
				}
			}
		}

		if !assistantPersisted {
			return nil, persistErr
		}
		return &assistantMsg, persistErr
	}
}

// maxMessageContentBytes is the maximum allowed size for a single message content (100KB).
const maxMessageContentBytes = 100 * 1024

// maxMessages is the maximum number of messages allowed in a single chat request.
const maxMessages = 500

// validRoles contains the allowed values for ChatMessage.Role.
var validRoles = map[string]bool{
	"user":      true,
	"assistant": true,
	"system":    true,
}

// validateChatRequest validates the chat request payload and returns an error
// message string. An empty string means the request is valid.
func validateChatRequest(req *models.ChatRequest) string {
	if len(req.Messages) == 0 {
		return "messages must not be empty"
	}
	if len(req.Messages) > maxMessages {
		return fmt.Sprintf("messages count exceeds maximum of %d", maxMessages)
	}

	for i, msg := range req.Messages {
		if !validRoles[msg.Role] {
			return fmt.Sprintf("messages[%d].role must be one of: user, assistant, system", i)
		}
		if len(msg.Content) > maxMessageContentBytes {
			return fmt.Sprintf("messages[%d].content exceeds maximum size of %d bytes", i, maxMessageContentBytes)
		}
	}

	if req.SessionID != "" {
		if _, err := uuid.Parse(req.SessionID); err != nil {
			return "session_id must be a valid UUID"
		}
	}

	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

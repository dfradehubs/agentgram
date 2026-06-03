package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/dfradehubs/agentgram-api/internal/audit"
	lf "github.com/dfradehubs/agentgram-api/internal/langfuse"
	"github.com/dfradehubs/agentgram-api/internal/llm"
	"github.com/dfradehubs/agentgram-api/internal/mcp"
	"github.com/dfradehubs/agentgram-api/internal/metrics"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/sessionnamer"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"go.uber.org/zap"
)

// MCPHandler handles MCP server endpoints
type MCPHandler struct {
	registry       *mcp.Registry
	orchestrator   *mcp.ChatOrchestrator
	store          store.SessionStore
	sessionNamer   *sessionnamer.Namer
	audit          *audit.Logger
	langfuseTracer *lf.Tracer
	chatEventRepo  repository.ChatEventRepository
	oauth2Mgr      *mcp.OAuth2Manager
	mcpRepo        repository.MCPServerRepository
	logger         *zap.Logger
}

// NewMCPHandler creates a new MCP handler
func NewMCPHandler(llmRepo repository.LLMModelRepository, registry *mcp.Registry, sessionStore store.SessionStore, maxToolCallRounds int, auditLogger *audit.Logger, logger *zap.Logger, lfTracer *lf.Tracer, oauth2Mgr *mcp.OAuth2Manager, mcpRepo repository.MCPServerRepository, chatEventRepo ...repository.ChatEventRepository) *MCPHandler {
	var namer *sessionnamer.Namer
	ctx := context.Background()
	if namerModels, err := llmRepo.ListByRole(ctx, "session_namer"); err == nil && len(namerModels) > 0 {
		model := namerModels[0]
		provider, provErr := llm.NewProvider(model)
		if provErr == nil && lfTracer != nil && lfTracer.Enabled() {
			provider = lf.WrapProvider(provider, "session-namer", model.Model)
		}
		if provErr == nil {
			namer = sessionnamer.NewWithProvider(provider, logger)
		} else {
			namer = sessionnamer.New(model, logger)
		}
	}

	h := &MCPHandler{
		registry:       registry,
		orchestrator:   mcp.NewChatOrchestrator(llmRepo, maxToolCallRounds, logger),
		store:          sessionStore,
		sessionNamer:   namer,
		audit:          auditLogger,
		langfuseTracer: lfTracer,
		oauth2Mgr:      oauth2Mgr,
		mcpRepo:        mcpRepo,
		logger:         logger,
	}
	if len(chatEventRepo) > 0 {
		h.chatEventRepo = chatEventRepo[0]
	}
	return h
}

// MCPServerResponse is the public representation of an MCP server
type MCPServerResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Transport     string `json:"transport"`
	Status        string `json:"status"`
	StatusError   string `json:"status_error,omitempty"`
	ToolCount     int    `json:"tool_count"`
	AuthType      string `json:"auth_type,omitempty"`
	OAuth2Connected bool  `json:"oauth2_connected,omitempty"`
}

func serverToResponse(s *mcp.ServerInfo) MCPServerResponse {
	status, statusErr := s.GetStatus()
	tools := s.GetTools()
	return MCPServerResponse{
		ID:          s.Config.ID,
		Name:        s.Config.Name,
		Description: s.Config.Description,
		Transport:   s.Config.Transport,
		Status:      status,
		StatusError: statusErr,
		ToolCount:   len(tools),
		AuthType:    s.Config.AuthType,
	}
}

// resolveExtraHeaders builds the Authorization header for a given MCP server+user.
// For forward_auth: forwards the user's JWT.
// For oauth2: retrieves the user's OAuth2 token from the store.
// Returns nil headers and "oauth2_consent_required" error string if the user needs to authorize.
func (h *MCPHandler) resolveExtraHeaders(ctx context.Context, server *mcp.ServerInfo, userEmail string) (map[string]string, string) {
	if server.Config.ForwardAuth {
		authHeader := middleware.GetAuthHeaderFromContext(ctx)
		if authHeader != "" {
			return map[string]string{"Authorization": authHeader}, ""
		}
		return nil, ""
	}

	if server.Config.IsOAuth2() && h.oauth2Mgr != nil && h.mcpRepo != nil {
		mcpServer, err := h.mcpRepo.Get(ctx, server.Config.ID)
		if err != nil {
			h.logger.Warn("failed to get MCP server config for OAuth2", zap.String("id", server.Config.ID), zap.Error(err))
			return nil, ""
		}
		token, err := h.oauth2Mgr.GetToken(ctx, mcpServer, userEmail)
		if err != nil {
			h.logger.Warn("MCP OAuth2 token error", zap.String("id", server.Config.ID), zap.Error(err))
		}
		if token == nil {
			return nil, "oauth2_consent_required"
		}
		return map[string]string{"Authorization": "Bearer " + token.AccessToken}, ""
	}

	return nil, ""
}

// ListServers handles GET /api/mcp/servers
func (h *MCPHandler) ListServers(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userEmail := claims.GetEmail()
	userGroups := claims.GetGroups()
	servers := h.registry.List()

	// Filter accessible servers
	var accessible []*mcp.ServerInfo
	for _, s := range servers {
		if !mcp.HasAccess(s, userEmail, userGroups) {
			h.logger.Debug("MCP server hidden from user",
				zap.String("mcp_id", s.Config.ID),
				zap.String("user_email", userEmail),
				zap.Strings("user_groups", userGroups),
				zap.Strings("allowed_users", s.Config.AllowedUsers),
				zap.Strings("allowed_groups", s.Config.AllowedGroups))
			continue
		}
		accessible = append(accessible, s)
	}

	// Lazy-initialize servers that need per-user auth and aren't connected yet.
	// Run in parallel so we don't block longer than the slowest server.
	{
		var wg sync.WaitGroup
		for _, s := range accessible {
			if (s.Config.ForwardAuth || s.Config.IsOAuth2()) && !s.Client.IsInitialized() {
				wg.Add(1)
				go func(info *mcp.ServerInfo) {
					defer wg.Done()
					headers, _ := h.resolveExtraHeaders(r.Context(), info, userEmail)
					if headers == nil {
						return
					}
					if err := h.registry.EnsureInitialized(info, headers); err != nil {
						h.logger.Warn("per-user auth init on ListServers failed",
							zap.String("server_id", info.Config.ID),
							zap.Error(err))
					}
				}(s)
			}
		}
		wg.Wait()
	}

	result := make([]MCPServerResponse, 0, len(accessible))
	for _, s := range accessible {
		resp := serverToResponse(s)
		if s.Config.IsOAuth2() && h.oauth2Mgr != nil && h.mcpRepo != nil {
			mcpServer, err := h.mcpRepo.Get(r.Context(), s.Config.ID)
			if err == nil {
				token, _ := h.oauth2Mgr.GetToken(r.Context(), mcpServer, userEmail)
				resp.OAuth2Connected = token != nil
			}
		}
		result = append(result, resp)
	}

	h.logger.Debug("MCP servers listed",
		zap.String("user_email", userEmail),
		zap.Strings("user_groups", userGroups),
		zap.Int("total_servers", len(servers)),
		zap.Int("accessible_servers", len(result)))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"servers": result})
}

// ListTools handles GET /api/mcp/servers/{id}/tools
func (h *MCPHandler) ListTools(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	server, err := h.registry.Get(serverID)
	if err != nil {
		writeJSONError(w, "server not found", http.StatusNotFound)
		return
	}

	if !mcp.HasAccess(server, claims.GetEmail(), claims.GetGroups()) {
		writeJSONError(w, "access denied", http.StatusForbidden)
		return
	}

	if server.Config.ForwardAuth || server.Config.IsOAuth2() {
		extraHeaders, authErr := h.resolveExtraHeaders(r.Context(), server, claims.GetEmail())
		if authErr != "" {
			writeJSONError(w, authErr, http.StatusForbidden)
			return
		}
		if err := h.registry.EnsureInitialized(server, extraHeaders); err != nil {
			h.logger.Warn("MCP lazy-init failed on ListTools",
				zap.String("server_id", serverID),
				zap.Error(err))
		}
	}

	// Return cached tools from registry
	tools := server.GetTools()
	if tools == nil {
		tools = []mcp.Tool{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"tools": tools})
}

// Reconnect handles POST /api/mcp/servers/{id}/reconnect
func (h *MCPHandler) Reconnect(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	server, err := h.registry.Get(serverID)
	if err != nil {
		writeJSONError(w, "server not found", http.StatusNotFound)
		return
	}

	if !mcp.HasAccess(server, claims.GetEmail(), claims.GetGroups()) {
		writeJSONError(w, "access denied", http.StatusForbidden)
		return
	}

	// Build extra headers for authenticated servers
	extraHeaders, authErr := h.resolveExtraHeaders(r.Context(), server, claims.GetEmail())
	if authErr != "" {
		writeJSONError(w, authErr, http.StatusForbidden)
		return
	}

	info, err := h.registry.Reconnect(serverID, extraHeaders)
	if err != nil {
		h.logger.Warn("MCP reconnect failed", zap.String("server_id", serverID), zap.Error(err))
		// Still return the server info with error status
		if info != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(serverToResponse(info))
			return
		}
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(serverToResponse(info))
}

// Chat handles POST /api/mcp/servers/{id}/chat
func (h *MCPHandler) Chat(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	server, err := h.registry.Get(serverID)
	if err != nil {
		writeJSONError(w, "server not found", http.StatusNotFound)
		return
	}

	userEmail := claims.GetEmail()
	if !mcp.HasAccess(server, userEmail, claims.GetGroups()) {
		writeJSONError(w, "access denied", http.StatusForbidden)
		return
	}

	// Build extra headers for authenticated MCP servers
	extraHeaders, authErr := h.resolveExtraHeaders(r.Context(), server, userEmail)
	if authErr != "" {
		writeJSONError(w, authErr, http.StatusForbidden)
		return
	}
	if server.Config.ForwardAuth || server.Config.IsOAuth2() {
		if err := h.registry.EnsureInitialized(server, extraHeaders); err != nil {
			h.logger.Error("MCP lazy-init failed",
				zap.String("server_id", serverID),
				zap.Error(err))
			writeJSONError(w, "MCP server connection failed: "+err.Error(), http.StatusBadGateway)
			return
		}
	}

	var req mcp.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Messages) == 0 {
		writeJSONError(w, "messages required", http.StatusBadRequest)
		return
	}

	h.audit.Log(userEmail, audit.ActionMCPChat,
		zap.String("mcp_id", serverID),
		zap.String("model_id", req.ModelID))

	sessionKey := "mcp:" + serverID

	// Create or load session (with ownership verification)
	sessionID := req.SessionID
	isNewSession := false
	if sessionID == "" {
		isNewSession = true
		session, err := h.store.CreateSession(r.Context(), userEmail, sessionKey, firstMessagePreview(req.Messages))
		if err != nil {
			h.logger.Error("failed to create MCP session", zap.Error(err))
			writeJSONError(w, "failed to create session", http.StatusInternalServerError)
			return
		}
		sessionID = session.SessionID
	} else {
		// Verify the session belongs to the authenticated user
		session, err := h.store.GetSession(r.Context(), sessionID)
		if err != nil || session == nil {
			writeJSONError(w, "session not found", http.StatusNotFound)
			return
		}
		if session.UserID != userEmail {
			writeJSONError(w, "access denied", http.StatusForbidden)
			return
		}
	}

	// Start Langfuse trace for MCP chat
	ctx := r.Context()
	requestID := chiMiddleware.GetReqID(ctx)
	var lfTrace *lf.Trace
	if h.langfuseTracer != nil && h.langfuseTracer.Enabled() {
		lfTrace = h.langfuseTracer.StartTrace(ctx, "mcp-chat", userEmail, sessionID, map[string]interface{}{
			"mcp_server_id":   serverID,
			"mcp_server_name": server.Config.Name,
			"model_id":        req.ModelID,
			"request_id":      requestID,
		})
		if len(req.Messages) > 0 {
			lastMsg := req.Messages[len(req.Messages)-1]
			if lastMsg.Role == "user" {
				lfTrace.SetInput(truncate(lastMsg.Content, 1000))
			}
		}
		ctx = lf.ContextWithTrace(ctx, lfTrace)
	}

	requestStart := time.Now()
	if metrics.IsEnabled() {
		metrics.ActiveStreams.WithLabelValues("mcp", serverID).Inc()
	}

	runResult, err := h.orchestrator.Chat(ctx, w, server, &req, sessionID, h.store, extraHeaders)

	durationMs := int(time.Since(requestStart).Milliseconds())
	if metrics.IsEnabled() {
		metrics.ActiveStreams.WithLabelValues("mcp", serverID).Dec()
	}

	// End Langfuse trace
	if lfTrace != nil {
		success := err == nil && (runResult == nil || runResult.Error == "")
		var output interface{}
		if err != nil {
			output = err.Error()
		} else if runResult != nil && runResult.AssistantText != "" {
			output = truncate(runResult.AssistantText, 2000)
		}
		lfTrace.End(success, output)
	}

	if err != nil {
		h.logger.Error("MCP chat error",
			zap.String("server_id", serverID),
			zap.Error(err))
	}

	// Record chat event
	{
		status := "ok"
		var errType, errMsg string
		if err != nil {
			status = "error"
			errMsg = err.Error()
			errType = classifyError(errMsg)
		} else if runResult != nil && runResult.Error != "" {
			status = "error"
			errMsg = runResult.Error
			errType = classifyError(errMsg)
		}
		var toolCallInfos []models.ToolCallInfo
		var tokenUsage *models.TokenUsage
		if runResult != nil {
			for _, tc := range runResult.ToolCalls {
				toolCallInfos = append(toolCallInfos, models.ToolCallInfo{Name: tc.Name, DurationMs: tc.DurationMs})
			}
			if runResult.TokenUsage != nil && runResult.TokenUsage.Total > 0 {
				tokenUsage = &models.TokenUsage{
					Input:  runResult.TokenUsage.Input,
					Output: runResult.TokenUsage.Output,
					Total:  runResult.TokenUsage.Total,
				}
			}
		}
		event := &models.ChatEvent{
			ResourceType: "mcp",
			ResourceID:   serverID,
			ResourceName: server.Config.Name,
			UserEmail:    userEmail,
			SessionID:    sessionID,
			Status:       status,
			ErrorType:    errType,
			ErrorMsg:     errMsg,
			DurationMs:   durationMs,
			MessageCount: len(req.Messages),
			ToolCalls:    toolCallInfos,
			TokenUsage:   tokenUsage,
			LLMModel:     req.ModelID,
		}
		recordChatEvent(h.chatEventRepo, event, h.logger)
	}

	// Async: generate a short LLM-based session name for new sessions
	if isNewSession && h.sessionNamer != nil && runResult != nil && runResult.AssistantText != "" {
		userContent := ""
		for _, m := range req.Messages {
			if m.Role == "user" && m.Content != "" {
				userContent = m.Content
			}
		}
		go func() {
			namerCtx := lf.ContextWithTrace(context.Background(), lfTrace)
			namerCtx, cancel := context.WithTimeout(namerCtx, 10*time.Second)
			defer cancel()
			name, err := h.sessionNamer.GenerateName(namerCtx, userContent, runResult.AssistantText)
			if err != nil {
				h.logger.Warn("session namer failed", zap.String("session_id", sessionID), zap.Error(err))
				return
			}
			if name != "" {
				if _, err := h.store.RenameSession(namerCtx, sessionID, name); err != nil {
					h.logger.Error("failed to rename session", zap.String("session_id", sessionID), zap.Error(err))
				}
			}
		}()
	}
}

// ChatMulti handles POST /api/mcp/chat (multi-server)
func (h *MCPHandler) ChatMulti(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req mcp.MultiChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Messages) == 0 {
		writeJSONError(w, "messages required", http.StatusBadRequest)
		return
	}
	if len(req.ServerIDs) < 2 {
		writeJSONError(w, "at least 2 server_ids required", http.StatusBadRequest)
		return
	}

	userEmail := claims.GetEmail()

	// Build extra headers per server (forward_auth or oauth2)
	var extraHeaders map[string]string

	// Validate access to all servers
	var servers []*mcp.ServerInfo
	for _, sid := range req.ServerIDs {
		server, err := h.registry.Get(sid)
		if err != nil {
			writeJSONError(w, "server not found: "+sid, http.StatusNotFound)
			return
		}
		if !mcp.HasAccess(server, userEmail, claims.GetGroups()) {
			writeJSONError(w, "access denied to: "+sid, http.StatusForbidden)
			return
		}

		// Lazy-initialize servers that require per-user auth
		if server.Config.ForwardAuth || server.Config.IsOAuth2() {
			headers, authErr := h.resolveExtraHeaders(r.Context(), server, userEmail)
			if authErr != "" {
				writeJSONError(w, authErr, http.StatusForbidden)
				return
			}
			if extraHeaders == nil {
				extraHeaders = headers
			}
			if err := h.registry.EnsureInitialized(server, headers); err != nil {
				h.logger.Error("MCP lazy-init failed",
					zap.String("server_id", sid),
					zap.Error(err))
				writeJSONError(w, "MCP server connection failed: "+sid, http.StatusBadGateway)
				return
			}
		}

		servers = append(servers, server)
	}

	h.audit.Log(userEmail, audit.ActionMCPChat,
		zap.Strings("mcp_ids", req.ServerIDs),
		zap.String("model_id", req.ModelID))

	sessionID := req.SessionID
	isNewMultiSession := false
	if sessionID == "" {
		isNewMultiSession = true
		session, err := h.store.CreateSession(r.Context(), userEmail, "mcp:multi", firstMessagePreview(req.Messages))
		if err != nil {
			h.logger.Error("failed to create multi-MCP session", zap.Error(err))
			writeJSONError(w, "failed to create session", http.StatusInternalServerError)
			return
		}
		// Store server IDs so the session can be reopened later
		session.AgentIDs = req.ServerIDs
		session.IsMultiAgent = true
		if err := h.store.SaveSession(r.Context(), session); err != nil {
			h.logger.Error("failed to save multi-MCP session metadata", zap.Error(err))
		}
		sessionID = session.SessionID
	} else {
		// Verify the session belongs to the authenticated user
		session, err := h.store.GetSession(r.Context(), sessionID)
		if err != nil || session == nil {
			writeJSONError(w, "session not found", http.StatusNotFound)
			return
		}
		if session.UserID != userEmail {
			writeJSONError(w, "access denied", http.StatusForbidden)
			return
		}
	}

	// Start Langfuse trace for multi-MCP chat
	ctx := r.Context()
	requestID := chiMiddleware.GetReqID(ctx)
	var lfTrace *lf.Trace
	if h.langfuseTracer != nil && h.langfuseTracer.Enabled() {
		lfTrace = h.langfuseTracer.StartTrace(ctx, "mcp-chat-multi", userEmail, sessionID, map[string]interface{}{
			"mcp_server_ids": req.ServerIDs,
			"model_id":       req.ModelID,
			"request_id":     requestID,
		})
		if len(req.Messages) > 0 {
			lastMsg := req.Messages[len(req.Messages)-1]
			if lastMsg.Role == "user" {
				lfTrace.SetInput(truncate(lastMsg.Content, 1000))
			}
		}
		ctx = lf.ContextWithTrace(ctx, lfTrace)
	}

	requestStart := time.Now()
	resourceID := "multi:" + strings.Join(req.ServerIDs, ",")
	if metrics.IsEnabled() {
		metrics.ActiveStreams.WithLabelValues("mcp", resourceID).Inc()
	}

	runResult, err := h.orchestrator.ChatMulti(ctx, w, servers, &req, sessionID, h.store, extraHeaders)

	durationMs := int(time.Since(requestStart).Milliseconds())
	if metrics.IsEnabled() {
		metrics.ActiveStreams.WithLabelValues("mcp", resourceID).Dec()
	}

	// End Langfuse trace
	if lfTrace != nil {
		success := err == nil && (runResult == nil || runResult.Error == "")
		var output interface{}
		if err != nil {
			output = err.Error()
		} else if runResult != nil && runResult.AssistantText != "" {
			output = truncate(runResult.AssistantText, 2000)
		}
		lfTrace.End(success, output)
	}

	if err != nil {
		h.logger.Error("Multi-MCP chat error", zap.Error(err))
	}

	// Record chat event
	{
		status := "ok"
		var errType, errMsg string
		if err != nil {
			status = "error"
			errMsg = err.Error()
			errType = classifyError(errMsg)
		} else if runResult != nil && runResult.Error != "" {
			status = "error"
			errMsg = runResult.Error
			errType = classifyError(errMsg)
		}
		var toolCallInfos []models.ToolCallInfo
		var tokenUsage *models.TokenUsage
		if runResult != nil {
			for _, tc := range runResult.ToolCalls {
				toolCallInfos = append(toolCallInfos, models.ToolCallInfo{Name: tc.Name, DurationMs: tc.DurationMs})
			}
			if runResult.TokenUsage != nil && runResult.TokenUsage.Total > 0 {
				tokenUsage = &models.TokenUsage{
					Input:  runResult.TokenUsage.Input,
					Output: runResult.TokenUsage.Output,
					Total:  runResult.TokenUsage.Total,
				}
			}
		}
		event := &models.ChatEvent{
			ResourceType: "mcp",
			ResourceID:   resourceID,
			ResourceName: "Multi-MCP",
			UserEmail:    userEmail,
			SessionID:    sessionID,
			Status:       status,
			ErrorType:    errType,
			ErrorMsg:     errMsg,
			DurationMs:   durationMs,
			MessageCount: len(req.Messages),
			ToolCalls:    toolCallInfos,
			TokenUsage:   tokenUsage,
			LLMModel:     req.ModelID,
		}
		recordChatEvent(h.chatEventRepo, event, h.logger)
	}

	// Async: generate a short LLM-based session name for new sessions
	if isNewMultiSession && h.sessionNamer != nil && runResult != nil && runResult.AssistantText != "" {
		userContent := ""
		for _, m := range req.Messages {
			if m.Role == "user" && m.Content != "" {
				userContent = m.Content
			}
		}
		go func() {
			namerCtx := lf.ContextWithTrace(context.Background(), lfTrace)
			namerCtx, cancel := context.WithTimeout(namerCtx, 10*time.Second)
			defer cancel()
			name, err := h.sessionNamer.GenerateName(namerCtx, userContent, runResult.AssistantText)
			if err != nil {
				h.logger.Warn("session namer failed", zap.String("session_id", sessionID), zap.Error(err))
				return
			}
			if name != "" {
				if _, err := h.store.RenameSession(namerCtx, sessionID, name); err != nil {
					h.logger.Error("failed to rename session", zap.String("session_id", sessionID), zap.Error(err))
				}
			}
		}()
	}
}

// ListSessions handles GET /api/mcp/servers/{id}/sessions
func (h *MCPHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sessions, err := h.store.ListSessions(r.Context(), claims.GetEmail(), "mcp:"+serverID)
	if err != nil {
		writeJSONError(w, "failed to list sessions", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"sessions": sessions})
}

// GetSession handles GET /api/mcp/servers/{id}/sessions/{sid}
func (h *MCPHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sid")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	session, err := h.store.GetSession(r.Context(), sessionID)
	if err != nil {
		writeJSONError(w, "session not found", http.StatusNotFound)
		return
	}

	if session.UserID != claims.GetEmail() {
		writeJSONError(w, "access denied", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(session)
}

// RenameSession handles PATCH /api/mcp/servers/{id}/sessions/{sid}
func (h *MCPHandler) RenameSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sid")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var body struct {
		SessionName string `json:"session_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SessionName == "" {
		writeJSONError(w, "session_name required", http.StatusBadRequest)
		return
	}

	// Verify ownership before renaming
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

	session, err := h.store.RenameSession(r.Context(), sessionID, body.SessionName)
	if err != nil {
		writeJSONError(w, "failed to rename session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessionMetadataResponse(session))
}

// DeleteSession handles DELETE /api/mcp/servers/{id}/sessions/{sid}
func (h *MCPHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	sessionID := chi.URLParam(r, "sid")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify ownership before deleting
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

	if err := h.store.DeleteSession(r.Context(), sessionID, claims.GetEmail(), "mcp:"+serverID); err != nil {
		writeJSONError(w, "failed to delete session", http.StatusInternalServerError)
		return
	}

	h.audit.Log(claims.GetEmail(), audit.ActionSessionDelete,
		zap.String("mcp_id", serverID),
		zap.String("session_id", sessionID))

	w.WriteHeader(http.StatusNoContent)
}

// ListMultiMCPSessions handles GET /api/mcp/sessions
func (h *MCPHandler) ListMultiMCPSessions(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sessions, err := h.store.ListSessions(r.Context(), claims.GetEmail(), "mcp:multi")
	if err != nil {
		writeJSONError(w, "failed to list sessions", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"sessions": sessions})
}

// GetMultiMCPSession handles GET /api/mcp/sessions/{sid}
func (h *MCPHandler) GetMultiMCPSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sid")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	session, err := h.store.GetSession(r.Context(), sessionID)
	if err != nil {
		writeJSONError(w, "session not found", http.StatusNotFound)
		return
	}

	if session.UserID != claims.GetEmail() {
		writeJSONError(w, "access denied", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(session)
}

// RenameMultiMCPSession handles PATCH /api/mcp/sessions/{sid}
func (h *MCPHandler) RenameMultiMCPSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sid")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var body struct {
		SessionName string `json:"session_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SessionName == "" {
		writeJSONError(w, "session_name required", http.StatusBadRequest)
		return
	}

	// Verify ownership before renaming
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

	session, err := h.store.RenameSession(r.Context(), sessionID, body.SessionName)
	if err != nil {
		writeJSONError(w, "failed to rename session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessionMetadataResponse(session))
}

// DeleteMultiMCPSession handles DELETE /api/mcp/sessions/{sid}
func (h *MCPHandler) DeleteMultiMCPSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sid")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify ownership before deleting
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

	if err := h.store.DeleteSession(r.Context(), sessionID, claims.GetEmail(), "mcp:multi"); err != nil {
		writeJSONError(w, "failed to delete session", http.StatusInternalServerError)
		return
	}

	h.audit.Log(claims.GetEmail(), audit.ActionSessionDelete,
		zap.String("session_id", sessionID))

	w.WriteHeader(http.StatusNoContent)
}

// firstMessagePreview returns a short preview from the first user message for naming sessions
func firstMessagePreview(messages []mcp.ChatMessage) string {
	for _, m := range messages {
		if m.Role == "user" && m.Content != "" {
			name := m.Content
			if len(name) > 50 {
				name = name[:50] + "..."
			}
			return name
		}
	}
	return "MCP Chat"
}

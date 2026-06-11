package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"strings"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/config"
	"github.com/dfradehubs/agentgram-api/internal/identity"
	lf "github.com/dfradehubs/agentgram-api/internal/langfuse"
	"github.com/dfradehubs/agentgram-api/internal/mcp"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/proxy"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/service"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"go.uber.org/zap"
)

// Handler is the HTTP handler for MCP protocol requests
type Handler struct {
	server         *Server
	registry       *agents.Registry
	mcpRegistry    *mcp.Registry
	proxy          *proxy.Proxy
	sessionStore   store.SessionStore
	userService    *service.UserService
	groupRepo      repository.GroupRepository
	oidcClient     *auth.OIDCClient
	langfuseTracer *lf.Tracer
	cfg            *config.Config
	oauth2Mgr      *mcp.OAuth2Manager
	mcpRepo        repository.MCPServerRepository
	logger         *zap.Logger
}

// NewHandler creates a new MCP HTTP handler
func NewHandler(
	registry *agents.Registry,
	mcpRegistry *mcp.Registry,
	sessionStore store.SessionStore,
	userService *service.UserService,
	groupRepo repository.GroupRepository,
	oidcClient *auth.OIDCClient,
	cfg *config.Config,
	logger *zap.Logger,
	lfTracer *lf.Tracer,
	oauth2Mgr *mcp.OAuth2Manager,
	mcpRepo repository.MCPServerRepository,
) *Handler {
	return &Handler{
		server:         NewServer(registry, mcpRegistry, userService, logger),
		registry:       registry,
		mcpRegistry:    mcpRegistry,
		proxy:          proxy.NewProxy(logger),
		sessionStore:   sessionStore,
		userService:    userService,
		groupRepo:      groupRepo,
		oidcClient:     oidcClient,
		langfuseTracer: lfTracer,
		cfg:            cfg,
		oauth2Mgr:      oauth2Mgr,
		mcpRepo:        mcpRepo,
		logger:         logger,
	}
}

// HandleMCP handles POST /mcp — the main MCP protocol endpoint
func (h *Handler) HandleMCP(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		h.writeUnauthorized(w)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, h.server.MarshalError(nil, errCodeParse, "failed to read body"))
		return
	}

	userEmail := claims.GetEmail()
	userGroups := claims.GetGroups()
	mcpSessionID := r.Header.Get("Mcp-Session-Id")

	// Parse to check if this is a tools/call — we handle it specially
	var req jsonRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.writeJSON(w, http.StatusOK, h.server.MarshalError(nil, errCodeParse, "parse error"))
		return
	}

	if req.Method == "tools/call" {
		h.handleToolsCall(w, r, req, userEmail, userGroups, mcpSessionID)
		return
	}

	// All other methods handled by the server
	resp, newSessionID, err := h.server.HandleMessage(body, userEmail, userGroups, mcpSessionID)
	if err != nil {
		h.logger.Error("MCP message handling error", zap.Error(err))
		h.writeJSON(w, http.StatusInternalServerError, h.server.MarshalError(nil, errCodeInternal, "internal error"))
		return
	}

	if newSessionID != "" {
		w.Header().Set("Mcp-Session-Id", newSessionID)
	}

	// Notifications don't get a response
	if resp == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// HandleSSE handles GET /mcp — SSE stream for server-to-client notifications.
// MCP Streamable HTTP transport requires this endpoint. We keep the connection
// open but don't send any events (we don't use server-initiated notifications).
func (h *Handler) HandleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Keep connection open until client disconnects
	flusher, ok := w.(http.Flusher)
	if ok {
		flusher.Flush()
	}
	<-r.Context().Done()
}

// HandleSessionTerminate handles DELETE /mcp — terminates the MCP session
func (h *Handler) HandleSessionTerminate(w http.ResponseWriter, r *http.Request) {
	// MCP spec: DELETE terminates the session
	w.WriteHeader(http.StatusOK)
}

// mcpBaseScopes is the scope set always advertised to MCP clients via both
// oauth-protected-resource (RFC 9728) and oauth-authorization-server (RFC 8414).
// Kept in one place so both metadata endpoints stay in sync — clients rely on
// the AS metadata for scopes_supported, so any drift breaks authorization UX.
// These are required for the gateway to work: groups drives permissions and
// email drives identity. Deployments add their own scopes via
// mcp_server.extra_scopes (see supportedScopes).
var mcpBaseScopes = []string{"openid", "profile", "email", "groups", "offline_access"}

// supportedScopes returns the scopes advertised to MCP clients: the required
// base set plus any deployment-specific extras from mcp_server.extra_scopes.
// The common extra is a Keycloak client scope carrying an audience mapper
// (e.g. "mcp:custom-audience") so that strict clients like Claude — which only
// request scopes advertised in the metadata — get a token whose `aud` the
// upstream agent (ADK) accepts. Duplicates are dropped so a redundant base
// scope in config doesn't appear twice.
func (h *Handler) supportedScopes() []string {
	var extra []string
	if h.cfg != nil {
		extra = h.cfg.MCPServer.ExtraScopes
	}
	scopes := make([]string, 0, len(mcpBaseScopes)+len(extra))
	seen := make(map[string]struct{}, len(mcpBaseScopes)+len(extra))
	for _, s := range append(append([]string{}, mcpBaseScopes...), extra...) {
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		scopes = append(scopes, s)
	}
	return scopes
}

// authServerMetadataTimeout bounds the upstream fetch to the Keycloak openid-configuration.
const authServerMetadataTimeout = 5 * time.Second

func (h *Handler) getIssuer() string {
	issuer := h.cfg.MCPServer.Issuer
	if issuer == "" {
		issuer = h.cfg.Auth.Keycloak.Issuer
	}

	if issuer == "" {
		return ""
	}

	// Ensure the issuer has an absolute scheme to pass SDK URL parsing checks
	if !strings.HasPrefix(issuer, "http://") && !strings.HasPrefix(issuer, "https://") {
		if strings.Contains(issuer, "localhost") || strings.Contains(issuer, "127.0.0.1") {
			issuer = "http://" + issuer
		} else {
			issuer = "https://" + issuer
		}
	}

	return issuer
}

// HandleResourceMetadata handles GET /.well-known/oauth-protected-resource and the
// path-based variants (e.g. /.well-known/oauth-protected-resource/mcp) per RFC 9728.
// The resource identifier stays at the host root for backwards compatibility:
// existing clients have tokens whose audience matches "https://host" and would
// break if we started advertising a path-suffixed resource.
func (h *Handler) HandleResourceMetadata(w http.ResponseWriter, r *http.Request) {
	// Advertise THIS server as the authorization server, not Keycloak directly.
	// MCP clients (Claude Code, Cursor) follow authorization_servers[0] to discover
	// the AS metadata (RFC 8414) and then perform Dynamic Client Registration
	// (RFC 7591) against the registration_endpoint they find in that document. If we
	// point them at the Keycloak issuer, they read Keycloak's own metadata whose
	// registration_endpoint is clients-registrations/openid-connect — which Istio
	// RBACs to 403 ("RBAC: access denied"), breaking auth before it starts. Pointing
	// at our own host makes clients read HandleAuthServerMetadata instead, which
	// proxies Keycloak's real authorization/token endpoints but overrides
	// registration_endpoint to our static /register handler.
	resource := fmt.Sprintf("https://%s", r.Host)

	metadata := map[string]interface{}{
		"resource":              resource,
		"authorization_servers": []string{resource},
		"scopes_supported":      h.supportedScopes(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(metadata)
}

// HandleAuthServerMetadata handles GET /.well-known/oauth-authorization-server and
// its path-based variants per RFC 8414 Section 3. Some MCP clients probe this
// location on the resource server host rather than following the
// authorization_servers pointer from oauth-protected-resource, and ignore the
// protected-resource scopes_supported if they can't find an AS metadata
// document locally. We proxy Keycloak's openid-configuration so clients get
// the real authorization/token endpoints, and override scopes_supported with
// our advertised list to keep both metadata documents aligned.
func (h *Handler) HandleAuthServerMetadata(w http.ResponseWriter, r *http.Request) {
	issuer := strings.TrimRight(h.getIssuer(), "/")
	if issuer == "" {
		http.Error(w, "authorization server not configured", http.StatusServiceUnavailable)
		return
	}

	metadata, err := h.fetchIssuerMetadata(r.Context(), issuer)
	if err != nil {
		h.logger.Warn("failed to fetch issuer metadata, serving minimal fallback",
			zap.String("issuer", issuer),
			zap.Error(err))
		metadata = map[string]interface{}{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/protocol/openid-connect/auth",
			"token_endpoint":                        issuer + "/protocol/openid-connect/token",
			"response_types_supported":              []string{"code"},
			"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
			"token_endpoint_auth_methods_supported": []string{"none", "client_secret_post", "client_secret_basic"},
			"code_challenge_methods_supported":      []string{"S256"},
		}
	}

	metadata["scopes_supported"] = h.supportedScopes()

	// Configure the Dynamic Client Registration (DCR - RFC 7591) based on selected mode
	switch h.cfg.MCPServer.DCRMode {
	case "upstream":
		// Keep the upstream Keycloak registration_endpoint as is, if present
	case "disabled":
		delete(metadata, "registration_endpoint")
	case "static":
		fallthrough
	default:
		// Override registration_endpoint to route DCR through our static handler
		// instead of Keycloak's (which is gated by Istio/oauth2-proxy and would
		// also allow arbitrary realm client creation). See HandleClientRegistration.
		metadata["registration_endpoint"] = fmt.Sprintf("https://%s/register", r.Host)
	}

	// Drop mtls_endpoint_aliases entirely. Keycloak populates it with its own
	// registration/token/introspection endpoints; some MCP clients read the
	// nested registration_endpoint from here and bypass our proxy, hitting
	// Keycloak's clients-registrations endpoint (which Istio RBACs to 403).
	// We don't offer mTLS-bound endpoints anyway, so the field is misleading.
	delete(metadata, "mtls_endpoint_aliases")

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(metadata)
}

// HandleClientRegistration implements RFC 7591 Dynamic Client Registration.
// It always returns the pre-registered public client (agentgram-mcp) configured
// in Keycloak instead of actually registering a new client. MCP clients like
// Cursor insist on DCR even when a client_id is configured statically, so
// returning a canned response lets them proceed without exposing Keycloak's
// realm to anonymous client creation.
//
// The client_id is public (advertised in docs/MCP.md); handing it out via this
// endpoint doesn't leak anything. Keycloak still enforces scopes, PKCE, and
// the realm's redirect_uri policy on the subsequent authorize/token calls.
func (h *Handler) HandleClientRegistration(w http.ResponseWriter, r *http.Request) {
	clientID := h.cfg.MCPServer.ClientID
	if clientID == "" {
		http.Error(w, "mcp client_id not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		RedirectURIs []string `json:"redirect_uris"`
		ClientName   string   `json:"client_name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	redirectURIs := req.RedirectURIs
	if len(redirectURIs) == 0 {
		redirectURIs = []string{"http://localhost/callback"}
	}

	response := map[string]interface{}{
		"client_id":                  clientID,
		"client_id_issued_at":        time.Now().Unix(),
		"redirect_uris":              redirectURIs,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
		"scope":                      strings.Join(h.supportedScopes(), " "),
		"application_type":           "native",
	}

	h.logger.Info("MCP dynamic client registration served (static)",
		zap.String("client_id", clientID),
		zap.String("user_agent", r.UserAgent()),
		zap.Strings("redirect_uris", redirectURIs))

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// fetchIssuerMetadata retrieves the raw openid-configuration JSON from the
// authorization server and decodes it into a generic map to preserve every
// field the upstream advertises (jwks_uri, grant_types_supported, etc.).
func (h *Handler) fetchIssuerMetadata(ctx context.Context, issuer string) (map[string]interface{}, error) {
	metadataURL := issuer + "/.well-known/openid-configuration"

	fetchCtx, cancel := context.WithTimeout(ctx, authServerMetadataTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build metadata request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", metadataURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, metadataURL)
	}

	var metadata map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("decode metadata: %w", err)
	}
	return metadata, nil
}

// handleToolsCall handles the tools/call MCP method
func (h *Handler) handleToolsCall(w http.ResponseWriter, r *http.Request, req jsonRPCRequest, userEmail string, userGroups []string, mcpSessionID string) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		h.writeJSON(w, http.StatusOK, h.server.MarshalError(req.ID, errCodeInvalidParams, "invalid params"))
		return
	}

	// Handle list_agents utility tool
	if params.Name == "list_agents" {
		agentsList := h.server.ListAccessibleAgents(userEmail, userGroups)
		data, _ := json.MarshalIndent(agentsList, "", "  ")
		h.writeJSON(w, http.StatusOK, h.server.MarshalToolResult(req.ID, string(data), false))
		return
	}

	// Handle MCP server tool calls (mcp_{serverID}__{toolName})
	if serverID, mcpToolName, ok := GetMCPToolFromName(params.Name); ok {
		h.handleMCPToolCall(w, r, req, serverID, mcpToolName, params.Arguments, userEmail, userGroups)
		return
	}

	// Extract agent ID from tool name
	agentID, ok := GetAgentIDFromToolName(params.Name)
	if !ok {
		h.writeJSON(w, http.StatusOK, h.server.MarshalError(req.ID, errCodeInvalidParams, fmt.Sprintf("unknown tool: %s", params.Name)))
		return
	}

	// Parse tool arguments
	var args struct {
		Question  string `json:"question"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(params.Arguments, &args); err != nil {
		h.writeJSON(w, http.StatusOK, h.server.MarshalError(req.ID, errCodeInvalidParams, "invalid arguments"))
		return
	}

	if args.Question == "" {
		h.writeJSON(w, http.StatusOK, h.server.MarshalError(req.ID, errCodeInvalidParams, "question is required"))
		return
	}

	// Look up the agent
	agent, err := h.registry.Get(agentID)
	if err != nil {
		h.writeJSON(w, http.StatusOK, h.server.MarshalError(req.ID, errCodeInvalidParams, fmt.Sprintf("agent not found: %s", agentID)))
		return
	}

	// Verify permissions
	isAdmin, _ := h.userService.IsAdmin(r.Context(), userEmail, userGroups)
	if !isAdmin {
		inheritedMap, _ := h.groupRepo.GetAllInheritedPermissions(r.Context())
		inherited := inheritedMap[agentID]
		if !agents.HasAccessWithInherited(agent, userEmail, userGroups, inherited) {
			h.writeJSON(w, http.StatusOK, h.server.MarshalToolResult(req.ID, "Access denied to this agent", true))
			return
		}
	}

	// Resolve session: use provided session_id, or look up from MCP session mapping
	sessionID := args.SessionID
	if sessionID == "" && mcpSessionID != "" {
		if sid, ok := h.server.sessions.GetAgentSession(mcpSessionID, agentID); ok {
			sessionID = sid
		}
	}

	// Start Langfuse trace for the MCP agent call
	var lfTrace *lf.Trace
	if h.langfuseTracer != nil && h.langfuseTracer.Enabled() {
		lfTrace = h.langfuseTracer.StartTrace(r.Context(), "mcp:chat", userEmail, sessionID, map[string]interface{}{
			"agent_id":       agentID,
			"agent_name":     agent.Name,
			"agent_protocol": agent.Protocol,
			"source":         "mcp",
		})
		lfTrace.SetInput(truncateString(args.Question, 1000))
	}

	// Start Langfuse span for the agent proxy call
	var agentSpan *lf.Span
	if lfTrace != nil && lfTrace.IsEnabled() {
		agentSpan = lfTrace.StartToolCall(fmt.Sprintf("proxy:%s", agentID), map[string]interface{}{
			"agent_name":     agent.Name,
			"agent_protocol": agent.Protocol,
			"question":       truncateString(args.Question, 1000),
		})
	}

	// Use SSE streaming response to send progress notifications while the agent works.
	// This prevents MCP client timeouts (default 60s) on long-running agent calls.
	// The MCP SDK resets its timeout when it receives notifications/progress events.
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

	// Run the agent call in a goroutine so we can send progress notifications
	type callResult struct {
		text      string
		sessionID string
		err       error
	}
	done := make(chan callResult, 1)
	go func() {
		result, resultSessionID, err := h.callAgent(r.Context(), agent, args.Question, sessionID, userEmail, userGroups)
		done <- callResult{text: result, sessionID: resultSessionID, err: err}
	}()

	// Send progress notifications every 15s to keep the MCP client timeout alive
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
				`{"jsonrpc":"2.0","method":"notifications/progress","params":{"progressToken":%s,"progress":%d,"message":"Agent %s is working... %ds elapsed"}}`,
				string(req.ID), elapsedSeconds, agentID, elapsedSeconds)
			flushSSE([]byte(progressNotification))
			h.logger.Debug("MCP progress notification sent",
				zap.String("agent_id", agentID),
				zap.Int("elapsed_seconds", elapsedSeconds))
		case <-r.Context().Done():
			// Client disconnected
			h.logger.Warn("MCP client disconnected during agent call",
				zap.String("agent_id", agentID))
			if agentSpan != nil {
				agentSpan.EndWithError(fmt.Errorf("client disconnected"))
			}
			if lfTrace != nil {
				lfTrace.End(false, "client disconnected")
			}
			return
		}
	}

	if cr.err != nil {
		h.logger.Error("MCP tools/call agent error",
			zap.String("agent_id", agentID),
			zap.String("user_email", userEmail),
			zap.Error(cr.err))
		if agentSpan != nil {
			agentSpan.EndWithError(cr.err)
		}
		if lfTrace != nil {
			lfTrace.End(false, cr.err.Error())
		}
		flushSSE(h.server.MarshalToolResult(req.ID, "Agent call failed. Please try again.", true))
		return
	}

	// End Langfuse agent span with response
	if agentSpan != nil {
		agentSpan.End(truncateString(cr.text, 2000))
	}

	// Store the agent session mapping for future calls
	if mcpSessionID != "" && cr.sessionID != "" {
		h.server.sessions.SetAgentSession(mcpSessionID, agentID, cr.sessionID)
	}

	// Build response text with session context
	responseText := cr.text
	if cr.sessionID != "" {
		responseText += fmt.Sprintf("\n\n---\n[session_id: %s]", cr.sessionID)
	}

	// End Langfuse trace with output
	if lfTrace != nil {
		lfTrace.End(true, truncateString(responseText, 2000))
	}

	jsonResponse := h.server.MarshalToolResult(req.ID, responseText, false)
	h.logger.Info("MCP tools/call response",
		zap.String("agent_id", agentID),
		zap.Int("response_text_len", len(responseText)),
		zap.Int("json_response_bytes", len(jsonResponse)),
		zap.Int("elapsed_seconds", elapsedSeconds))
	flushSSE(jsonResponse)
}

// handleMCPToolCall handles a tools/call for an MCP server tool.
// It forwards the call to the remote MCP server via the MCP client.
func (h *Handler) handleMCPToolCall(w http.ResponseWriter, r *http.Request, req jsonRPCRequest, serverID, toolName string, arguments json.RawMessage, userEmail string, userGroups []string) {
	if h.mcpRegistry == nil {
		h.writeJSON(w, http.StatusOK, h.server.MarshalToolResult(req.ID, "MCP servers not available", true))
		return
	}

	server, err := h.mcpRegistry.Get(serverID)
	if err != nil || server == nil {
		h.writeJSON(w, http.StatusOK, h.server.MarshalError(req.ID, errCodeInvalidParams, fmt.Sprintf("MCP server not found: %s", serverID)))
		return
	}

	// Verify permissions
	if !mcp.HasAccess(server, userEmail, userGroups) {
		h.writeJSON(w, http.StatusOK, h.server.MarshalToolResult(req.ID, "Access denied to this MCP server", true))
		return
	}

	// Parse arguments into map
	var args map[string]interface{}
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &args); err != nil {
			h.writeJSON(w, http.StatusOK, h.server.MarshalError(req.ID, errCodeInvalidParams, "invalid arguments"))
			return
		}
	}

	// Forward user JWT for forward_auth servers, or resolve OAuth2 token
	var extraHeaders map[string]string
	if server.Config.ForwardAuth {
		authHeader := middleware.GetAuthHeaderFromContext(r.Context())
		if authHeader != "" {
			extraHeaders = map[string]string{"Authorization": authHeader}
		}
	} else if server.Config.IsOAuth2() && h.oauth2Mgr != nil && h.mcpRepo != nil {
		mcpServer, srvErr := h.mcpRepo.Get(r.Context(), serverID)
		if srvErr == nil {
			token, _ := h.oauth2Mgr.GetToken(r.Context(), mcpServer, userEmail)
			if token != nil {
				extraHeaders = map[string]string{"Authorization": "Bearer " + token.AccessToken}
			} else {
				h.writeJSON(w, http.StatusOK, h.server.MarshalToolResult(req.ID, "OAuth2 authorization required. Please connect via the Agentgram web UI first.", true))
				return
			}
		}
	}

	// Identity headers ride along with the credential so the initialize
	// handshake (background context) also carries them; the tool call itself
	// gets them from r.Context() at the MCP client level.
	extraHeaders = identity.Merge(r.Context(), extraHeaders)

	// Lazy-initialize server if needed
	if (server.Config.ForwardAuth || server.Config.IsOAuth2()) && extraHeaders != nil {
		if err := h.mcpRegistry.EnsureInitialized(server, extraHeaders); err != nil {
			h.logger.Error("failed to initialize MCP server",
				zap.String("server_id", serverID),
				zap.Error(err))
			h.writeJSON(w, http.StatusOK, h.server.MarshalToolResult(req.ID, "MCP server initialization failed", true))
			return
		}
	}

	// Start Langfuse trace for the MCP tool call
	var lfTrace *lf.Trace
	if h.langfuseTracer != nil && h.langfuseTracer.Enabled() {
		lfTrace = h.langfuseTracer.StartTrace(r.Context(), "mcp:tool", userEmail, "", map[string]interface{}{
			"server_id":   serverID,
			"server_name": server.Config.Name,
			"tool_name":   toolName,
			"source":      "mcp",
		})
		lfTrace.SetInput(args)
	}

	var toolSpan *lf.Span
	if lfTrace != nil && lfTrace.IsEnabled() {
		toolSpan = lfTrace.StartToolCall(fmt.Sprintf("mcp:%s/%s", serverID, toolName), args)
	}

	// Use SSE streaming with progress notifications (same pattern as agent calls)
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
		result *mcp.ToolResult
		err    error
	}
	done := make(chan callResult, 1)
	go func() {
		result, err := server.Client.CallToolWithHeaders(r.Context(), toolName, args, extraHeaders)
		done <- callResult{result: result, err: err}
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
				`{"jsonrpc":"2.0","method":"notifications/progress","params":{"progressToken":%s,"progress":%d,"message":"MCP %s: %s is working... %ds elapsed"}}`,
				string(req.ID), elapsedSeconds, serverID, toolName, elapsedSeconds)
			flushSSE([]byte(progressNotification))
		case <-r.Context().Done():
			h.logger.Warn("MCP client disconnected during MCP tool call",
				zap.String("server_id", serverID),
				zap.String("tool_name", toolName))
			if toolSpan != nil {
				toolSpan.EndWithError(fmt.Errorf("client disconnected"))
			}
			if lfTrace != nil {
				lfTrace.End(false, "client disconnected")
			}
			return
		}
	}

	if cr.err != nil {
		h.logger.Error("MCP tool call error",
			zap.String("server_id", serverID),
			zap.String("tool_name", toolName),
			zap.Error(cr.err))
		if toolSpan != nil {
			toolSpan.EndWithError(cr.err)
		}
		if lfTrace != nil {
			lfTrace.End(false, cr.err.Error())
		}
		flushSSE(h.server.MarshalToolResult(req.ID, fmt.Sprintf("MCP tool call failed: %s", cr.err), true))
		return
	}

	// Extract text from tool result
	var resultText strings.Builder
	if cr.result != nil {
		for _, content := range cr.result.Content {
			if content.Type == "text" {
				resultText.WriteString(content.Text)
			}
		}
	}

	text := resultText.String()
	if text == "" {
		text = "(no output)"
	}

	// End Langfuse spans
	if toolSpan != nil {
		toolSpan.End(truncateString(text, 2000))
	}
	if lfTrace != nil {
		lfTrace.End(true, truncateString(text, 2000))
	}

	jsonResponse := h.server.MarshalToolResult(req.ID, text, cr.result != nil && cr.result.IsError)
	h.logger.Info("MCP tools/call response (MCP server)",
		zap.String("server_id", serverID),
		zap.String("tool_name", toolName),
		zap.Int("response_text_len", len(text)),
		zap.Int("json_response_bytes", len(jsonResponse)),
		zap.Int("elapsed_seconds", elapsedSeconds))
	flushSSE(jsonResponse)
}

// callAgent invokes an agent via the Agentgram proxy and returns the full text response.
// This consumes the SSE stream internally and accumulates the response.
func (h *Handler) callAgent(ctx context.Context, agent *models.Agent, question string, sessionID string, userEmail string, userGroups []string) (string, string, error) {
	// Create Agentgram session for message persistence
	session, err := h.sessionStore.CreateSession(ctx, userEmail, agent.ID, truncateString(question, 50))
	if err != nil {
		return "", "", fmt.Errorf("failed to create session: %w", err)
	}
	agentgramSessionID := session.SessionID

	// Build chat request — don't pass sessionID so the agent creates its own session.
	// The proxy handles agent-side session mapping via store.SetAgentSessionID.
	chatReq := &models.ChatRequest{
		Messages: []models.ChatMessage{
			{
				Role:      "user",
				Content:   question,
				UserEmail: userEmail,
			},
		},
	}

	// Use a buffer to capture the SSE response instead of writing to http.ResponseWriter
	buf := &sseCapture{}

	// The MCP user's JWT, used only when the agent's auth method is "forward".
	// In bearer mode the proxy resolves a per-user/group API key instead.
	authHeader := middleware.GetAuthHeaderFromContext(ctx)

	reqIDSuffix := agentgramSessionID
	if len(reqIDSuffix) > 8 {
		reqIDSuffix = reqIDSuffix[:8]
	}
	// UserEmail/UserGroups travel as explicit options (not via context):
	// the proxy call below runs on a detached context.Background().
	opts := proxy.HandleOptions{
		ThreadID:   agentgramSessionID,
		RequestID:  fmt.Sprintf("mcp-%s", reqIDSuffix),
		UserEmail:  userEmail,
		UserGroups: userGroups,
	}

	// Call the proxy with its own context, independent of the HTTP request.
	// This prevents the proxy call from being canceled if the HTTP client
	// disconnects or the gateway times out — we need the full response.
	callCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	proxyResult, err := h.proxy.Handle(callCtx, buf, agent, chatReq, authHeader, opts)
	if err != nil {
		return "", agentgramSessionID, err
	}

	// Save the messages to the session store (use callCtx, not the HTTP ctx)
	if proxyResult != nil && proxyResult.AssistantText != "" {
		saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer saveCancel()
		// Save user message
		if err := h.sessionStore.AddMessage(saveCtx, agentgramSessionID, models.ChatMessage{
			Role:      "user",
			Content:   question,
			UserEmail: userEmail,
		}); err != nil {
			h.logger.Error("failed to persist user message", zap.String("session_id", agentgramSessionID), zap.Error(err))
		}
		// Save assistant message
		if err := h.sessionStore.AddMessage(saveCtx, agentgramSessionID, models.ChatMessage{
			Role:    "assistant",
			Content: proxyResult.AssistantText,
			AgentID: agent.ID,
		}); err != nil {
			h.logger.Error("failed to persist assistant message", zap.String("session_id", agentgramSessionID), zap.Error(err))
		}
	}

	text := ""
	if proxyResult != nil {
		text = proxyResult.AssistantText
		logLevel := h.logger.Info
		if text == "" {
			logLevel = h.logger.Warn
		}
		logLevel("MCP callAgent result",
			zap.String("agent_id", agent.ID),
			zap.Int("text_len", len(text)),
			zap.Int("text_bytes", len([]byte(text))),
			zap.String("error", proxyResult.Error),
			zap.Int("sse_buf_len", buf.buf.Len()),
			zap.String("session_id", agentgramSessionID))
		if text == "" && proxyResult.Error == "" {
			// Log the raw SSE buffer for debugging empty responses
			rawSSE := buf.buf.String()
			if len(rawSSE) > 2000 {
				rawSSE = rawSSE[:2000] + "...[truncated]"
			}
			h.logger.Warn("MCP callAgent: empty text with no error, raw SSE buffer",
				zap.String("agent_id", agent.ID),
				zap.String("sse_buffer_preview", rawSSE))
		}
	} else {
		h.logger.Warn("MCP callAgent: nil proxyResult", zap.String("agent_id", agent.ID))
	}

	if text == "" && proxyResult != nil && proxyResult.Error != "" {
		return "", agentgramSessionID, fmt.Errorf("agent error: %s", proxyResult.Error)
	}

	return text, agentgramSessionID, nil
}

// writeJSON writes a JSON-RPC response
func (h *Handler) writeJSON(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
}

// writeUnauthorized writes a 401 with the MCP-required WWW-Authenticate header.
// The body uses the JSON-RPC 2.0 error shape to satisfy MCP clients (e.g. Cursor
// 2.6.x) that parse the body with a strict JSON-RPC schema before reacting to
// the HTTP status; a plain {"error":"unauthorized"} makes them abort with a Zod
// validation error and never trigger the OAuth flow.
func (h *Handler) writeUnauthorized(w http.ResponseWriter) {
	resourceMetadataURL := fmt.Sprintf("https://%s/.well-known/oauth-protected-resource", h.cfg.Server.Host)
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s"`, resourceMetadataURL))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32001,"message":"Unauthorized"}}`))
}

// truncateString truncates a string to max length
func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// sseCapture implements http.ResponseWriter to capture SSE output without sending it
type sseCapture struct {
	buf     bytes.Buffer
	headers http.Header
	status  int
}

func (s *sseCapture) Header() http.Header {
	if s.headers == nil {
		s.headers = make(http.Header)
	}
	return s.headers
}

func (s *sseCapture) Write(b []byte) (int, error) {
	return s.buf.Write(b)
}

func (s *sseCapture) WriteHeader(statusCode int) {
	s.status = statusCode
}

func (s *sseCapture) Flush() {
	// no-op for capture
}

// String returns the captured SSE body (useful for debugging)
func (s *sseCapture) String() string {
	return s.buf.String()
}

package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/mcp"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

// MCPOAuthHandler handles OAuth2 flows for MCP servers.
type MCPOAuthHandler struct {
	oauth2Mgr   *mcp.OAuth2Manager
	mcpRepo     repository.MCPServerRepository
	mcpRegistry *mcp.Registry
	logger      *zap.Logger
}

// NewMCPOAuthHandler creates a new MCP OAuth handler.
func NewMCPOAuthHandler(oauth2Mgr *mcp.OAuth2Manager, mcpRepo repository.MCPServerRepository, mcpRegistry *mcp.Registry, logger *zap.Logger) *MCPOAuthHandler {
	return &MCPOAuthHandler{
		oauth2Mgr:   oauth2Mgr,
		mcpRepo:     mcpRepo,
		mcpRegistry: mcpRegistry,
		logger:      logger,
	}
}

// Login initiates the OAuth2 authorization flow for an MCP server.
// GET /auth/mcp-oauth/{serverId}/login?return_url=...
func (h *MCPOAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverId")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	server, err := h.mcpRepo.Get(r.Context(), serverID)
	if err != nil {
		writeJSONError(w, "server not found", http.StatusNotFound)
		return
	}

	if server.GetAuthType() != models.MCPAuthOAuth2 {
		writeJSONError(w, "server does not use OAuth2", http.StatusBadRequest)
		return
	}

	info, err := h.mcpRegistry.Get(serverID)
	if err != nil {
		writeJSONError(w, "server not registered", http.StatusNotFound)
		return
	}

	if !mcp.HasAccess(info, claims.GetEmail(), claims.GetGroups()) {
		writeJSONError(w, "access denied", http.StatusForbidden)
		return
	}

	mappings, err := h.mcpRepo.ListScopeMappings(r.Context(), serverID)
	if err != nil {
		h.logger.Warn("failed to load scope mappings", zap.Error(err))
	}

	scopes := mcp.ResolveScopes(server.OAuth2Scopes, claims.GetGroups(), mappings)

	returnURL := r.URL.Query().Get("return_url")
	if returnURL == "" {
		returnURL = "/"
	}

	authURL, err := h.oauth2Mgr.GetAuthorizationURL(r.Context(), server, claims.GetEmail(), scopes, returnURL)
	if err != nil {
		h.logger.Error("failed to build OAuth2 URL", zap.Error(err))
		writeJSONError(w, "failed to initiate OAuth2", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback handles the OAuth2 authorization code callback.
// GET /auth/mcp-oauth/callback?code=...&state=...
func (h *MCPOAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		errDesc := r.URL.Query().Get("error_description")
		if errDesc == "" {
			errDesc = r.URL.Query().Get("error")
		}
		if errDesc == "" {
			errDesc = "missing code or state"
		}
		http.Error(w, errDesc, http.StatusBadRequest)
		return
	}

	serverID, err := h.oauth2Mgr.PeekStateServerID(r.Context(), state)
	if err != nil {
		h.logger.Error("OAuth2 callback: invalid state", zap.Error(err))
		http.Error(w, "invalid or expired authorization state", http.StatusBadRequest)
		return
	}

	server, err := h.mcpRepo.Get(r.Context(), serverID)
	if err != nil {
		h.logger.Error("OAuth2 callback: server not found", zap.String("server_id", serverID), zap.Error(err))
		http.Error(w, "MCP server not found", http.StatusBadRequest)
		return
	}

	_, returnURL, err := h.oauth2Mgr.ExchangeCode(r.Context(), server, code, state)
	if err != nil {
		h.logger.Error("OAuth2 code exchange failed", zap.Error(err))
		http.Error(w, "authorization failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	if returnURL == "" {
		returnURL = "/"
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<!DOCTYPE html><html><body><script>
		try {
			if (window.opener) {
				window.opener.postMessage({type: 'mcp-oauth-complete', server_id: '` + server.ID + `'}, '*');
			}
		} catch(e) {}
		// Always try to close — if this is a popup it closes,
		// if not (e.g. same tab), redirect to returnURL
		window.close();
		// If window.close() didn't work (not a popup), redirect
		setTimeout(function() { window.location.href = '` + returnURL + `'; }, 500);
	</script></body></html>`))
}

// Status returns the OAuth2 connection status for a user+server.
// GET /api/mcp/servers/{id}/oauth2/status
func (h *MCPOAuthHandler) Status(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	server, err := h.mcpRepo.Get(r.Context(), serverID)
	if err != nil {
		writeJSONError(w, "server not found", http.StatusNotFound)
		return
	}

	if server.GetAuthType() != models.MCPAuthOAuth2 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"auth_type": server.GetAuthType(),
			"connected": true,
		})
		return
	}

	token, err := h.oauth2Mgr.GetToken(r.Context(), server, claims.GetEmail())
	if err != nil {
		h.logger.Warn("failed to get MCP OAuth2 token", zap.Error(err))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"auth_type": "oauth2",
		"connected": token != nil,
		"scopes":    func() string { if token != nil { return token.Scopes }; return "" }(),
	})
}

// Disconnect removes a user's OAuth2 token for an MCP server.
// POST /api/mcp/servers/{id}/oauth2/disconnect
func (h *MCPOAuthHandler) Disconnect(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := h.oauth2Mgr.DisconnectUser(r.Context(), claims.GetEmail(), serverID); err != nil {
		h.logger.Error("failed to disconnect MCP OAuth2", zap.Error(err))
		writeJSONError(w, "failed to disconnect", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// ListScopeMappings returns the scope mappings for an MCP server.
// GET /api/admin/mcp/{id}/scope-mappings
func (h *MCPOAuthHandler) ListScopeMappings(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	mappings, err := h.mcpRepo.ListScopeMappings(r.Context(), serverID)
	if err != nil {
		writeJSONError(w, "failed to list scope mappings", http.StatusInternalServerError)
		return
	}
	if mappings == nil {
		mappings = []models.MCPOAuth2ScopeMapping{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"mappings": mappings})
}

// UpsertScopeMapping creates or updates a scope mapping.
// PUT /api/admin/mcp/{id}/scope-mappings
func (h *MCPOAuthHandler) UpsertScopeMapping(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	var req struct {
		GroupName string `json:"group_name"`
		Scopes    string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.GroupName == "" || req.Scopes == "" {
		writeJSONError(w, "group_name and scopes required", http.StatusBadRequest)
		return
	}

	mapping := &models.MCPOAuth2ScopeMapping{
		MCPServerID: serverID,
		GroupName:   req.GroupName,
		Scopes:      req.Scopes,
	}
	if err := h.mcpRepo.UpsertScopeMapping(r.Context(), mapping); err != nil {
		writeJSONError(w, "failed to save scope mapping", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// DeleteScopeMapping deletes a scope mapping.
// DELETE /api/admin/mcp/{id}/scope-mappings/{mappingId}
func (h *MCPOAuthHandler) DeleteScopeMapping(w http.ResponseWriter, r *http.Request) {
	mappingID := chi.URLParam(r, "mappingId")
	if err := h.mcpRepo.DeleteScopeMapping(r.Context(), mappingID); err != nil {
		writeJSONError(w, "failed to delete scope mapping", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

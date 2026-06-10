package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/mcp"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/security"
	"go.uber.org/zap"
)

// AdminMCPHandler handles admin CRUD for MCP servers
type AdminMCPHandler struct {
	mcpRepo     repository.MCPServerRepository
	auditRepo   repository.AuditRepository
	mcpRegistry *mcp.Registry
	oauth2Mgr   *mcp.OAuth2Manager
	logger      *zap.Logger
}

// NewAdminMCPHandler creates a new admin MCP handler
func NewAdminMCPHandler(mcpRepo repository.MCPServerRepository, auditRepo repository.AuditRepository, mcpRegistry *mcp.Registry, oauth2Mgr *mcp.OAuth2Manager, logger *zap.Logger) *AdminMCPHandler {
	return &AdminMCPHandler{
		mcpRepo:     mcpRepo,
		auditRepo:   auditRepo,
		mcpRegistry: mcpRegistry,
		oauth2Mgr:   oauth2Mgr,
		logger:      logger,
	}
}

// AdminMCPRequest is the request body for creating/updating MCP servers
type AdminMCPRequest struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Transport     string            `json:"transport"`
	URL           string            `json:"url"`
	Headers       map[string]string `json:"headers"`
	ForwardAuth   bool              `json:"forward_auth"`
	AllowedUsers  []string          `json:"allowed_users"`
	AllowedGroups []string          `json:"allowed_groups"`

	AuthType            string `json:"auth_type"`
	OAuth2AuthServerURL string `json:"oauth2_auth_server_url"`
	OAuth2ClientID      string `json:"oauth2_client_id"`
	OAuth2ClientSecret  string `json:"oauth2_client_secret"`
	OAuth2Scopes        string `json:"oauth2_scopes"`
	BearerToken         string `json:"bearer_token"`

	// Bearer mode: configurable auth header + per user/group API key rules.
	// APIKeyRules nil means "leave existing rules untouched" on update.
	AuthHeaderName string                 `json:"auth_header_name"`
	APIKeyRules    []models.MCPAPIKeyRule `json:"api_key_rules"`
}

// validateMCPAuth validates the bearer-mode auth header and API key rules.
// Returns a client-facing error message, or "" when valid.
func validateMCPAuth(req *AdminMCPRequest) string {
	if req.AuthHeaderName != "" {
		if err := security.ValidateHeaders(map[string]string{req.AuthHeaderName: "x"}); err != nil {
			return fmt.Sprintf("invalid auth_header_name: %s", err.Error())
		}
		switch strings.ToLower(req.AuthHeaderName) {
		case "host", "content-length", "content-type", "transfer-encoding", "connection":
			return fmt.Sprintf("invalid auth_header_name: %s", req.AuthHeaderName)
		}
	}

	seen := make(map[string]struct{}, len(req.APIKeyRules))
	for _, rule := range req.APIKeyRules {
		if rule.SubjectType != "user" && rule.SubjectType != "group" {
			return fmt.Sprintf("invalid api_key_rules subject_type: %s", rule.SubjectType)
		}
		if rule.Subject == "" || rule.APIKey == "" {
			return "api_key_rules entries require subject and api_key"
		}
		key := rule.SubjectType + "\x00" + rule.Subject
		if _, dup := seen[key]; dup {
			return fmt.Sprintf("duplicate api_key_rules entry: %s %s", rule.SubjectType, rule.Subject)
		}
		seen[key] = struct{}{}
	}
	return ""
}

// ListMCPServers handles GET /api/admin/mcp
func (h *AdminMCPHandler) ListMCPServers(w http.ResponseWriter, r *http.Request) {
	servers, err := h.mcpRepo.List(r.Context())
	if err != nil {
		h.logger.Error("list mcp servers failed", zap.Error(err))
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"servers": servers})
}

// GetMCPServer handles GET /api/admin/mcp/{id}
func (h *AdminMCPHandler) GetMCPServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	server, err := h.mcpRepo.Get(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"mcp server not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(server)
}

// CreateMCPServer handles POST /api/admin/mcp
func (h *AdminMCPHandler) CreateMCPServer(w http.ResponseWriter, r *http.Request) {
	var req AdminMCPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.ID == "" || req.Name == "" || req.Transport == "" || req.URL == "" {
		http.Error(w, `{"error":"id, name, transport, and url are required"}`, http.StatusBadRequest)
		return
	}

	// Validate URL against SSRF
	if err := security.ValidateEndpointURL(req.URL); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid url: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	// Validate headers against blocked list
	if err := security.ValidateHeaders(req.Headers); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid headers: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if msg := validateMCPAuth(&req); msg != "" {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, msg), http.StatusBadRequest)
		return
	}

	server := &models.MCPServer{
		ID:                  req.ID,
		Name:                req.Name,
		Description:         req.Description,
		Transport:           req.Transport,
		URL:                 req.URL,
		Headers:             req.Headers,
		ForwardAuth:         req.ForwardAuth,
		AllowedUsers:        req.AllowedUsers,
		AllowedGroups:       req.AllowedGroups,
		AuthType:            req.AuthType,
		OAuth2AuthServerURL: req.OAuth2AuthServerURL,
		OAuth2ClientID:      req.OAuth2ClientID,
		OAuth2ClientSecret:  req.OAuth2ClientSecret,
		OAuth2Scopes:        req.OAuth2Scopes,
		BearerToken:         req.BearerToken,
		AuthHeaderName:      req.AuthHeaderName,
		APIKeyRules:         req.APIKeyRules,
	}

	if server.GetAuthType() == models.MCPAuthOAuth2 && h.oauth2Mgr != nil {
		if server.OAuth2AuthServerURL == "" || server.OAuth2ClientID == "" || server.OAuth2Scopes == "" {
			discovered, err := h.oauth2Mgr.DiscoverFromMCP(r.Context(), server.URL)
			if err != nil {
				h.logger.Warn("MCP OAuth2 auto-discovery partial failure (continuing with available data)",
					zap.String("server_id", server.ID),
					zap.Error(err))
			}
			if discovered != nil {
				if server.OAuth2AuthServerURL == "" && discovered.AuthServerURL != "" {
					server.OAuth2AuthServerURL = discovered.AuthServerURL
				}
				if server.OAuth2Scopes == "" && discovered.Scopes != "" {
					server.OAuth2Scopes = discovered.Scopes
				}
				if server.OAuth2ClientID == "" && discovered.ClientID != "" {
					server.OAuth2ClientID = discovered.ClientID
					server.OAuth2ClientSecret = discovered.ClientSecret
				}
			}
		}

		if server.OAuth2ClientID == "" && server.OAuth2AuthServerURL != "" {
			clientID, clientSecret, err := h.oauth2Mgr.EnsureClientRegistered(r.Context(), server)
			if err != nil {
				h.logger.Warn("DCR fallback failed", zap.String("server_id", server.ID), zap.Error(err))
			} else {
				server.OAuth2ClientID = clientID
				server.OAuth2ClientSecret = clientSecret
			}
		}
	}

	if err := h.mcpRepo.Create(r.Context(), server); err != nil {
		h.logger.Error("create mcp server failed", zap.Error(err))
		http.Error(w, `{"error":"failed to create mcp server"}`, http.StatusInternalServerError)
		return
	}

	claims := middleware.GetUserFromContext(r.Context())
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       "create",
		ResourceType: "mcp_server",
		ResourceID:   req.ID,
	})

	h.mcpRegistry.Refresh(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(server)
}

// UpdateMCPServer handles PUT /api/admin/mcp/{id}
func (h *AdminMCPHandler) UpdateMCPServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req AdminMCPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Validate URL against SSRF
	if req.URL != "" {
		if err := security.ValidateEndpointURL(req.URL); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid url: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}
	}

	// Validate headers against blocked list
	if err := security.ValidateHeaders(req.Headers); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid headers: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if msg := validateMCPAuth(&req); msg != "" {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, msg), http.StatusBadRequest)
		return
	}

	server := &models.MCPServer{
		ID:                  id,
		Name:                req.Name,
		Description:         req.Description,
		Transport:           req.Transport,
		URL:                 req.URL,
		Headers:             req.Headers,
		ForwardAuth:         req.ForwardAuth,
		AllowedUsers:        req.AllowedUsers,
		AllowedGroups:       req.AllowedGroups,
		AuthType:            req.AuthType,
		OAuth2AuthServerURL: req.OAuth2AuthServerURL,
		OAuth2ClientID:      req.OAuth2ClientID,
		OAuth2ClientSecret:  req.OAuth2ClientSecret,
		OAuth2Scopes:        req.OAuth2Scopes,
		BearerToken:         req.BearerToken,
		AuthHeaderName:      req.AuthHeaderName,
		APIKeyRules:         req.APIKeyRules,
	}

	if server.GetAuthType() == models.MCPAuthOAuth2 && h.oauth2Mgr != nil {
		if server.OAuth2AuthServerURL == "" || server.OAuth2ClientID == "" || server.OAuth2Scopes == "" {
			discovered, err := h.oauth2Mgr.DiscoverFromMCP(r.Context(), server.URL)
			if err != nil {
				h.logger.Warn("MCP OAuth2 auto-discovery partial failure on update",
					zap.String("server_id", server.ID),
					zap.Error(err))
			}
			if discovered != nil {
				if server.OAuth2AuthServerURL == "" && discovered.AuthServerURL != "" {
					server.OAuth2AuthServerURL = discovered.AuthServerURL
				}
				if server.OAuth2Scopes == "" && discovered.Scopes != "" {
					server.OAuth2Scopes = discovered.Scopes
				}
				if server.OAuth2ClientID == "" && discovered.ClientID != "" {
					server.OAuth2ClientID = discovered.ClientID
					server.OAuth2ClientSecret = discovered.ClientSecret
				}
			}
		}

		if server.OAuth2ClientID == "" && server.OAuth2AuthServerURL != "" {
			clientID, clientSecret, err := h.oauth2Mgr.EnsureClientRegistered(r.Context(), server)
			if err != nil {
				h.logger.Warn("DCR fallback failed on update", zap.String("server_id", server.ID), zap.Error(err))
			} else {
				server.OAuth2ClientID = clientID
				server.OAuth2ClientSecret = clientSecret
			}
		}
	}

	if err := h.mcpRepo.Update(r.Context(), server); err != nil {
		h.logger.Error("update mcp server failed", zap.Error(err))
		http.Error(w, `{"error":"failed to update mcp server"}`, http.StatusInternalServerError)
		return
	}

	// Replace API key rules only when present in the request, so older API
	// clients that omit the field don't wipe existing rules.
	if req.APIKeyRules != nil {
		if err := h.mcpRepo.ReplaceAPIKeyRules(r.Context(), id, req.APIKeyRules); err != nil {
			h.logger.Error("replace mcp api key rules failed", zap.Error(err))
			http.Error(w, `{"error":"failed to update api key rules"}`, http.StatusInternalServerError)
			return
		}
	}

	claims := middleware.GetUserFromContext(r.Context())
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       "update",
		ResourceType: "mcp_server",
		ResourceID:   id,
	})

	h.mcpRegistry.Refresh(r.Context())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(server)
}

// DeleteMCPServer handles DELETE /api/admin/mcp/{id}
func (h *AdminMCPHandler) DeleteMCPServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.mcpRepo.Delete(r.Context(), id); err != nil {
		http.Error(w, `{"error":"mcp server not found"}`, http.StatusNotFound)
		return
	}

	claims := middleware.GetUserFromContext(r.Context())
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       "delete",
		ResourceType: "mcp_server",
		ResourceID:   id,
	})

	h.mcpRegistry.Refresh(r.Context())

	w.WriteHeader(http.StatusNoContent)
}

// UpdateMCPPermissions handles PUT /api/admin/mcp/{id}/permissions
func (h *AdminMCPHandler) UpdateMCPPermissions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req PermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if err := h.mcpRepo.UpdatePermissions(r.Context(), id, req.AllowedUsers, req.AllowedGroups); err != nil {
		h.logger.Error("update mcp permissions failed", zap.Error(err))
		http.Error(w, `{"error":"failed to update permissions"}`, http.StatusInternalServerError)
		return
	}

	claims := middleware.GetUserFromContext(r.Context())
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       "update_permissions",
		ResourceType: "mcp_server",
		ResourceID:   id,
	})

	h.mcpRegistry.Refresh(r.Context())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"allowed_users":  req.AllowedUsers,
		"allowed_groups": req.AllowedGroups,
	})
}

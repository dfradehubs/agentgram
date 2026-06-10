package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

// MCPServerConfig holds configuration for a single MCP server (previously in config package)
type MCPServerConfig struct {
	ID            string
	Name          string
	Description   string
	Transport     string
	URL           string
	Headers       map[string]string
	ForwardAuth   bool
	AllowedUsers  []string
	AllowedGroups []string

	AuthType            string
	OAuth2AuthServerURL string
	OAuth2ClientID      string
	OAuth2ClientSecret  string
	OAuth2Scopes        string
	BearerToken         string
	AuthHeaderName      string
	APIKeyRules         []models.MCPAPIKeyRule
}

// IsOAuth2 returns true if the server uses OAuth2 authentication.
func (c *MCPServerConfig) IsOAuth2() bool {
	return c.AuthType == models.MCPAuthOAuth2
}

// IsBearer returns true if the server uses a static bearer token.
func (c *MCPServerConfig) IsBearer() bool {
	return c.AuthType == models.MCPAuthBearer
}

// IsBearerPerUser returns true for bearer servers that resolve the key per
// user/group. These initialize lazily (like forward/oauth2) so each request
// carries the calling user's key instead of a shared static token.
func (c *MCPServerConfig) IsBearerPerUser() bool {
	return c.IsBearer() && len(c.APIKeyRules) > 0
}

// ServerInfo holds MCP server config, its client, and runtime status
type ServerInfo struct {
	Config      MCPServerConfig
	Client      *Client
	Status      string // "connected", "error", "disconnected"
	StatusError string // error message if Status == "error"
	Tools       []Tool // cached tools from last successful ListTools
	mu          sync.RWMutex
}

// GetStatus returns the current status thread-safely
func (s *ServerInfo) GetStatus() (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Status, s.StatusError
}

// GetTools returns the cached tools thread-safely
func (s *ServerInfo) GetTools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Tools
}

// Registry manages MCP server connections
type Registry struct {
	servers         map[string]*ServerInfo
	mu              sync.RWMutex
	logger          *zap.Logger
	mcpRepo         repository.MCPServerRepository // nil when running without DB
	toolCallTimeout time.Duration
	stopCh          chan struct{}
}

// NewDBRegistry creates a new DB-backed MCP registry.
// toolCallTimeout is the max duration for a single MCP tool call (0 means no limit).
func NewDBRegistry(mcpRepo repository.MCPServerRepository, toolCallTimeout time.Duration, logger *zap.Logger) *Registry {
	return &Registry{
		servers:         make(map[string]*ServerInfo),
		logger:          logger,
		mcpRepo:         mcpRepo,
		toolCallTimeout: toolCallTimeout,
		stopCh:          make(chan struct{}),
	}
}

// StartPeriodicRefresh starts a background goroutine that reloads MCP servers from DB
// every interval. This keeps all API pods in sync when servers are added/removed via admin.
func (r *Registry) StartPeriodicRefresh(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := r.LoadFromDB(ctx); err != nil {
					r.logger.Warn("periodic MCP registry refresh failed", zap.Error(err))
				}
				cancel()
			case <-r.stopCh:
				return
			}
		}
	}()
}

// Stop stops the periodic refresh goroutine
func (r *Registry) Stop() {
	close(r.stopCh)
}

// LoadFromDB loads MCP servers from the database and initializes clients
func (r *Registry) LoadFromDB(ctx context.Context) error {
	if r.mcpRepo == nil {
		return fmt.Errorf("no MCP repository configured")
	}

	servers, err := r.mcpRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("load MCP servers from DB: %w", err)
	}

	r.mu.Lock()

	// Track existing servers to preserve connections
	oldServers := r.servers
	r.servers = make(map[string]*ServerInfo)

	for _, s := range servers {
		serverCfg := mcpServerToConfig(s)

		// Reuse existing connection if server hasn't changed
		if old, ok := oldServers[s.ID]; ok && old.Config.URL == serverCfg.URL && old.Config.Transport == serverCfg.Transport {
			r.servers[s.ID] = old
			// Update config in case permissions changed
			old.Config = serverCfg
			delete(oldServers, s.ID)
			continue
		}

		r.logger.Info("MCP server registered from DB",
			zap.String("id", s.ID),
			zap.String("name", s.Name),
			zap.String("url", s.URL))

		client := NewClient(serverCfg.URL, serverCfg.Transport, effectiveHeaders(serverCfg), r.toolCallTimeout, r.logger)
		info := &ServerInfo{
			Config: serverCfg,
			Client: client,
			Status: "disconnected",
		}
		r.servers[s.ID] = info

		if serverCfg.ForwardAuth || serverCfg.IsOAuth2() || serverCfg.IsBearerPerUser() {
			r.logger.Info("MCP server requires per-user auth, will init on first user request",
				zap.String("id", s.ID),
				zap.String("name", s.Name),
				zap.String("auth_type", serverCfg.AuthType))
		} else {
			go r.initializeServer(info)
		}
	}

	r.mu.Unlock()
	return nil
}

// Refresh reloads MCP servers from DB (called after admin CRUD operations)
func (r *Registry) Refresh(ctx context.Context) error {
	return r.LoadFromDB(ctx)
}

func mcpServerToConfig(s *models.MCPServer) MCPServerConfig {
	return MCPServerConfig{
		ID:                  s.ID,
		Name:                s.Name,
		Description:         s.Description,
		Transport:           s.Transport,
		URL:                 s.URL,
		Headers:             s.Headers,
		ForwardAuth:         s.ForwardAuth,
		AllowedUsers:        s.AllowedUsers,
		AllowedGroups:       s.AllowedGroups,
		AuthType:            s.GetAuthType(),
		OAuth2AuthServerURL: s.OAuth2AuthServerURL,
		OAuth2ClientID:      s.OAuth2ClientID,
		OAuth2ClientSecret:  s.OAuth2ClientSecret,
		OAuth2Scopes:        s.OAuth2Scopes,
		BearerToken:         s.BearerToken,
		AuthHeaderName:      s.AuthHeaderName,
		APIKeyRules:         s.APIKeyRules,
	}
}

// effectiveHeaders returns the headers map that should be sent to the MCP server,
// including any auto-derived headers (e.g., Authorization for bearer auth).
// The configured Headers take precedence, so an explicit "Authorization" in
// Headers won't be overwritten by a stale BearerToken.
func effectiveHeaders(cfg MCPServerConfig) map[string]string {
	// Per-user bearer servers resolve the key per request (extraHeaders), so no
	// shared credential is baked into the static client headers.
	if !cfg.IsBearer() || cfg.IsBearerPerUser() || cfg.BearerToken == "" {
		return cfg.Headers
	}

	name, value := bearerHeader(cfg.AuthHeaderName, cfg.BearerToken)
	merged := make(map[string]string, len(cfg.Headers)+1)
	merged[name] = value
	for k, v := range cfg.Headers {
		merged[k] = v
	}
	return merged
}

func (r *Registry) initializeServer(info *ServerInfo) {
	r.logger.Info("MCP server initializing",
		zap.String("id", info.Config.ID),
		zap.String("url", info.Config.URL))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := info.Client.Initialize(ctx); err != nil {
		info.mu.Lock()
		info.Status = "error"
		info.StatusError = err.Error()
		info.mu.Unlock()
		r.logger.Warn("MCP server initialization failed",
			zap.String("id", info.Config.ID),
			zap.String("name", info.Config.Name),
			zap.String("url", info.Config.URL),
			zap.Error(err))
		return
	}

	// List tools after successful init
	tools, err := info.Client.ListTools(ctx)
	if err != nil {
		info.mu.Lock()
		info.Status = "error"
		info.StatusError = fmt.Sprintf("initialized but failed to list tools: %s", err)
		info.mu.Unlock()
		r.logger.Warn("MCP tool listing failed",
			zap.String("id", info.Config.ID),
			zap.Error(err))
		return
	}

	info.mu.Lock()
	info.Status = "connected"
	info.StatusError = ""
	info.Tools = tools
	info.mu.Unlock()

	r.logger.Info("MCP initialized",
		zap.String("id", info.Config.ID),
		zap.String("name", info.Config.Name),
		zap.Int("tools", len(tools)))
}

// EnsureInitialized initializes a forward_auth server lazily on first request with user headers
func (r *Registry) EnsureInitialized(info *ServerInfo, extraHeaders map[string]string) error {
	if info.Client.IsInitialized() {
		return nil
	}

	r.logger.Info("lazy-initializing forward_auth MCP server",
		zap.String("id", info.Config.ID),
		zap.String("url", info.Config.URL))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := info.Client.InitializeWithHeaders(ctx, extraHeaders); err != nil {
		info.mu.Lock()
		info.Status = "error"
		info.StatusError = err.Error()
		info.mu.Unlock()
		return fmt.Errorf("MCP initialize failed: %w", err)
	}

	tools, err := info.Client.ListToolsWithHeaders(ctx, extraHeaders)
	if err != nil {
		info.mu.Lock()
		info.Status = "error"
		info.StatusError = fmt.Sprintf("initialized but failed to list tools: %s", err)
		info.mu.Unlock()
		return fmt.Errorf("MCP tool listing failed: %w", err)
	}

	info.mu.Lock()
	info.Status = "connected"
	info.StatusError = ""
	info.Tools = tools
	info.mu.Unlock()

	r.logger.Info("MCP lazy-initialized",
		zap.String("id", info.Config.ID),
		zap.String("name", info.Config.Name),
		zap.Int("tools", len(tools)))

	return nil
}

// Reconnect re-initializes a specific server with optional extra headers (for forward_auth)
func (r *Registry) Reconnect(serverID string, extraHeaders map[string]string) (*ServerInfo, error) {
	r.mu.RLock()
	info, ok := r.servers[serverID]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("MCP server not found: %s", serverID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := info.Client.ReconnectWithHeaders(ctx, extraHeaders); err != nil {
		info.mu.Lock()
		info.Status = "error"
		info.StatusError = err.Error()
		info.Tools = nil
		info.mu.Unlock()
		return info, fmt.Errorf("reconnect failed: %w", err)
	}

	tools, err := info.Client.ListToolsWithHeaders(ctx, extraHeaders)
	if err != nil {
		info.mu.Lock()
		info.Status = "error"
		info.StatusError = fmt.Sprintf("reconnected but failed to list tools: %s", err)
		info.Tools = nil
		info.mu.Unlock()
		return info, fmt.Errorf("tool listing failed after reconnect: %w", err)
	}

	info.mu.Lock()
	info.Status = "connected"
	info.StatusError = ""
	info.Tools = tools
	info.mu.Unlock()

	r.logger.Info("MCP reconnected",
		zap.String("id", info.Config.ID),
		zap.String("name", info.Config.Name),
		zap.Int("tools", len(tools)))

	return info, nil
}

// Get returns a server by ID
func (r *Registry) Get(id string) (*ServerInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	server, ok := r.servers[id]
	if !ok {
		return nil, fmt.Errorf("MCP server not found: %s", id)
	}
	return server, nil
}

// List returns all registered servers
func (r *Registry) List() []*ServerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ServerInfo, 0, len(r.servers))
	for _, s := range r.servers {
		result = append(result, s)
	}
	return result
}

// HasAccess checks if a user has access to an MCP server
func HasAccess(server *ServerInfo, userEmail string, userGroups []string) bool {
	// Check allowed_users (wildcard or exact match)
	for _, u := range server.Config.AllowedUsers {
		if u == "*" {
			return true
		}
		if strings.EqualFold(u, userEmail) {
			return true
		}
	}

	// Check allowed_groups
	groupSet := make(map[string]bool)
	for _, g := range userGroups {
		groupSet[strings.ToLower(g)] = true
	}
	for _, allowed := range server.Config.AllowedGroups {
		if groupSet[strings.ToLower(allowed)] {
			return true
		}
	}

	return false
}

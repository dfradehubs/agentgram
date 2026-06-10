package models

import "time"

// MCPAuthType constants
const (
	MCPAuthNone    = "none"
	MCPAuthForward = "forward"
	MCPAuthOAuth2  = "oauth2"
	MCPAuthBearer  = "bearer"
)

// MCPServer represents an MCP server stored in the database
type MCPServer struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Transport     string            `json:"transport"`
	URL           string            `json:"url"`
	Headers       map[string]string `json:"headers"`
	ForwardAuth   bool              `json:"forward_auth"`
	AllowedUsers  []string          `json:"allowed_users,omitempty"`
	AllowedGroups []string          `json:"allowed_groups,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`

	AuthType            string `json:"auth_type"`
	OAuth2AuthServerURL string `json:"oauth2_auth_server_url,omitempty"`
	OAuth2ClientID      string `json:"oauth2_client_id,omitempty"`
	OAuth2ClientSecret  string `json:"oauth2_client_secret,omitempty"`
	OAuth2Scopes        string `json:"oauth2_scopes,omitempty"`
	BearerToken         string `json:"bearer_token,omitempty"`

	// Bearer mode: header that carries the key. Default "Authorization"
	// (sent as "Bearer <key>"); any other header (e.g. "X-API-Key") sends
	// the key verbatim without prefix.
	AuthHeaderName string `json:"auth_header_name,omitempty"`

	// Bearer mode: per user/group API keys. Resolution: user match >
	// group match (by position) > BearerToken fallback.
	APIKeyRules []MCPAPIKeyRule `json:"api_key_rules,omitempty"`
}

// GetAuthType returns the effective auth type, falling back to forward_auth for legacy compat.
func (s *MCPServer) GetAuthType() string {
	if s.AuthType != "" {
		return s.AuthType
	}
	if s.ForwardAuth {
		return MCPAuthForward
	}
	return MCPAuthNone
}

// MCPAPIKeyRule maps a user email or group to the API key agentgram sends to
// the MCP server in bearer mode. Mirrors models.AgentAPIKeyRule.
type MCPAPIKeyRule struct {
	ID          string `json:"id,omitempty"`
	MCPServerID string `json:"mcp_server_id,omitempty"`
	SubjectType string `json:"subject_type"` // "user" | "group"
	Subject     string `json:"subject"`
	APIKey      string `json:"api_key"`
	// Priority orders group rules: lower is evaluated first (ASC). A user-exact
	// rule always wins over group rules regardless of priority.
	Priority int `json:"priority"`
}

// MCPOAuth2ScopeMapping maps an Agentgram group to additional OAuth2 scopes for an MCP server.
type MCPOAuth2ScopeMapping struct {
	ID          string    `json:"id"`
	MCPServerID string    `json:"mcp_server_id"`
	GroupName   string    `json:"group_name"`
	Scopes      string    `json:"scopes"`
	CreatedAt   time.Time `json:"created_at"`
}

// MCPOAuth2Token represents an encrypted OAuth2 token stored in Redis for a user+server pair.
type MCPOAuth2Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresAt    int64  `json:"expires_at"`
	Scopes       string `json:"scopes"`
}

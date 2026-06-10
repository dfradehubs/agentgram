package models

// Agent outbound authentication methods. Mirrors the MCP server auth_type
// enum (see mcp_server.go) so admins get the same mental model on both.
const (
	AgentAuthNone    = "none"
	AgentAuthForward = "forward"
	AgentAuthOAuth2  = "oauth2" // reserved for phase 2 — not implemented yet
	AgentAuthBearer  = "bearer"
)

// Agent represents a remote agent configuration
type Agent struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Category    string `yaml:"category" json:"category"`
	Protocol    string `yaml:"protocol" json:"protocol"` // "custom" | "a2a" | "adk"
	Endpoint    string `yaml:"endpoint" json:"-"`

	// Path to get agent-card.json (A2A only)
	AgentCardPath string `yaml:"agent_card_path" json:"-"`

	// Headers to send to the agent
	Headers map[string]string `yaml:"headers" json:"-"`

	// If true, forwards the user's JWT to the agent.
	// Legacy flag: superseded by AuthType ("forward"); see GetAuthType.
	ForwardAuthorization bool `yaml:"forward_authorization" json:"-"`

	// Outbound auth method: "none" | "forward" | "bearer" ("oauth2" reserved).
	// Empty falls back to ForwardAuthorization for backwards compatibility.
	AuthType string `yaml:"auth_type" json:"-"`

	// Bearer mode: fallback API key when no APIKeyRules entry matches the user.
	BearerToken string `yaml:"bearer_token" json:"-"`

	// Bearer mode: header that carries the key. Default "Authorization"
	// (sent as "Bearer <key>"); any other header (e.g. "X-API-Key") sends
	// the key verbatim without prefix.
	AuthHeaderName string `yaml:"auth_header_name" json:"-"`

	// Bearer mode: per user/group API keys. Resolution: user match >
	// group match (by position) > BearerToken fallback.
	APIKeyRules []AgentAPIKeyRule `yaml:"api_key_rules" json:"-"`

	// Permissions
	AllowedGroups []string `yaml:"allowed_groups" json:"-"`
	AllowedUsers  []string `yaml:"allowed_users" json:"-"`

	// Rate limiting
	RateLimit *RateLimitConfig `yaml:"rate_limit" json:"-"`

	// Health check
	HealthCheck *HealthCheckConfig `yaml:"health_check" json:"-"`

	// Polling (A2A only)
	Polling *PollingConfig `yaml:"polling" json:"-"`

	// Custom format configuration (custom protocol only)
	CustomFormat *CustomFormatConfig `yaml:"custom_format" json:"-"`

	PipelineFinalAgent string `yaml:"pipeline_final_agent" json:"-"`

	// ADK-specific configuration
	ADKAppName string `yaml:"adk_app_name" json:"-"` // App name for ADK protocol (default: agent ID)
	ADKUserID  string `yaml:"adk_user_id" json:"-"`  // User ID for ADK protocol (default: "agentgram")

	// Context management (multi-agent)
	MaxContextTokens   int     `yaml:"max_context_tokens" json:"max_context_tokens"`   // Max tokens for context window (default 200000)
	SummarizeThreshold float64 `yaml:"summarize_threshold" json:"summarize_threshold"` // Fraction of max tokens to trigger summarization (default 0.8)

	// If true, the agent benefits from a GitHub token for full read/write access
	RequireGitHubToken bool `yaml:"require_github_token" json:"-"`

	// Health status (runtime)
	Status string `json:"status"` // "healthy" | "unhealthy" | "unknown"
}

// GetAuthType returns the effective outbound auth method, falling back to
// the legacy ForwardAuthorization flag when auth_type is not set (mirrors
// MCPServer.GetAuthType).
func (a *Agent) GetAuthType() string {
	if a.AuthType != "" {
		return a.AuthType
	}
	if a.ForwardAuthorization {
		return AgentAuthForward
	}
	return AgentAuthNone
}

// AgentAPIKeyRule maps a user email or group to the API key agentgram sends
// to the agent in bearer mode.
type AgentAPIKeyRule struct {
	ID          string `yaml:"-" json:"id,omitempty"`
	AgentID     string `yaml:"-" json:"agent_id,omitempty"`
	SubjectType string `yaml:"subject_type" json:"subject_type"` // "user" | "group"
	Subject     string `yaml:"subject" json:"subject"`
	APIKey      string `yaml:"api_key" json:"api_key"`
	// Priority orders group rules: lower is evaluated first (ASC). A user-exact
	// rule always wins over group rules regardless of priority.
	Priority int `yaml:"priority" json:"priority"`
}

// RateLimitConfig rate limiting configuration
type RateLimitConfig struct {
	RequestsPerMinute int `yaml:"requests_per_minute" json:"requests_per_minute"`
	RequestsPerHour   int `yaml:"requests_per_hour" json:"requests_per_hour"`
}

// HealthCheckConfig health check configuration
type HealthCheckConfig struct {
	Enabled         bool   `yaml:"enabled" json:"enabled"`
	URL             string `yaml:"url" json:"url,omitempty"`           // Full URL (optional, overrides endpoint + path)
	Endpoint        string `yaml:"endpoint" json:"endpoint,omitempty"` // Path appended to agent endpoint (default: /health)
	IntervalSeconds int    `yaml:"interval_seconds" json:"interval_seconds"`
	TimeoutSeconds  int    `yaml:"timeout_seconds" json:"timeout_seconds"`
}

// PollingConfig polling configuration for A2A
type PollingConfig struct {
	IntervalMS     int `yaml:"interval_ms" json:"interval_ms"`
	TimeoutSeconds int `yaml:"timeout_seconds" json:"timeout_seconds"`
}

// CustomFormatConfig configures request/response format for custom protocol agents
type CustomFormatConfig struct {
	RequestTemplate     string `json:"request_template,omitempty" yaml:"request_template"`
	ResponseContentPath string `json:"response_content_path,omitempty" yaml:"response_content_path"`
	RequestMethod       string `json:"request_method,omitempty" yaml:"request_method"`
	RequestContentType  string `json:"request_content_type,omitempty" yaml:"request_content_type"`
}

// AgentResponse is the response for the frontend (without sensitive data)
type AgentResponse struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	Category           string `json:"category"`
	Protocol           string `json:"protocol"`
	Status             string `json:"status"`
	RequireGitHubToken bool   `json:"require_github_token,omitempty"`
}

// ToResponse converts an Agent to AgentResponse (without sensitive data)
func (a *Agent) ToResponse() AgentResponse {
	return AgentResponse{
		ID:                 a.ID,
		Name:               a.Name,
		Description:        a.Description,
		Category:           a.Category,
		Protocol:           a.Protocol,
		Status:             a.Status,
		RequireGitHubToken: a.RequireGitHubToken,
	}
}

package config

// Config represents the application configuration loaded from YAML
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Auth      AuthConfig      `yaml:"auth"`
	CORS      CORSConfig      `yaml:"cors"`
	Logging   LoggingConfig   `yaml:"logging"`
	Tracing   TracingConfig   `yaml:"tracing"`
	Langfuse  LangfuseConfig  `yaml:"langfuse"`
	Redis     RedisConfig     `yaml:"redis"`
	Database  DatabaseConfig  `yaml:"database"`
	Metrics   MetricsConfig   `yaml:"metrics"`
	MCPServer MCPServerConfig `yaml:"mcp_server"`
}

// LangfuseConfig holds Langfuse observability configuration
type LangfuseConfig struct {
	Enabled         bool    `yaml:"enabled"`
	PublicKey       string  `yaml:"public_key"`
	SecretKey       string  `yaml:"secret_key"`
	Host            string  `yaml:"host"`
	Environment     string  `yaml:"environment"`
	InputCostPer1M  float64 `yaml:"input_cost_per_1m"`
	OutputCostPer1M float64 `yaml:"output_cost_per_1m"`
}

// CORSConfig holds CORS middleware configuration
type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
}

// MetricsConfig holds observability metrics configuration
type MetricsConfig struct {
	Enabled         bool   `yaml:"enabled"`
	RetentionDays   int    `yaml:"retention_days"`
	CleanupInterval string `yaml:"cleanup_interval"` // e.g. "1h", "30m"
}

// DatabaseConfig holds PostgreSQL connection configuration
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
	MaxConns int    `yaml:"max_conns"`
}

// TracingConfig holds OpenTelemetry tracing configuration
type TracingConfig struct {
	Enabled     bool    `yaml:"enabled"`
	Endpoint    string  `yaml:"endpoint"`     // OTLP gRPC endpoint (default "localhost:4317")
	ServiceName string  `yaml:"service_name"` // service.name resource attribute (default "agentgram-api")
	SampleRate  float64 `yaml:"sample_rate"`  // trace sampling rate 0.0-1.0 (default 1.0)
	Insecure    bool    `yaml:"insecure"`     // use insecure gRPC connection (default true, for local sidecar)
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Addr         string `yaml:"addr"`
	Password     string `yaml:"password"`
	DB           int    `yaml:"db"`
	PoolSize     int    `yaml:"pool_size"`
	MinIdleConns int    `yaml:"min_idle_conns"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port string `yaml:"port"`
	Host string `yaml:"host"` // Public hostname (e.g. "agentgram.example.com"), used for OAuth metadata
}

// MCPServerConfig holds configuration for the MCP server endpoint
type MCPServerConfig struct {
	Enabled           bool          `yaml:"enabled"`
	Issuer            string        `yaml:"issuer"`               // Independent Keycloak issuer for MCP server. Defaults to Auth.Keycloak.Issuer
	ClientID          string        `yaml:"client_id"`            // Keycloak client ID for MCP clients (e.g. "agentgram-mcp")
	DCRMode           string        `yaml:"dcr_mode"`             // Dynamic Client Registration mode: "static" (canned response), "upstream" (Keycloak DCR), or "disabled"
	ToolCallTimeout   string        `yaml:"tool_call_timeout"`    // Max duration for a single MCP tool call (e.g. "2m", "5m"). Default: "2m"
	MaxToolCallRounds int           `yaml:"max_tool_call_rounds"` // Max LLM ↔ tool iterations per chat request. Default: 10
	StaticTokens      []StaticToken `yaml:"static_tokens"`        // Service-account tokens that bypass Keycloak (e.g. internal automation)
}

// StaticToken declares a long-lived bearer token mapped to synthetic claims.
// Authentication accepts it before falling back to JWT validation, the same
// way the per-MCP `api_keys` block works in kubernetes-mcp / defectdojo-mcp.
type StaticToken struct {
	Name   string   `yaml:"name"`   // Human-readable identifier ("ci-bot", "automation-bot", ...)
	Token  string   `yaml:"token"`  // The bearer value; load via ${ENV:...} in deployments
	Email  string   `yaml:"email"`  // Synthetic email surfaced as claim.Email
	Groups []string `yaml:"groups"` // Synthetic Keycloak group memberships
}

// AuthConfig holds authentication configuration with nested providers
type AuthConfig struct {
	Enabled       bool     `yaml:"enabled"`
	SessionMaxAge int      `yaml:"session_max_age"` // seconds, default 86400 (24h)
	CookieSecure  bool     `yaml:"cookie_secure"`
	AdminGroups   []string `yaml:"admin_groups"`
	AdminUsers    []string `yaml:"admin_users"`
	Keycloak      KeycloakConfig      `yaml:"keycloak"`
	GitHub        GitHubOAuthConfig   `yaml:"github"`
	Google        GoogleOAuthConfig   `yaml:"google"`
	Basic         BasicAuthConfig     `yaml:"basic"`
}

// KeycloakConfig holds Keycloak OIDC configuration
type KeycloakConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Issuer        string `yaml:"issuer"`
	JWKSCacheTTL  int    `yaml:"jwks_cache_ttl"` // seconds
	ClientID      string `yaml:"client_id"`
	ClientSecret  string `yaml:"client_secret"`
	RedirectURI   string `yaml:"redirect_uri"`
	PostLogoutURI string `yaml:"post_logout_uri"`
}

// GitHubOAuthConfig holds GitHub OAuth configuration
type GitHubOAuthConfig struct {
	Enabled      bool   `yaml:"enabled"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURL  string `yaml:"redirect_url"`
}

// GoogleOAuthConfig holds Google OAuth configuration (future)
type GoogleOAuthConfig struct {
	Enabled bool `yaml:"enabled"`
}

// BasicAuthConfig holds basic auth configuration
type BasicAuthConfig struct {
	Enabled   bool              `yaml:"enabled"`
	SeedUsers []BasicSeedUser   `yaml:"seed_users"` // Bootstrap users created on startup
}

// BasicSeedUser represents a user to seed on startup
type BasicSeedUser struct {
	Username string `yaml:"username"`
	Email    string `yaml:"email"`
	Password string `yaml:"password"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`  // "debug" | "info" | "warn" | "error"
	Format string `yaml:"format"` // "json" | "console"
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	var cfg Config
	if err := LoadYAML(path, &cfg); err != nil {
		return nil, err
	}

	// Set server defaults
	if cfg.Server.Port == "" {
		cfg.Server.Port = "8080"
	}
	if cfg.Auth.Keycloak.JWKSCacheTTL == 0 {
		cfg.Auth.Keycloak.JWKSCacheTTL = 3600
	}
	if cfg.Auth.SessionMaxAge == 0 {
		cfg.Auth.SessionMaxAge = 86400
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}

	// Langfuse defaults
	if cfg.Langfuse.Host == "" {
		cfg.Langfuse.Host = "https://cloud.langfuse.com"
	}

	// Tracing defaults
	if cfg.Tracing.Endpoint == "" {
		cfg.Tracing.Endpoint = "localhost:4317"
	}
	if cfg.Tracing.ServiceName == "" {
		cfg.Tracing.ServiceName = "agentgram-api"
	}
	if cfg.Tracing.SampleRate == 0 {
		cfg.Tracing.SampleRate = 1.0
	}
	if cfg.Tracing.Enabled && !cfg.Tracing.Insecure {
		// Default to insecure for local sidecar unless explicitly set in YAML
		cfg.Tracing.Insecure = true
	}

	// Redis defaults
	if cfg.Redis.Addr == "" {
		cfg.Redis.Addr = "localhost:6379"
	}
	if cfg.Redis.PoolSize == 0 {
		cfg.Redis.PoolSize = 10
	}
	if cfg.Redis.MinIdleConns == 0 {
		cfg.Redis.MinIdleConns = 5
	}

	// Database defaults
	if cfg.Database.Host == "" {
		cfg.Database.Host = "localhost"
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 5432
	}
	if cfg.Database.SSLMode == "" {
		cfg.Database.SSLMode = "require"
	}
	if cfg.Database.MaxConns == 0 {
		cfg.Database.MaxConns = 25
	}

	// MCP defaults
	if cfg.MCPServer.ToolCallTimeout == "" {
		cfg.MCPServer.ToolCallTimeout = "2m"
	}
	if cfg.MCPServer.MaxToolCallRounds == 0 {
		cfg.MCPServer.MaxToolCallRounds = 10
	}
	if cfg.MCPServer.DCRMode == "" {
		cfg.MCPServer.DCRMode = "static"
	}

	// Metrics defaults
	if cfg.Metrics.RetentionDays == 0 {
		cfg.Metrics.RetentionDays = 30
	}
	if cfg.Metrics.CleanupInterval == "" {
		cfg.Metrics.CleanupInterval = "1h"
	}

	return &cfg, nil
}

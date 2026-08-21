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
	Seed      SeedConfig      `yaml:"seed"`
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

// DatabaseConfig holds database connection configuration
type DatabaseConfig struct {
	// Driver is "postgres" (default) or "sqlite". sqlite is laptop/single-node.
	Driver   string `yaml:"driver"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
	MaxConns int    `yaml:"max_conns"`
	// Path is the SQLite file path when Driver is "sqlite" (default agentgram.db).
	Path string `yaml:"path"`
	// Embedded starts a private PostgreSQL process (laptop mode, no Docker).
	Embedded bool `yaml:"embedded"`
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
	// Embedded starts an in-process Redis (miniredis). Addr is ignored.
	// Single-node / laptop mode only — not for multi-instance deployments.
	Embedded bool `yaml:"embedded"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port string `yaml:"port"`
	Host string `yaml:"host"` // Public hostname (e.g. "agentgram.example.com"), used for OAuth metadata
	// WebStaticDir, if set, serves a SPA/static UI from this directory on unmatched GET routes.
	WebStaticDir string `yaml:"web_static_dir"`
}

// MCPServerConfig holds configuration for the MCP server endpoint
type MCPServerConfig struct {
	Enabled      bool          `yaml:"enabled"`
	Issuer       string        `yaml:"issuer"`        // Independent Keycloak issuer for MCP server. Defaults to Auth.Keycloak.Issuer
	ClientID     string        `yaml:"client_id"`     // Keycloak client ID for MCP clients (e.g. "agentgram-mcp")
	DCRMode      string        `yaml:"dcr_mode"`      // Dynamic Client Registration mode: "static" (canned response), "upstream" (Keycloak DCR), or "disabled"
	ExtraScopes  []string      `yaml:"extra_scopes"`  // Extra OAuth scopes advertised to MCP clients on top of the required base set. Typically a Keycloak client scope with an audience mapper (e.g. "mcp:custom-audience") so strict clients like Claude get a token whose aud the upstream agent accepts.
	StaticTokens []StaticToken `yaml:"static_tokens"` // Service-account tokens that bypass Keycloak (e.g. internal automation)
	// NOTE: tool-call timeout and max tool-call rounds are runtime settings now
	// (admin → General Configuration), not YAML — see internal/settings.
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
	Enabled       bool              `yaml:"enabled"`
	SessionMaxAge int               `yaml:"session_max_age"` // seconds, default 86400 (24h)
	CookieSecure  bool              `yaml:"cookie_secure"`
	AdminGroups   []string          `yaml:"admin_groups"`
	AdminUsers    []string          `yaml:"admin_users"`
	EditorGroups  []string          `yaml:"editor_groups"`
	ViewerGroups  []string          `yaml:"viewer_groups"`
	Keycloak      KeycloakConfig    `yaml:"keycloak"`
	GitHub        GitHubOAuthConfig `yaml:"github"`
	Google        GoogleOAuthConfig `yaml:"google"`
	Basic         BasicAuthConfig   `yaml:"basic"`
}

// KeycloakConfig holds OIDC configuration. Any spec-compliant provider works
// (Keycloak, Authentik, Auth0, Zitadel, Google). Endpoints are discovered from
// {issuer}/.well-known/openid-configuration; Keycloak path fallbacks remain.
type KeycloakConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Issuer        string `yaml:"issuer"`
	JWKSCacheTTL  int    `yaml:"jwks_cache_ttl"` // seconds
	ClientID      string `yaml:"client_id"`
	ClientSecret  string `yaml:"client_secret"`
	RedirectURI   string `yaml:"redirect_uri"`
	PostLogoutURI string `yaml:"post_logout_uri"`
	// DisplayName is shown on the login button. Defaults to "OIDC".
	DisplayName string `yaml:"display_name"`
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
	Enabled   bool            `yaml:"enabled"`
	SeedUsers []BasicSeedUser `yaml:"seed_users"` // Bootstrap users created on startup
}

// BasicSeedUser represents a user to seed on startup
type BasicSeedUser struct {
	Username string `yaml:"username"`
	Email    string `yaml:"email"`
	Password string `yaml:"password"`
}

// SeedConfig controls first-run data so a fresh instance can chat immediately.
type SeedConfig struct {
	// DemoAgent registers a built-in echo agent at /demo/chat (no extra container).
	DemoAgent bool `yaml:"demo_agent"`
	// Agents are created if the id does not already exist. Empty endpoint skips the row.
	Agents []SeedAgent `yaml:"agents"`
}

// SeedAgent is a declarative agent inserted on startup.
type SeedAgent struct {
	ID            string   `yaml:"id"`
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description"`
	Category      string   `yaml:"category"`
	Protocol      string   `yaml:"protocol"`
	Endpoint      string   `yaml:"endpoint"`
	AllowedGroups []string `yaml:"allowed_groups"`
	AllowedUsers  []string `yaml:"allowed_users"`
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

	if cfg.Auth.Keycloak.DisplayName == "" {
		cfg.Auth.Keycloak.DisplayName = "OIDC"
	}
	if cfg.Database.Driver == "" {
		cfg.Database.Driver = "postgres"
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "agentgram.db"
	}

	// Redis defaults
	if cfg.Redis.Addr == "" && !cfg.Redis.Embedded {
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

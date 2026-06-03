package server

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/audit"
	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/config"
	"github.com/redis/go-redis/v9"

	"github.com/dfradehubs/agentgram-api/internal/crypto"
	"github.com/dfradehubs/agentgram-api/internal/handlers"
	lf "github.com/dfradehubs/agentgram-api/internal/langfuse"
	"github.com/dfradehubs/agentgram-api/internal/mcp"
	"github.com/dfradehubs/agentgram-api/internal/mcpserver"
	"github.com/dfradehubs/agentgram-api/internal/metrics"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/pubsub"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/service"
	slackpkg "github.com/dfradehubs/agentgram-api/internal/slack"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"go.uber.org/zap"
)

// AdminDeps holds dependencies for admin API
type AdminDeps struct {
	UserService     *service.UserService
	AgentRepo       repository.AgentRepository
	MCPRepo         repository.MCPServerRepository
	UserRepo        repository.UserRepository
	AuditRepo       repository.AuditRepository
	LLMRepo         repository.LLMModelRepository
	GroupRepo       repository.GroupRepository
	MCPRegistry     *mcp.Registry
	ChatEventRepo   repository.ChatEventRepository
	BasicAuthRepo   repository.BasicAuthRepository
	PubSubHub       *pubsub.Hub
	ShareRepo       repository.SharedSessionRepository
	LangfuseTracer  *lf.Tracer
	SlackRepo       repository.SlackIntegrationRepository
	SlackLinkRepo   repository.SlackUserLinkRepository
	BotManager      *slackpkg.BotManager
	DataCipher      *crypto.AESCrypto
	RedisClient     *redis.Client
	OAuth2Manager   *mcp.OAuth2Manager
}

// SetupRoutes configures all server routes
func SetupRoutes(cfg *config.Config, registry *agents.Registry, sessionStore store.SessionStore, authSessionStore store.AuthSessionStore, cookieCrypto *auth.CookieCrypto, logger *zap.Logger, adminDeps *AdminDeps) http.Handler {
	r := chi.NewRouter()

	// Global middlewares
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CORS(middleware.CORSConfig{
		AllowedOrigins: cfg.CORS.AllowedOrigins,
	}))
	r.Use(middleware.BodyLimit(1 << 20)) // 1MB max request body
	r.Use(middleware.NewLogging(logger).Handler)
	r.Use(chiMiddleware.Recoverer)
	// NOTE: No global Timeout middleware — SSE streaming endpoints (chat, broadcast, MCP)
	// need long-lived connections. Non-streaming endpoints are bounded by client timeouts.

	// Audit logger
	auditLogger := audit.New(logger)

	// MCP registry (always DB-backed)
	mcpRegistry := adminDeps.MCPRegistry

	// LLM repo for handlers
	llmRepo := adminDeps.LLMRepo

	// Handlers
	healthHandler := handlers.NewHealthHandler(registry)
	configHandler := handlers.NewConfigHandler(llmRepo)
	agentsHandler := handlers.NewAgentsHandler(registry, adminDeps.UserService, adminDeps.GroupRepo, logger)
	proxyHandler := handlers.NewProxyHandler(llmRepo, registry, adminDeps.UserService, adminDeps.GroupRepo, sessionStore, adminDeps.PubSubHub, auditLogger, logger, adminDeps.LangfuseTracer, adminDeps.ChatEventRepo)
	sessionsHandler := handlers.NewSessionsHandler(registry, sessionStore, adminDeps.GroupRepo, adminDeps.UserService, auditLogger, logger)
	mcpHandler := handlers.NewMCPHandler(llmRepo, mcpRegistry, sessionStore, cfg.MCPServer.MaxToolCallRounds, auditLogger, logger, adminDeps.LangfuseTracer, adminDeps.OAuth2Manager, adminDeps.MCPRepo, adminDeps.ChatEventRepo)
	chartHandler := handlers.NewChartHandler(llmRepo, adminDeps.LangfuseTracer, logger)

	// User handler
	var userHandler *handlers.UserHandler
	if adminDeps.UserService != nil {
		userHandler = handlers.NewUserHandlerWithService(adminDeps.UserService, logger)
	} else {
		userHandler = handlers.NewUserHandler(logger)
	}

	// Public endpoints (no auth)
	r.Get("/health", healthHandler.Liveness)
	r.Get("/health/ready", healthHandler.Readiness)

	// Swagger UI
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// Prometheus metrics endpoint (token-protected if METRICS_TOKEN is set)
	if metrics.IsEnabled() {
		metricsTokenAuth := middleware.MetricsAuth(os.Getenv("METRICS_TOKEN"))
		r.With(metricsTokenAuth).Handle("/metrics", metrics.Handler())
	}

	// OIDC client (shared between MCP server and auth endpoints)
	var oidcClient *auth.OIDCClient
	if cfg.Auth.Keycloak.Enabled {
		keycloakProvider := auth.NewKeycloakProvider(
			cfg.Auth.Keycloak.Issuer,
			time.Duration(cfg.Auth.Keycloak.JWKSCacheTTL)*time.Second,
		)
		oidcClient = auth.NewOIDCClient(cfg.Auth.Keycloak, keycloakProvider)
	}

	// MCP server endpoint (exposes agents as MCP tools for Claude Code and other MCP clients)
	if cfg.MCPServer.Enabled {
		mcpServerHandler := mcpserver.NewHandler(registry, mcpRegistry, sessionStore, adminDeps.UserService, adminDeps.GroupRepo, oidcClient, cfg, logger, adminDeps.LangfuseTracer, adminDeps.OAuth2Manager, adminDeps.MCPRepo)

		// Public: OAuth2 Protected Resource Metadata (RFC 9728)
		// Serve at both root and path-based locations per RFC 9728 Section 3.1:
		//   /.well-known/oauth-protected-resource       (root resource)
		//   /.well-known/oauth-protected-resource/mcp   (path-based for /mcp resource)
		r.Get("/.well-known/oauth-protected-resource", mcpServerHandler.HandleResourceMetadata)
		r.Get("/.well-known/oauth-protected-resource/*", mcpServerHandler.HandleResourceMetadata)

		// Public: OAuth2 Authorization Server Metadata (RFC 8414)
		// Some MCP clients probe this location on the resource server host rather
		// than following the authorization_servers pointer, and ignore
		// scopes_supported from the protected-resource document if they can't
		// find an AS metadata document locally. Serve at both root and path-based
		// locations per RFC 8414 Section 3.1:
		//   /.well-known/oauth-authorization-server       (root resource)
		//   /.well-known/oauth-authorization-server/mcp   (path-based for /mcp resource)
		r.Get("/.well-known/oauth-authorization-server", mcpServerHandler.HandleAuthServerMetadata)
		r.Get("/.well-known/oauth-authorization-server/*", mcpServerHandler.HandleAuthServerMetadata)

		// Public: Dynamic Client Registration (RFC 7591)
		// Returns the pre-registered public client (agentgram-mcp) statically
		// instead of creating new clients in Keycloak. Required for MCP clients
		// (e.g. Cursor) that insist on DCR even when a client_id is configured.
		// See Handler.HandleClientRegistration for the security rationale.
		r.Post("/register", mcpServerHandler.HandleClientRegistration)

		// Protected: MCP protocol endpoint
		r.Route("/mcp", func(r chi.Router) {
			if cfg.Auth.Enabled {
				mcpIssuer := cfg.MCPServer.Issuer
				if mcpIssuer == "" {
					mcpIssuer = cfg.Auth.Keycloak.Issuer
				}
				keycloakProvider := auth.NewKeycloakProvider(
					mcpIssuer,
					time.Duration(cfg.Auth.Keycloak.JWKSCacheTTL)*time.Second,
				)
				// MCP tokens don't carry audience claims (same as web frontend tokens).
				// Validate signature + issuer only — no audience check.
				jwtValidator := auth.NewJWTValidator(keycloakProvider, "")
				mcpAuth := middleware.NewMCPAuth(jwtValidator, logger, cfg.Server.Host, cfg.MCPServer.StaticTokens)
				r.Use(mcpAuth.Handler)
			}
			r.Post("/", mcpServerHandler.HandleMCP)
			r.Get("/", mcpServerHandler.HandleSSE)
			r.Delete("/", mcpServerHandler.HandleSessionTerminate)
		})

		logger.Info("MCP server endpoint enabled", zap.String("path", "/mcp"))
	}

	// Public auth providers endpoint
	r.Get("/auth/providers", handleAuthProviders(cfg))

	// Fallback /auth/session for when auth is disabled (returns anonymous user)
	if !cfg.Auth.Enabled || !cfg.Auth.Keycloak.Enabled {
		r.Get("/auth/session", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"authenticated": true,
				"email":         "anonymous@localhost",
				"name":          "Anonymous",
				"groups":        []string{"*"},
			})
		})
		r.Post("/auth/logout", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		})
	}

	// Basic auth login (public, no auth middleware)
	authRateLimiter := middleware.NewRateLimiter(5)
	if cfg.Auth.Basic.Enabled && adminDeps.BasicAuthRepo != nil {
		basicAuthHandler := handlers.NewBasicAuthHandler(adminDeps.BasicAuthRepo, authSessionStore, cfg, logger)
		r.With(authRateLimiter.IPHandler("auth")).Post("/auth/basic/login", basicAuthHandler.Login)
		logger.Info("basic auth enabled")
	}

	// OIDC auth endpoints (no auth middleware)
	// GitHub OAuth client (optional, used for auth + Slack linking)
	var githubClient *auth.GitHubOAuthClient
	if cfg.Auth.GitHub.Enabled {
		githubClient = auth.NewGitHubOAuthClient(cfg.Auth.GitHub)
		logger.Info("GitHub OAuth enabled",
			zap.String("redirect_url", cfg.Auth.GitHub.RedirectURL))
	}

	if cfg.Auth.Keycloak.Enabled {
		authHandler := handlers.NewAuthHandler(oidcClient, githubClient, authSessionStore, cookieCrypto, cfg, logger)

		r.Get("/auth/login", authHandler.Login)
		r.Get("/auth/callback", authHandler.Callback)
		r.Post("/auth/logout", authHandler.Logout)
		r.Get("/auth/session", authHandler.GetSession)

		// GitHub OAuth routes
		r.Get("/auth/github/login", authHandler.GitHubLogin)
		r.Get("/auth/github/callback", authHandler.GitHubCallback)
		r.Get("/auth/github/status", authHandler.GitHubStatus)
		r.Post("/auth/github/disconnect", authHandler.GitHubDisconnect)

		// Slack account linking (public, no auth — user visits from browser)
		if adminDeps.SlackLinkRepo != nil && adminDeps.DataCipher != nil {
			hostURL := "https://" + cfg.Server.Host
			slackLinkHandler := handlers.NewSlackLinkHandler(adminDeps.SlackLinkRepo, adminDeps.SlackRepo, oidcClient, githubClient, authSessionStore, adminDeps.DataCipher, adminDeps.RedisClient, hostURL, logger)
			r.Get("/auth/slack/link", slackLinkHandler.LinkStart)
			r.Get("/auth/slack/callback", slackLinkHandler.LinkCallback)
			r.Get("/auth/slack/github", slackLinkHandler.GitHubLinkStart)
			r.Get("/auth/slack/github/callback", slackLinkHandler.GitHubLinkCallback)
			logger.Info("Slack account linking enabled")
		}

		logger.Info("OIDC authentication enabled",
			zap.String("issuer", cfg.Auth.Keycloak.Issuer),
			zap.String("redirect_uri", cfg.Auth.Keycloak.RedirectURI))
	}

	// MCP OAuth2 endpoints
	if adminDeps.OAuth2Manager != nil {
		mcpOAuthHandler := handlers.NewMCPOAuthHandler(adminDeps.OAuth2Manager, adminDeps.MCPRepo, mcpRegistry, logger)

		// Login requires authentication (user must be logged in to start OAuth flow)
		if cfg.Auth.Enabled {
			keycloakProvider := auth.NewKeycloakProvider(
				cfg.Auth.Keycloak.Issuer,
				time.Duration(cfg.Auth.Keycloak.JWKSCacheTTL)*time.Second,
			)
			jwtValidator := auth.NewJWTValidator(keycloakProvider, cfg.Auth.Keycloak.ClientID)
			loginAuth := middleware.NewAuth(jwtValidator, authSessionStore, oidcClient, cookieCrypto, logger, cfg.Auth.SessionMaxAge, cfg.Auth.CookieSecure)
			r.With(loginAuth.Handler).Get("/auth/mcp-oauth/{serverId}/login", mcpOAuthHandler.Login)
		} else {
			r.With(middleware.NoAuth(logger)).Get("/auth/mcp-oauth/{serverId}/login", mcpOAuthHandler.Login)
		}

		// Callback is public (OAuth2 redirect from external auth server)
		r.Get("/auth/mcp-oauth/callback", mcpOAuthHandler.Callback)
		logger.Info("MCP OAuth2 endpoints enabled")
	}

	// API endpoints
	r.Route("/api", func(r chi.Router) {
		// Apply authentication middleware based on config
		if cfg.Auth.Enabled {
			logger.Info("authentication enabled",
				zap.String("issuer", cfg.Auth.Keycloak.Issuer))
			keycloakProvider := auth.NewKeycloakProvider(
				cfg.Auth.Keycloak.Issuer,
				time.Duration(cfg.Auth.Keycloak.JWKSCacheTTL)*time.Second,
			)
			jwtValidator := auth.NewJWTValidator(keycloakProvider, cfg.Auth.Keycloak.ClientID)
			authMiddleware := middleware.NewAuth(jwtValidator, authSessionStore, oidcClient, cookieCrypto, logger, cfg.Auth.SessionMaxAge, cfg.Auth.CookieSecure)
			r.Use(authMiddleware.Handler)
		} else {
			logger.Warn("authentication disabled")
			r.Use(middleware.NoAuth(logger))
		}

		// Read state (unread tracking, backed by Redis)
		readStateHandler := handlers.NewReadStateHandler(sessionStore, adminDeps.PubSubHub, logger)
		r.Get("/read-state", readStateHandler.GetReadState)
		r.Put("/read-state", readStateHandler.BatchUpdate)
		r.Put("/read-state/{sessionId}", readStateHandler.MarkRead)
		r.Get("/read-state/subscribe", readStateHandler.Subscribe)

		// Rate limiter for chat endpoints (60 req/min per user per agent)
		chatRateLimiter := middleware.NewRateLimiter(60)

		// Config (authenticated)
		r.Get("/config", configHandler.GetConfig)

		// User
		r.Get("/me", userHandler.Me)

		// Agents
		r.Get("/agents", agentsHandler.ListAgents)
		r.Get("/agents/{agentId}", agentsHandler.GetAgent)
		r.Get("/agents/{agentId}/health", agentsHandler.GetAgentHealth)

		// Chat endpoint (SSE response with AG-UI events)
		r.With(chatRateLimiter.ChatHandler).Post("/agents/{agentId}/chat", proxyHandler.Chat)

		// Chart extraction (LLM-based, rate limited)
		chartRateLimiter := middleware.NewRateLimiter(10)
		r.With(chartRateLimiter.ChatHandler).Post("/chart/extract", chartHandler.Extract)

		// Sessions (backed by Redis)
		r.Get("/agents/{agentId}/sessions", sessionsHandler.ListSessions)
		r.Get("/agents/{agentId}/sessions/{sessionId}", sessionsHandler.GetSession)
		r.Patch("/agents/{agentId}/sessions/{sessionId}", sessionsHandler.RenameSession)
		r.Delete("/agents/{agentId}/sessions/{sessionId}", sessionsHandler.DeleteSession)
		r.Post("/agents/{agentId}/sessions/{sessionId}/charts", sessionsHandler.PatchCharts)

		// Slack sessions (independent concept — not a group)
		slackSessionsHandler := handlers.NewSlackSessionsHandler(sessionStore, logger)
		r.Get("/slack/sessions", slackSessionsHandler.ListSlackSessions)

		// Session sharing
		shareHandler := handlers.NewShareHandler(registry, sessionStore, adminDeps.ShareRepo, adminDeps.GroupRepo, auditLogger, logger)
		r.Post("/agents/{agentId}/sessions/{sessionId}/share", shareHandler.CreateShare)
		r.Delete("/agents/{agentId}/sessions/{sessionId}/share", shareHandler.RevokeShare)
		r.Get("/shared/{token}", shareHandler.GetSharedSession)
		r.Post("/shared/{token}/clone", shareHandler.CloneSharedSession)

		// MCP endpoints
		r.Get("/mcp/servers", mcpHandler.ListServers)
		r.Get("/mcp/servers/{id}/tools", mcpHandler.ListTools)
		r.Post("/mcp/servers/{id}/reconnect", mcpHandler.Reconnect)
		r.Post("/mcp/servers/{id}/chat", mcpHandler.Chat)
		r.Get("/mcp/servers/{id}/sessions", mcpHandler.ListSessions)
		r.Get("/mcp/servers/{id}/sessions/{sid}", mcpHandler.GetSession)
		r.Patch("/mcp/servers/{id}/sessions/{sid}", mcpHandler.RenameSession)
		r.Delete("/mcp/servers/{id}/sessions/{sid}", mcpHandler.DeleteSession)

		// MCP OAuth2 user endpoints (status, disconnect)
		if adminDeps.OAuth2Manager != nil {
			mcpOAuthHandler := handlers.NewMCPOAuthHandler(adminDeps.OAuth2Manager, adminDeps.MCPRepo, mcpRegistry, logger)
			r.Get("/mcp/servers/{id}/oauth2/status", mcpOAuthHandler.Status)
			r.Post("/mcp/servers/{id}/oauth2/disconnect", mcpOAuthHandler.Disconnect)
		}

		// Real-time session subscription (SSE)
		subscribeHandler := handlers.NewSubscribeHandler(adminDeps.PubSubHub, sessionStore, adminDeps.GroupRepo, adminDeps.UserService, logger)
		r.Get("/sessions/{sessionId}/subscribe", subscribeHandler.Subscribe)

		// Live reconnect to an in-flight run after a page reload (SSE replay + live)
		runStreamHandler := handlers.NewRunStreamHandler(adminDeps.RedisClient, sessionStore, registry, adminDeps.GroupRepo, adminDeps.UserService, logger)
		r.Get("/agents/{agentId}/sessions/{sessionId}/stream", runStreamHandler.Stream)

		// Agent groups (user-facing)
		groupsHandler := handlers.NewGroupsHandler(adminDeps.GroupRepo, sessionStore, adminDeps.UserService, registry, logger)
		r.Get("/groups", groupsHandler.ListGroups)
		r.Post("/groups", groupsHandler.CreateGroup)
		r.Put("/groups/{groupId}", groupsHandler.UpdateGroup)
		r.Delete("/groups/{groupId}", groupsHandler.DeleteGroup)
		r.Get("/groups/{groupId}/sessions", groupsHandler.ListGroupSessions)
		r.Post("/groups/{groupId}/sessions", groupsHandler.AddGroupSession)
		r.Delete("/groups/{groupId}/sessions/{sessionId}", groupsHandler.RemoveGroupSession)

		// Multi-MCP endpoints
		r.Post("/mcp/chat", mcpHandler.ChatMulti)
		r.Get("/mcp/sessions", mcpHandler.ListMultiMCPSessions)
		r.Get("/mcp/sessions/{sid}", mcpHandler.GetMultiMCPSession)
		r.Patch("/mcp/sessions/{sid}", mcpHandler.RenameMultiMCPSession)
		r.Delete("/mcp/sessions/{sid}", mcpHandler.DeleteMultiMCPSession)

		// Admin API
		if adminDeps.UserService != nil {
			r.Route("/admin", func(r chi.Router) {
				adminMiddleware := middleware.NewAdmin(adminDeps.UserService, logger)
				r.Use(adminMiddleware.Handler)

				// Admin agents
				adminAgentsHandler := handlers.NewAdminAgentsHandler(adminDeps.AgentRepo, adminDeps.AuditRepo, adminDeps.GroupRepo, registry, logger)
				r.Get("/agents", adminAgentsHandler.ListAgents)
				r.Post("/agents", adminAgentsHandler.CreateAgent)
				r.Get("/agents/{id}", adminAgentsHandler.GetAgent)
				r.Put("/agents/{id}", adminAgentsHandler.UpdateAgent)
				r.Delete("/agents/{id}", adminAgentsHandler.DeleteAgent)
				r.Put("/agents/{id}/permissions", adminAgentsHandler.UpdatePermissions)

				// Admin groups
				adminGroupsHandler := handlers.NewAdminGroupsHandler(adminDeps.GroupRepo, adminDeps.AuditRepo, logger)
				r.Get("/groups", adminGroupsHandler.ListGroups)
				r.Post("/groups", adminGroupsHandler.CreateGroup)
				r.Get("/groups/{id}", adminGroupsHandler.GetGroup)
				r.Put("/groups/{id}", adminGroupsHandler.UpdateGroup)
				r.Delete("/groups/{id}", adminGroupsHandler.DeleteGroup)
				r.Put("/groups/{id}/permissions", adminGroupsHandler.UpdateGroupPermissions)

				// Admin MCP
				adminMCPHandler := handlers.NewAdminMCPHandler(adminDeps.MCPRepo, adminDeps.AuditRepo, mcpRegistry, adminDeps.OAuth2Manager, logger)
				r.Get("/mcp", adminMCPHandler.ListMCPServers)
				r.Post("/mcp", adminMCPHandler.CreateMCPServer)
				r.Get("/mcp/{id}", adminMCPHandler.GetMCPServer)
				r.Put("/mcp/{id}", adminMCPHandler.UpdateMCPServer)
				r.Delete("/mcp/{id}", adminMCPHandler.DeleteMCPServer)
				r.Put("/mcp/{id}/permissions", adminMCPHandler.UpdateMCPPermissions)

				// Admin MCP OAuth2 scope mappings
				if adminDeps.OAuth2Manager != nil {
					mcpOAuthHandler := handlers.NewMCPOAuthHandler(adminDeps.OAuth2Manager, adminDeps.MCPRepo, mcpRegistry, logger)
					r.Get("/mcp/{id}/scope-mappings", mcpOAuthHandler.ListScopeMappings)
					r.Put("/mcp/{id}/scope-mappings", mcpOAuthHandler.UpsertScopeMapping)
					r.Delete("/mcp/{id}/scope-mappings/{mappingId}", mcpOAuthHandler.DeleteScopeMapping)
				}

				// Admin LLM
				adminLLMHandler := handlers.NewAdminLLMHandler(adminDeps.LLMRepo, adminDeps.AuditRepo, logger)
				r.Get("/llm", adminLLMHandler.ListLLMModels)
				r.Post("/llm", adminLLMHandler.CreateLLMModel)
				r.Get("/llm/{id}", adminLLMHandler.GetLLMModel)
				r.Put("/llm/{id}", adminLLMHandler.UpdateLLMModel)
				r.Delete("/llm/{id}", adminLLMHandler.DeleteLLMModel)

				// Admin users
				adminUsersHandler := handlers.NewAdminUsersHandler(adminDeps.UserRepo, adminDeps.AuditRepo, cfg.Auth.AdminUsers, cfg.Auth.AdminGroups, oidcClient, logger)
				r.Get("/users", adminUsersHandler.ListUsers)
				r.Put("/users/{email}/role", adminUsersHandler.UpdateRole)

				// Admin metrics (observability)
				if adminDeps.ChatEventRepo != nil {
					metricsHandler := handlers.NewAdminMetricsHandler(adminDeps.ChatEventRepo, logger)
					r.Route("/metrics", func(r chi.Router) {
						r.Get("/overview", metricsHandler.Overview)
						r.Get("/overview/timeline", metricsHandler.OverviewTimeline)
						r.Get("/overview/top", metricsHandler.TopResources)
						r.Get("/overview/users", metricsHandler.OverviewUsers)
						r.Get("/overview/errors", metricsHandler.OverviewErrors)
						r.Get("/overview/error-events", metricsHandler.OverviewErrorEvents)

						// Per user
						r.Get("/users/{email}", metricsHandler.UserStats)
						r.Get("/users/{email}/timeline", metricsHandler.UserTimeline)
						r.Get("/users/{email}/resources", metricsHandler.UserTopResources)

						// Per resource type
						for _, rt := range []struct{ urlPrefix, dbType string }{
							{"agents", "agent"},
							{"mcp", "mcp"},
						} {
							rt := rt
							r.Get("/"+rt.urlPrefix+"/{id}", metricsHandler.ResourceStats(rt.dbType))
							r.Get("/"+rt.urlPrefix+"/{id}/timeline", metricsHandler.ResourceTimeline(rt.dbType))
							r.Get("/"+rt.urlPrefix+"/{id}/users", metricsHandler.ResourceUsers(rt.dbType))
							r.Get("/"+rt.urlPrefix+"/{id}/errors", metricsHandler.ResourceErrors(rt.dbType))
							r.Get("/"+rt.urlPrefix+"/{id}/error-events", metricsHandler.ResourceErrorEvents(rt.dbType))
						}
					})
				}

				// Admin basic auth user management
				if cfg.Auth.Basic.Enabled && adminDeps.BasicAuthRepo != nil {
					basicAuthHandler := handlers.NewBasicAuthHandler(adminDeps.BasicAuthRepo, authSessionStore, cfg, logger)
					r.Get("/basic-auth/users", basicAuthHandler.ListUsers)
					r.Post("/basic-auth/users", basicAuthHandler.CreateUser)
					r.Delete("/basic-auth/users/{id}", basicAuthHandler.DeleteUser)
				}

				// Admin Slack integrations
				if adminDeps.SlackRepo != nil && adminDeps.BotManager != nil {
					slackHandler := handlers.NewAdminSlackHandler(adminDeps.SlackRepo, adminDeps.AuditRepo, adminDeps.BotManager, logger)
					r.Get("/agents/{id}/slack", slackHandler.Get)
					r.Put("/agents/{id}/slack", slackHandler.Upsert)
					r.Delete("/agents/{id}/slack", slackHandler.Delete)
					r.Post("/agents/{id}/slack/test", slackHandler.TestConnection)
				}

				// Admin Slack user links (list, revoke)
				if adminDeps.SlackLinkRepo != nil && adminDeps.DataCipher != nil {
					hostURL := "https://" + cfg.Server.Host
					slackLinkAdminHandler := handlers.NewSlackLinkHandler(adminDeps.SlackLinkRepo, adminDeps.SlackRepo, oidcClient, githubClient, authSessionStore, adminDeps.DataCipher, adminDeps.RedisClient, hostURL, logger)
					r.Get("/slack/links", slackLinkAdminHandler.AdminListLinks)
					r.Delete("/slack/links", slackLinkAdminHandler.AdminRevokeLink)
					r.Delete("/slack/links/github", slackLinkAdminHandler.AdminRevokeGitHub)
				}

				logger.Info("admin API enabled")
			})
		}
	})

	return r
}

// authProvider describes an enabled auth provider for the frontend
type authProvider struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	LoginURL string `json:"login_url"`
}

// handleAuthProviders returns the list of enabled auth providers
func handleAuthProviders(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providers := make([]authProvider, 0)

		if cfg.Auth.Keycloak.Enabled {
			providers = append(providers, authProvider{
				Name:     "Keycloak",
				Type:     "oidc",
				LoginURL: "/auth/login",
			})
		}
		// GitHub OAuth is NOT a login method — it forwards tokens to agents.
		// It is intentionally excluded from the providers list.
		if cfg.Auth.Basic.Enabled {
			providers = append(providers, authProvider{
				Name:     "Basic",
				Type:     "basic",
				LoginURL: "/auth/basic/login",
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providers)
	}
}

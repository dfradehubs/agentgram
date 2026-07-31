package server

import (
	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/config"
	"github.com/dfradehubs/agentgram-api/internal/handlers"
	"github.com/dfradehubs/agentgram-api/internal/mcp"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"go.uber.org/zap"
)

// buildAdminRouter builds the /api/admin router as a standalone mux. It is mounted
// under /api/admin for the web admin and handed to the MCP server handler, whose
// administration tools replay requests against it in-process — so both surfaces
// share one set of handlers, one role gate and one audit trail.
//
// Routes are registered with paths relative to the mount point.
func buildAdminRouter(
	cfg *config.Config,
	adminDeps *AdminDeps,
	registry *agents.Registry,
	mcpRegistry *mcp.Registry,
	oidcClient *auth.OIDCClient,
	githubClient *auth.GitHubOAuthClient,
	authSessionStore store.AuthSessionStore,
	logger *zap.Logger,
) chi.Router {
	r := chi.NewRouter()

	// Record every configuration change in audit_events, whichever surface it came
	// from. Mounted here so endpoints added later are covered automatically.
	r.Use(middleware.NewAdminAudit(adminDeps.AuditEventRepo, adminDeps.SettingsService, logger).Handler)

	editorGate := middleware.RequireRole(adminDeps.UserService, models.RoleEditor, logger)
	adminGate := middleware.RequireRole(adminDeps.UserService, models.RoleAdmin, logger)

	// Handlers for the create/edit resources are shared between the
	// editor zone (read + create + edit) and the admin zone (delete +
	// permissions), so they're instantiated once here.
	adminAgentsHandler := handlers.NewAdminAgentsHandler(adminDeps.AgentRepo, adminDeps.AuditRepo, adminDeps.GroupRepo, registry, logger)
	adminMCPHandler := handlers.NewAdminMCPHandler(adminDeps.MCPRepo, adminDeps.AuditRepo, mcpRegistry, adminDeps.OAuth2Manager, logger)
	var adminSkillsHandler *handlers.AdminSkillsHandler
	if adminDeps.SkillRepo != nil {
		adminSkillsHandler = handlers.NewAdminSkillsHandler(adminDeps.SkillRepo, adminDeps.AuditRepo, logger)
	}

	// Editor+ zone: view, create and edit agents, MCP servers and skills.
	r.Group(func(r chi.Router) {
		r.Use(editorGate.Handler)

		r.Get("/agents", adminAgentsHandler.ListAgents)
		r.Post("/agents", adminAgentsHandler.CreateAgent)
		r.Get("/agents/{id}", adminAgentsHandler.GetAgent)
		r.Put("/agents/{id}", adminAgentsHandler.UpdateAgent)

		r.Get("/mcp", adminMCPHandler.ListMCPServers)
		r.Post("/mcp", adminMCPHandler.CreateMCPServer)
		r.Get("/mcp/{id}", adminMCPHandler.GetMCPServer)
		r.Put("/mcp/{id}", adminMCPHandler.UpdateMCPServer)

		if adminSkillsHandler != nil {
			r.Get("/skills", adminSkillsHandler.ListSkills)
			r.Post("/skills", adminSkillsHandler.CreateSkill)
			r.Get("/skills/{id}", adminSkillsHandler.GetSkill)
			r.Put("/skills/{id}", adminSkillsHandler.UpdateSkill)
		}
	})

	// Admin-only zone: delete + permissions of the above, and every
	// other admin section (settings, groups, observability, audit, LLM,
	// users, integrations). Editors are forbidden here.
	r.Group(func(r chi.Router) {
		r.Use(adminGate.Handler)

		// Delete + permissions of the editable resources
		r.Delete("/agents/{id}", adminAgentsHandler.DeleteAgent)
		r.Put("/agents/{id}/permissions", adminAgentsHandler.UpdatePermissions)
		r.Delete("/mcp/{id}", adminMCPHandler.DeleteMCPServer)
		r.Put("/mcp/{id}/permissions", adminMCPHandler.UpdateMCPPermissions)
		if adminSkillsHandler != nil {
			r.Delete("/skills/{id}", adminSkillsHandler.DeleteSkill)
			r.Put("/skills/{id}/permissions", adminSkillsHandler.UpdateSkillPermissions)
		}

		// Admin general configuration (runtime settings)
		if adminDeps.SettingsService != nil && adminDeps.SettingsRepo != nil {
			adminSettingsHandler := handlers.NewAdminSettingsHandler(adminDeps.SettingsService, adminDeps.SettingsRepo, adminDeps.AuditRepo, logger)
			r.Get("/settings", adminSettingsHandler.ListSettings)
			r.Put("/settings", adminSettingsHandler.UpdateSettings)
		}

		// Admin groups
		adminGroupsHandler := handlers.NewAdminGroupsHandler(adminDeps.GroupRepo, adminDeps.AuditRepo, logger)
		r.Get("/groups", adminGroupsHandler.ListGroups)
		r.Post("/groups", adminGroupsHandler.CreateGroup)
		r.Get("/groups/{id}", adminGroupsHandler.GetGroup)
		r.Put("/groups/{id}", adminGroupsHandler.UpdateGroup)
		r.Delete("/groups/{id}", adminGroupsHandler.DeleteGroup)
		r.Put("/groups/{id}/permissions", adminGroupsHandler.UpdateGroupPermissions)

		// Admin audit log (detailed activity with prompt/response content)
		if adminDeps.AuditEventRepo != nil {
			adminAuditHandler := handlers.NewAdminAuditHandler(adminDeps.AuditEventRepo, logger)
			r.Get("/audit", adminAuditHandler.ListAuditEvents)
		}

		// Admin MCP OAuth2 scope mappings
		if adminDeps.OAuth2Manager != nil {
			mcpOAuthHandler := handlers.NewMCPOAuthHandler(adminDeps.OAuth2Manager, adminDeps.MCPRepo, mcpRegistry, logger)
			r.Get("/mcp/{id}/scope-mappings", mcpOAuthHandler.ListScopeMappings)
			r.Put("/mcp/{id}/scope-mappings", mcpOAuthHandler.UpsertScopeMapping)
			r.Delete("/mcp/{id}/scope-mappings/{mappingId}", mcpOAuthHandler.DeleteScopeMapping)
		}

		// Admin LLM
		adminProviderHandler := handlers.NewAdminProviderHandler(adminDeps.ProviderRepo, adminDeps.AuditRepo, logger)
		r.Get("/llm-providers", adminProviderHandler.List)
		r.Post("/llm-providers", adminProviderHandler.Create)
		r.Get("/llm-providers/{id}", adminProviderHandler.Get)
		r.Put("/llm-providers/{id}", adminProviderHandler.Update)
		r.Delete("/llm-providers/{id}", adminProviderHandler.Delete)

		adminLLMHandler := handlers.NewAdminLLMHandler(adminDeps.LLMRepo, adminDeps.AuditRepo, logger)
		r.Get("/llm", adminLLMHandler.ListLLMModels)
		r.Post("/llm", adminLLMHandler.CreateLLMModel)
		r.Get("/llm/{id}", adminLLMHandler.GetLLMModel)
		r.Put("/llm/{id}", adminLLMHandler.UpdateLLMModel)
		r.Delete("/llm/{id}", adminLLMHandler.DeleteLLMModel)

		// Admin users
		adminUsersHandler := handlers.NewAdminUsersHandler(adminDeps.UserRepo, adminDeps.AuditRepo, cfg.Auth.AdminUsers, logger)
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

	}) // end admin-only zone

	return r
}

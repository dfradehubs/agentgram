package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"

	_ "github.com/dfradehubs/agentgram-api/docs/swagger" // swagger generated docs
	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/config"
	"github.com/dfradehubs/agentgram-api/internal/crypto"
	"github.com/dfradehubs/agentgram-api/internal/db"
	lf "github.com/dfradehubs/agentgram-api/internal/langfuse"
	"github.com/dfradehubs/agentgram-api/internal/mcp"
	"github.com/dfradehubs/agentgram-api/internal/metrics"
	"github.com/dfradehubs/agentgram-api/internal/proxy"
	"github.com/dfradehubs/agentgram-api/internal/pubsub"
	slackpkg "github.com/dfradehubs/agentgram-api/internal/slack"
	"github.com/dfradehubs/agentgram-api/internal/repository/postgres"
	"github.com/dfradehubs/agentgram-api/internal/server"
	"github.com/dfradehubs/agentgram-api/internal/service"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"github.com/dfradehubs/agentgram-api/internal/summarizer"
	"github.com/dfradehubs/agentgram-api/internal/tracing"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// @title Agentgram API
// @version 1.0
// @description Multiplexer API for AI agents with AG-UI protocol. Connects to remote agents, handles authentication, permissions, and emits AG-UI SSE events.
// @host localhost:8080
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description JWT Bearer token (e.g. "Bearer eyJhbG...")
// @securityDefinitions.apikey CookieAuth
// @in cookie
// @name session
// @description OIDC session cookie
func main() {
	// Load configuration from YAML
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./configs/config.yaml"
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("failed to load config from %s: %v", configPath, err)
	}

	// Setup tracing
	tracingShutdown, err := tracing.Init(context.Background(), cfg.Tracing)
	if err != nil {
		log.Fatalf("failed to init tracing: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tracingShutdown(ctx)
	}()

	// Setup logger
	logger := setupLogger(cfg)
	defer logger.Sync()

	logger.Info("starting agentgram backend",
		zap.String("port", cfg.Server.Port),
		zap.String("config_path", configPath))

	// Create Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.Addr,
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		PoolSize:     cfg.Redis.PoolSize,
		MinIdleConns: cfg.Redis.MinIdleConns,
	})

	// Verify Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Fatal("failed to connect to Redis",
			zap.String("addr", cfg.Redis.Addr),
			zap.Error(err))
	}
	logger.Info("connected to Redis", zap.String("addr", cfg.Redis.Addr))

	// Create session store
	sessionStore := store.NewRedisSessionStore(rdb, logger)

	// Create auth session store (for OIDC sessions)
	var authSessionStore store.AuthSessionStore
	if cfg.Auth.Keycloak.Enabled {
		authSessionStore = store.NewRedisAuthSessionStore(rdb, cfg.Auth.SessionMaxAge, logger)
		logger.Info("OIDC auth session store initialized")
	}

	// PostgreSQL is required
	if cfg.Database.User == "" || cfg.Database.DBName == "" {
		logger.Fatal("PostgreSQL configuration is required (database.user and database.dbname)")
	}

	logger.Info("PostgreSQL configured, initializing database",
		zap.String("host", cfg.Database.Host),
		zap.Int("port", cfg.Database.Port),
		zap.String("dbname", cfg.Database.DBName))

	// Run migrations
	migrationsPath := os.Getenv("MIGRATIONS_PATH")
	if migrationsPath == "" {
		migrationsPath = "/migrations"
	}
	if err := db.RunMigrations(cfg.Database, migrationsPath); err != nil {
		logger.Fatal("failed to run migrations", zap.Error(err))
	}
	logger.Info("database migrations complete")

	// Create connection pool
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer dbCancel()
	pool, err := db.NewPool(dbCtx, cfg.Database)
	if err != nil {
		logger.Fatal("failed to connect to PostgreSQL", zap.Error(err))
	}
	logger.Info("connected to PostgreSQL")

	// Initialize cookie encryption key (auto-generated, stored in DB)
	var cookieCrypto *auth.CookieCrypto
	{
		encKey, err := store.GetOrCreateEncryptionKey(dbCtx, pool, logger)
		if err != nil {
			logger.Fatal("failed to get/create encryption key", zap.Error(err))
		}
		cookieCrypto, err = auth.NewCookieCrypto(encKey)
		if err != nil {
			logger.Fatal("failed to create cookie crypto", zap.Error(err))
		}
	}

	// Initialize metrics
	metrics.Init(cfg.Metrics.Enabled)
	if cfg.Metrics.Enabled {
		logger.Info("Prometheus metrics enabled")
	}

	// Initialize Langfuse tracer
	lfTracer := lf.New(&cfg.Langfuse, logger)
	defer lfTracer.Close()

	// Initialize data encryption key for API keys at rest.
	// Uses ENCRYPTION_KEY env var if set, otherwise auto-generates and stores in DB.
	encryptionKey, err := store.GetOrCreateSettingsKey(dbCtx, pool, "encryption_key", logger)
	if err != nil {
		logger.Fatal("failed to get/create encryption key", zap.Error(err))
	}
	dataCipher, err := crypto.New(encryptionKey)
	if err != nil {
		logger.Fatal("failed to create data cipher", zap.Error(err))
	}

	// Create repositories
	userRepo := postgres.NewUserRepository(pool)
	agentRepo := postgres.NewAgentRepository(pool)
	mcpRepo := postgres.NewMCPServerRepository(pool)
	auditRepo := postgres.NewAuditRepository(pool)
	llmRepo := postgres.NewLLMModelRepository(pool, dataCipher)
	chatEventRepo := postgres.NewChatEventRepository(pool)
	basicAuthRepo := postgres.NewBasicAuthRepository(pool)
	groupRepo := postgres.NewGroupRepository(pool)
	shareRepo := postgres.NewSharedSessionRepository(pool)
	slackRepo := postgres.NewSlackIntegrationRepository(pool, dataCipher)
	slackLinkRepo := postgres.NewSlackUserLinkRepository(pool, dataCipher)

	// Migrate plaintext API keys to encrypted form
	migrated, err := llmRepo.MigrateEncryptKeys(dbCtx)
	if err != nil {
		logger.Fatal("failed to encrypt existing LLM API keys", zap.Error(err))
	}
	if migrated > 0 {
		logger.Info("encrypted plaintext LLM API keys", zap.Int("count", migrated))
	}

	// Create services
	userService := service.NewUserService(userRepo, cfg.Auth.AdminUsers, cfg.Auth.AdminGroups)
	bootstrapService := service.NewBootstrapService(userRepo, basicAuthRepo, logger)

	// Seed admin users from config.yaml
	seedCtx, seedCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer seedCancel()
	if err := bootstrapService.SeedAdminUsers(seedCtx, cfg); err != nil {
		logger.Fatal("failed to seed admin users", zap.Error(err))
	}
	if err := bootstrapService.SeedBasicAuthUsers(seedCtx, cfg); err != nil {
		logger.Fatal("failed to seed basic auth users", zap.Error(err))
	}

	// Create DB-backed agent registry and load cache
	registry := agents.NewDBRegistry(agentRepo, logger)
	loadCtx, loadCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer loadCancel()
	if err := registry.LoadFromDB(loadCtx); err != nil {
		logger.Fatal("failed to load agents from DB", zap.Error(err))
	}
	logger.Info("loaded agents from database", zap.Int("agent_count", registry.Count()))

	// Periodically refresh agent registry from DB (keeps all pods in sync)
	registry.StartAutoRefresh()
	defer registry.StopAutoRefresh()

	// Create DB-backed MCP registry and load
	mcpToolCallTimeout := 2 * time.Minute // default
	if d, err := time.ParseDuration(cfg.MCPServer.ToolCallTimeout); err == nil {
		mcpToolCallTimeout = d
	}
	mcpRegistry := mcp.NewDBRegistry(mcpRepo, mcpToolCallTimeout, logger)
	if err := mcpRegistry.LoadFromDB(loadCtx); err != nil {
		logger.Fatal("failed to load MCP servers from DB", zap.Error(err))
	}
	logger.Info("loaded MCP servers from database")

	// Periodically refresh MCP registry from DB (keeps all pods in sync)
	mcpRegistry.StartPeriodicRefresh(30 * time.Second)
	defer mcpRegistry.Stop()

	// Initialize MCP OAuth2 Manager (for OAuth2-protected MCP servers)
	var oauth2Mgr *mcp.OAuth2Manager
	{
		mcpTokenStore := store.NewMCPTokenStore(rdb, cookieCrypto, logger)
		host := cfg.Server.Host
		if host == "" {
			host = "localhost:" + cfg.Server.Port
		}
		scheme := "https"
		if !cfg.Auth.CookieSecure {
			scheme = "http"
		}
		callbackURL := scheme + "://" + host + "/auth/mcp-oauth/callback"
		oauth2Mgr = mcp.NewOAuth2Manager(mcpTokenStore, rdb, callbackURL, logger)
		logger.Info("MCP OAuth2 manager initialized", zap.String("callback_url", callbackURL))
	}

	// Start chat_events cleanup goroutine
	if cfg.Metrics.Enabled && cfg.Metrics.RetentionDays > 0 {
		cleanupInterval := time.Hour
		if cfg.Metrics.CleanupInterval != "" {
			if d, err := time.ParseDuration(cfg.Metrics.CleanupInterval); err == nil {
				cleanupInterval = d
			}
		}
		go func() {
			ticker := time.NewTicker(cleanupInterval)
			defer ticker.Stop()
			for range ticker.C {
				cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
				deleted, err := chatEventRepo.Cleanup(cleanCtx, cfg.Metrics.RetentionDays)
				cleanCancel()
				if err != nil {
					logger.Warn("chat_events cleanup failed", zap.Error(err))
				} else if deleted > 0 {
					logger.Info("chat_events cleanup", zap.Int64("deleted", deleted))
				}
			}
		}()
		logger.Info("chat_events cleanup started",
			zap.Int("retention_days", cfg.Metrics.RetentionDays),
			zap.Duration("interval", cleanupInterval))
	}

	// Create Pub/Sub hub for real-time collaborative sessions
	pubsubHub := pubsub.NewHub(rdb, logger)

	// Create Slack bot manager (OIDC client for group resolution, nil if auth disabled)
	var oidcClientForSlack *auth.OIDCClient
	if cfg.Auth.Keycloak.Enabled {
		keycloakProv := auth.NewKeycloakProvider(cfg.Auth.Keycloak.Issuer, time.Duration(cfg.Auth.Keycloak.JWKSCacheTTL)*time.Second)
		oidcClientForSlack = auth.NewOIDCClient(cfg.Auth.Keycloak, keycloakProv)
	}
	// Create summarizer for Slack thread context
	var slackSummarizer *summarizer.Summarizer
	if summarizerModels, err := llmRepo.ListByRole(loadCtx, "summarizer"); err == nil && len(summarizerModels) > 0 {
		slackSummarizer = summarizer.New(summarizerModels[0], logger)
	}

	var githubClientForSlack *auth.GitHubOAuthClient
	if cfg.Auth.GitHub.Enabled {
		githubClientForSlack = auth.NewGitHubOAuthClient(cfg.Auth.GitHub)
	}

	hostURL := "https://" + cfg.Server.Host
	slackBotManager := slackpkg.NewBotManager(rdb, slackRepo, slackLinkRepo, registry, groupRepo, sessionStore, proxy.NewProxy(logger), oidcClientForSlack, githubClientForSlack, dataCipher, slackSummarizer, lfTracer, chatEventRepo, hostURL, logger)
	slackCtx, slackCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := slackBotManager.Start(slackCtx); err != nil {
		logger.Warn("failed to start some Slack bots", zap.Error(err))
	}
	slackCancel()
	defer slackBotManager.Stop()

	// Register Slack metrics
	if cfg.Metrics.Enabled {
		slackpkg.RegisterMetrics()
	}

	// Build admin deps
	adminDeps := &server.AdminDeps{
		UserService:    userService,
		AgentRepo:      agentRepo,
		MCPRepo:        mcpRepo,
		UserRepo:       userRepo,
		AuditRepo:      auditRepo,
		LLMRepo:        llmRepo,
		GroupRepo:      groupRepo,
		MCPRegistry:    mcpRegistry,
		ChatEventRepo:  chatEventRepo,
		BasicAuthRepo:  basicAuthRepo,
		PubSubHub:      pubsubHub,
		ShareRepo:      shareRepo,
		LangfuseTracer: lfTracer,
		SlackRepo:      slackRepo,
		SlackLinkRepo:  slackLinkRepo,
		BotManager:     slackBotManager,
		DataCipher:     dataCipher,
		RedisClient:    rdb,
		OAuth2Manager:  oauth2Mgr,
	}

	// Create and start server (pass rdb as closer for graceful shutdown)
	srv := server.New(cfg, registry, sessionStore, authSessionStore, cookieCrypto, logger, adminDeps, rdb)
	if err := srv.Start(); err != nil {
		logger.Fatal("server error", zap.Error(err))
	}
}

func setupLogger(cfg *config.Config) *zap.Logger {
	var zapConfig zap.Config

	// Configure based on format: console or json
	if cfg.Logging.Format == "console" {
		zapConfig = zap.NewDevelopmentConfig()
		zapConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		zapConfig = zap.NewProductionConfig()
	}

	// Configure log level
	switch cfg.Logging.Level {
	case "debug":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	logger, err := zapConfig.Build()
	if err != nil {
		log.Fatalf("failed to setup logger: %v", err)
	}

	return logger
}

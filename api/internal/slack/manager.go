package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/crypto"
	lf "github.com/dfradehubs/agentgram-api/internal/langfuse"
	"github.com/dfradehubs/agentgram-api/internal/proxy"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"github.com/dfradehubs/agentgram-api/internal/summarizer"
	"go.uber.org/zap"
)

const syncInterval = 60 * time.Second

// BotManager orchestrates the lifecycle of multiple Slack bots (one per agent).
// All pods run all bots; event dedup via Redis prevents duplicate processing.
type BotManager struct {
	bots       map[string]*Bot
	mu         sync.RWMutex
	rdb        *redis.Client
	instanceID string
	slackRepo  repository.SlackIntegrationRepository
	linkRepo   repository.SlackUserLinkRepository
	registry   *agents.Registry
	groupRepo  repository.GroupRepository
	sessionStore store.SessionStore
	proxyInst  *proxy.Proxy
	oidcClient   *auth.OIDCClient
	githubClient *auth.GitHubOAuthClient
	cipher       *crypto.AESCrypto
	summarizer *summarizer.Summarizer
	lfTracer      *lf.Tracer
	chatEventRepo repository.ChatEventRepository
	hostURL       string
	logger     *zap.Logger
	stopCh     chan struct{}
	stopped    chan struct{}
}

// NewBotManager creates a new BotManager.
func NewBotManager(
	rdb *redis.Client,
	slackRepo repository.SlackIntegrationRepository,
	linkRepo repository.SlackUserLinkRepository,
	registry *agents.Registry,
	groupRepo repository.GroupRepository,
	sessionStore store.SessionStore,
	proxyInst *proxy.Proxy,
	oidcClient *auth.OIDCClient,
	githubClient *auth.GitHubOAuthClient,
	cipher *crypto.AESCrypto,
	sum *summarizer.Summarizer,
	lfTracer *lf.Tracer,
	chatEventRepo repository.ChatEventRepository,
	hostURL string,
	logger *zap.Logger,
) *BotManager {
	return &BotManager{
		bots:         make(map[string]*Bot),
		rdb:          rdb,
		instanceID:   uuid.New().String(),
		slackRepo:    slackRepo,
		linkRepo:     linkRepo,
		registry:     registry,
		groupRepo:    groupRepo,
		sessionStore: sessionStore,
		proxyInst:    proxyInst,
		oidcClient:    oidcClient,
		githubClient:  githubClient,
		cipher:        cipher,
		summarizer:   sum,
		lfTracer:      lfTracer,
		chatEventRepo: chatEventRepo,
		hostURL:       hostURL,
		logger:       logger.Named("slack-manager"),
		stopCh:       make(chan struct{}),
		stopped:      make(chan struct{}),
	}
}

// Start loads enabled integrations from DB and starts their bots.
// It also starts a periodic sync goroutine.
func (m *BotManager) Start(ctx context.Context) error {
	integrations, err := m.slackRepo.ListEnabled(ctx)
	if err != nil {
		return err
	}

	for _, integ := range integrations {
		if err := m.startBot(integ.AgentID, integ.BotToken, integ.AppToken); err != nil {
			m.logger.Warn("failed to start slack bot",
				zap.String("agent_id", integ.AgentID),
				zap.Error(err))
		}
	}

	m.logger.Info("slack bot manager started",
		zap.Int("bots", len(m.bots)),
		zap.String("instance_id", m.instanceID))

	// Periodic sync
	go m.syncLoop()
	// Listen for reprocess messages (after account linking)
	go m.reprocessListener()
	return nil
}

// Stop gracefully shuts down all bots.
func (m *BotManager) Stop() {
	close(m.stopCh)
	<-m.stopped

	m.mu.Lock()
	defer m.mu.Unlock()

	for agentID, bot := range m.bots {
		m.logger.Info("stopping slack bot", zap.String("agent_id", agentID))
		bot.Stop()
		delete(m.bots, agentID)
	}
	SlackBotsActive.Set(0)
	m.logger.Info("slack bot manager stopped")
}

// StartBot starts or restarts a bot for a given agent.
func (m *BotManager) StartBot(agentID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	integ, err := m.slackRepo.Get(ctx, agentID)
	if err != nil {
		return err
	}
	if integ == nil || !integ.Enabled {
		return nil
	}

	m.mu.Lock()
	if existing, ok := m.bots[agentID]; ok {
		existing.Stop()
		delete(m.bots, agentID)
	}
	m.mu.Unlock()

	return m.startBot(agentID, integ.BotToken, integ.AppToken)
}

// StopBot stops a bot for a given agent.
func (m *BotManager) StopBot(agentID string) {
	m.mu.Lock()
	bot, ok := m.bots[agentID]
	if ok {
		delete(m.bots, agentID)
	}
	m.mu.Unlock()

	if ok {
		bot.Stop()
		SlackBotsActive.Dec()
		m.logger.Info("stopped slack bot", zap.String("agent_id", agentID))
	}
}

// RestartBot restarts a bot (e.g., after token change).
func (m *BotManager) RestartBot(agentID string) error {
	m.StopBot(agentID)
	return m.StartBot(agentID)
}

// GetStatus returns the current status for an agent's Slack integration.
func (m *BotManager) GetStatus(agentID string) (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	integ, err := m.slackRepo.Get(ctx, agentID)
	if err != nil || integ == nil {
		return "disconnected", ""
	}
	return integ.Status, integ.StatusMessage
}

func (m *BotManager) startBot(agentID, botToken, appToken string) error {
	dedup := NewEventDedup(m.rdb, m.instanceID)
	emailResolver := NewEmailResolver(5*time.Minute, 500)
	handler := NewMessageHandler(
		m.registry,
		m.groupRepo,
		m.sessionStore,
		m.rdb,
		m.proxyInst,
		emailResolver,
		dedup,
		m.oidcClient,
		m.githubClient,
		m.linkRepo,
		m.cipher,
		m.summarizer,
		m.lfTracer,
		m.chatEventRepo,
		m.hostURL,
		m.logger,
	)

	bot := NewBot(agentID, botToken, appToken, handler, m.onBotStatus, m.logger)
	bot.Start()

	m.mu.Lock()
	m.bots[agentID] = bot
	m.mu.Unlock()

	SlackBotsActive.Inc()
	return nil
}

func (m *BotManager) onBotStatus(agentID, status, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.slackRepo.UpdateStatus(ctx, agentID, status, message); err != nil {
		m.logger.Warn("failed to update bot status in DB",
			zap.String("agent_id", agentID),
			zap.String("status", status),
			zap.Error(err))
	}
}

func (m *BotManager) syncLoop() {
	defer close(m.stopped)
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.syncFromDB()
		}
	}
}

func (m *BotManager) syncFromDB() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	integrations, err := m.slackRepo.ListEnabled(ctx)
	if err != nil {
		m.logger.Warn("failed to sync slack integrations from DB", zap.Error(err))
		return
	}

	// Build set of enabled agent IDs
	enabled := make(map[string]bool)
	for _, integ := range integrations {
		enabled[integ.AgentID] = true
	}

	// Stop bots that are no longer enabled
	m.mu.RLock()
	var toStop []string
	for agentID := range m.bots {
		if !enabled[agentID] {
			toStop = append(toStop, agentID)
		}
	}
	m.mu.RUnlock()

	for _, agentID := range toStop {
		m.StopBot(agentID)
		m.logger.Info("stopped disabled slack bot during sync", zap.String("agent_id", agentID))
	}

	// Start bots that are enabled but not running
	for _, integ := range integrations {
		m.mu.RLock()
		_, running := m.bots[integ.AgentID]
		m.mu.RUnlock()

		if !running {
			if err := m.startBot(integ.AgentID, integ.BotToken, integ.AppToken); err != nil {
				m.logger.Warn("failed to start slack bot during sync",
					zap.String("agent_id", integ.AgentID),
					zap.Error(err))
			} else {
				m.logger.Info("started slack bot during sync", zap.String("agent_id", integ.AgentID))
			}
		}
	}
}

// reprocessListener listens for reprocess messages after account linking.
func (m *BotManager) reprocessListener() {
	sub := m.rdb.Subscribe(context.Background(), "slack:reprocess")
	defer sub.Close()

	ch := sub.Channel()
	for {
		select {
		case <-m.stopCh:
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var payload struct {
				SlackUserID string `json:"slack_user_id"`
				ChannelID   string `json:"channel_id"`
				ThreadTS    string `json:"thread_ts"`
				AgentID     string `json:"agent_id"`
				Text        string `json:"text"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
				m.logger.Warn("invalid reprocess payload", zap.Error(err))
				continue
			}

			m.mu.RLock()
			bot, ok := m.bots[payload.AgentID]
			m.mu.RUnlock()
			if !ok || bot == nil {
				continue
			}

			m.logger.Info("reprocessing message after account linking",
				zap.String("agent_id", payload.AgentID),
				zap.String("slack_user_id", payload.SlackUserID))

			// Deterministic event TS so dedup works across pods
			eventTS := fmt.Sprintf("link-%s-%s-%s", payload.SlackUserID, payload.ChannelID, payload.ThreadTS)
			go bot.handler.HandleMessage(
				context.Background(),
				bot.client,
				payload.AgentID,
				eventTS,
				payload.SlackUserID,
				payload.ChannelID,
				payload.ThreadTS,
				payload.Text,
			)
		}
	}
}


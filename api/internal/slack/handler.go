package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/crypto"
	lf "github.com/dfradehubs/agentgram-api/internal/langfuse"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/proxy"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"github.com/dfradehubs/agentgram-api/internal/summarizer"
	slackapi "github.com/slack-go/slack"
	"go.uber.org/zap"
)

const (
	threadCacheTTL       = 60 * time.Second
	threadCacheMaxItems  = 100
	groupsCacheTTL       = 5 * time.Minute
	SlackSessionsGroupID = "slack-sessions"
)

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

type cachedThread struct {
	messages []slackapi.Message
	expiry   time.Time
}

type cachedGroups struct {
	groups []string
	expiry time.Time
}

// MessageHandler processes Slack messages: auth, session, proxy call, response.
type MessageHandler struct {
	registry      *agents.Registry
	groupRepo     repository.GroupRepository
	sessionStore  store.SessionStore
	rdb           *redis.Client
	proxy         *proxy.Proxy
	emailResolver *EmailResolver
	dedup         *EventDedup
	oidcClient    *auth.OIDCClient
	githubClient  *auth.GitHubOAuthClient
	linkRepo      repository.SlackUserLinkRepository
	cipher        *crypto.AESCrypto
	summarizer    *summarizer.Summarizer
	lfTracer      *lf.Tracer
	chatEventRepo repository.ChatEventRepository
	hostURL       string
	formatter     *Formatter
	logger        *zap.Logger

	// Thread context cache
	threadCache   map[string]cachedThread
	threadCacheMu sync.Mutex

	// Groups cache (email → groups)
	groupsCache   map[string]cachedGroups
	groupsCacheMu sync.Mutex

}

// NewMessageHandler creates a new MessageHandler.
func NewMessageHandler(
	registry *agents.Registry,
	groupRepo repository.GroupRepository,
	sessionStore store.SessionStore,
	rdb *redis.Client,
	proxyInstance *proxy.Proxy,
	emailResolver *EmailResolver,
	dedup *EventDedup,
	oidcClient *auth.OIDCClient,
	githubClient *auth.GitHubOAuthClient,
	linkRepo repository.SlackUserLinkRepository,
	cipher *crypto.AESCrypto,
	sum *summarizer.Summarizer,
	lfTracer *lf.Tracer,
	chatEventRepo repository.ChatEventRepository,
	hostURL string,
	logger *zap.Logger,
) *MessageHandler {
	return &MessageHandler{
		registry:      registry,
		groupRepo:     groupRepo,
		sessionStore:  sessionStore,
		rdb:           rdb,
		proxy:         proxyInstance,
		emailResolver: emailResolver,
		dedup:         dedup,
		oidcClient:    oidcClient,
		githubClient:  githubClient,
		linkRepo:      linkRepo,
		cipher:        cipher,
		summarizer:    sum,
		lfTracer:      lfTracer,
		chatEventRepo: chatEventRepo,
		hostURL:       hostURL,
		formatter:     &Formatter{},
		logger:        logger,
		threadCache:   make(map[string]cachedThread),
		groupsCache:   make(map[string]cachedGroups),
	}
}

// HandleMessage processes a single Slack message event.
func (h *MessageHandler) HandleMessage(ctx context.Context, client *slackapi.Client, agentID, eventTS, slackUserID, channelID, threadTS, text string) {
	requestStart := time.Now()
	logger := h.logger.With(
		zap.String("agent_id", agentID),
		zap.String("channel_id", channelID),
		zap.String("thread_ts", threadTS),
	)

	// 1. Dedup: only one pod processes this event per agent
	// Key includes agentID so different bots can process the same Slack message
	if !h.dedup.TryClaim(ctx, eventTS+":"+agentID) {
		logger.Debug("event already claimed by another pod")
		return
	}

	// 2. Resolve email
	email, err := h.emailResolver.Resolve(ctx, client, slackUserID)
	if err != nil {
		logger.Warn("failed to resolve Slack user email", zap.Error(err))
		h.postError(client, channelID, threadTS, errInternal)
		return
	}
	logger = logger.With(zap.String("user_email", email))

	// 3. Get agent
	agent, err := h.registry.Get(agentID)
	if err != nil || agent == nil {
		logger.Warn("agent not found in registry")
		h.postError(client, channelID, threadTS, errAgentUnavail)
		return
	}

	// 4. Check permissions
	groups := h.resolveUserGroups(ctx, email)
	inherited, _ := h.groupRepo.GetAllInheritedPermissions(ctx)
	if !agents.HasAccessWithInherited(agent, email, groups, inherited[agentID]) {
		logger.Info("user denied access to agent")
		h.postError(client, channelID, threadTS, errNoPermission)
		return
	}

	SlackEventsTotal.WithLabelValues(agentID, "message").Inc()

	// 5. Get or create shared session (one per Slack thread, shared by all users and agents)
	sessionID := fmt.Sprintf("slack-%s-%s", channelID, threadTS)
	sessionName := truncateString(text, 50)
	if sessionName == "" {
		sessionName = "Slack thread"
	}

	session, err := h.sessionStore.CreateSessionWithID(ctx, sessionID, email, agentID, sessionName)
	if err != nil {
		logger.Error("failed to get/create session", zap.Error(err))
		h.postError(client, channelID, threadTS, errInternal)
		return
	}

	// Mark as Slack session + multi-agent (no group — Slack Sessions is its own concept)
	needsSave := false
	if session.Source != "slack" {
		session.Source = "slack"
		session.IsMultiAgent = true
		session.SlackThreadID = sessionID
		needsSave = true
	}

	// Add this agent to the session if not already present
	if !containsString(session.AgentIDs, agentID) {
		session.AgentIDs = append(session.AgentIDs, agentID)
		needsSave = true
	}

	if needsSave {
		h.sessionStore.SaveSession(ctx, session)
	}

	// Register this user as participant
	h.sessionStore.RegisterSlackParticipant(ctx, sessionID, email, agentID)

	// 6. Build user message with thread context
	// Read the thread to find messages the bot hasn't seen (between last bot reply and now)
	userMessage := h.buildUserMessage(ctx, client, channelID, threadTS, text, agentID)
	if userMessage == "" {
		return
	}

	// Save the user message to session (with identity, like proxy handler)
	h.sessionStore.AddMessage(ctx, sessionID, models.ChatMessage{
		Role:              "user",
		Content:           userMessage,
		UserEmail:         email,
		UserName:          email,
		BroadcastAgentIDs: []string{agentID},
	})

	// 7. Build ChatRequest — use multi-agent context preparation (like proxy handler)
	// This calculates delta: messages from other agents since this agent's last interaction
	fullSession, err := h.sessionStore.GetSession(ctx, sessionID)
	if err != nil || fullSession == nil {
		logger.Error("failed to get session for chat request", zap.Error(err))
		h.postError(client, channelID, threadTS, errInternal)
		return
	}

	// Use per-user agent session mapping: each user has their own agent session
	// because agents validate sessions against the user's JWT. The shared context
	// between users is handled by PrepareMessagesForMultiAgent.
	// Fallback to the shared (legacy) key for backward compatibility with
	// sessions created before per-user mapping was introduced.
	agentSessionID, _ := h.sessionStore.GetUserAgentSessionID(ctx, sessionID, agentID, email)
	if agentSessionID == "" {
		agentSessionID, _ = h.sessionStore.GetAgentSessionID(ctx, sessionID, agentID)
	}
	hasAgentSession := agentSessionID != ""

	lastUserMsg := models.ChatMessage{
		Role:      "user",
		Content:   userMessage,
		UserEmail: email,
		UserName:  email,
	}

	var messagesToSend []models.ChatMessage
	if len(fullSession.AgentIDs) > 1 || !hasAgentSession {
		// Multi-agent or first interaction: send context from other agents
		result := proxy.PrepareMessagesForMultiAgent(
			fullSession, agentID, lastUserMsg, hasAgentSession,
			true, // sendContext
			agent.MaxContextTokens, agent.SummarizeThreshold,
			h.summarizer, ctx,
		)
		messagesToSend = result.Messages
	} else {
		// Single agent, has session: send all messages
		messagesToSend = proxy.PrepareMessagesForAgent(fullSession, lastUserMsg, hasAgentSession)
	}

	// Prefix [UserName] on last user message so agent knows who's asking
	if lastIdx := len(messagesToSend) - 1; lastIdx >= 0 && messagesToSend[lastIdx].Role == "user" {
		messagesToSend[lastIdx].Content = fmt.Sprintf("[%s]: %s", email, messagesToSend[lastIdx].Content)
	}

	chatReq := &models.ChatRequest{
		Messages:  messagesToSend,
		SessionID: agentSessionID,
	}

	// 8. Get user's real JWT via account linking (offline refresh token)
	authHeader, err := h.getUserAuthHeader(ctx, slackUserID)
	if err != nil {
		logger.Info("user not linked, sending link message", zap.Error(err))
		h.sendLinkMessage(client, channelID, threadTS, slackUserID, agentID, text)
		return
	}

	// 8b. Check GitHub token if agent requires it (with auto-refresh)
	if agent.RequireGitHubToken {
		ghToken, err := h.getGitHubToken(ctx, slackUserID)
		if err != nil {
			h.sendGitHubLinkMessage(client, channelID, threadTS, slackUserID)
			return
		}
		ctx = context.WithValue(ctx, middleware.GitHubTokenContextKey, ghToken)
	}

	// 9. Langfuse trace
	var lfTrace *lf.Trace
	var agentSpan *lf.Span
	if h.lfTracer != nil && h.lfTracer.Enabled() {
		lfTrace = h.lfTracer.StartTrace(ctx, "slack-chat", email, sessionID, map[string]interface{}{
			"agent_id":       agentID,
			"agent_name":     agent.Name,
			"agent_protocol": agent.Protocol,
			"source":         "slack",
			"channel":        channelID,
		})
		if userMessage != "" {
			lfTrace.SetInput(userMessage)
		}
		ctx = lf.ContextWithTrace(ctx, lfTrace)

		// Agent call span (same format as proxy handler)
		msgs := make([]map[string]string, 0, len(chatReq.Messages))
		for _, m := range chatReq.Messages {
			content := m.Content
			if len(content) > 1000 {
				content = content[:1000] + "..."
			}
			msgs = append(msgs, map[string]string{"role": m.Role, "content": content})
		}
		agentSpan = lfTrace.StartToolCall(fmt.Sprintf("proxy:%s", agentID), map[string]interface{}{
			"agent_name":     agent.Name,
			"agent_protocol": agent.Protocol,
			"messages":       msgs,
		})
	}

	// 10. Post "Escribiendo..." immediately BEFORE calling the proxy
	sw := NewStreamingWriter(client, channelID, threadTS, agentID, h.formatter, logger)
	sw.PostInitialMessage()
	result, err := h.proxy.Handle(ctx, sw, agent, chatReq, authHeader, proxy.HandleOptions{
		ThreadID: sessionID,
	})
	sw.Finalize()

	// Close agent span
	if agentSpan != nil {
		if err != nil {
			agentSpan.EndWithError(err)
		} else if result != nil {
			text := result.AssistantText
			if len(text) > 2000 {
				text = text[:2000] + "..."
			}
			agentSpan.End(text)
		} else {
			agentSpan.End(nil)
		}
	}

	if err != nil {
		logger.Error("proxy call failed", zap.Error(err))
		if sw.FullText() == "" {
			h.postError(client, channelID, threadTS, classifyError(err))
		}
		if lfTrace != nil {
			lfTrace.End(false, map[string]interface{}{"error": err.Error()})
		}
		// Record error event
		if h.chatEventRepo != nil {
			go func() {
				evtCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				h.chatEventRepo.Insert(evtCtx, &models.ChatEvent{
					ResourceType: "agent", ResourceID: agentID, ResourceName: agent.Name,
					Protocol: agent.Protocol, UserEmail: email, SessionID: sessionID,
					Status: "error", ErrorType: "proxy_error", ErrorMsg: err.Error(),
					DurationMs: int(time.Since(requestStart).Milliseconds()), Source: "slack",
				})
			}()
		}
		SlackMessagesForwardedTotal.WithLabelValues(agentID, "error").Inc()
		return
	}

	// Save per-user agent session mapping (each user gets their own agent session)
	if result != nil && result.Error == "" && result.AgentSessionID != "" && result.AgentSessionID != agentSessionID {
		if err := h.sessionStore.SetUserAgentSessionID(ctx, sessionID, agentID, email, result.AgentSessionID); err != nil {
			logger.Warn("failed to save agent session mapping", zap.Error(err))
		}
	} else if agentSessionID == "" {
		// No agent session ID returned — map our session ID so next call has one
		h.sessionStore.SetUserAgentSessionID(ctx, sessionID, agentID, email, sessionID)
	}

	// 10. Save assistant response with real tool calls (same as proxy handler)
	if result != nil && (result.AssistantText != "" || len(result.ToolCalls) > 0) {
		assistantMsg := models.ChatMessage{
			Role:    "assistant",
			Content: result.AssistantText,
			AgentID: agentID, // Tag with responding agent (for multi-agent UI)
		}

		// Convert ContentParts from proxy result
		if len(result.ContentParts) > 0 {
			for _, cp := range result.ContentParts {
				assistantMsg.ContentParts = append(assistantMsg.ContentParts, models.ContentPart{
					Type:      cp.Type,
					Text:      cp.Text,
					ToolIndex: models.IntPtr(cp.ToolIndex),
				})
			}
		}

		// Persist real tool calls
		if len(result.ToolCalls) > 0 {
			for _, tc := range result.ToolCalls {
				var args map[string]interface{}
				if tc.Args != "" {
					json.Unmarshal([]byte(tc.Args), &args)
				}
				assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, models.StoredToolCall{
					ID:   tc.ID,
					Name: tc.Name,
					Args: args,
				})
				var resp map[string]interface{}
				if tc.Result != "" {
					if json.Unmarshal([]byte(tc.Result), &resp) != nil {
						resp = map[string]interface{}{"text": tc.Result}
					}
				}
				assistantMsg.ToolResults = append(assistantMsg.ToolResults, models.StoredToolResult{
					ID:       tc.ID,
					Name:     tc.Name,
					Response: resp,
				})
			}
		}

		if err := h.sessionStore.AddMessage(ctx, sessionID, assistantMsg); err != nil {
			logger.Warn("failed to save assistant message to session", zap.Error(err))
		}
	}

	// End Langfuse trace with response
	if lfTrace != nil {
		var output interface{}
		if result != nil && result.AssistantText != "" {
			// Truncate for Langfuse output
			text := result.AssistantText
			if len(text) > 2000 {
				text = text[:2000] + "..."
			}
			output = text
		}
		lfTrace.End(true, output)
	}

	// Record chat event for observability
	if h.chatEventRepo != nil {
		status := "ok"
		var errType, errMsg string
		if result != nil && result.Error != "" {
			status = "error"
			errType = "agent_error"
			errMsg = result.Error
		}
		var toolCallInfos []models.ToolCallInfo
		if result != nil {
			for _, tc := range result.ToolCalls {
				toolCallInfos = append(toolCallInfos, models.ToolCallInfo{Name: tc.Name})
			}
		}
		durationMs := int(time.Since(requestStart).Milliseconds())
		event := &models.ChatEvent{
			ResourceType: "agent",
			ResourceID:   agentID,
			ResourceName: agent.Name,
			Protocol:     agent.Protocol,
			UserEmail:    email,
			SessionID:    sessionID,
			Status:       status,
			ErrorType:    errType,
			ErrorMsg:     errMsg,
			DurationMs:   durationMs,
			MessageCount: len(fullSession.Messages),
			ToolCalls:    toolCallInfos,
			Source:       "slack",
		}
		go func() {
			evtCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := h.chatEventRepo.Insert(evtCtx, event); err != nil {
				logger.Warn("failed to record slack chat event", zap.Error(err))
			}
		}()
	}

	SlackMessagesForwardedTotal.WithLabelValues(agentID, "success").Inc()
	responseLen := 0
	if result != nil {
		responseLen = len(result.AssistantText)
	}
	logger.Info("slack message processed", zap.Int("response_len", responseLen))
}

// buildUserMessage constructs the message to send to the agent.
// Reads the Slack thread, finds messages the bot hasn't seen (between last bot reply and now),
// summarizes them via LLM, and appends the user's actual question.
func (h *MessageHandler) buildUserMessage(ctx context.Context, client *slackapi.Client, channelID, threadTS, text, agentID string) string {
	threadMsgs := h.fetchThreadContext(ctx, client, channelID, threadTS, agentID)

	// Extract unread human messages: after the last bot reply, excluding the last message (current)
	unread := h.extractUnreadMessages(threadMsgs)

	if len(unread) == 0 {
		// No prior context — send the question directly
		return text
	}

	// Summarize unread context via LLM
	summary := h.summarizeContext(ctx, unread)

	if text != "" {
		if summary != "" {
			return summary + "\n\n" + text
		}
		return text
	}

	// Empty mention — the summary IS the message
	if summary != "" {
		return summary
	}
	return strings.Join(unread, "\n")
}

// extractUnreadMessages returns human messages between the last bot reply and the current message.
func (h *MessageHandler) extractUnreadMessages(threadMsgs []slackapi.Message) []string {
	if len(threadMsgs) == 0 {
		return nil
	}

	// Find index of last bot message
	lastBotIdx := -1
	for i, msg := range threadMsgs {
		if msg.BotID != "" || msg.SubType == "bot_message" {
			lastBotIdx = i
		}
	}

	// Collect human messages after last bot reply, excluding the last message (the one that triggered us)
	var unread []string
	startIdx := lastBotIdx + 1
	endIdx := len(threadMsgs) - 1 // exclude last message (current)

	for i := startIdx; i < endIdx; i++ {
		msg := threadMsgs[i]
		if msg.BotID != "" || msg.SubType == "bot_message" {
			continue
		}
		cleaned := stripMentions(strings.TrimSpace(msg.Text))
		if cleaned != "" {
			unread = append(unread, cleaned)
		}
	}
	return unread
}

// summarizeContext uses the LLM summarizer to condense unread messages into context.
// Falls back to simple concatenation if summarizer is unavailable.
func (h *MessageHandler) summarizeContext(ctx context.Context, messages []string) string {
	if h.summarizer == nil || len(messages) == 0 {
		return strings.Join(messages, "\n")
	}

	// Convert to ChatMessages for the summarizer
	var chatMsgs []models.ChatMessage
	for _, m := range messages {
		chatMsgs = append(chatMsgs, models.ChatMessage{Role: "user", Content: m})
	}

	summary, err := h.summarizer.Summarize(ctx, chatMsgs)
	if err != nil {
		h.logger.Warn("failed to summarize thread context, using raw messages", zap.Error(err))
		return strings.Join(messages, "\n")
	}
	return summary
}

// stripMentions removes Slack user mentions (<@UXXXX>) from text.
func stripMentions(text string) string {
	result := text
	for strings.Contains(result, "<@") {
		start := strings.Index(result, "<@")
		end := strings.Index(result[start:], ">")
		if end == -1 {
			break
		}
		result = result[:start] + result[start+end+1:]
	}
	return strings.TrimSpace(result)
}

// getGitHubToken gets the GitHub token for a Slack user, refreshing if expired.
func (h *MessageHandler) getGitHubToken(ctx context.Context, slackUserID string) (string, error) {
	link, err := h.linkRepo.GetBySlackUserID(ctx, slackUserID)
	if err != nil || link == nil || link.GitHubToken == "" {
		return "", fmt.Errorf("no github token")
	}

	// Try the existing token first (validate with GitHub API)
	if h.githubClient != nil {
		_, err := h.githubClient.GetUser(ctx, link.GitHubToken)
		if err == nil {
			return link.GitHubToken, nil // Token still valid
		}

		// Token expired — try refresh
		if link.GitHubRefreshToken != "" {
			refreshResp, refreshErr := h.githubClient.RefreshGitHubToken(ctx, link.GitHubRefreshToken)
			if refreshErr == nil {
				// Save new tokens
				h.linkRepo.SetGitHubToken(ctx, slackUserID, refreshResp.AccessToken, refreshResp.RefreshToken)
				h.logger.Info("github token refreshed for slack user", zap.String("slack_user_id", slackUserID))
				return refreshResp.AccessToken, nil
			}
			h.logger.Warn("github token refresh failed", zap.String("slack_user_id", slackUserID), zap.Error(refreshErr))
		}

		// Both failed — clear token and ask to re-link
		h.linkRepo.RevokeGitHub(ctx, slackUserID)
		return "", fmt.Errorf("github token expired and refresh failed")
	}

	return link.GitHubToken, nil
}

// resolveUserGroups fetches user groups from Keycloak with caching.
func (h *MessageHandler) resolveUserGroups(ctx context.Context, email string) []string {
	h.groupsCacheMu.Lock()
	if c, ok := h.groupsCache[email]; ok && time.Now().Before(c.expiry) {
		h.groupsCacheMu.Unlock()
		return c.groups
	}
	h.groupsCacheMu.Unlock()

	if h.oidcClient == nil {
		return nil
	}

	groups, err := h.oidcClient.GetUserGroups(ctx, email)
	if err != nil {
		h.logger.Warn("failed to get user groups from Keycloak", zap.String("email", email), zap.Error(err))
		return nil
	}

	h.groupsCacheMu.Lock()
	h.groupsCache[email] = cachedGroups{groups: groups, expiry: time.Now().Add(groupsCacheTTL)}
	h.groupsCacheMu.Unlock()
	return groups
}

// fetchThreadContext fetches the full thread from Slack with LRU caching.
func (h *MessageHandler) fetchThreadContext(ctx context.Context, client *slackapi.Client, channelID, threadTS, agentID string) []slackapi.Message {
	cacheKey := channelID + ":" + threadTS

	h.threadCacheMu.Lock()
	if c, ok := h.threadCache[cacheKey]; ok && time.Now().Before(c.expiry) {
		h.threadCacheMu.Unlock()
		return c.messages
	}
	h.threadCacheMu.Unlock()

	var allMessages []slackapi.Message
	cursor := ""
	for {
		params := &slackapi.GetConversationRepliesParameters{
			ChannelID: channelID,
			Timestamp: threadTS,
			Cursor:    cursor,
			Limit:     100,
		}
		msgs, hasMore, nextCursor, err := client.GetConversationRepliesContext(ctx, params)
		if err != nil {
			h.logger.Warn("failed to fetch thread replies", zap.String("agent_id", agentID), zap.Error(err))
			break
		}
		allMessages = append(allMessages, msgs...)
		SlackAPICallsTotal.WithLabelValues(agentID, "conversations.replies").Inc()
		if !hasMore {
			break
		}
		cursor = nextCursor
	}

	// Cache with eviction
	h.threadCacheMu.Lock()
	if len(h.threadCache) >= threadCacheMaxItems {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range h.threadCache {
			if oldestKey == "" || v.expiry.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.expiry
			}
		}
		delete(h.threadCache, oldestKey)
	}
	h.threadCache[cacheKey] = cachedThread{messages: allMessages, expiry: time.Now().Add(threadCacheTTL)}
	h.threadCacheMu.Unlock()

	return allMessages
}

func (h *MessageHandler) postError(client *slackapi.Client, channelID, threadTS, msg string) {
	_, _, err := client.PostMessage(
		channelID,
		slackapi.MsgOptionText(msg, false),
		slackapi.MsgOptionTS(threadTS),
	)
	if err != nil {
		h.logger.Warn("failed to post error to Slack", zap.Error(err))
	}
}

// getUserAuthHeader returns the user's real JWT by refreshing their offline token.
// Returns error if user is not linked or token is revoked/expired.
// Always does a fresh refresh to ensure revocations take effect immediately.
func (h *MessageHandler) getUserAuthHeader(ctx context.Context, slackUserID string) (string, error) {
	if h.oidcClient == nil {
		return "", fmt.Errorf("OIDC not configured")
	}

	// Get the user's linked refresh token from DB
	link, err := h.linkRepo.GetBySlackUserID(ctx, slackUserID)
	if err != nil {
		return "", fmt.Errorf("failed to get link: %w", err)
	}
	if link == nil {
		return "", fmt.Errorf("user not linked")
	}

	// Refresh the token via Keycloak (always fresh — no cache)
	resp, err := h.oidcClient.RefreshToken(ctx, link.RefreshToken)
	if err != nil {
		// Token revoked or expired → delete the link
		h.logger.Warn("refresh token failed, deleting link",
			zap.String("slack_user_id", slackUserID),
			zap.String("email", link.Email),
			zap.Error(err))
		h.linkRepo.Delete(ctx, slackUserID)
		return "", fmt.Errorf("token expired or revoked")
	}

	// If Keycloak rotated the refresh token, update it in DB
	if resp.RefreshToken != "" && resp.RefreshToken != link.RefreshToken {
		if err := h.linkRepo.Upsert(ctx, &models.SlackUserLink{
			SlackUserID:  slackUserID,
			Email:        link.Email,
			RefreshToken: resp.RefreshToken,
		}); err != nil {
			h.logger.Warn("failed to update rotated refresh token", zap.Error(err))
		}
	}

	return "Bearer " + resp.AccessToken, nil
}

// sendLinkMessage sends an account linking message as an ephemeral message (only visible to the user).
// This prevents other users in the channel from seeing/hijacking the link URL.
func (h *MessageHandler) sendLinkMessage(client *slackapi.Client, channelID, threadTS, slackUserID, agentID, text string) {
	// Encrypt slack user ID + context so callback can reprocess the message after linking
	payload, _ := json.Marshal(map[string]string{
		"u": slackUserID,
		"c": channelID,
		"t": threadTS,
		"a": agentID,
		"q": text,
	})
	encPayload, err := h.cipher.Encrypt(string(payload))
	if err != nil {
		h.logger.Error("failed to encrypt link payload", zap.Error(err))
		h.postError(client, channelID, threadTS, errInternal)
		return
	}

	linkURL := fmt.Sprintf("%s/auth/slack/link?slack_user_id=%s", h.hostURL, url.QueryEscape(encPayload))
	msg := fmt.Sprintf(":link: To use this agent, first link your account:\n<%s|Link account>", linkURL)

	// Send DM + ephemeral in thread
	h.sendLinkViaDM(client, slackUserID, msg)
	client.PostEphemeral(channelID, slackUserID,
		slackapi.MsgOptionText(msg, false),
		slackapi.MsgOptionTS(threadTS))
}

// sendGitHubLinkMessage asks the user to connect GitHub via DM + ephemeral fallback.
func (h *MessageHandler) sendGitHubLinkMessage(client *slackapi.Client, channelID, threadTS, slackUserID string) {
	encUserID, err := h.cipher.Encrypt(slackUserID)
	if err != nil {
		h.postError(client, channelID, threadTS, errInternal)
		return
	}

	linkURL := fmt.Sprintf("%s/auth/slack/github?slack_user_id=%s", h.hostURL, url.QueryEscape(encUserID))
	msg := fmt.Sprintf(":octocat: This agent needs access to GitHub. Link your account:\n<%s|Connect GitHub>", linkURL)

	// Try DM first, fallback to ephemeral in thread
	h.sendLinkViaDM(client, slackUserID, msg)
	// Also post ephemeral in thread so it's visible immediately
	client.PostEphemeral(channelID, slackUserID,
		slackapi.MsgOptionText(msg, false),
		slackapi.MsgOptionTS(threadTS))
}

// sendLinkViaDM sends the account linking message as a direct message.
func (h *MessageHandler) sendLinkViaDM(client *slackapi.Client, slackUserID, msg string) {
	channel, _, _, err := client.OpenConversation(&slackapi.OpenConversationParameters{Users: []string{slackUserID}})
	if err != nil {
		h.logger.Warn("failed to open DM conversation for link message",
			zap.String("slack_user_id", slackUserID), zap.Error(err))
		return
	}
	_, _, err = client.PostMessage(channel.ID, slackapi.MsgOptionText(msg, false))
	if err != nil {
		h.logger.Warn("failed to send DM link message",
			zap.String("slack_user_id", slackUserID),
			zap.String("channel", channel.ID), zap.Error(err))
	}
}


func truncateString(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

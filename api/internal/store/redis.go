package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"go.uber.org/zap"
)

// SessionStore defines the interface for session persistence
type SessionStore interface {
	// UI Sessions
	CreateSession(ctx context.Context, userID, agentID, sessionName string) (*models.Session, error)
	// CreateSessionWithID creates a session with a deterministic ID (idempotent: returns existing if already present).
	CreateSessionWithID(ctx context.Context, sessionID, userID, agentID, sessionName string) (*models.Session, error)
	GetSession(ctx context.Context, sessionID string) (*models.Session, error)
	GetSessionPaginated(ctx context.Context, sessionID string, limit int, before int) (*models.SessionGetResponse, error)
	ListSessions(ctx context.Context, userID, agentID string) ([]models.Session, error)
	SaveSession(ctx context.Context, session *models.Session) error
	RenameSession(ctx context.Context, sessionID, newName string) (*models.Session, error)
	DeleteSession(ctx context.Context, sessionID, userID, agentID string) error
	AddMessage(ctx context.Context, sessionID string, msg models.ChatMessage) error

	// Agent session mapping
	GetAgentSessionID(ctx context.Context, sessionID, agentID string) (string, error)
	SetAgentSessionID(ctx context.Context, sessionID, agentID, agentSessionID string) error

	// Per-user agent session mapping (for Slack multi-user threads where each user has their own JWT)
	GetUserAgentSessionID(ctx context.Context, sessionID, agentID, userEmail string) (string, error)
	SetUserAgentSessionID(ctx context.Context, sessionID, agentID, userEmail, agentSessionID string) error

	// Append chart content_parts to an assistant message.
	// assistantOffset is the 0-based offset from the end among assistant messages
	// (0 = last assistant, 1 = second-to-last, etc.)
	AppendChartsToAssistant(ctx context.Context, sessionID string, assistantOffset int, charts []map[string]interface{}) error

	// Clone session (for sharing)
	CloneSession(ctx context.Context, sourceSessionID, newUserID, newAgentID, newName string) (*models.Session, error)

	// Slack: register user as participant (adds session to their user_sessions set)
	RegisterSlackParticipant(ctx context.Context, sessionID, userEmail, agentID string) error
	// IsParticipant checks if a user is a participant of a session
	IsParticipant(ctx context.Context, sessionID, userEmail string) bool
	// ListSlackSessions returns all Slack session IDs where the user is a participant
	ListSlackSessions(ctx context.Context, userEmail string) ([]string, error)

	// Read state (unread tracking)
	GetReadState(ctx context.Context, userEmail string) (map[string]int, error)
	SetReadCount(ctx context.Context, userEmail, sessionID string, count int) error
	SetReadStateBatch(ctx context.Context, userEmail string, state map[string]int) error

	// Live run streaming (reconnect after reload)
	// AppendRunEvent buffers a serialized AG-UI event into the session's run
	// stream so a reconnecting client can replay it.
	AppendRunEvent(ctx context.Context, sessionID string, data []byte) error
	// SetActiveRun marks a run as in-flight for the session (token identifies the run).
	SetActiveRun(ctx context.Context, sessionID, token string) error
	// ClearActiveRun clears the active-run flag only if it still holds the given token.
	ClearActiveRun(ctx context.Context, sessionID, token string) error
	// HasActiveRun reports whether a run is currently in flight for the session.
	HasActiveRun(ctx context.Context, sessionID string) bool
}

const sessionTTL = 7 * 24 * time.Hour // 7 days

// RedisSessionStore implements SessionStore using Redis
type RedisSessionStore struct {
	rdb    *redis.Client
	logger *zap.Logger
}

// NewRedisSessionStore creates a new Redis-backed session store
func NewRedisSessionStore(rdb *redis.Client, logger *zap.Logger) *RedisSessionStore {
	return &RedisSessionStore{
		rdb:    rdb,
		logger: logger,
	}
}

func sessionKey(sessionID string) string {
	return fmt.Sprintf("session:%s", sessionID)
}

func userSessionsKey(userID, agentID string) string {
	return fmt.Sprintf("user_sessions:%s:%s", userID, agentID)
}

func agentSessionMapKey(sessionID, agentID string) string {
	return fmt.Sprintf("agent_session_map:%s:%s", sessionID, agentID)
}

func (s *RedisSessionStore) CreateSession(ctx context.Context, userID, agentID, sessionName string) (*models.Session, error) {
	now := time.Now().Unix()
	session := &models.Session{
		SessionID:    uuid.New().String(),
		SessionName:  sessionName,
		UserID:       userID,
		AppName:      agentID,
		CreatedAt:    now,
		LastActivity: now,
		MessageCount: 0,
		Messages:     []models.ChatMessage{},
	}

	data, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, sessionKey(session.SessionID), data, sessionTTL)
	pipe.SAdd(ctx, userSessionsKey(userID, agentID), session.SessionID)
	pipe.Expire(ctx, userSessionsKey(userID, agentID), sessionTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

func (s *RedisSessionStore) CreateSessionWithID(ctx context.Context, sessionID, userID, agentID, sessionName string) (*models.Session, error) {
	now := time.Now().Unix()
	session := &models.Session{
		SessionID:    sessionID,
		SessionName:  sessionName,
		UserID:       userID,
		AppName:      agentID,
		CreatedAt:    now,
		LastActivity: now,
		MessageCount: 0,
		Messages:     []models.ChatMessage{},
	}

	data, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	// Idempotent: SETNX only writes if key does not exist
	ok, err := s.rdb.SetNX(ctx, sessionKey(sessionID), data, sessionTTL).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to create session with id: %w", err)
	}
	if !ok {
		// Session already exists — return the existing one
		return s.GetSession(ctx, sessionID)
	}

	// New session created — register in user sessions set
	s.rdb.SAdd(ctx, userSessionsKey(userID, agentID), sessionID)
	s.rdb.Expire(ctx, userSessionsKey(userID, agentID), sessionTTL)
	return session, nil
}

// RegisterSlackParticipant registers the user as a Slack session participant.
func (s *RedisSessionStore) RegisterSlackParticipant(ctx context.Context, sessionID, userEmail, agentID string) error {
	pipe := s.rdb.Pipeline()
	// Track participant per session (for access control)
	pKey := fmt.Sprintf("slack:participants:%s", sessionID)
	pipe.SAdd(ctx, pKey, userEmail)
	pipe.Expire(ctx, pKey, sessionTTL)
	// Track user's Slack sessions (for listing)
	uKey := fmt.Sprintf("slack:user:%s", userEmail)
	pipe.SAdd(ctx, uKey, sessionID)
	pipe.Expire(ctx, uKey, sessionTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// IsParticipant checks if a user participated in a Slack session.
func (s *RedisSessionStore) IsParticipant(ctx context.Context, sessionID, userEmail string) bool {
	pKey := fmt.Sprintf("slack:participants:%s", sessionID)
	ok, err := s.rdb.SIsMember(ctx, pKey, userEmail).Result()
	return err == nil && ok
}

// ListSlackSessions returns all Slack session IDs where the user is a participant.
func (s *RedisSessionStore) ListSlackSessions(ctx context.Context, userEmail string) ([]string, error) {
	uKey := fmt.Sprintf("slack:user:%s", userEmail)
	return s.rdb.SMembers(ctx, uKey).Result()
}

func (s *RedisSessionStore) SaveSession(ctx context.Context, session *models.Session) error {
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}
	return s.rdb.Set(ctx, sessionKey(session.SessionID), data, sessionTTL).Err()
}

func (s *RedisSessionStore) GetSession(ctx context.Context, sessionID string) (*models.Session, error) {
	data, err := s.rdb.Get(ctx, sessionKey(sessionID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	var session models.Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// GetSessionPaginated returns a session with paginated messages.
// Messages are returned in chronological order (oldest first within page).
// The most recent messages are returned first (latest page), using cursor-based
// pagination via the `before` parameter to load older messages.
// If limit <= 0, all messages are returned (backward compatible).
func (s *RedisSessionStore) GetSessionPaginated(ctx context.Context, sessionID string, limit int, before int) (*models.SessionGetResponse, error) {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, nil
	}

	allMessages := session.Messages
	totalCount := len(allMessages)

	// No pagination requested: return all messages (backward compatible)
	if limit <= 0 {
		session.Messages = nil // Exclude from session object (messages go in the top-level field)
		return &models.SessionGetResponse{
			Session:  *session,
			Messages: allMessages,
			HasMore:  false,
		}, nil
	}

	// Determine the end index (exclusive) for this page
	end := totalCount
	if before > 0 && before < totalCount {
		end = before
	}

	// Determine the start index
	start := end - limit
	if start < 0 {
		start = 0
	}

	pageMessages := allMessages[start:end]
	hasMore := start > 0

	session.Messages = nil // Exclude from session object

	resp := &models.SessionGetResponse{
		Session:  *session,
		Messages: pageMessages,
		HasMore:  hasMore,
	}
	if hasMore {
		resp.NextCursor = start
	}

	return resp, nil
}

func (s *RedisSessionStore) ListSessions(ctx context.Context, userID, agentID string) ([]models.Session, error) {
	sessionIDs, err := s.rdb.SMembers(ctx, userSessionsKey(userID, agentID)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list session IDs: %w", err)
	}

	if len(sessionIDs) == 0 {
		return []models.Session{}, nil
	}

	// Fetch all sessions in a pipeline
	pipe := s.rdb.Pipeline()
	cmds := make([]*redis.StringCmd, len(sessionIDs))
	for i, id := range sessionIDs {
		cmds[i] = pipe.Get(ctx, sessionKey(id))
	}
	_, _ = pipe.Exec(ctx) // Some may be nil if deleted

	sessions := make([]models.Session, 0, len(sessionIDs))
	for _, cmd := range cmds {
		data, err := cmd.Bytes()
		if err != nil {
			continue // skip missing sessions
		}
		var session models.Session
		if err := json.Unmarshal(data, &session); err != nil {
			continue
		}
		// Exclude messages for list view
		session.Messages = nil
		// Skip group sessions (they appear via group sessions API)
		if session.GroupID != "" {
			continue
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

func (s *RedisSessionStore) RenameSession(ctx context.Context, sessionID, newName string) (*models.Session, error) {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, nil
	}

	session.SessionName = newName
	session.LastActivity = time.Now().Unix()

	data, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := s.rdb.Set(ctx, sessionKey(sessionID), data, sessionTTL).Err(); err != nil {
		return nil, fmt.Errorf("failed to update session: %w", err)
	}

	return session, nil
}

func (s *RedisSessionStore) DeleteSession(ctx context.Context, sessionID, userID, agentID string) error {
	pipe := s.rdb.Pipeline()
	pipe.Del(ctx, sessionKey(sessionID))
	pipe.SRem(ctx, userSessionsKey(userID, agentID), sessionID)

	// Delete all agent session mappings for this session
	iter := s.rdb.Scan(ctx, 0, fmt.Sprintf("agent_session_map:%s:*", sessionID), 100).Iterator()
	for iter.Next(ctx) {
		pipe.Del(ctx, iter.Val())
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// addMessageLua is an atomic Lua script that appends a message to a session.
// This avoids the read-modify-write race condition when multiple agents
// respond in parallel (broadcast).
var addMessageLua = redis.NewScript(`
local data = redis.call('GET', KEYS[1])
if not data then
  return redis.error_reply('session not found')
end
local session = cjson.decode(data)
local msg = cjson.decode(ARGV[1])
if not session.messages then
  session.messages = {}
end
-- Deduplicate: for user messages, check if an identical user message was
-- already added recently (handles parallel multi-agent requests).
-- Scan backwards through at most 5 recent messages to find a match.
if msg.role == 'user' then
  local count = #session.messages
  local lookback = math.min(count, 5)
  for i = count, count - lookback + 1, -1 do
    local prev = session.messages[i]
    if prev.role == 'user' and prev.content == msg.content then
      -- Merge broadcast_agent_ids from the new message into the existing one
      if msg.broadcast_agent_ids then
        if not prev.broadcast_agent_ids then
          prev.broadcast_agent_ids = {}
        end
        for _, newId in ipairs(msg.broadcast_agent_ids) do
          local found = false
          for _, existingId in ipairs(prev.broadcast_agent_ids) do
            if existingId == newId then found = true; break end
          end
          if not found then
            table.insert(prev.broadcast_agent_ids, newId)
            -- Save the updated session with merged broadcast IDs
            redis.call('SET', KEYS[1], cjson.encode(session), 'EX', tonumber(ARGV[3]))
          end
        end
      end
      return 'DEDUP'
    end
  end
end
table.insert(session.messages, msg)
session.message_count = #session.messages
session.last_activity = tonumber(ARGV[2])
redis.call('SET', KEYS[1], cjson.encode(session), 'EX', tonumber(ARGV[3]))
return 'OK'
`)

func (s *RedisSessionStore) AddMessage(ctx context.Context, sessionID string, msg models.ChatMessage) error {
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	now := time.Now().Unix()
	ttlSeconds := int64(sessionTTL.Seconds())

	err = addMessageLua.Run(ctx, s.rdb,
		[]string{sessionKey(sessionID)},
		string(msgJSON),
		now,
		ttlSeconds,
	).Err()
	if err != nil {
		return fmt.Errorf("failed to add message atomically: %w", err)
	}

	return nil
}

func (s *RedisSessionStore) CloneSession(ctx context.Context, sourceSessionID, newUserID, newAgentID, newName string) (*models.Session, error) {
	source, err := s.GetSession(ctx, sourceSessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get source session: %w", err)
	}
	if source == nil {
		return nil, fmt.Errorf("source session not found")
	}

	now := time.Now().Unix()
	cloned := &models.Session{
		SessionID:    uuid.New().String(),
		SessionName:  newName,
		UserID:       newUserID,
		AppName:      newAgentID,
		CreatedAt:    now,
		LastActivity: now,
		MessageCount: len(source.Messages),
		Messages:     source.Messages,
	}

	data, err := json.Marshal(cloned)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cloned session: %w", err)
	}

	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, sessionKey(cloned.SessionID), data, sessionTTL)
	pipe.SAdd(ctx, userSessionsKey(newUserID, newAgentID), cloned.SessionID)
	pipe.Expire(ctx, userSessionsKey(newUserID, newAgentID), sessionTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to create cloned session: %w", err)
	}

	return cloned, nil
}

func (s *RedisSessionStore) GetAgentSessionID(ctx context.Context, sessionID, agentID string) (string, error) {
	val, err := s.rdb.Get(ctx, agentSessionMapKey(sessionID, agentID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get agent session ID: %w", err)
	}
	return val, nil
}

func (s *RedisSessionStore) SetAgentSessionID(ctx context.Context, sessionID, agentID, agentSessionID string) error {
	if err := s.rdb.Set(ctx, agentSessionMapKey(sessionID, agentID), agentSessionID, sessionTTL).Err(); err != nil {
		return fmt.Errorf("failed to set agent session ID: %w", err)
	}
	return nil
}

func userAgentSessionMapKey(sessionID, agentID, userEmail string) string {
	return fmt.Sprintf("agent_session_map:%s:%s:%s", sessionID, agentID, userEmail)
}

func (s *RedisSessionStore) GetUserAgentSessionID(ctx context.Context, sessionID, agentID, userEmail string) (string, error) {
	val, err := s.rdb.Get(ctx, userAgentSessionMapKey(sessionID, agentID, userEmail)).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get user agent session ID: %w", err)
	}
	return val, nil
}

func (s *RedisSessionStore) SetUserAgentSessionID(ctx context.Context, sessionID, agentID, userEmail, agentSessionID string) error {
	if err := s.rdb.Set(ctx, userAgentSessionMapKey(sessionID, agentID, userEmail), agentSessionID, sessionTTL).Err(); err != nil {
		return fmt.Errorf("failed to set user agent session ID: %w", err)
	}
	return nil
}

// appendChartsLua atomically appends chart content_parts to an assistant message.
// ARGV[1] = charts JSON array, ARGV[2] = TTL seconds, ARGV[3] = assistant offset from end (0 = last)
var appendChartsLua = redis.NewScript(`
local data = redis.call('GET', KEYS[1])
if not data then
  return redis.error_reply('session not found')
end
local session = cjson.decode(data)
if not session.messages then
  return redis.error_reply('no messages')
end
local offset = tonumber(ARGV[3]) or 0
-- Walk backwards to find the Nth assistant message (offset 0 = last)
local found = 0
local targetIdx = nil
for i = #session.messages, 1, -1 do
  if session.messages[i].role == 'assistant' then
    if found == offset then
      targetIdx = i
      break
    end
    found = found + 1
  end
end
if not targetIdx then
  return redis.error_reply('assistant message not found at offset')
end
local msg = session.messages[targetIdx]
if not msg.content_parts then
  msg.content_parts = {}
end
local charts = cjson.decode(ARGV[1])
for _, c in ipairs(charts) do
  table.insert(msg.content_parts, {type = 'chart', chart = c})
end
session.messages[targetIdx] = msg
redis.call('SET', KEYS[1], cjson.encode(session), 'EX', tonumber(ARGV[2]))
return 'OK'
`)

func (s *RedisSessionStore) AppendChartsToAssistant(ctx context.Context, sessionID string, assistantOffset int, charts []map[string]interface{}) error {
	chartsJSON, err := json.Marshal(charts)
	if err != nil {
		return fmt.Errorf("failed to marshal charts: %w", err)
	}
	ttlSeconds := int64(sessionTTL.Seconds())
	err = appendChartsLua.Run(ctx, s.rdb,
		[]string{sessionKey(sessionID)},
		string(chartsJSON),
		ttlSeconds,
		assistantOffset,
	).Err()
	if err != nil {
		return fmt.Errorf("failed to append charts: %w", err)
	}
	return nil
}

// Read state keys
func readStateKey(userEmail string) string {
	return fmt.Sprintf("read_state:%s", userEmail)
}

// GetReadState returns the read message counts for all sessions of a user
func (s *RedisSessionStore) GetReadState(ctx context.Context, userEmail string) (map[string]int, error) {
	raw, err := s.rdb.HGetAll(ctx, readStateKey(userEmail)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get read state: %w", err)
	}

	state := make(map[string]int, len(raw))
	for sessionID, val := range raw {
		var count int
		if _, err := fmt.Sscanf(val, "%d", &count); err == nil {
			state[sessionID] = count
		}
	}
	return state, nil
}

// SetReadCount sets the read message count for a single session
func (s *RedisSessionStore) SetReadCount(ctx context.Context, userEmail, sessionID string, count int) error {
	key := readStateKey(userEmail)
	pipe := s.rdb.Pipeline()
	pipe.HSet(ctx, key, sessionID, fmt.Sprintf("%d", count))
	pipe.Expire(ctx, key, sessionTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to set read count: %w", err)
	}
	return nil
}

// SetReadStateBatch sets read message counts for multiple sessions at once
func (s *RedisSessionStore) SetReadStateBatch(ctx context.Context, userEmail string, state map[string]int) error {
	if len(state) == 0 {
		return nil
	}

	key := readStateKey(userEmail)
	pipe := s.rdb.Pipeline()
	for sessionID, count := range state {
		pipe.HSet(ctx, key, sessionID, fmt.Sprintf("%d", count))
	}
	pipe.Expire(ctx, key, sessionTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to set read state batch: %w", err)
	}
	return nil
}

// Live run streaming (reconnect after reload).

const (
	runEventsTTL    = 10 * time.Minute // matches the agent run context timeout
	runEventsMaxLen = 5000             // cap buffered events per run
	activeRunTTL    = 10 * time.Minute
)

// RunEventsKey is the Redis Stream key holding a session's buffered run events.
func RunEventsKey(sessionID string) string {
	return fmt.Sprintf("run_events:%s", sessionID)
}

// ActiveRunKey is the Redis key flagging an in-flight run for a session.
func ActiveRunKey(sessionID string) string {
	return fmt.Sprintf("active_run:%s", sessionID)
}

// AppendRunEvent appends a serialized AG-UI event to the session's run stream.
func (s *RedisSessionStore) AppendRunEvent(ctx context.Context, sessionID string, data []byte) error {
	key := RunEventsKey(sessionID)
	if err := s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: key,
		MaxLen: runEventsMaxLen,
		Approx: true,
		Values: map[string]interface{}{"e": data},
	}).Err(); err != nil {
		return fmt.Errorf("failed to append run event: %w", err)
	}
	// Refresh TTL so the buffer is cleaned up after the run window.
	s.rdb.Expire(ctx, key, runEventsTTL)
	return nil
}

// SetActiveRun marks a run as in-flight for the session and starts a fresh event
// buffer for it. Dropping any leftover stream from a previous run is essential:
// otherwise a reconnect would replay the previous run's RUN_FINISHED and close
// before reaching the current run's events.
func (s *RedisSessionStore) SetActiveRun(ctx context.Context, sessionID, token string) error {
	s.rdb.Del(ctx, RunEventsKey(sessionID))
	if err := s.rdb.Set(ctx, ActiveRunKey(sessionID), token, activeRunTTL).Err(); err != nil {
		return fmt.Errorf("failed to set active run: %w", err)
	}
	return nil
}

// clearActiveRunScript deletes the active-run flag only if it still holds the
// given token, so a newer run that started while this one was finishing is not
// clobbered.
var clearActiveRunScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

// ClearActiveRun clears the active-run flag if it still holds the given token.
func (s *RedisSessionStore) ClearActiveRun(ctx context.Context, sessionID, token string) error {
	if err := clearActiveRunScript.Run(ctx, s.rdb, []string{ActiveRunKey(sessionID)}, token).Err(); err != nil && err != redis.Nil {
		return fmt.Errorf("failed to clear active run: %w", err)
	}
	return nil
}

// HasActiveRun reports whether a run is currently in flight for the session.
func (s *RedisSessionStore) HasActiveRun(ctx context.Context, sessionID string) bool {
	n, err := s.rdb.Exists(ctx, ActiveRunKey(sessionID)).Result()
	return err == nil && n > 0
}


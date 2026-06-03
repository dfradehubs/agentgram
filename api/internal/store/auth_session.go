package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// AuthSession represents an authenticated user session
type AuthSession struct {
	SessionID    string   `json:"session_id"`
	Email        string   `json:"email"`
	Name         string   `json:"name,omitempty"`
	Sub          string   `json:"sub"`
	Groups       []string `json:"groups"`
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	IDToken      string   `json:"id_token"`
	ExpiresAt    int64    `json:"expires_at"`
	CreatedAt    int64    `json:"created_at"`
}

// AuthSessionStore defines the interface for auth session persistence
type AuthSessionStore interface {
	Create(ctx context.Context, session *AuthSession) error
	Get(ctx context.Context, sessionID string) (*AuthSession, error)
	Update(ctx context.Context, session *AuthSession) error
	Delete(ctx context.Context, sessionID string) error
	SaveState(ctx context.Context, state, nonce string) error
	ValidateState(ctx context.Context, state string) (nonce string, err error)
}

// RedisAuthSessionStore implements AuthSessionStore using Redis
type RedisAuthSessionStore struct {
	rdb       *redis.Client
	maxAge    time.Duration
	logger    *zap.Logger
}

// NewRedisAuthSessionStore creates a new Redis-backed auth session store
func NewRedisAuthSessionStore(rdb *redis.Client, maxAgeSeconds int, logger *zap.Logger) *RedisAuthSessionStore {
	return &RedisAuthSessionStore{
		rdb:    rdb,
		maxAge: time.Duration(maxAgeSeconds) * time.Second,
		logger: logger,
	}
}

func authSessionKey(sessionID string) string {
	return fmt.Sprintf("auth_session:%s", sessionID)
}

func oidcStateKey(state string) string {
	return fmt.Sprintf("oidc_state:%s", state)
}

// GenerateSessionID generates a cryptographically secure session ID
func GenerateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (s *RedisAuthSessionStore) Create(ctx context.Context, session *AuthSession) error {
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal auth session: %w", err)
	}

	if err := s.rdb.Set(ctx, authSessionKey(session.SessionID), data, s.maxAge).Err(); err != nil {
		return fmt.Errorf("failed to create auth session: %w", err)
	}

	return nil
}

func (s *RedisAuthSessionStore) Get(ctx context.Context, sessionID string) (*AuthSession, error) {
	data, err := s.rdb.Get(ctx, authSessionKey(sessionID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get auth session: %w", err)
	}

	var session AuthSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal auth session: %w", err)
	}

	return &session, nil
}

func (s *RedisAuthSessionStore) Update(ctx context.Context, session *AuthSession) error {
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal auth session: %w", err)
	}

	// Rolling session: always reset TTL to maxAge on update
	if err := s.rdb.Set(ctx, authSessionKey(session.SessionID), data, s.maxAge).Err(); err != nil {
		return fmt.Errorf("failed to update auth session: %w", err)
	}

	return nil
}

func (s *RedisAuthSessionStore) Delete(ctx context.Context, sessionID string) error {
	if err := s.rdb.Del(ctx, authSessionKey(sessionID)).Err(); err != nil {
		return fmt.Errorf("failed to delete auth session: %w", err)
	}
	return nil
}

func (s *RedisAuthSessionStore) SaveState(ctx context.Context, state, nonce string) error {
	if err := s.rdb.Set(ctx, oidcStateKey(state), nonce, 5*time.Minute).Err(); err != nil {
		return fmt.Errorf("failed to save OIDC state: %w", err)
	}
	return nil
}

func (s *RedisAuthSessionStore) ValidateState(ctx context.Context, state string) (string, error) {
	key := oidcStateKey(state)

	nonce, err := s.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("invalid or expired OIDC state")
	}
	if err != nil {
		return "", fmt.Errorf("failed to validate OIDC state: %w", err)
	}

	// Delete state after validation (one-time use)
	s.rdb.Del(ctx, key)

	return nonce, nil
}

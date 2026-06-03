package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// MCPTokenStore manages encrypted OAuth2 tokens in Redis, keyed by user+server.
type MCPTokenStore struct {
	rdb    *redis.Client
	crypto *auth.CookieCrypto
	logger *zap.Logger
}

// NewMCPTokenStore creates a new MCP token store with AES-GCM encryption.
func NewMCPTokenStore(rdb *redis.Client, crypto *auth.CookieCrypto, logger *zap.Logger) *MCPTokenStore {
	return &MCPTokenStore{rdb: rdb, crypto: crypto, logger: logger}
}

func mcpTokenKey(userEmail, mcpServerID string) string {
	return fmt.Sprintf("mcp_oauth:%s:%s", userEmail, mcpServerID)
}

// Save encrypts and stores an OAuth2 token for a user+server pair.
func (s *MCPTokenStore) Save(ctx context.Context, userEmail, mcpServerID string, token *models.MCPOAuth2Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}

	encrypted, err := s.crypto.Encrypt(string(data))
	if err != nil {
		return fmt.Errorf("encrypt token: %w", err)
	}

	ttl := 30 * 24 * time.Hour
	if token.ExpiresAt > 0 {
		remaining := time.Until(time.Unix(token.ExpiresAt, 0))
		if remaining > 0 && remaining < ttl {
			ttl = remaining + time.Hour
		}
	}

	key := mcpTokenKey(userEmail, mcpServerID)
	if err := s.rdb.Set(ctx, key, encrypted, ttl).Err(); err != nil {
		return fmt.Errorf("save token: %w", err)
	}

	return nil
}

// Get retrieves and decrypts an OAuth2 token for a user+server pair.
// Returns nil, nil if no token exists.
func (s *MCPTokenStore) Get(ctx context.Context, userEmail, mcpServerID string) (*models.MCPOAuth2Token, error) {
	key := mcpTokenKey(userEmail, mcpServerID)
	encrypted, err := s.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}

	decrypted, err := s.crypto.Decrypt(encrypted)
	if err != nil {
		s.rdb.Del(ctx, key)
		return nil, fmt.Errorf("decrypt token (cleared): %w", err)
	}

	var token models.MCPOAuth2Token
	if err := json.Unmarshal([]byte(decrypted), &token); err != nil {
		s.rdb.Del(ctx, key)
		return nil, fmt.Errorf("unmarshal token (cleared): %w", err)
	}

	return &token, nil
}

// Delete removes the OAuth2 token for a user+server pair.
func (s *MCPTokenStore) Delete(ctx context.Context, userEmail, mcpServerID string) error {
	return s.rdb.Del(ctx, mcpTokenKey(userEmail, mcpServerID)).Err()
}

// IsExpired checks if a token is expired (with 30s buffer).
func IsTokenExpired(token *models.MCPOAuth2Token) bool {
	if token.ExpiresAt == 0 {
		return false
	}
	return time.Now().Unix() >= token.ExpiresAt-30
}

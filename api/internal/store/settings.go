package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// SettingsStore reads and writes app_settings key/value pairs.
type SettingsStore struct {
	pool *pgxpool.Pool
}

// NewSettingsStore creates a new SettingsStore.
func NewSettingsStore(pool *pgxpool.Pool) *SettingsStore {
	return &SettingsStore{pool: pool}
}

// Get returns the value for a key, or empty string if not found.
func (s *SettingsStore) Get(ctx context.Context, key string) (string, error) {
	var value string
	err := s.pool.QueryRow(ctx, "SELECT value FROM app_settings WHERE key = $1", key).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get setting %q: %w", key, err)
	}
	return value, nil
}

// Set upserts a key/value pair.
func (s *SettingsStore) Set(ctx context.Context, key, value string) error {
	_, err := s.pool.Exec(ctx,
		"INSERT INTO app_settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = $2",
		key, value,
	)
	if err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}
	return nil
}

const cookieEncryptionKeyName = "cookie_encryption_key"

// GetOrCreateEncryptionKey returns the cookie encryption key from the database.
// If it does not exist, a new 32-byte random key is generated and stored.
// Uses INSERT ... ON CONFLICT DO NOTHING to avoid TOCTOU races when multiple
// pods start simultaneously — only the first insert wins, all others read back
// the winning value.
func GetOrCreateEncryptionKey(ctx context.Context, pool *pgxpool.Pool, logger *zap.Logger) (string, error) {
	ss := NewSettingsStore(pool)

	// Generate a candidate key (cheap even if we don't end up using it)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate encryption key: %w", err)
	}
	hexKey := hex.EncodeToString(key)

	// Attempt insert — if the row already exists, this is a no-op
	_, err := pool.Exec(ctx,
		"INSERT INTO app_settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO NOTHING",
		cookieEncryptionKeyName, hexKey,
	)
	if err != nil {
		return "", fmt.Errorf("upsert encryption key: %w", err)
	}

	// Always read back the winning value
	stored, err := ss.Get(ctx, cookieEncryptionKeyName)
	if err != nil {
		return "", fmt.Errorf("read encryption key: %w", err)
	}

	if stored == hexKey {
		logger.Info("generated and stored new cookie encryption key")
	} else {
		logger.Info("loaded existing cookie encryption key from database")
	}

	return stored, nil
}

// GetOrCreateSettingsKey returns a 32-byte hex-encoded key for the given setting name.
// Priority: ENCRYPTION_KEY env var > existing DB value > auto-generated.
// If the env var is set, its value takes precedence and is stored in the DB.
// Otherwise, behaves like GetOrCreateEncryptionKey (race-safe across pods).
func GetOrCreateSettingsKey(ctx context.Context, pool *pgxpool.Pool, name string, logger *zap.Logger) (string, error) {
	ss := NewSettingsStore(pool)

	// Check env var first (allows operator to provide their own key)
	envKey := strings.ToUpper(name) // e.g. "encryption_key" -> "ENCRYPTION_KEY"
	if envVal := os.Getenv(envKey); envVal != "" {
		// Validate format
		decoded, err := hex.DecodeString(envVal)
		if err != nil || len(decoded) != 32 {
			return "", fmt.Errorf("env %s must be 64 hex chars (32 bytes)", envKey)
		}
		// Store in DB so all pods use the same key
		if err := ss.Set(ctx, name, envVal); err != nil {
			return "", fmt.Errorf("persist %s from env: %w", name, err)
		}
		logger.Info("using encryption key from environment variable", zap.String("setting", name))
		return envVal, nil
	}

	// Generate a candidate key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate key for %s: %w", name, err)
	}
	hexKey := hex.EncodeToString(key)

	// Attempt insert — if the row already exists, this is a no-op
	_, err := pool.Exec(ctx,
		"INSERT INTO app_settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO NOTHING",
		name, hexKey,
	)
	if err != nil {
		return "", fmt.Errorf("upsert %s: %w", name, err)
	}

	// Always read back the winning value
	stored, err := ss.Get(ctx, name)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", name, err)
	}

	if stored == hexKey {
		logger.Info("generated and stored new encryption key", zap.String("setting", name))
	} else {
		logger.Info("loaded existing encryption key from database", zap.String("setting", name))
	}

	return stored, nil
}

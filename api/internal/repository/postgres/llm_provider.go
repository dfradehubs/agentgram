package postgres

import (
	"context"
	"fmt"

	"github.com/dfradehubs/agentgram-api/internal/crypto"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LLMProviderRepository struct {
	pool   *pgxpool.Pool
	cipher *crypto.AESCrypto
}

func NewLLMProviderRepository(pool *pgxpool.Pool, cipher *crypto.AESCrypto) *LLMProviderRepository {
	return &LLMProviderRepository{pool: pool, cipher: cipher}
}

func (r *LLMProviderRepository) encryptKey(key string) (string, error) {
	if r.cipher == nil || key == "" || crypto.IsEncrypted(key) {
		return key, nil
	}
	return r.cipher.Encrypt(key)
}

func (r *LLMProviderRepository) decryptKey(key string) (string, error) {
	if r.cipher == nil || key == "" {
		return key, nil
	}
	return r.cipher.Decrypt(key)
}

func (r *LLMProviderRepository) Create(ctx context.Context, provider *models.LLMProvider) error {
	key, err := r.encryptKey(provider.APIKey)
	if err != nil {
		return fmt.Errorf("encrypt provider api key: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO llm_providers (id, name, provider_type, api_key, endpoint, enabled)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		provider.ID, provider.Name, provider.ProviderType, key, provider.Endpoint, provider.Enabled)
	if err != nil {
		return fmt.Errorf("insert llm provider: %w", err)
	}
	return nil
}

func (r *LLMProviderRepository) Get(ctx context.Context, id string) (*models.LLMProvider, error) {
	var provider models.LLMProvider
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, provider_type, api_key, endpoint, enabled, created_at, updated_at
		FROM llm_providers WHERE id=$1`, id).Scan(
		&provider.ID, &provider.Name, &provider.ProviderType, &provider.APIKey,
		&provider.Endpoint, &provider.Enabled, &provider.CreatedAt, &provider.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get llm provider: %w", err)
	}
	provider.APIKey, err = r.decryptKey(provider.APIKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt provider api key: %w", err)
	}
	return &provider, nil
}

func (r *LLMProviderRepository) List(ctx context.Context) ([]*models.LLMProvider, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, provider_type, api_key, endpoint, enabled, created_at, updated_at
		FROM llm_providers ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("list llm providers: %w", err)
	}
	defer rows.Close()
	providers := make([]*models.LLMProvider, 0)
	for rows.Next() {
		var provider models.LLMProvider
		if err := rows.Scan(&provider.ID, &provider.Name, &provider.ProviderType, &provider.APIKey,
			&provider.Endpoint, &provider.Enabled, &provider.CreatedAt, &provider.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan llm provider: %w", err)
		}
		provider.APIKey, err = r.decryptKey(provider.APIKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt provider api key for %s: %w", provider.ID, err)
		}
		providers = append(providers, &provider)
	}
	return providers, rows.Err()
}

func (r *LLMProviderRepository) Update(ctx context.Context, provider *models.LLMProvider) error {
	key, err := r.encryptKey(provider.APIKey)
	if err != nil {
		return fmt.Errorf("encrypt provider api key: %w", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin provider update: %w", err)
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE llm_providers SET name=$2, provider_type=$3, api_key=$4, endpoint=$5,
			enabled=$6, updated_at=NOW() WHERE id=$1`,
		provider.ID, provider.Name, provider.ProviderType, key, provider.Endpoint, provider.Enabled)
	if err != nil {
		return fmt.Errorf("update llm provider: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("llm provider not found: %s", provider.ID)
	}
	legacyType := provider.ProviderType
	if legacyType == "custom" {
		legacyType = "openai"
	}
	if _, err := tx.Exec(ctx, `
		UPDATE llm_models SET provider=$2, api_key=$3, endpoint=$4, updated_at=NOW()
		WHERE provider_id=$1`, provider.ID, legacyType, key, provider.Endpoint); err != nil {
		return fmt.Errorf("sync legacy llm configuration: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *LLMProviderRepository) Delete(ctx context.Context, id string) error {
	var count int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM llm_models WHERE provider_id=$1`, id).Scan(&count); err != nil {
		return fmt.Errorf("count provider models: %w", err)
	}
	if count > 0 {
		return repository.ErrProviderInUse
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM llm_providers WHERE id=$1`, id)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23503" {
			return repository.ErrProviderInUse
		}
		return fmt.Errorf("delete llm provider: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("llm provider not found: %s", id)
	}
	return nil
}

func (r *LLMProviderRepository) MigrateEncryptKeys(ctx context.Context) (int, error) {
	if r.cipher == nil {
		return 0, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT id, api_key FROM llm_providers WHERE api_key <> '' AND api_key NOT LIKE 'enc:%'`)
	if err != nil {
		return 0, fmt.Errorf("query plaintext provider keys: %w", err)
	}
	type item struct{ id, key string }
	var items []item
	for rows.Next() {
		var value item
		if err := rows.Scan(&value.id, &value.key); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, value)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	for _, value := range items {
		key, err := r.cipher.Encrypt(value.key)
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `UPDATE llm_providers SET api_key=$2 WHERE id=$1`, value.id, key); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `UPDATE llm_models SET api_key=$2 WHERE provider_id=$1`, value.id, key); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(items), nil
}

package postgres

import (
	"context"
	"fmt"

	"github.com/dfradehubs/agentgram-api/internal/crypto"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LLMModelRepository implements repository.LLMModelRepository with PostgreSQL
type LLMModelRepository struct {
	pool   *pgxpool.Pool
	cipher *crypto.AESCrypto
}

// NewLLMModelRepository creates a new PostgreSQL LLM model repository.
// The cipher parameter encrypts/decrypts API keys at rest. If nil, keys are stored in plaintext.
func NewLLMModelRepository(pool *pgxpool.Pool, cipher *crypto.AESCrypto) *LLMModelRepository {
	return &LLMModelRepository{pool: pool, cipher: cipher}
}

// encryptKey encrypts an API key before storing. Returns plaintext if cipher is nil.
func (r *LLMModelRepository) encryptKey(key string) (string, error) {
	if r.cipher == nil || key == "" {
		return key, nil
	}
	// Already encrypted — skip double encryption
	if crypto.IsEncrypted(key) {
		return key, nil
	}
	return r.cipher.Encrypt(key)
}

// decryptKey decrypts an API key after reading. Returns as-is if cipher is nil or value is plaintext.
func (r *LLMModelRepository) decryptKey(key string) (string, error) {
	if r.cipher == nil || key == "" {
		return key, nil
	}
	return r.cipher.Decrypt(key)
}

func (r *LLMModelRepository) Create(ctx context.Context, model *models.LLMModel) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if model.IsDefault {
		if _, err := tx.Exec(ctx,
			`UPDATE llm_models SET is_default = false WHERE role = $1 AND is_default = true`,
			model.Role,
		); err != nil {
			return fmt.Errorf("clear default: %w", err)
		}
	}

	encKey, err := r.encryptKey(model.APIKey)
	if err != nil {
		return fmt.Errorf("encrypt api key: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO llm_models (id, name, provider, model, api_key, role, enabled, is_default)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		model.ID, model.Name, model.Provider, model.Model, encKey, model.Role, model.Enabled, model.IsDefault,
	); err != nil {
		return fmt.Errorf("insert llm model: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *LLMModelRepository) Get(ctx context.Context, id string) (*models.LLMModel, error) {
	var m models.LLMModel
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, provider, model, api_key, role, enabled, is_default, created_at, updated_at
		 FROM llm_models WHERE id = $1`, id,
	).Scan(&m.ID, &m.Name, &m.Provider, &m.Model, &m.APIKey, &m.Role, &m.Enabled, &m.IsDefault, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get llm model: %w", err)
	}

	m.APIKey, err = r.decryptKey(m.APIKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt api key for %s: %w", id, err)
	}

	return &m, nil
}

func (r *LLMModelRepository) List(ctx context.Context) ([]*models.LLMModel, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, provider, model, api_key, role, enabled, is_default, created_at, updated_at
		 FROM llm_models ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list llm models: %w", err)
	}
	defer rows.Close()

	var result []*models.LLMModel
	for rows.Next() {
		var m models.LLMModel
		if err := rows.Scan(&m.ID, &m.Name, &m.Provider, &m.Model, &m.APIKey, &m.Role, &m.Enabled, &m.IsDefault, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan llm model: %w", err)
		}
		m.APIKey, err = r.decryptKey(m.APIKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt api key for %s: %w", m.ID, err)
		}
		result = append(result, &m)
	}
	return result, nil
}

func (r *LLMModelRepository) ListByRole(ctx context.Context, role string) ([]*models.LLMModel, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, provider, model, api_key, role, enabled, is_default, created_at, updated_at
		 FROM llm_models WHERE role = $1 AND enabled = true ORDER BY name`, role,
	)
	if err != nil {
		return nil, fmt.Errorf("list llm models by role: %w", err)
	}
	defer rows.Close()

	var result []*models.LLMModel
	for rows.Next() {
		var m models.LLMModel
		if err := rows.Scan(&m.ID, &m.Name, &m.Provider, &m.Model, &m.APIKey, &m.Role, &m.Enabled, &m.IsDefault, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan llm model: %w", err)
		}
		m.APIKey, err = r.decryptKey(m.APIKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt api key for %s: %w", m.ID, err)
		}
		result = append(result, &m)
	}
	return result, nil
}

func (r *LLMModelRepository) Update(ctx context.Context, model *models.LLMModel) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if model.IsDefault {
		if _, err := tx.Exec(ctx,
			`UPDATE llm_models SET is_default = false WHERE role = $1 AND is_default = true AND id != $2`,
			model.Role, model.ID,
		); err != nil {
			return fmt.Errorf("clear default: %w", err)
		}
	}

	encKey, err := r.encryptKey(model.APIKey)
	if err != nil {
		return fmt.Errorf("encrypt api key: %w", err)
	}

	tag, err := tx.Exec(ctx,
		`UPDATE llm_models SET name=$2, provider=$3, model=$4, api_key=$5, role=$6, enabled=$7, is_default=$8, updated_at=NOW()
		 WHERE id=$1`,
		model.ID, model.Name, model.Provider, model.Model, encKey, model.Role, model.Enabled, model.IsDefault,
	)
	if err != nil {
		return fmt.Errorf("update llm model: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("llm model not found: %s", model.ID)
	}

	return tx.Commit(ctx)
}

func (r *LLMModelRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM llm_models WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete llm model: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("llm model not found: %s", id)
	}
	return nil
}

func (r *LLMModelRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM llm_models`).Scan(&count)
	return count, err
}

// MigrateEncryptKeys reads all LLM models and encrypts any plaintext API keys in-place.
// This is idempotent: already-encrypted keys (with "enc:" prefix) are skipped.
func (r *LLMModelRepository) MigrateEncryptKeys(ctx context.Context) (int, error) {
	if r.cipher == nil {
		return 0, nil
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, api_key FROM llm_models WHERE api_key != '' AND api_key NOT LIKE 'enc:%'`,
	)
	if err != nil {
		return 0, fmt.Errorf("query plaintext keys: %w", err)
	}
	defer rows.Close()

	type row struct {
		id  string
		key string
	}
	var toEncrypt []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.key); err != nil {
			return 0, fmt.Errorf("scan: %w", err)
		}
		toEncrypt = append(toEncrypt, r)
	}

	if len(toEncrypt) == 0 {
		return 0, nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, item := range toEncrypt {
		enc, err := r.cipher.Encrypt(item.key)
		if err != nil {
			return 0, fmt.Errorf("encrypt key for %s: %w", item.id, err)
		}
		if _, err := tx.Exec(ctx, `UPDATE llm_models SET api_key = $1 WHERE id = $2`, enc, item.id); err != nil {
			return 0, fmt.Errorf("update key for %s: %w", item.id, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	return len(toEncrypt), nil
}

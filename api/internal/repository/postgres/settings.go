package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SettingsRepository implements settings.Repository with PostgreSQL.
type SettingsRepository struct {
	pool *pgxpool.Pool
}

// NewSettingsRepository creates a new PostgreSQL settings repository.
func NewSettingsRepository(pool *pgxpool.Pool) *SettingsRepository {
	return &SettingsRepository{pool: pool}
}

// GetAll returns every stored setting override as a key→value map.
func (r *SettingsRepository) GetAll(ctx context.Context) (map[string]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT key, value FROM runtime_config`)
	if err != nil {
		return nil, fmt.Errorf("list app settings: %w", err)
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scan app setting: %w", err)
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SetMany upserts several setting overrides in a single transaction.
func (r *SettingsRepository) SetMany(ctx context.Context, values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin settings tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for key, value := range values {
		if _, err := tx.Exec(ctx,
			`INSERT INTO runtime_config (key, value, updated_at) VALUES ($1, $2, NOW())
			 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
			key, value,
		); err != nil {
			return fmt.Errorf("set setting %s: %w", key, err)
		}
	}
	return tx.Commit(ctx)
}

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/crypto"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SlackIntegrationRepository implements repository.SlackIntegrationRepository with PostgreSQL.
type SlackIntegrationRepository struct {
	pool   *pgxpool.Pool
	cipher *crypto.AESCrypto
}

// NewSlackIntegrationRepository creates a new PostgreSQL Slack integration repository.
func NewSlackIntegrationRepository(pool *pgxpool.Pool, cipher *crypto.AESCrypto) *SlackIntegrationRepository {
	return &SlackIntegrationRepository{pool: pool, cipher: cipher}
}

func (r *SlackIntegrationRepository) encrypt(val string) (string, error) {
	if r.cipher == nil || val == "" {
		return val, nil
	}
	if crypto.IsEncrypted(val) {
		return val, nil
	}
	return r.cipher.Encrypt(val)
}

func (r *SlackIntegrationRepository) decrypt(val string) (string, error) {
	if r.cipher == nil || val == "" {
		return val, nil
	}
	return r.cipher.Decrypt(val)
}

func (r *SlackIntegrationRepository) Upsert(ctx context.Context, integration *models.SlackIntegration) error {
	encBot, err := r.encrypt(integration.BotToken)
	if err != nil {
		return fmt.Errorf("encrypt bot_token: %w", err)
	}
	encApp, err := r.encrypt(integration.AppToken)
	if err != nil {
		return fmt.Errorf("encrypt app_token: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO slack_integrations (agent_id, bot_token, app_token, enabled, workspace_id, workspace_name, status, status_message)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (agent_id) DO UPDATE SET
			bot_token = EXCLUDED.bot_token,
			app_token = EXCLUDED.app_token,
			enabled = EXCLUDED.enabled,
			workspace_id = EXCLUDED.workspace_id,
			workspace_name = EXCLUDED.workspace_name,
			status = EXCLUDED.status,
			status_message = EXCLUDED.status_message,
			updated_at = NOW()`,
		integration.AgentID, encBot, encApp, integration.Enabled,
		integration.WorkspaceID, integration.WorkspaceName,
		integration.Status, integration.StatusMessage,
	)
	if err != nil {
		return fmt.Errorf("upsert slack integration: %w", err)
	}
	return nil
}

func (r *SlackIntegrationRepository) Get(ctx context.Context, agentID string) (*models.SlackIntegration, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, agent_id, bot_token, app_token, enabled, workspace_id, workspace_name, status, status_message, created_at, updated_at
		FROM slack_integrations WHERE agent_id = $1`, agentID)

	var s models.SlackIntegration
	err := row.Scan(&s.ID, &s.AgentID, &s.BotToken, &s.AppToken, &s.Enabled,
		&s.WorkspaceID, &s.WorkspaceName, &s.Status, &s.StatusMessage, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get slack integration: %w", err)
	}

	s.BotToken, err = r.decrypt(s.BotToken)
	if err != nil {
		return nil, fmt.Errorf("decrypt bot_token: %w", err)
	}
	s.AppToken, err = r.decrypt(s.AppToken)
	if err != nil {
		return nil, fmt.Errorf("decrypt app_token: %w", err)
	}
	return &s, nil
}

func (r *SlackIntegrationRepository) Delete(ctx context.Context, agentID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM slack_integrations WHERE agent_id = $1`, agentID)
	if err != nil {
		return fmt.Errorf("delete slack integration: %w", err)
	}
	return nil
}

func (r *SlackIntegrationRepository) ListEnabled(ctx context.Context) ([]*models.SlackIntegration, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, agent_id, bot_token, app_token, enabled, workspace_id, workspace_name, status, status_message, created_at, updated_at
		FROM slack_integrations WHERE enabled = TRUE`)
	if err != nil {
		return nil, fmt.Errorf("list enabled slack integrations: %w", err)
	}
	defer rows.Close()

	var result []*models.SlackIntegration
	for rows.Next() {
		var s models.SlackIntegration
		if err := rows.Scan(&s.ID, &s.AgentID, &s.BotToken, &s.AppToken, &s.Enabled,
			&s.WorkspaceID, &s.WorkspaceName, &s.Status, &s.StatusMessage, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan slack integration: %w", err)
		}
		s.BotToken, err = r.decrypt(s.BotToken)
		if err != nil {
			return nil, fmt.Errorf("decrypt bot_token: %w", err)
		}
		s.AppToken, err = r.decrypt(s.AppToken)
		if err != nil {
			return nil, fmt.Errorf("decrypt app_token: %w", err)
		}
		result = append(result, &s)
	}
	return result, nil
}

func (r *SlackIntegrationRepository) UpdateStatus(ctx context.Context, agentID, status, statusMessage string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE slack_integrations SET status = $1, status_message = $2, updated_at = $3
		WHERE agent_id = $4`, status, statusMessage, time.Now(), agentID)
	if err != nil {
		return fmt.Errorf("update slack integration status: %w", err)
	}
	return nil
}

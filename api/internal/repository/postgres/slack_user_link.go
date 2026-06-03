package postgres

import (
	"context"
	"fmt"

	"github.com/dfradehubs/agentgram-api/internal/crypto"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SlackUserLinkRepository implements repository.SlackUserLinkRepository with PostgreSQL.
type SlackUserLinkRepository struct {
	pool   *pgxpool.Pool
	cipher *crypto.AESCrypto
}

func NewSlackUserLinkRepository(pool *pgxpool.Pool, cipher *crypto.AESCrypto) *SlackUserLinkRepository {
	return &SlackUserLinkRepository{pool: pool, cipher: cipher}
}

func (r *SlackUserLinkRepository) encrypt(val string) (string, error) {
	if r.cipher == nil || val == "" {
		return val, nil
	}
	if crypto.IsEncrypted(val) {
		return val, nil
	}
	return r.cipher.Encrypt(val)
}

func (r *SlackUserLinkRepository) decrypt(val string) (string, error) {
	if r.cipher == nil || val == "" {
		return val, nil
	}
	return r.cipher.Decrypt(val)
}

func (r *SlackUserLinkRepository) Upsert(ctx context.Context, link *models.SlackUserLink) error {
	encToken, err := r.encrypt(link.RefreshToken)
	if err != nil {
		return fmt.Errorf("encrypt refresh_token: %w", err)
	}
	encGH, err := r.encrypt(link.GitHubToken)
	if err != nil {
		return fmt.Errorf("encrypt github_token: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO slack_user_links (slack_user_id, email, refresh_token, github_token)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (slack_user_id) DO UPDATE SET
			email = EXCLUDED.email,
			refresh_token = EXCLUDED.refresh_token,
			github_token = CASE WHEN EXCLUDED.github_token = '' THEN slack_user_links.github_token ELSE EXCLUDED.github_token END,
			updated_at = NOW()`,
		link.SlackUserID, link.Email, encToken, encGH,
	)
	if err != nil {
		return fmt.Errorf("upsert slack user link: %w", err)
	}
	return nil
}

func (r *SlackUserLinkRepository) SetGitHubToken(ctx context.Context, slackUserID, githubToken, githubRefreshToken string) error {
	encGH, err := r.encrypt(githubToken)
	if err != nil {
		return fmt.Errorf("encrypt github_token: %w", err)
	}
	encRefresh, err := r.encrypt(githubRefreshToken)
	if err != nil {
		return fmt.Errorf("encrypt github_refresh_token: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE slack_user_links SET github_token = $1, github_refresh_token = $2, updated_at = NOW()
		WHERE slack_user_id = $3`, encGH, encRefresh, slackUserID)
	if err != nil {
		return fmt.Errorf("set github_token: %w", err)
	}
	return nil
}

func (r *SlackUserLinkRepository) RevokeGitHub(ctx context.Context, slackUserID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE slack_user_links SET github_token = '', github_refresh_token = '', updated_at = NOW()
		WHERE slack_user_id = $1`, slackUserID)
	return err
}

func (r *SlackUserLinkRepository) GetBySlackUserID(ctx context.Context, slackUserID string) (*models.SlackUserLink, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT slack_user_id, email, refresh_token, github_token, github_refresh_token, created_at, updated_at
		FROM slack_user_links WHERE slack_user_id = $1`, slackUserID)

	var l models.SlackUserLink
	err := row.Scan(&l.SlackUserID, &l.Email, &l.RefreshToken, &l.GitHubToken, &l.GitHubRefreshToken, &l.CreatedAt, &l.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get slack user link: %w", err)
	}

	l.RefreshToken, err = r.decrypt(l.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("decrypt refresh_token: %w", err)
	}
	l.GitHubToken, err = r.decrypt(l.GitHubToken)
	if err != nil {
		return nil, fmt.Errorf("decrypt github_token: %w", err)
	}
	l.GitHubRefreshToken, err = r.decrypt(l.GitHubRefreshToken)
	if err != nil {
		return nil, fmt.Errorf("decrypt github_refresh_token: %w", err)
	}
	l.HasGitHub = l.GitHubToken != ""
	return &l, nil
}

func (r *SlackUserLinkRepository) Delete(ctx context.Context, slackUserID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM slack_user_links WHERE slack_user_id = $1`, slackUserID)
	if err != nil {
		return fmt.Errorf("delete slack user link: %w", err)
	}
	return nil
}

func (r *SlackUserLinkRepository) ListAll(ctx context.Context) ([]*models.SlackUserLink, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT slack_user_id, email, github_token != '' as has_github, created_at, updated_at
		FROM slack_user_links ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list slack user links: %w", err)
	}
	defer rows.Close()

	var result []*models.SlackUserLink
	for rows.Next() {
		var l models.SlackUserLink
		if err := rows.Scan(&l.SlackUserID, &l.Email, &l.HasGitHub, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan slack user link: %w", err)
		}
		result = append(result, &l)
	}
	return result, nil
}

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SharedSessionRepository implements repository.SharedSessionRepository with PostgreSQL
type SharedSessionRepository struct {
	pool *pgxpool.Pool
}

// NewSharedSessionRepository creates a new PostgreSQL shared session repository
func NewSharedSessionRepository(pool *pgxpool.Pool) *SharedSessionRepository {
	return &SharedSessionRepository{pool: pool}
}

func (r *SharedSessionRepository) Create(ctx context.Context, sessionID, agentID, sharedBy string, expiresAt time.Time) (*models.SharedSession, error) {
	var ss models.SharedSession
	err := r.pool.QueryRow(ctx,
		`INSERT INTO shared_sessions (session_id, agent_id, shared_by, expires_at)
		 VALUES ($1, $2, $3, $4)
		 RETURNING token, session_id, agent_id, shared_by, created_at, expires_at, revoked`,
		sessionID, agentID, sharedBy, expiresAt,
	).Scan(&ss.Token, &ss.SessionID, &ss.AgentID, &ss.SharedBy, &ss.CreatedAt, &ss.ExpiresAt, &ss.Revoked)
	if err != nil {
		return nil, fmt.Errorf("create shared session: %w", err)
	}
	return &ss, nil
}

func (r *SharedSessionRepository) GetByToken(ctx context.Context, token string) (*models.SharedSession, error) {
	var ss models.SharedSession
	err := r.pool.QueryRow(ctx,
		`SELECT token, session_id, agent_id, shared_by, created_at, expires_at, revoked
		 FROM shared_sessions
		 WHERE token = $1 AND revoked = FALSE AND expires_at > NOW()`,
		token,
	).Scan(&ss.Token, &ss.SessionID, &ss.AgentID, &ss.SharedBy, &ss.CreatedAt, &ss.ExpiresAt, &ss.Revoked)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get shared session by token: %w", err)
	}
	return &ss, nil
}

func (r *SharedSessionRepository) GetBySessionID(ctx context.Context, sessionID string) (*models.SharedSession, error) {
	var ss models.SharedSession
	err := r.pool.QueryRow(ctx,
		`SELECT token, session_id, agent_id, shared_by, created_at, expires_at, revoked
		 FROM shared_sessions
		 WHERE session_id = $1 AND revoked = FALSE AND expires_at > NOW()
		 ORDER BY created_at DESC LIMIT 1`,
		sessionID,
	).Scan(&ss.Token, &ss.SessionID, &ss.AgentID, &ss.SharedBy, &ss.CreatedAt, &ss.ExpiresAt, &ss.Revoked)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get shared session by session id: %w", err)
	}
	return &ss, nil
}

func (r *SharedSessionRepository) Revoke(ctx context.Context, sessionID, userEmail string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE shared_sessions SET revoked = TRUE
		 WHERE session_id = $1 AND shared_by = $2 AND revoked = FALSE`,
		sessionID, userEmail,
	)
	if err != nil {
		return fmt.Errorf("revoke shared session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no active share found")
	}
	return nil
}

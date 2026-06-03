package postgres

import (
	"context"
	"fmt"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BasicAuthRepository implements repository.BasicAuthRepository with PostgreSQL
type BasicAuthRepository struct {
	pool *pgxpool.Pool
}

// NewBasicAuthRepository creates a new PostgreSQL basic auth repository
func NewBasicAuthRepository(pool *pgxpool.Pool) *BasicAuthRepository {
	return &BasicAuthRepository{pool: pool}
}

func (r *BasicAuthRepository) GetByUsername(ctx context.Context, username string) (*models.BasicAuthUser, error) {
	var u models.BasicAuthUser
	err := r.pool.QueryRow(ctx,
		`SELECT id, username, email, password_hash, created_at, updated_at
		 FROM basic_auth_users WHERE username = $1`, username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get basic auth user: %w", err)
	}
	return &u, nil
}

func (r *BasicAuthRepository) Create(ctx context.Context, user *models.BasicAuthUser) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO basic_auth_users (username, email, password_hash) VALUES ($1, $2, $3)`,
		user.Username, user.Email, user.PasswordHash,
	)
	if err != nil {
		return fmt.Errorf("create basic auth user: %w", err)
	}
	return nil
}

func (r *BasicAuthRepository) List(ctx context.Context) ([]*models.BasicAuthUser, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, username, email, password_hash, created_at, updated_at
		 FROM basic_auth_users ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list basic auth users: %w", err)
	}
	defer rows.Close()

	users := make([]*models.BasicAuthUser, 0)
	for rows.Next() {
		var u models.BasicAuthUser
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan basic auth user: %w", err)
		}
		users = append(users, &u)
	}
	return users, nil
}

func (r *BasicAuthRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM basic_auth_users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete basic auth user: %w", err)
	}
	return nil
}

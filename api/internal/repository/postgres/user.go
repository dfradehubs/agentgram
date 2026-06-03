package postgres

import (
	"context"
	"fmt"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRepository implements repository.UserRepository with PostgreSQL
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository creates a new PostgreSQL user repository
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (email, role, last_access_at) VALUES ($1, $2, NOW())
		 ON CONFLICT (email) DO NOTHING`,
		user.Email, user.Role,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, role, created_at, updated_at, last_access_at FROM users WHERE LOWER(email) = LOWER($1)`,
		email,
	).Scan(&u.ID, &u.Email, &u.Role, &u.CreatedAt, &u.UpdatedAt, &u.LastAccessAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) List(ctx context.Context) ([]*models.User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, email, role, created_at, updated_at, last_access_at FROM users ORDER BY email`,
	)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.CreatedAt, &u.UpdatedAt, &u.LastAccessAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, &u)
	}
	return users, nil
}

func (r *UserRepository) UpdateRole(ctx context.Context, email, role string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET role = $1, updated_at = NOW() WHERE LOWER(email) = LOWER($2)`,
		role, email,
	)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user not found: %s", email)
	}
	return nil
}

func (r *UserRepository) UpdateLastAccess(ctx context.Context, email string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET last_access_at = NOW() WHERE LOWER(email) = LOWER($1)`,
		email,
	)
	if err != nil {
		return fmt.Errorf("update last access: %w", err)
	}
	return nil
}

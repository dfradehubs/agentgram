package postgres

import (
	"context"
	"fmt"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SkillRepository implements repository.SkillRepository with PostgreSQL.
type SkillRepository struct {
	pool *pgxpool.Pool
}

// NewSkillRepository creates a new PostgreSQL skill repository.
func NewSkillRepository(pool *pgxpool.Pool) *SkillRepository {
	return &SkillRepository{pool: pool}
}

func (r *SkillRepository) Create(ctx context.Context, s *models.Skill) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`INSERT INTO skills (id, name, description, content) VALUES ($1,$2,$3,$4)`,
		s.ID, s.Name, s.Description, s.Content)
	if err != nil {
		return fmt.Errorf("insert skill: %w", err)
	}
	if err := insertPermissions(ctx, tx, "skill_allowed_users", "skill_id", s.ID, "user_email", s.AllowedUsers); err != nil {
		return err
	}
	if err := insertPermissions(ctx, tx, "skill_allowed_groups", "skill_id", s.ID, "group_name", s.AllowedGroups); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *SkillRepository) Get(ctx context.Context, id string) (*models.Skill, error) {
	var s models.Skill
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, description, content, created_at, updated_at FROM skills WHERE id = $1`, id).
		Scan(&s.ID, &s.Name, &s.Description, &s.Content, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get skill: %w", err)
	}
	if err := r.loadPermissions(ctx, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SkillRepository) List(ctx context.Context) ([]*models.Skill, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, description, content, created_at, updated_at FROM skills ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()

	var skills []*models.Skill
	for rows.Next() {
		var s models.Skill
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Content, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan skill: %w", err)
		}
		skills = append(skills, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, s := range skills {
		if err := r.loadPermissions(ctx, s); err != nil {
			return nil, err
		}
	}
	return skills, nil
}

// loadPermissions fills AllowedUsers/AllowedGroups from the satellite tables.
func (r *SkillRepository) loadPermissions(ctx context.Context, s *models.Skill) error {
	users, err := queryStrings(ctx, r.pool, `SELECT user_email FROM skill_allowed_users WHERE skill_id = $1`, s.ID)
	if err != nil {
		return err
	}
	groups, err := queryStrings(ctx, r.pool, `SELECT group_name FROM skill_allowed_groups WHERE skill_id = $1`, s.ID)
	if err != nil {
		return err
	}
	s.AllowedUsers = users
	s.AllowedGroups = groups
	return nil
}

func (r *SkillRepository) Update(ctx context.Context, s *models.Skill) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`UPDATE skills SET name=$2, description=$3, content=$4, updated_at=NOW() WHERE id=$1`,
		s.ID, s.Name, s.Description, s.Content)
	if err != nil {
		return fmt.Errorf("update skill: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("skill not found: %s", s.ID)
	}

	_, _ = tx.Exec(ctx, `DELETE FROM skill_allowed_users WHERE skill_id = $1`, s.ID)
	_, _ = tx.Exec(ctx, `DELETE FROM skill_allowed_groups WHERE skill_id = $1`, s.ID)
	if err := insertPermissions(ctx, tx, "skill_allowed_users", "skill_id", s.ID, "user_email", s.AllowedUsers); err != nil {
		return err
	}
	if err := insertPermissions(ctx, tx, "skill_allowed_groups", "skill_id", s.ID, "group_name", s.AllowedGroups); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *SkillRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM skills WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete skill: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("skill not found: %s", id)
	}
	return nil
}

func (r *SkillRepository) UpdatePermissions(ctx context.Context, id string, users, groups []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	_, _ = tx.Exec(ctx, `DELETE FROM skill_allowed_users WHERE skill_id = $1`, id)
	_, _ = tx.Exec(ctx, `DELETE FROM skill_allowed_groups WHERE skill_id = $1`, id)
	if err := insertPermissions(ctx, tx, "skill_allowed_users", "skill_id", id, "user_email", users); err != nil {
		return err
	}
	if err := insertPermissions(ctx, tx, "skill_allowed_groups", "skill_id", id, "group_name", groups); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditRepository implements repository.AuditRepository with PostgreSQL
type AuditRepository struct {
	pool *pgxpool.Pool
}

// NewAuditRepository creates a new PostgreSQL audit repository
func NewAuditRepository(pool *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{pool: pool}
}

func (r *AuditRepository) Log(ctx context.Context, entry *models.AuditEntry) error {
	detailsJSON, _ := json.Marshal(entry.Details)

	_, err := r.pool.Exec(ctx,
		`INSERT INTO audit_log (user_email, action, resource_type, resource_id, details)
		 VALUES ($1, $2, $3, $4, $5)`,
		entry.UserEmail, entry.Action, entry.ResourceType, entry.ResourceID, detailsJSON,
	)
	if err != nil {
		return fmt.Errorf("log audit: %w", err)
	}
	return nil
}

func (r *AuditRepository) List(ctx context.Context, limit, offset int) ([]*models.AuditEntry, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_email, action, resource_type, resource_id, details, created_at
		 FROM audit_log ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	defer rows.Close()

	var entries []*models.AuditEntry
	for rows.Next() {
		var e models.AuditEntry
		var detailsJSON []byte
		if err := rows.Scan(&e.ID, &e.UserEmail, &e.Action, &e.ResourceType, &e.ResourceID, &detailsJSON, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit: %w", err)
		}
		if len(detailsJSON) > 0 {
			_ = json.Unmarshal(detailsJSON, &e.Details)
		}
		entries = append(entries, &e)
	}
	return entries, nil
}

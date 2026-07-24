package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditEventRepository implements repository.AuditEventRepository with PostgreSQL.
type AuditEventRepository struct {
	pool *pgxpool.Pool
}

// NewAuditEventRepository creates a new PostgreSQL audit event repository.
func NewAuditEventRepository(pool *pgxpool.Pool) *AuditEventRepository {
	return &AuditEventRepository{pool: pool}
}

func (r *AuditEventRepository) Insert(ctx context.Context, e *models.AuditEvent) error {
	var toolCallsJSON, tokenUsageJSON []byte
	if len(e.ToolCalls) > 0 {
		toolCallsJSON, _ = json.Marshal(e.ToolCalls)
	}
	if e.TokenUsage != nil {
		tokenUsageJSON, _ = json.Marshal(e.TokenUsage)
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO audit_events
		  (user_email, user_groups, resource_type, resource_id, resource_name,
		   source, client, session_id, action, prompt, response, tool_calls, token_usage, llm_model,
		   status, error_type, error_msg, duration_ms)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		e.UserEmail, e.UserGroups, e.ResourceType, e.ResourceID, nullStr(e.ResourceName),
		e.Source, nullStr(e.Client), nullStr(e.SessionID), e.Action, e.Prompt, e.Response, toolCallsJSON, tokenUsageJSON, nullStr(e.LLMModel),
		e.Status, nullStr(e.ErrorType), nullStr(e.ErrorMsg), e.DurationMs)
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

// List returns a filtered, paginated page of audit events plus the total count
// matching the filter (for pagination).
func (r *AuditEventRepository) List(ctx context.Context, f models.AuditEventFilter) ([]*models.AuditEvent, int, error) {
	var conds []string
	var args []interface{}
	add := func(cond string, val interface{}) {
		args = append(args, val)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if f.From != nil {
		add("created_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("created_at <= $%d", *f.To)
	}
	if f.UserEmail != "" {
		add("user_email = $%d", f.UserEmail)
	}
	if f.Group != "" {
		add("$%d = ANY(user_groups)", f.Group)
	}
	if f.ResourceType != "" {
		add("resource_type = $%d", f.ResourceType)
	}
	if f.SessionID != "" {
		add("session_id = $%d", f.SessionID)
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := r.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit events: %w", err)
	}

	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	args = append(args, limit)
	limitPos := len(args)
	args = append(args, f.Offset)
	offsetPos := len(args)

	rows, err := r.pool.Query(ctx,
		`SELECT id, user_email, user_groups, resource_type, resource_id, resource_name,
		        source, client, session_id, action, prompt, response, tool_calls, token_usage, llm_model,
		        status, error_type, error_msg, duration_ms, created_at
		 FROM audit_events `+where+
			fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", limitPos, offsetPos),
		args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()

	var events []*models.AuditEvent
	for rows.Next() {
		var e models.AuditEvent
		var resName, client, sessionID, llmModel, errType, errMsg *string
		var toolCallsJSON, tokenUsageJSON []byte
		if err := rows.Scan(&e.ID, &e.UserEmail, &e.UserGroups, &e.ResourceType, &e.ResourceID, &resName,
			&e.Source, &client, &sessionID, &e.Action, &e.Prompt, &e.Response, &toolCallsJSON, &tokenUsageJSON, &llmModel,
			&e.Status, &errType, &errMsg, &e.DurationMs, &e.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan audit event: %w", err)
		}
		e.ResourceName = deref(resName)
		e.Client = deref(client)
		e.SessionID = deref(sessionID)
		e.LLMModel = deref(llmModel)
		e.ErrorType = deref(errType)
		e.ErrorMsg = deref(errMsg)
		if len(toolCallsJSON) > 0 {
			_ = json.Unmarshal(toolCallsJSON, &e.ToolCalls)
		}
		if len(tokenUsageJSON) > 0 {
			_ = json.Unmarshal(tokenUsageJSON, &e.TokenUsage)
		}
		events = append(events, &e)
	}
	return events, total, rows.Err()
}

// Cleanup deletes audit events older than retentionDays.
func (r *AuditEventRepository) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM audit_events WHERE created_at < NOW() - ($1 || ' days')::interval`,
		fmt.Sprintf("%d", retentionDays))
	if err != nil {
		return 0, fmt.Errorf("cleanup audit events: %w", err)
	}
	return tag.RowsAffected(), nil
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

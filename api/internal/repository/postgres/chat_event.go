package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ChatEventRepository implements repository.ChatEventRepository with PostgreSQL
type ChatEventRepository struct {
	pool *pgxpool.Pool
}

// NewChatEventRepository creates a new PostgreSQL chat event repository
func NewChatEventRepository(pool *pgxpool.Pool) *ChatEventRepository {
	return &ChatEventRepository{pool: pool}
}

func (r *ChatEventRepository) Insert(ctx context.Context, event *models.ChatEvent) error {
	var toolCallsJSON, tokenUsageJSON []byte
	if len(event.ToolCalls) > 0 {
		toolCallsJSON, _ = json.Marshal(event.ToolCalls)
	}
	if event.TokenUsage != nil {
		tokenUsageJSON, _ = json.Marshal(event.TokenUsage)
	}

	source := event.Source
	if source == "" {
		source = "web"
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO chat_events (
			resource_type, resource_id, resource_name, protocol, user_email, session_id,
			status, error_type, error_msg, duration_ms, ttfb_ms, message_count,
			tool_calls, token_usage, llm_model, session_rotated, source
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		event.ResourceType, event.ResourceID, event.ResourceName, event.Protocol,
		event.UserEmail, event.SessionID, event.Status, event.ErrorType, event.ErrorMsg,
		event.DurationMs, event.TTFBMs, event.MessageCount,
		toolCallsJSON, tokenUsageJSON, event.LLMModel, event.SessionRotated, source,
	)
	if err != nil {
		return fmt.Errorf("insert chat_event: %w", err)
	}
	return nil
}

func (r *ChatEventRepository) ResourceStats(ctx context.Context, resourceType, resourceID string, from, to time.Time, source string) (*models.ResourceStats, error) {
	var stats models.ResourceStats
	var avgTTFB *float64
	var tokenJSON []byte

	err := r.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) AS total_requests,
			COUNT(*) FILTER (WHERE status = 'ok') AS success_count,
			COUNT(*) FILTER (WHERE status = 'error') AS error_count,
			CASE WHEN COUNT(*) > 0 THEN ROUND(COUNT(*) FILTER (WHERE status = 'error')::numeric / COUNT(*)::numeric * 100, 2) ELSE 0 END AS error_rate,
			COALESCE(AVG(duration_ms), 0) AS avg_duration_ms,
			COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_ms), 0) AS p95_duration_ms,
			AVG(ttfb_ms) FILTER (WHERE ttfb_ms IS NOT NULL) AS avg_ttfb_ms,
			COUNT(DISTINCT user_email) AS unique_users,
			COUNT(*) FILTER (WHERE session_rotated = TRUE) AS context_rotations,
			COALESCE(SUM(jsonb_array_length(COALESCE(tool_calls, '[]'::jsonb))), 0) AS total_tool_calls,
			(SELECT jsonb_build_object(
				'input', COALESCE(SUM((token_usage->>'input')::int), 0),
				'output', COALESCE(SUM((token_usage->>'output')::int), 0),
				'total', COALESCE(SUM((token_usage->>'total')::int), 0)
			) FROM chat_events WHERE resource_type = $1 AND resource_id = $2 AND created_at BETWEEN $3 AND $4 AND token_usage IS NOT NULL AND ($5 = '' OR source = $5)) AS token_usage,
			(SELECT llm_model FROM chat_events WHERE resource_type = $1 AND resource_id = $2 AND llm_model IS NOT NULL AND llm_model != '' AND ($5 = '' OR source = $5) ORDER BY created_at DESC LIMIT 1) AS llm_model
		FROM chat_events
		WHERE resource_type = $1 AND resource_id = $2 AND created_at BETWEEN $3 AND $4 AND ($5 = '' OR source = $5)`,
		resourceType, resourceID, from, to, source,
	).Scan(
		&stats.TotalRequests, &stats.SuccessCount, &stats.ErrorCount, &stats.ErrorRate,
		&stats.AvgDurationMs, &stats.P95DurationMs, &avgTTFB,
		&stats.UniqueUsers, &stats.ContextRotations, &stats.TotalToolCalls,
		&tokenJSON, &stats.LLMModel,
	)
	if err != nil {
		return nil, fmt.Errorf("resource stats: %w", err)
	}

	stats.AvgTTFBMs = avgTTFB
	if len(tokenJSON) > 0 {
		var tu models.TokenUsage
		if json.Unmarshal(tokenJSON, &tu) == nil {
			stats.TokenUsage = &tu
		}
	}

	return &stats, nil
}

func (r *ChatEventRepository) ResourceTimeline(ctx context.Context, resourceType, resourceID string, from, to time.Time, interval string, source string) ([]models.TimelineBucket, error) {
	pgInterval := toPostgresInterval(interval)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT
			time_bucket('%s', created_at) AS bucket,
			COUNT(*) AS requests,
			COUNT(*) FILTER (WHERE status = 'error') AS errors,
			COALESCE(AVG(duration_ms), 0) AS avg_duration,
			AVG(ttfb_ms) FILTER (WHERE ttfb_ms IS NOT NULL) AS avg_ttfb
		FROM chat_events
		WHERE resource_type = $1 AND resource_id = $2 AND created_at BETWEEN $3 AND $4 AND ($5 = '' OR source = $5)
		GROUP BY bucket ORDER BY bucket`, pgInterval),
		resourceType, resourceID, from, to, source,
	)
	if err != nil {
		// Fallback if time_bucket is not available (no timescaledb)
		return r.resourceTimelineFallback(ctx, resourceType, resourceID, from, to, pgInterval, source)
	}
	defer rows.Close()
	return scanTimelineBuckets(rows)
}

func (r *ChatEventRepository) resourceTimelineFallback(ctx context.Context, resourceType, resourceID string, from, to time.Time, interval string, source string) ([]models.TimelineBucket, error) {
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT
			to_timestamp(EXTRACT(EPOCH FROM created_at)::bigint / EXTRACT(EPOCH FROM INTERVAL '%s')::bigint * EXTRACT(EPOCH FROM INTERVAL '%s')::bigint) AS bucket,
			COUNT(*) AS requests,
			COUNT(*) FILTER (WHERE status = 'error') AS errors,
			COALESCE(AVG(duration_ms), 0) AS avg_duration,
			AVG(ttfb_ms) FILTER (WHERE ttfb_ms IS NOT NULL) AS avg_ttfb
		FROM chat_events
		WHERE resource_type = $1 AND resource_id = $2 AND created_at BETWEEN $3 AND $4 AND ($5 = '' OR source = $5)
		GROUP BY bucket ORDER BY bucket`, interval, interval),
		resourceType, resourceID, from, to, source,
	)
	if err != nil {
		return nil, fmt.Errorf("resource timeline fallback: %w", err)
	}
	defer rows.Close()
	return scanTimelineBuckets(rows)
}

func (r *ChatEventRepository) ResourceUsers(ctx context.Context, resourceType, resourceID string, from, to time.Time, limit int, source string) ([]models.UserStat, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			user_email,
			COUNT(*) AS requests,
			COUNT(*) FILTER (WHERE status = 'error') AS errors,
			MAX(created_at) AS last_access
		FROM chat_events
		WHERE resource_type = $1 AND resource_id = $2 AND created_at BETWEEN $3 AND $4 AND ($6 = '' OR source = $6)
		GROUP BY user_email ORDER BY requests DESC LIMIT $5`,
		resourceType, resourceID, from, to, limit, source,
	)
	if err != nil {
		return nil, fmt.Errorf("resource users: %w", err)
	}
	defer rows.Close()

	users := make([]models.UserStat, 0)
	for rows.Next() {
		var u models.UserStat
		if err := rows.Scan(&u.UserEmail, &u.Requests, &u.Errors, &u.LastAccess); err != nil {
			return nil, fmt.Errorf("scan user stat: %w", err)
		}
		users = append(users, u)
	}
	return users, nil
}

func (r *ChatEventRepository) ResourceErrors(ctx context.Context, resourceType, resourceID string, from, to time.Time, source string) ([]models.ErrorStat, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			COALESCE(error_type, 'unknown') AS error_type,
			COUNT(*) AS count,
			MAX(created_at) AS last_seen,
			(array_agg(error_msg ORDER BY created_at DESC))[1] AS last_msg
		FROM chat_events
		WHERE resource_type = $1 AND resource_id = $2 AND status = 'error' AND created_at BETWEEN $3 AND $4 AND ($5 = '' OR source = $5)
		GROUP BY error_type ORDER BY count DESC`,
		resourceType, resourceID, from, to, source,
	)
	if err != nil {
		return nil, fmt.Errorf("resource errors: %w", err)
	}
	defer rows.Close()

	errs := make([]models.ErrorStat, 0)
	for rows.Next() {
		var e models.ErrorStat
		var lastMsg *string
		if err := rows.Scan(&e.ErrorType, &e.Count, &e.LastSeen, &lastMsg); err != nil {
			return nil, fmt.Errorf("scan error stat: %w", err)
		}
		if lastMsg != nil {
			e.LastMsg = *lastMsg
		}
		errs = append(errs, e)
	}
	return errs, nil
}

func (r *ChatEventRepository) GlobalStats(ctx context.Context, from, to time.Time, source string) (*models.GlobalStats, error) {
	var stats models.GlobalStats
	var tokenJSON []byte
	err := r.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) AS total_requests,
			COUNT(*) FILTER (WHERE status = 'ok') AS success_count,
			COUNT(*) FILTER (WHERE status = 'error') AS error_count,
			CASE WHEN COUNT(*) > 0 THEN ROUND(COUNT(*) FILTER (WHERE status = 'error')::numeric / COUNT(*)::numeric * 100, 2) ELSE 0 END AS error_rate,
			COALESCE(AVG(duration_ms), 0) AS avg_duration_ms,
			COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_ms), 0) AS p95_duration_ms,
			COUNT(DISTINCT user_email) AS unique_users,
			COUNT(DISTINCT resource_id) AS active_agents,
			(SELECT jsonb_build_object(
				'input', COALESCE(SUM((token_usage->>'input')::int), 0),
				'output', COALESCE(SUM((token_usage->>'output')::int), 0),
				'total', COALESCE(SUM((token_usage->>'total')::int), 0)
			) FROM chat_events WHERE created_at BETWEEN $1 AND $2 AND token_usage IS NOT NULL AND ($3 = '' OR source = $3)) AS token_usage
		FROM chat_events WHERE created_at BETWEEN $1 AND $2 AND ($3 = '' OR source = $3)`,
		from, to, source,
	).Scan(
		&stats.TotalRequests, &stats.SuccessCount, &stats.ErrorCount, &stats.ErrorRate,
		&stats.AvgDurationMs, &stats.P95DurationMs, &stats.UniqueUsers, &stats.ActiveAgents,
		&tokenJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("global stats: %w", err)
	}
	if len(tokenJSON) > 0 {
		var tu models.TokenUsage
		if json.Unmarshal(tokenJSON, &tu) == nil {
			stats.TokenUsage = &tu
		}
	}
	return &stats, nil
}

func (r *ChatEventRepository) GlobalTimeline(ctx context.Context, from, to time.Time, interval string, source string) ([]models.TimelineBucket, error) {
	pgInterval := toPostgresInterval(interval)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT
			to_timestamp(EXTRACT(EPOCH FROM created_at)::bigint / EXTRACT(EPOCH FROM INTERVAL '%s')::bigint * EXTRACT(EPOCH FROM INTERVAL '%s')::bigint) AS bucket,
			COUNT(*) AS requests,
			COUNT(*) FILTER (WHERE status = 'error') AS errors,
			COALESCE(AVG(duration_ms), 0) AS avg_duration,
			AVG(ttfb_ms) FILTER (WHERE ttfb_ms IS NOT NULL) AS avg_ttfb
		FROM chat_events WHERE created_at BETWEEN $1 AND $2 AND ($3 = '' OR source = $3)
		GROUP BY bucket ORDER BY bucket`, pgInterval, pgInterval),
		from, to, source,
	)
	if err != nil {
		return nil, fmt.Errorf("global timeline: %w", err)
	}
	defer rows.Close()
	return scanTimelineBuckets(rows)
}

func (r *ChatEventRepository) GlobalUsers(ctx context.Context, from, to time.Time, limit int, source string) ([]models.UserStat, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			user_email,
			COUNT(*) AS requests,
			COUNT(*) FILTER (WHERE status = 'error') AS errors,
			MAX(created_at) AS last_access
		FROM chat_events
		WHERE created_at BETWEEN $1 AND $2 AND ($4 = '' OR source = $4)
		GROUP BY user_email ORDER BY requests DESC LIMIT $3`,
		from, to, limit, source,
	)
	if err != nil {
		return nil, fmt.Errorf("global users: %w", err)
	}
	defer rows.Close()

	users := make([]models.UserStat, 0)
	for rows.Next() {
		var u models.UserStat
		if err := rows.Scan(&u.UserEmail, &u.Requests, &u.Errors, &u.LastAccess); err != nil {
			return nil, fmt.Errorf("scan user stat: %w", err)
		}
		users = append(users, u)
	}
	return users, nil
}

func (r *ChatEventRepository) TopResources(ctx context.Context, from, to time.Time, limit int, source string) ([]models.ResourceRanking, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			resource_type,
			resource_id,
			COALESCE(MAX(resource_name), resource_id) AS resource_name,
			COUNT(*) AS requests,
			CASE WHEN COUNT(*) > 0 THEN ROUND(COUNT(*) FILTER (WHERE status = 'error')::numeric / COUNT(*)::numeric * 100, 2) ELSE 0 END AS error_rate,
			COALESCE(AVG(duration_ms), 0) AS avg_duration_ms
		FROM chat_events WHERE created_at BETWEEN $1 AND $2 AND ($4 = '' OR source = $4)
		GROUP BY resource_type, resource_id ORDER BY requests DESC LIMIT $3`,
		from, to, limit, source,
	)
	if err != nil {
		return nil, fmt.Errorf("top resources: %w", err)
	}
	defer rows.Close()

	rankings := make([]models.ResourceRanking, 0)
	for rows.Next() {
		var r models.ResourceRanking
		if err := rows.Scan(&r.ResourceType, &r.ResourceID, &r.ResourceName, &r.Requests, &r.ErrorRate, &r.AvgDurationMs); err != nil {
			return nil, fmt.Errorf("scan resource ranking: %w", err)
		}
		rankings = append(rankings, r)
	}
	return rankings, nil
}

func (r *ChatEventRepository) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM chat_events WHERE created_at < NOW() - ($1 || ' days')::interval`,
		fmt.Sprintf("%d", retentionDays),
	)
	if err != nil {
		return 0, fmt.Errorf("cleanup chat_events: %w", err)
	}
	return tag.RowsAffected(), nil
}

// scanTimelineBuckets scans timeline query rows into TimelineBucket slices
func scanTimelineBuckets(rows interface{ Next() bool; Scan(...interface{}) error }) ([]models.TimelineBucket, error) {
	buckets := make([]models.TimelineBucket, 0)
	for rows.Next() {
		var b models.TimelineBucket
		if err := rows.Scan(&b.Timestamp, &b.Requests, &b.Errors, &b.AvgDuration, &b.AvgTTFB); err != nil {
			return nil, fmt.Errorf("scan timeline bucket: %w", err)
		}
		buckets = append(buckets, b)
	}
	return buckets, nil
}

// UserStats returns aggregate stats for a specific user
func (r *ChatEventRepository) UserStats(ctx context.Context, email string, from, to time.Time, source string) (*models.UserDetailStats, error) {
	var stats models.UserDetailStats
	var tokenJSON []byte
	err := r.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) AS total_requests,
			CASE WHEN COUNT(*) > 0 THEN ROUND(COUNT(*) FILTER (WHERE status = 'error')::numeric / COUNT(*)::numeric * 100, 2) ELSE 0 END AS error_rate,
			COALESCE(AVG(duration_ms), 0) AS avg_duration_ms,
			COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_ms), 0) AS p95_duration_ms,
			COUNT(DISTINCT resource_id) AS active_agents,
			(SELECT jsonb_build_object(
				'input', COALESCE(SUM((token_usage->>'input')::int), 0),
				'output', COALESCE(SUM((token_usage->>'output')::int), 0),
				'total', COALESCE(SUM((token_usage->>'total')::int), 0)
			) FROM chat_events WHERE user_email = $1 AND created_at BETWEEN $2 AND $3 AND token_usage IS NOT NULL AND ($4 = '' OR source = $4)) AS token_usage
		FROM chat_events WHERE user_email = $1 AND created_at BETWEEN $2 AND $3 AND ($4 = '' OR source = $4)`,
		email, from, to, source,
	).Scan(
		&stats.TotalRequests, &stats.ErrorRate, &stats.AvgDurationMs,
		&stats.P95DurationMs, &stats.ActiveAgents, &tokenJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("user stats: %w", err)
	}
	if len(tokenJSON) > 0 {
		var tu models.TokenUsage
		if json.Unmarshal(tokenJSON, &tu) == nil {
			stats.TokenUsage = &tu
		}
	}
	return &stats, nil
}

// UserTimeline returns timeline buckets for a specific user
func (r *ChatEventRepository) UserTimeline(ctx context.Context, email string, from, to time.Time, interval string, source string) ([]models.TimelineBucket, error) {
	pgInterval := toPostgresInterval(interval)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT
			to_timestamp(EXTRACT(EPOCH FROM created_at)::bigint / EXTRACT(EPOCH FROM INTERVAL '%s')::bigint * EXTRACT(EPOCH FROM INTERVAL '%s')::bigint) AS bucket,
			COUNT(*) AS requests,
			COUNT(*) FILTER (WHERE status = 'error') AS errors,
			COALESCE(AVG(duration_ms), 0) AS avg_duration,
			AVG(ttfb_ms) FILTER (WHERE ttfb_ms IS NOT NULL) AS avg_ttfb
		FROM chat_events WHERE user_email = $1 AND created_at BETWEEN $2 AND $3 AND ($4 = '' OR source = $4)
		GROUP BY bucket ORDER BY bucket`, pgInterval, pgInterval),
		email, from, to, source,
	)
	if err != nil {
		return nil, fmt.Errorf("user timeline: %w", err)
	}
	defer rows.Close()
	return scanTimelineBuckets(rows)
}

// UserTopResources returns the top resources for a specific user
func (r *ChatEventRepository) UserTopResources(ctx context.Context, email string, from, to time.Time, limit int, source string) ([]models.ResourceRanking, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			resource_type,
			resource_id,
			COALESCE(MAX(resource_name), resource_id) AS resource_name,
			COUNT(*) AS requests,
			CASE WHEN COUNT(*) > 0 THEN ROUND(COUNT(*) FILTER (WHERE status = 'error')::numeric / COUNT(*)::numeric * 100, 2) ELSE 0 END AS error_rate,
			COALESCE(AVG(duration_ms), 0) AS avg_duration_ms
		FROM chat_events WHERE user_email = $1 AND created_at BETWEEN $2 AND $3 AND ($5 = '' OR source = $5)
		GROUP BY resource_type, resource_id ORDER BY requests DESC LIMIT $4`,
		email, from, to, limit, source,
	)
	if err != nil {
		return nil, fmt.Errorf("user top resources: %w", err)
	}
	defer rows.Close()

	rankings := make([]models.ResourceRanking, 0)
	for rows.Next() {
		var rk models.ResourceRanking
		if err := rows.Scan(&rk.ResourceType, &rk.ResourceID, &rk.ResourceName, &rk.Requests, &rk.ErrorRate, &rk.AvgDurationMs); err != nil {
			return nil, fmt.Errorf("scan resource ranking: %w", err)
		}
		rankings = append(rankings, rk)
	}
	return rankings, nil
}

// UserResourceStats returns aggregate stats for a specific user + resource combination
func (r *ChatEventRepository) UserResourceStats(ctx context.Context, email, resourceType, resourceID string, from, to time.Time, source string) (*models.UserDetailStats, error) {
	var stats models.UserDetailStats
	var tokenJSON []byte
	err := r.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) AS total_requests,
			CASE WHEN COUNT(*) > 0 THEN ROUND(COUNT(*) FILTER (WHERE status = 'error')::numeric / COUNT(*)::numeric * 100, 2) ELSE 0 END AS error_rate,
			COALESCE(AVG(duration_ms), 0) AS avg_duration_ms,
			COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_ms), 0) AS p95_duration_ms,
			1 AS active_agents,
			(SELECT jsonb_build_object(
				'input', COALESCE(SUM((token_usage->>'input')::int), 0),
				'output', COALESCE(SUM((token_usage->>'output')::int), 0),
				'total', COALESCE(SUM((token_usage->>'total')::int), 0)
			) FROM chat_events WHERE user_email = $1 AND resource_type = $2 AND resource_id = $3 AND created_at BETWEEN $4 AND $5 AND token_usage IS NOT NULL AND ($6 = '' OR source = $6)) AS token_usage
		FROM chat_events WHERE user_email = $1 AND resource_type = $2 AND resource_id = $3 AND created_at BETWEEN $4 AND $5 AND ($6 = '' OR source = $6)`,
		email, resourceType, resourceID, from, to, source,
	).Scan(
		&stats.TotalRequests, &stats.ErrorRate, &stats.AvgDurationMs,
		&stats.P95DurationMs, &stats.ActiveAgents, &tokenJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("user resource stats: %w", err)
	}
	if len(tokenJSON) > 0 {
		var tu models.TokenUsage
		if json.Unmarshal(tokenJSON, &tu) == nil {
			stats.TokenUsage = &tu
		}
	}
	return &stats, nil
}

// UserResourceTimeline returns timeline buckets for a specific user + resource
func (r *ChatEventRepository) UserResourceTimeline(ctx context.Context, email, resourceType, resourceID string, from, to time.Time, interval string, source string) ([]models.TimelineBucket, error) {
	pgInterval := toPostgresInterval(interval)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT
			to_timestamp(EXTRACT(EPOCH FROM created_at)::bigint / EXTRACT(EPOCH FROM INTERVAL '%s')::bigint * EXTRACT(EPOCH FROM INTERVAL '%s')::bigint) AS bucket,
			COUNT(*) AS requests,
			COUNT(*) FILTER (WHERE status = 'error') AS errors,
			COALESCE(AVG(duration_ms), 0) AS avg_duration,
			AVG(ttfb_ms) FILTER (WHERE ttfb_ms IS NOT NULL) AS avg_ttfb
		FROM chat_events WHERE user_email = $1 AND resource_type = $2 AND resource_id = $3 AND created_at BETWEEN $4 AND $5 AND ($6 = '' OR source = $6)
		GROUP BY bucket ORDER BY bucket`, pgInterval, pgInterval),
		email, resourceType, resourceID, from, to, source,
	)
	if err != nil {
		return nil, fmt.Errorf("user resource timeline: %w", err)
	}
	defer rows.Close()
	return scanTimelineBuckets(rows)
}

// ResourceErrorEvents returns individual error events for a specific resource
func (r *ChatEventRepository) ResourceErrorEvents(ctx context.Context, resourceType, resourceID string, from, to time.Time, limit int, source string) ([]models.ErrorEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, created_at, resource_type, resource_id, COALESCE(resource_name, resource_id),
			COALESCE(error_type, 'unknown'), COALESCE(error_msg, ''), duration_ms, user_email
		FROM chat_events
		WHERE resource_type = $1 AND resource_id = $2 AND status = 'error' AND created_at BETWEEN $3 AND $4 AND ($6 = '' OR source = $6)
		ORDER BY created_at DESC LIMIT $5`,
		resourceType, resourceID, from, to, limit, source,
	)
	if err != nil {
		return nil, fmt.Errorf("resource error events: %w", err)
	}
	defer rows.Close()

	events := make([]models.ErrorEvent, 0)
	for rows.Next() {
		var e models.ErrorEvent
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.ResourceType, &e.ResourceID, &e.ResourceName,
			&e.ErrorType, &e.ErrorMsg, &e.DurationMs, &e.UserEmail); err != nil {
			return nil, fmt.Errorf("scan error event: %w", err)
		}
		events = append(events, e)
	}
	return events, nil
}

// GlobalErrorEvents returns individual error events across all resources
func (r *ChatEventRepository) GlobalErrorEvents(ctx context.Context, from, to time.Time, limit int, source string) ([]models.ErrorEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, created_at, resource_type, resource_id, COALESCE(resource_name, resource_id),
			COALESCE(error_type, 'unknown'), COALESCE(error_msg, ''), duration_ms, user_email
		FROM chat_events
		WHERE status = 'error' AND created_at BETWEEN $1 AND $2 AND ($4 = '' OR source = $4)
		ORDER BY created_at DESC LIMIT $3`,
		from, to, limit, source,
	)
	if err != nil {
		return nil, fmt.Errorf("global error events: %w", err)
	}
	defer rows.Close()

	events := make([]models.ErrorEvent, 0)
	for rows.Next() {
		var e models.ErrorEvent
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.ResourceType, &e.ResourceID, &e.ResourceName,
			&e.ErrorType, &e.ErrorMsg, &e.DurationMs, &e.UserEmail); err != nil {
			return nil, fmt.Errorf("scan error event: %w", err)
		}
		events = append(events, e)
	}
	return events, nil
}

// GlobalErrors returns aggregated error stats across all resources
func (r *ChatEventRepository) GlobalErrors(ctx context.Context, from, to time.Time, source string) ([]models.ErrorStat, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			COALESCE(error_type, 'unknown') AS error_type,
			COUNT(*) AS count,
			MAX(created_at) AS last_seen,
			(array_agg(error_msg ORDER BY created_at DESC))[1] AS last_msg
		FROM chat_events
		WHERE status = 'error' AND created_at BETWEEN $1 AND $2 AND ($3 = '' OR source = $3)
		GROUP BY error_type ORDER BY count DESC`,
		from, to, source,
	)
	if err != nil {
		return nil, fmt.Errorf("global errors: %w", err)
	}
	defer rows.Close()

	errs := make([]models.ErrorStat, 0)
	for rows.Next() {
		var e models.ErrorStat
		var lastMsg *string
		if err := rows.Scan(&e.ErrorType, &e.Count, &e.LastSeen, &lastMsg); err != nil {
			return nil, fmt.Errorf("scan error stat: %w", err)
		}
		if lastMsg != nil {
			e.LastMsg = *lastMsg
		}
		errs = append(errs, e)
	}
	return errs, nil
}

// toPostgresInterval converts shorthand interval to PostgreSQL interval string
func toPostgresInterval(interval string) string {
	switch interval {
	case "5m":
		return "5 minutes"
	case "15m":
		return "15 minutes"
	case "30m":
		return "30 minutes"
	case "1h":
		return "1 hour"
	case "6h":
		return "6 hours"
	case "1d":
		return "1 day"
	default:
		return "1 hour"
	}
}

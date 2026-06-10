package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentRepository implements repository.AgentRepository with PostgreSQL
type AgentRepository struct {
	pool *pgxpool.Pool
}

// NewAgentRepository creates a new PostgreSQL agent repository
func NewAgentRepository(pool *pgxpool.Pool) *AgentRepository {
	return &AgentRepository{pool: pool}
}

func (r *AgentRepository) Create(ctx context.Context, agent *models.Agent, allowedUsers, allowedGroups []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	headersJSON, _ := json.Marshal(agent.Headers)
	rateLimitJSON, _ := json.Marshal(agent.RateLimit)
	healthCheckJSON, _ := json.Marshal(agent.HealthCheck)
	pollingJSON, _ := json.Marshal(agent.Polling)
	customFormatJSON, _ := json.Marshal(agent.CustomFormat)

	_, err = tx.Exec(ctx,
		`INSERT INTO agents (id, name, description, category, protocol, endpoint,
		  agent_card_path, forward_authorization, require_github_token,
		  pipeline_final_agent, adk_app_name, adk_user_id, headers,
		  rate_limit, health_check, polling, custom_format,
		  max_context_tokens, summarize_threshold,
		  auth_type, bearer_token, auth_header_name)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`,
		agent.ID, agent.Name, agent.Description, agent.Category,
		agent.Protocol, agent.Endpoint, agent.AgentCardPath,
		agent.ForwardAuthorization, agent.RequireGitHubToken,
		agent.PipelineFinalAgent, agent.ADKAppName, agent.ADKUserID,
		headersJSON, rateLimitJSON, healthCheckJSON, pollingJSON, customFormatJSON,
		agent.MaxContextTokens, agent.SummarizeThreshold,
		agent.AuthType, agent.BearerToken, agent.AuthHeaderName,
	)
	if err != nil {
		return fmt.Errorf("insert agent: %w", err)
	}

	if err := insertPermissions(ctx, tx, "agent_allowed_users", "agent_id", agent.ID, "user_email", allowedUsers); err != nil {
		return err
	}
	if err := insertPermissions(ctx, tx, "agent_allowed_groups", "agent_id", agent.ID, "group_name", allowedGroups); err != nil {
		return err
	}
	if err := insertAPIKeyRules(ctx, tx, agent.ID, agent.APIKeyRules); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *AgentRepository) Get(ctx context.Context, id string) (*models.Agent, []string, []string, error) {
	agent, err := r.scanAgent(ctx,
		`SELECT id, name, description, category, protocol, endpoint,
		  agent_card_path, forward_authorization, require_github_token,
		  pipeline_final_agent, adk_app_name, adk_user_id, headers,
		  rate_limit, health_check, polling, custom_format,
		  max_context_tokens, summarize_threshold,
		  auth_type, bearer_token, auth_header_name, created_at, updated_at
		 FROM agents WHERE id = $1`, id)
	if err != nil {
		return nil, nil, nil, err
	}

	users, groups, err := r.GetPermissions(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}

	rules, err := r.ListAPIKeyRules(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}
	agent.APIKeyRules = rules

	return agent, users, groups, nil
}

func (r *AgentRepository) List(ctx context.Context) ([]*models.Agent, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, description, category, protocol, endpoint,
		  agent_card_path, forward_authorization, require_github_token,
		  pipeline_final_agent, adk_app_name, adk_user_id, headers,
		  rate_limit, health_check, polling, custom_format,
		  max_context_tokens, summarize_threshold,
		  auth_type, bearer_token, auth_header_name, created_at, updated_at
		 FROM agents ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []*models.Agent
	for rows.Next() {
		agent, err := r.scanAgentRow(rows)
		if err != nil {
			return nil, err
		}

		// Load permissions
		users, groups, err := r.GetPermissions(ctx, agent.ID)
		if err != nil {
			return nil, err
		}
		agent.AllowedUsers = users
		agent.AllowedGroups = groups

		rules, err := r.ListAPIKeyRules(ctx, agent.ID)
		if err != nil {
			return nil, err
		}
		agent.APIKeyRules = rules
		agents = append(agents, agent)
	}
	return agents, nil
}

func (r *AgentRepository) Update(ctx context.Context, agent *models.Agent) error {
	headersJSON, _ := json.Marshal(agent.Headers)
	rateLimitJSON, _ := json.Marshal(agent.RateLimit)
	healthCheckJSON, _ := json.Marshal(agent.HealthCheck)
	pollingJSON, _ := json.Marshal(agent.Polling)
	customFormatJSON, _ := json.Marshal(agent.CustomFormat)

	tag, err := r.pool.Exec(ctx,
		`UPDATE agents SET name=$2, description=$3, category=$4, protocol=$5,
		  endpoint=$6, agent_card_path=$7, forward_authorization=$8,
		  require_github_token=$9, pipeline_final_agent=$10,
		  adk_app_name=$11, adk_user_id=$12, headers=$13,
		  rate_limit=$14, health_check=$15, polling=$16, custom_format=$17,
		  max_context_tokens=$18, summarize_threshold=$19,
		  auth_type=$20, bearer_token=$21, auth_header_name=$22, updated_at=NOW()
		 WHERE id=$1`,
		agent.ID, agent.Name, agent.Description, agent.Category,
		agent.Protocol, agent.Endpoint, agent.AgentCardPath,
		agent.ForwardAuthorization, agent.RequireGitHubToken,
		agent.PipelineFinalAgent, agent.ADKAppName, agent.ADKUserID,
		headersJSON, rateLimitJSON, healthCheckJSON, pollingJSON, customFormatJSON,
		agent.MaxContextTokens, agent.SummarizeThreshold,
		agent.AuthType, agent.BearerToken, agent.AuthHeaderName,
	)
	if err != nil {
		return fmt.Errorf("update agent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("agent not found: %s", agent.ID)
	}
	return nil
}

func (r *AgentRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM agents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("agent not found: %s", id)
	}
	return nil
}

func (r *AgentRepository) GetPermissions(ctx context.Context, agentID string) ([]string, []string, error) {
	users, err := queryStrings(ctx, r.pool, `SELECT user_email FROM agent_allowed_users WHERE agent_id = $1`, agentID)
	if err != nil {
		return nil, nil, fmt.Errorf("get agent users: %w", err)
	}
	groups, err := queryStrings(ctx, r.pool, `SELECT group_name FROM agent_allowed_groups WHERE agent_id = $1`, agentID)
	if err != nil {
		return nil, nil, fmt.Errorf("get agent groups: %w", err)
	}
	return users, groups, nil
}

func (r *AgentRepository) UpdatePermissions(ctx context.Context, agentID string, users, groups []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	_, _ = tx.Exec(ctx, `DELETE FROM agent_allowed_users WHERE agent_id = $1`, agentID)
	_, _ = tx.Exec(ctx, `DELETE FROM agent_allowed_groups WHERE agent_id = $1`, agentID)

	if err := insertPermissions(ctx, tx, "agent_allowed_users", "agent_id", agentID, "user_email", users); err != nil {
		return err
	}
	if err := insertPermissions(ctx, tx, "agent_allowed_groups", "agent_id", agentID, "group_name", groups); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ListAPIKeyRules returns the bearer-mode API key rules of an agent, ordered
// by position (group precedence) then subject for determinism.
func (r *AgentRepository) ListAPIKeyRules(ctx context.Context, agentID string) ([]models.AgentAPIKeyRule, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, agent_id, subject_type, subject, api_key, priority
		 FROM agent_api_key_rules WHERE agent_id = $1 ORDER BY priority, subject`, agentID)
	if err != nil {
		return nil, fmt.Errorf("list api key rules: %w", err)
	}
	defer rows.Close()

	var rules []models.AgentAPIKeyRule
	for rows.Next() {
		var rule models.AgentAPIKeyRule
		if err := rows.Scan(&rule.ID, &rule.AgentID, &rule.SubjectType, &rule.Subject, &rule.APIKey, &rule.Priority); err != nil {
			return nil, fmt.Errorf("scan api key rule: %w", err)
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// ReplaceAPIKeyRules atomically replaces all API key rules of an agent
// (delete + insert in a tx, same pattern as UpdatePermissions).
func (r *AgentRepository) ReplaceAPIKeyRules(ctx context.Context, agentID string, rules []models.AgentAPIKeyRule) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM agent_api_key_rules WHERE agent_id = $1`, agentID); err != nil {
		return fmt.Errorf("delete api key rules: %w", err)
	}
	if err := insertAPIKeyRules(ctx, tx, agentID, rules); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func insertAPIKeyRules(ctx context.Context, tx pgx.Tx, agentID string, rules []models.AgentAPIKeyRule) error {
	for _, rule := range rules {
		_, err := tx.Exec(ctx,
			`INSERT INTO agent_api_key_rules (agent_id, subject_type, subject, api_key, priority)
			 VALUES ($1,$2,$3,$4,$5)`,
			agentID, rule.SubjectType, rule.Subject, rule.APIKey, rule.Priority)
		if err != nil {
			return fmt.Errorf("insert api key rule: %w", err)
		}
	}
	return nil
}

func (r *AgentRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM agents`).Scan(&count)
	return count, err
}

func (r *AgentRepository) scanAgent(ctx context.Context, query string, args ...interface{}) (*models.Agent, error) {
	row := r.pool.QueryRow(ctx, query, args...)
	agent, err := scanAgentFields(row.Scan)
	if err != nil {
		return nil, fmt.Errorf("scan agent: %w", err)
	}
	r.unmarshalAgentJSON(agent)
	return agent.agent, nil
}

func (r *AgentRepository) scanAgentRow(rows pgx.Rows) (*models.Agent, error) {
	agent, err := scanAgentFields(rows.Scan)
	if err != nil {
		return nil, fmt.Errorf("scan agent row: %w", err)
	}
	r.unmarshalAgentJSON(agent)
	return agent.agent, nil
}

type scannedAgent struct {
	agent                                                                      *models.Agent
	headersJSON, rateLimitJSON, healthCheckJSON, pollingJSON, customFormatJSON []byte
}

func scanAgentFields(scan func(dest ...interface{}) error) (*scannedAgent, error) {
	var agent models.Agent
	var s scannedAgent
	var agentCardPath, pipelineFinalAgent, adkAppName, adkUserID *string
	var createdAt, updatedAt interface{}

	err := scan(
		&agent.ID, &agent.Name, &agent.Description, &agent.Category,
		&agent.Protocol, &agent.Endpoint, &agentCardPath,
		&agent.ForwardAuthorization, &agent.RequireGitHubToken,
		&pipelineFinalAgent, &adkAppName, &adkUserID,
		&s.headersJSON, &s.rateLimitJSON, &s.healthCheckJSON, &s.pollingJSON,
		&s.customFormatJSON,
		&agent.MaxContextTokens, &agent.SummarizeThreshold,
		&agent.AuthType, &agent.BearerToken, &agent.AuthHeaderName,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if agentCardPath != nil {
		agent.AgentCardPath = *agentCardPath
	}
	if pipelineFinalAgent != nil {
		agent.PipelineFinalAgent = *pipelineFinalAgent
	}
	if adkAppName != nil {
		agent.ADKAppName = *adkAppName
	}
	if adkUserID != nil {
		agent.ADKUserID = *adkUserID
	}

	s.agent = &agent
	return &s, nil
}

func (r *AgentRepository) unmarshalAgentJSON(s *scannedAgent) {
	agent := s.agent
	if len(s.headersJSON) > 0 {
		_ = json.Unmarshal(s.headersJSON, &agent.Headers)
	}
	if agent.Headers == nil {
		agent.Headers = make(map[string]string)
	}
	if len(s.rateLimitJSON) > 0 && string(s.rateLimitJSON) != "null" {
		var rl models.RateLimitConfig
		if json.Unmarshal(s.rateLimitJSON, &rl) == nil {
			agent.RateLimit = &rl
		}
	}
	if len(s.healthCheckJSON) > 0 && string(s.healthCheckJSON) != "null" {
		var hc models.HealthCheckConfig
		if json.Unmarshal(s.healthCheckJSON, &hc) == nil {
			agent.HealthCheck = &hc
		}
	}
	if len(s.pollingJSON) > 0 && string(s.pollingJSON) != "null" {
		var p models.PollingConfig
		if json.Unmarshal(s.pollingJSON, &p) == nil {
			agent.Polling = &p
		}
	}
	if len(s.customFormatJSON) > 0 && string(s.customFormatJSON) != "null" {
		var cf models.CustomFormatConfig
		if json.Unmarshal(s.customFormatJSON, &cf) == nil {
			agent.CustomFormat = &cf
		}
	}
	agent.Status = "unknown"
}

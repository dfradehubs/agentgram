package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MCPServerRepository implements repository.MCPServerRepository with PostgreSQL
type MCPServerRepository struct {
	pool *pgxpool.Pool
}

// NewMCPServerRepository creates a new PostgreSQL MCP server repository
func NewMCPServerRepository(pool *pgxpool.Pool) *MCPServerRepository {
	return &MCPServerRepository{pool: pool}
}

func (r *MCPServerRepository) Create(ctx context.Context, server *models.MCPServer) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	headersJSON, _ := json.Marshal(server.Headers)

	authType := server.GetAuthType()

	_, err = tx.Exec(ctx,
		`INSERT INTO mcp_servers (id, name, description, transport, url, headers, forward_auth,
		  auth_type, oauth2_auth_server_url, oauth2_client_id, oauth2_client_secret, oauth2_scopes, bearer_token,
		  auth_header_name)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		server.ID, server.Name, server.Description, server.Transport, server.URL, headersJSON, server.ForwardAuth,
		authType, server.OAuth2AuthServerURL, server.OAuth2ClientID, server.OAuth2ClientSecret, server.OAuth2Scopes, server.BearerToken,
		server.AuthHeaderName,
	)
	if err != nil {
		return fmt.Errorf("insert mcp server: %w", err)
	}

	if err := insertPermissions(ctx, tx, "mcp_allowed_users", "mcp_server_id", server.ID, "user_email", server.AllowedUsers); err != nil {
		return err
	}
	if err := insertPermissions(ctx, tx, "mcp_allowed_groups", "mcp_server_id", server.ID, "group_name", server.AllowedGroups); err != nil {
		return err
	}
	if err := insertMCPAPIKeyRules(ctx, tx, server.ID, server.APIKeyRules); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *MCPServerRepository) Get(ctx context.Context, id string) (*models.MCPServer, error) {
	var s models.MCPServer
	var headersJSON []byte
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, description, transport, url, headers, forward_auth, created_at, updated_at,
		        auth_type, oauth2_auth_server_url, oauth2_client_id, oauth2_client_secret, oauth2_scopes, bearer_token,
		        auth_header_name
		 FROM mcp_servers WHERE id = $1`, id,
	).Scan(&s.ID, &s.Name, &s.Description, &s.Transport, &s.URL, &headersJSON, &s.ForwardAuth, &s.CreatedAt, &s.UpdatedAt,
		&s.AuthType, &s.OAuth2AuthServerURL, &s.OAuth2ClientID, &s.OAuth2ClientSecret, &s.OAuth2Scopes, &s.BearerToken,
		&s.AuthHeaderName)
	if err != nil {
		return nil, fmt.Errorf("get mcp server: %w", err)
	}

	if len(headersJSON) > 0 {
		_ = json.Unmarshal(headersJSON, &s.Headers)
	}
	if s.Headers == nil {
		s.Headers = make(map[string]string)
	}

	users, err := queryStrings(ctx, r.pool, `SELECT user_email FROM mcp_allowed_users WHERE mcp_server_id = $1`, id)
	if err != nil {
		return nil, err
	}
	groups, err := queryStrings(ctx, r.pool, `SELECT group_name FROM mcp_allowed_groups WHERE mcp_server_id = $1`, id)
	if err != nil {
		return nil, err
	}
	s.AllowedUsers = users
	s.AllowedGroups = groups

	rules, err := r.ListAPIKeyRules(ctx, id)
	if err != nil {
		return nil, err
	}
	s.APIKeyRules = rules

	return &s, nil
}

func (r *MCPServerRepository) List(ctx context.Context) ([]*models.MCPServer, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, description, transport, url, headers, forward_auth, created_at, updated_at,
		        auth_type, oauth2_auth_server_url, oauth2_client_id, oauth2_client_secret, oauth2_scopes, bearer_token,
		        auth_header_name
		 FROM mcp_servers ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list mcp servers: %w", err)
	}
	defer rows.Close()

	var servers []*models.MCPServer
	for rows.Next() {
		var s models.MCPServer
		var headersJSON []byte
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Transport, &s.URL, &headersJSON, &s.ForwardAuth, &s.CreatedAt, &s.UpdatedAt,
			&s.AuthType, &s.OAuth2AuthServerURL, &s.OAuth2ClientID, &s.OAuth2ClientSecret, &s.OAuth2Scopes, &s.BearerToken,
			&s.AuthHeaderName); err != nil {
			return nil, fmt.Errorf("scan mcp server: %w", err)
		}
		if len(headersJSON) > 0 {
			_ = json.Unmarshal(headersJSON, &s.Headers)
		}
		if s.Headers == nil {
			s.Headers = make(map[string]string)
		}

		users, err := queryStrings(ctx, r.pool, `SELECT user_email FROM mcp_allowed_users WHERE mcp_server_id = $1`, s.ID)
		if err != nil {
			return nil, err
		}
		groups, err := queryStrings(ctx, r.pool, `SELECT group_name FROM mcp_allowed_groups WHERE mcp_server_id = $1`, s.ID)
		if err != nil {
			return nil, err
		}
		s.AllowedUsers = users
		s.AllowedGroups = groups

		rules, err := r.ListAPIKeyRules(ctx, s.ID)
		if err != nil {
			return nil, err
		}
		s.APIKeyRules = rules

		servers = append(servers, &s)
	}
	return servers, nil
}

func (r *MCPServerRepository) Update(ctx context.Context, server *models.MCPServer) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	headersJSON, _ := json.Marshal(server.Headers)
	authType := server.GetAuthType()

	tag, err := tx.Exec(ctx,
		`UPDATE mcp_servers SET name=$2, description=$3, transport=$4, url=$5, headers=$6, forward_auth=$7,
		  auth_type=$8, oauth2_auth_server_url=$9, oauth2_client_id=$10, oauth2_client_secret=$11, oauth2_scopes=$12,
		  bearer_token=$13, auth_header_name=$14, updated_at=NOW()
		 WHERE id=$1`,
		server.ID, server.Name, server.Description, server.Transport, server.URL, headersJSON, server.ForwardAuth,
		authType, server.OAuth2AuthServerURL, server.OAuth2ClientID, server.OAuth2ClientSecret, server.OAuth2Scopes, server.BearerToken,
		server.AuthHeaderName,
	)
	if err != nil {
		return fmt.Errorf("update mcp server: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("mcp server not found: %s", server.ID)
	}

	_, _ = tx.Exec(ctx, `DELETE FROM mcp_allowed_users WHERE mcp_server_id = $1`, server.ID)
	_, _ = tx.Exec(ctx, `DELETE FROM mcp_allowed_groups WHERE mcp_server_id = $1`, server.ID)

	if err := insertPermissions(ctx, tx, "mcp_allowed_users", "mcp_server_id", server.ID, "user_email", server.AllowedUsers); err != nil {
		return err
	}
	if err := insertPermissions(ctx, tx, "mcp_allowed_groups", "mcp_server_id", server.ID, "group_name", server.AllowedGroups); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *MCPServerRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM mcp_servers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete mcp server: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("mcp server not found: %s", id)
	}
	return nil
}

func (r *MCPServerRepository) UpdatePermissions(ctx context.Context, serverID string, users, groups []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	_, _ = tx.Exec(ctx, `DELETE FROM mcp_allowed_users WHERE mcp_server_id = $1`, serverID)
	_, _ = tx.Exec(ctx, `DELETE FROM mcp_allowed_groups WHERE mcp_server_id = $1`, serverID)

	if err := insertPermissions(ctx, tx, "mcp_allowed_users", "mcp_server_id", serverID, "user_email", users); err != nil {
		return err
	}
	if err := insertPermissions(ctx, tx, "mcp_allowed_groups", "mcp_server_id", serverID, "group_name", groups); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *MCPServerRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM mcp_servers`).Scan(&count)
	return count, err
}

func (r *MCPServerRepository) ListScopeMappings(ctx context.Context, serverID string) ([]models.MCPOAuth2ScopeMapping, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, mcp_server_id, group_name, scopes, created_at
		 FROM mcp_oauth2_scope_mappings WHERE mcp_server_id = $1 ORDER BY group_name`, serverID)
	if err != nil {
		return nil, fmt.Errorf("list scope mappings: %w", err)
	}
	defer rows.Close()

	var mappings []models.MCPOAuth2ScopeMapping
	for rows.Next() {
		var m models.MCPOAuth2ScopeMapping
		if err := rows.Scan(&m.ID, &m.MCPServerID, &m.GroupName, &m.Scopes, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan scope mapping: %w", err)
		}
		mappings = append(mappings, m)
	}
	return mappings, nil
}

func (r *MCPServerRepository) UpsertScopeMapping(ctx context.Context, mapping *models.MCPOAuth2ScopeMapping) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO mcp_oauth2_scope_mappings (mcp_server_id, group_name, scopes)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (mcp_server_id, group_name) DO UPDATE SET scopes = EXCLUDED.scopes`,
		mapping.MCPServerID, mapping.GroupName, mapping.Scopes)
	if err != nil {
		return fmt.Errorf("upsert scope mapping: %w", err)
	}
	return nil
}

func (r *MCPServerRepository) DeleteScopeMapping(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM mcp_oauth2_scope_mappings WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete scope mapping: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("scope mapping not found: %s", id)
	}
	return nil
}

// ListAPIKeyRules returns the bearer-mode API key rules of an MCP server,
// ordered by position (group precedence) then subject for determinism.
func (r *MCPServerRepository) ListAPIKeyRules(ctx context.Context, serverID string) ([]models.MCPAPIKeyRule, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, mcp_server_id, subject_type, subject, api_key, priority
		 FROM mcp_api_key_rules WHERE mcp_server_id = $1 ORDER BY priority, subject`, serverID)
	if err != nil {
		return nil, fmt.Errorf("list mcp api key rules: %w", err)
	}
	defer rows.Close()

	var rules []models.MCPAPIKeyRule
	for rows.Next() {
		var rule models.MCPAPIKeyRule
		if err := rows.Scan(&rule.ID, &rule.MCPServerID, &rule.SubjectType, &rule.Subject, &rule.APIKey, &rule.Priority); err != nil {
			return nil, fmt.Errorf("scan mcp api key rule: %w", err)
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// ReplaceAPIKeyRules atomically replaces all API key rules of an MCP server.
func (r *MCPServerRepository) ReplaceAPIKeyRules(ctx context.Context, serverID string, rules []models.MCPAPIKeyRule) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM mcp_api_key_rules WHERE mcp_server_id = $1`, serverID); err != nil {
		return fmt.Errorf("delete mcp api key rules: %w", err)
	}
	if err := insertMCPAPIKeyRules(ctx, tx, serverID, rules); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func insertMCPAPIKeyRules(ctx context.Context, tx pgx.Tx, serverID string, rules []models.MCPAPIKeyRule) error {
	for _, rule := range rules {
		_, err := tx.Exec(ctx,
			`INSERT INTO mcp_api_key_rules (mcp_server_id, subject_type, subject, api_key, priority)
			 VALUES ($1,$2,$3,$4,$5)`,
			serverID, rule.SubjectType, rule.Subject, rule.APIKey, rule.Priority)
		if err != nil {
			return fmt.Errorf("insert mcp api key rule: %w", err)
		}
	}
	return nil
}

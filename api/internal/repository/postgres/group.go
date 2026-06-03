package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GroupRepository implements repository.GroupRepository with PostgreSQL
type GroupRepository struct {
	pool *pgxpool.Pool
}

// NewGroupRepository creates a new PostgreSQL group repository
func NewGroupRepository(pool *pgxpool.Pool) *GroupRepository {
	return &GroupRepository{pool: pool}
}

func (r *GroupRepository) Create(ctx context.Context, group *models.AgentGroup, allowedUsers, allowedGroups []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	agentIDsJSON, _ := json.Marshal(group.AgentIDs)

	_, err = tx.Exec(ctx,
		`INSERT INTO agent_groups (id, name, agent_ids, created_by) VALUES ($1, $2, $3, $4)`,
		group.ID, group.Name, agentIDsJSON, group.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("insert group: %w", err)
	}

	// Auto-include creator in allowed users
	creatorIncluded := false
	for _, u := range allowedUsers {
		if strings.EqualFold(u, group.CreatedBy) {
			creatorIncluded = true
			break
		}
	}
	if !creatorIncluded && group.CreatedBy != "" {
		allowedUsers = append(allowedUsers, group.CreatedBy)
	}

	if err := insertPermissions(ctx, tx, "agent_group_allowed_users", "group_id", group.ID, "user_email", allowedUsers); err != nil {
		return err
	}
	if err := insertPermissions(ctx, tx, "agent_group_allowed_groups", "group_id", group.ID, "group_name", allowedGroups); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *GroupRepository) Get(ctx context.Context, id string) (*models.AgentGroup, error) {
	group := &models.AgentGroup{}
	var agentIDsJSON []byte

	err := r.pool.QueryRow(ctx,
		`SELECT id, name, agent_ids, created_by, created_at, updated_at FROM agent_groups WHERE id = $1`, id,
	).Scan(&group.ID, &group.Name, &agentIDsJSON, &group.CreatedBy, &group.CreatedAt, &group.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get group: %w", err)
	}

	if len(agentIDsJSON) > 0 {
		_ = json.Unmarshal(agentIDsJSON, &group.AgentIDs)
	}

	users, groups, err := r.GetPermissions(ctx, id)
	if err != nil {
		return nil, err
	}
	group.AllowedUsers = users
	group.AllowedGroups = groups

	return group, nil
}

func (r *GroupRepository) List(ctx context.Context) ([]*models.AgentGroup, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, agent_ids, created_by, created_at, updated_at FROM agent_groups ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer rows.Close()

	var groups []*models.AgentGroup
	for rows.Next() {
		g := &models.AgentGroup{}
		var agentIDsJSON []byte
		if err := rows.Scan(&g.ID, &g.Name, &agentIDsJSON, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan group: %w", err)
		}
		if len(agentIDsJSON) > 0 {
			_ = json.Unmarshal(agentIDsJSON, &g.AgentIDs)
		}

		users, grps, err := r.GetPermissions(ctx, g.ID)
		if err != nil {
			return nil, err
		}
		g.AllowedUsers = users
		g.AllowedGroups = grps
		groups = append(groups, g)
	}
	return groups, nil
}

func (r *GroupRepository) Update(ctx context.Context, group *models.AgentGroup) error {
	agentIDsJSON, _ := json.Marshal(group.AgentIDs)

	tag, err := r.pool.Exec(ctx,
		`UPDATE agent_groups SET name=$2, agent_ids=$3, updated_at=NOW() WHERE id=$1`,
		group.ID, group.Name, agentIDsJSON,
	)
	if err != nil {
		return fmt.Errorf("update group: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("group not found: %s", group.ID)
	}
	return nil
}

func (r *GroupRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM agent_groups WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("group not found: %s", id)
	}
	return nil
}

func (r *GroupRepository) GetPermissions(ctx context.Context, groupID string) ([]string, []string, error) {
	users, err := queryStrings(ctx, r.pool, `SELECT user_email FROM agent_group_allowed_users WHERE group_id = $1`, groupID)
	if err != nil {
		return nil, nil, fmt.Errorf("get group users: %w", err)
	}
	groups, err := queryStrings(ctx, r.pool, `SELECT group_name FROM agent_group_allowed_groups WHERE group_id = $1`, groupID)
	if err != nil {
		return nil, nil, fmt.Errorf("get group groups: %w", err)
	}
	return users, groups, nil
}

func (r *GroupRepository) UpdatePermissions(ctx context.Context, groupID string, users, groups []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	_, _ = tx.Exec(ctx, `DELETE FROM agent_group_allowed_users WHERE group_id = $1`, groupID)
	_, _ = tx.Exec(ctx, `DELETE FROM agent_group_allowed_groups WHERE group_id = $1`, groupID)

	if err := insertPermissions(ctx, tx, "agent_group_allowed_users", "group_id", groupID, "user_email", users); err != nil {
		return err
	}
	if err := insertPermissions(ctx, tx, "agent_group_allowed_groups", "group_id", groupID, "group_name", groups); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *GroupRepository) ListAccessible(ctx context.Context, email string, userGroups []string) ([]*models.AgentGroup, error) {
	// Build query: user has access if created_by matches, or email in allowed_users, or wildcard *, or user group matches
	query := `
		SELECT DISTINCT g.id, g.name, g.agent_ids, g.created_by, g.created_at, g.updated_at
		FROM agent_groups g
		LEFT JOIN agent_group_allowed_users u ON g.id = u.group_id
		LEFT JOIN agent_group_allowed_groups gg ON g.id = gg.group_id
		WHERE g.created_by = $1
		   OR u.user_email = $1
		   OR u.user_email = '*'`

	args := []interface{}{email}

	if len(userGroups) > 0 {
		placeholders := make([]string, len(userGroups))
		for i, grp := range userGroups {
			args = append(args, grp)
			placeholders[i] = fmt.Sprintf("$%d", len(args))
		}
		query += fmt.Sprintf("\n		   OR LOWER(gg.group_name) IN (%s)", strings.Join(placeholders, ","))
	}

	query += "\n		ORDER BY g.name"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list accessible groups: %w", err)
	}
	defer rows.Close()

	var groups []*models.AgentGroup
	for rows.Next() {
		g := &models.AgentGroup{}
		var agentIDsJSON []byte
		if err := rows.Scan(&g.ID, &g.Name, &agentIDsJSON, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan accessible group: %w", err)
		}
		if len(agentIDsJSON) > 0 {
			_ = json.Unmarshal(agentIDsJSON, &g.AgentIDs)
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func (r *GroupRepository) AddSession(ctx context.Context, groupID, sessionID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO agent_group_sessions (group_id, session_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		groupID, sessionID,
	)
	if err != nil {
		return fmt.Errorf("add session to group: %w", err)
	}
	return nil
}

func (r *GroupRepository) RemoveSession(ctx context.Context, groupID, sessionID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM agent_group_sessions WHERE group_id = $1 AND session_id = $2`,
		groupID, sessionID,
	)
	if err != nil {
		return fmt.Errorf("remove session from group: %w", err)
	}
	return nil
}

func (r *GroupRepository) ListSessions(ctx context.Context, groupID string) ([]string, error) {
	return queryStrings(ctx, r.pool,
		`SELECT session_id FROM agent_group_sessions WHERE group_id = $1 ORDER BY created_at DESC`, groupID,
	)
}

// GetAllInheritedPermissions returns inherited permissions for all agents from their groups.
// Returns a map of agentID → InheritedPerms.
func (r *GroupRepository) GetAllInheritedPermissions(ctx context.Context) (map[string]*models.InheritedPerms, error) {
	// Get all groups with their permissions
	allGroups, err := r.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list groups for inheritance: %w", err)
	}

	result := make(map[string]*models.InheritedPerms)

	for _, g := range allGroups {
		for _, agentID := range g.AgentIDs {
			if result[agentID] == nil {
				result[agentID] = &models.InheritedPerms{}
			}
			perms := result[agentID]
			for _, u := range g.AllowedUsers {
				perms.Users = append(perms.Users, models.InheritedPermission{
					Value:     u,
					FromGroup: g.Name,
				})
			}
			for _, grp := range g.AllowedGroups {
				perms.Groups = append(perms.Groups, models.InheritedPermission{
					Value:     grp,
					FromGroup: g.Name,
				})
			}
		}
	}

	return result, nil
}

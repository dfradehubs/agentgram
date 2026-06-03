package repository

import (
	"context"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/models"
)

// UserRepository manages user persistence
type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	List(ctx context.Context) ([]*models.User, error)
	UpdateRole(ctx context.Context, email, role string) error
	UpdateLastAccess(ctx context.Context, email string) error
}

// AgentRepository manages agent persistence
type AgentRepository interface {
	Create(ctx context.Context, agent *models.Agent, allowedUsers, allowedGroups []string) error
	Get(ctx context.Context, id string) (*models.Agent, []string, []string, error)
	List(ctx context.Context) ([]*models.Agent, error)
	Update(ctx context.Context, agent *models.Agent) error
	Delete(ctx context.Context, id string) error
	GetPermissions(ctx context.Context, agentID string) (users []string, groups []string, err error)
	UpdatePermissions(ctx context.Context, agentID string, users, groups []string) error
	Count(ctx context.Context) (int, error)
}

// MCPServerRepository manages MCP server persistence
type MCPServerRepository interface {
	Create(ctx context.Context, server *models.MCPServer) error
	Get(ctx context.Context, id string) (*models.MCPServer, error)
	List(ctx context.Context) ([]*models.MCPServer, error)
	Update(ctx context.Context, server *models.MCPServer) error
	Delete(ctx context.Context, id string) error
	UpdatePermissions(ctx context.Context, serverID string, users, groups []string) error
	Count(ctx context.Context) (int, error)
	ListScopeMappings(ctx context.Context, serverID string) ([]models.MCPOAuth2ScopeMapping, error)
	UpsertScopeMapping(ctx context.Context, mapping *models.MCPOAuth2ScopeMapping) error
	DeleteScopeMapping(ctx context.Context, id string) error
}

// LLMModelRepository manages LLM model persistence
type LLMModelRepository interface {
	Create(ctx context.Context, model *models.LLMModel) error
	Get(ctx context.Context, id string) (*models.LLMModel, error)
	List(ctx context.Context) ([]*models.LLMModel, error)
	ListByRole(ctx context.Context, role string) ([]*models.LLMModel, error)
	Update(ctx context.Context, model *models.LLMModel) error
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int, error)
}

// ChatEventRepository manages chat event analytics persistence.
// All query methods accept an optional source filter ("web", "slack", or "" for all).
type ChatEventRepository interface {
	Insert(ctx context.Context, event *models.ChatEvent) error
	ResourceStats(ctx context.Context, resourceType, resourceID string, from, to time.Time, source string) (*models.ResourceStats, error)
	ResourceTimeline(ctx context.Context, resourceType, resourceID string, from, to time.Time, interval, source string) ([]models.TimelineBucket, error)
	ResourceUsers(ctx context.Context, resourceType, resourceID string, from, to time.Time, limit int, source string) ([]models.UserStat, error)
	ResourceErrors(ctx context.Context, resourceType, resourceID string, from, to time.Time, source string) ([]models.ErrorStat, error)
	GlobalStats(ctx context.Context, from, to time.Time, source string) (*models.GlobalStats, error)
	GlobalTimeline(ctx context.Context, from, to time.Time, interval, source string) ([]models.TimelineBucket, error)
	GlobalUsers(ctx context.Context, from, to time.Time, limit int, source string) ([]models.UserStat, error)
	TopResources(ctx context.Context, from, to time.Time, limit int, source string) ([]models.ResourceRanking, error)
	Cleanup(ctx context.Context, retentionDays int) (int64, error)
	UserStats(ctx context.Context, email string, from, to time.Time, source string) (*models.UserDetailStats, error)
	UserTimeline(ctx context.Context, email string, from, to time.Time, interval, source string) ([]models.TimelineBucket, error)
	UserTopResources(ctx context.Context, email string, from, to time.Time, limit int, source string) ([]models.ResourceRanking, error)
	UserResourceStats(ctx context.Context, email, resourceType, resourceID string, from, to time.Time, source string) (*models.UserDetailStats, error)
	UserResourceTimeline(ctx context.Context, email, resourceType, resourceID string, from, to time.Time, interval, source string) ([]models.TimelineBucket, error)
	ResourceErrorEvents(ctx context.Context, resourceType, resourceID string, from, to time.Time, limit int, source string) ([]models.ErrorEvent, error)
	GlobalErrorEvents(ctx context.Context, from, to time.Time, limit int, source string) ([]models.ErrorEvent, error)
	GlobalErrors(ctx context.Context, from, to time.Time, source string) ([]models.ErrorStat, error)
}

// GroupRepository manages agent group persistence
type GroupRepository interface {
	Create(ctx context.Context, group *models.AgentGroup, allowedUsers, allowedGroups []string) error
	Get(ctx context.Context, id string) (*models.AgentGroup, error)
	List(ctx context.Context) ([]*models.AgentGroup, error)
	Update(ctx context.Context, group *models.AgentGroup) error
	Delete(ctx context.Context, id string) error
	GetPermissions(ctx context.Context, groupID string) (users []string, groups []string, err error)
	UpdatePermissions(ctx context.Context, groupID string, users, groups []string) error
	ListAccessible(ctx context.Context, email string, userGroups []string) ([]*models.AgentGroup, error)
	GetAllInheritedPermissions(ctx context.Context) (map[string]*models.InheritedPerms, error)
	// Group sessions
	AddSession(ctx context.Context, groupID, sessionID string) error
	RemoveSession(ctx context.Context, groupID, sessionID string) error
	ListSessions(ctx context.Context, groupID string) ([]string, error)
}

// BasicAuthRepository manages basic auth user persistence
type BasicAuthRepository interface {
	GetByUsername(ctx context.Context, username string) (*models.BasicAuthUser, error)
	Create(ctx context.Context, user *models.BasicAuthUser) error
	List(ctx context.Context) ([]*models.BasicAuthUser, error)
	Delete(ctx context.Context, id string) error
}

// AuditRepository manages audit log persistence
type AuditRepository interface {
	Log(ctx context.Context, entry *models.AuditEntry) error
	List(ctx context.Context, limit, offset int) ([]*models.AuditEntry, error)
}

// SlackIntegrationRepository manages Slack bot integration persistence
type SlackIntegrationRepository interface {
	Upsert(ctx context.Context, integration *models.SlackIntegration) error
	Get(ctx context.Context, agentID string) (*models.SlackIntegration, error)
	Delete(ctx context.Context, agentID string) error
	ListEnabled(ctx context.Context) ([]*models.SlackIntegration, error)
	UpdateStatus(ctx context.Context, agentID, status, statusMessage string) error
}

// SlackUserLinkRepository manages Slack user ↔ Keycloak account linking
type SlackUserLinkRepository interface {
	Upsert(ctx context.Context, link *models.SlackUserLink) error
	GetBySlackUserID(ctx context.Context, slackUserID string) (*models.SlackUserLink, error)
	Delete(ctx context.Context, slackUserID string) error
	ListAll(ctx context.Context) ([]*models.SlackUserLink, error)
	SetGitHubToken(ctx context.Context, slackUserID, githubToken, githubRefreshToken string) error
	RevokeGitHub(ctx context.Context, slackUserID string) error
}

// SharedSessionRepository manages shared session link persistence
type SharedSessionRepository interface {
	Create(ctx context.Context, sessionID, agentID, sharedBy string, expiresAt time.Time) (*models.SharedSession, error)
	GetByToken(ctx context.Context, token string) (*models.SharedSession, error)
	GetBySessionID(ctx context.Context, sessionID string) (*models.SharedSession, error)
	Revoke(ctx context.Context, sessionID, userEmail string) error
}

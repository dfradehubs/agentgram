package agents

import (
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestHasAccess_AllowedUser(t *testing.T) {
	agent := &models.Agent{
		ID:            "test-agent",
		AllowedUsers:  []string{"user@example.com"},
		AllowedGroups: []string{},
	}

	// Allowed user
	assert.True(t, HasAccess(agent, "user@example.com", nil))

	// Disallowed user
	assert.False(t, HasAccess(agent, "other@example.com", nil))
}

func TestHasAccess_AllowedGroup(t *testing.T) {
	agent := &models.Agent{
		ID:            "test-agent",
		AllowedUsers:  []string{},
		AllowedGroups: []string{"google-workspace/sre@company.com"},
	}

	// User with an allowed group
	assert.True(t, HasAccess(agent, "user@example.com", []string{"google-workspace/sre@company.com"}))

	// User without an allowed group
	assert.False(t, HasAccess(agent, "user@example.com", []string{"google-workspace/other@company.com"}))
}

func TestHasAccess_CaseInsensitive(t *testing.T) {
	agent := &models.Agent{
		ID:            "test-agent",
		AllowedUsers:  []string{"User@Example.COM"},
		AllowedGroups: []string{"Google-Workspace/SRE@Company.com"},
	}

	// User - case insensitive
	assert.True(t, HasAccess(agent, "user@example.com", nil))

	// Group - case insensitive
	assert.True(t, HasAccess(agent, "other@example.com", []string{"google-workspace/sre@company.com"}))
}

func TestHasAccess_EmptyLists(t *testing.T) {
	agent := &models.Agent{
		ID:            "test-agent",
		AllowedUsers:  []string{},
		AllowedGroups: []string{},
	}

	// No restrictions defined -> deny (secure by default)
	assert.False(t, HasAccess(agent, "user@example.com", []string{"some-group"}))
}

func TestHasAccess_UserOrGroup(t *testing.T) {
	agent := &models.Agent{
		ID:            "test-agent",
		AllowedUsers:  []string{"admin@example.com"},
		AllowedGroups: []string{"google-workspace/team@company.com"},
	}

	// Access by user
	assert.True(t, HasAccess(agent, "admin@example.com", nil))

	// Access by group
	assert.True(t, HasAccess(agent, "other@example.com", []string{"google-workspace/team@company.com"}))

	// No access
	assert.False(t, HasAccess(agent, "other@example.com", []string{"google-workspace/other@company.com"}))
}

func TestFilterAgentsByAccess(t *testing.T) {
	agents := []*models.Agent{
		{
			ID:            "agent-1",
			AllowedUsers:  []string{"user@example.com"},
			AllowedGroups: []string{},
		},
		{
			ID:            "agent-2",
			AllowedUsers:  []string{},
			AllowedGroups: []string{"google-workspace/sre@company.com"},
		},
		{
			ID:            "agent-3",
			AllowedUsers:  []string{"other@example.com"},
			AllowedGroups: []string{},
		},
	}

	// User with access to agent-1 and agent-2
	filtered := FilterAgentsByAccess(agents, "user@example.com", []string{"google-workspace/sre@company.com"})

	assert.Len(t, filtered, 2)
	assert.Equal(t, "agent-1", filtered[0].ID)
	assert.Equal(t, "agent-2", filtered[1].ID)
}

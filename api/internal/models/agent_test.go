package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAgent_ToResponse(t *testing.T) {
	agent := &Agent{
		ID:          "test-agent",
		Name:        "Test Agent",
		Description: "A test agent",
		Category:    "test",
		Protocol:    "custom",
		Endpoint:    "http://localhost:9000/chat",
		Headers: map[string]string{
			"X-Api-Key": "secret-key",
		},
		AllowedGroups: []string{"admin"},
		AllowedUsers:  []string{"admin@example.com"},
		Status:        "healthy",
	}

	response := agent.ToResponse()

	// Should include public fields
	assert.Equal(t, "test-agent", response.ID)
	assert.Equal(t, "Test Agent", response.Name)
	assert.Equal(t, "A test agent", response.Description)
	assert.Equal(t, "test", response.Category)
	assert.Equal(t, "custom", response.Protocol)
	assert.Equal(t, "healthy", response.Status)
}

func TestAgent_GetAuthType(t *testing.T) {
	tests := []struct {
		name                 string
		authType             string
		forwardAuthorization bool
		want                 string
	}{
		{"explicit none", AgentAuthNone, false, AgentAuthNone},
		{"explicit forward", AgentAuthForward, false, AgentAuthForward},
		{"explicit bearer", AgentAuthBearer, false, AgentAuthBearer},
		{"empty without legacy flag defaults to none", "", false, AgentAuthNone},
		{"empty with legacy forward flag falls back to forward", "", true, AgentAuthForward},
		{"explicit auth_type wins over legacy flag", AgentAuthBearer, true, AgentAuthBearer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &Agent{AuthType: tt.authType, ForwardAuthorization: tt.forwardAuthorization}
			assert.Equal(t, tt.want, agent.GetAuthType())
		})
	}
}

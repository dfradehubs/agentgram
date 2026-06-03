package agents

import (
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_LoadAgents(t *testing.T) {
	agents := []models.Agent{
		{ID: "agent-1", Name: "Agent 1", Protocol: "custom", Endpoint: "http://localhost:9000"},
		{ID: "agent-2", Name: "Agent 2", Protocol: "a2a", Endpoint: "http://localhost:9001"},
	}

	registry := NewRegistry()
	err := registry.LoadAgents(agents)

	require.NoError(t, err)
	assert.Equal(t, 2, registry.Count())
}

func TestRegistry_Get(t *testing.T) {
	agents := []models.Agent{
		{ID: "agent-1", Name: "Agent 1", Protocol: "custom", Endpoint: "http://localhost:9000"},
	}

	registry := NewRegistry()
	registry.LoadAgents(agents)

	// Existing agent
	agent, err := registry.Get("agent-1")
	require.NoError(t, err)
	assert.Equal(t, "Agent 1", agent.Name)

	// Non-existing agent
	_, err = registry.Get("agent-unknown")
	assert.Error(t, err)
}

func TestRegistry_List(t *testing.T) {
	agents := []models.Agent{
		{ID: "agent-1", Name: "Agent 1", Protocol: "custom", Endpoint: "http://localhost:9000"},
		{ID: "agent-2", Name: "Agent 2", Protocol: "custom", Endpoint: "http://localhost:9001"},
	}

	registry := NewRegistry()
	registry.LoadAgents(agents)

	list := registry.List()
	assert.Len(t, list, 2)
}

func TestRegistry_UpdateStatus(t *testing.T) {
	agents := []models.Agent{
		{ID: "agent-1", Name: "Agent 1", Protocol: "custom", Endpoint: "http://localhost:9000"},
	}

	registry := NewRegistry()
	registry.LoadAgents(agents)

	// Update status
	err := registry.UpdateStatus("agent-1", "healthy")
	require.NoError(t, err)

	// Verify
	agent, _ := registry.Get("agent-1")
	assert.Equal(t, "healthy", agent.Status)

	// Error if not exists
	err = registry.UpdateStatus("agent-unknown", "healthy")
	assert.Error(t, err)
}

func TestRegistry_GetHealthyAgents(t *testing.T) {
	agents := []models.Agent{
		{ID: "agent-1", Name: "Agent 1", Protocol: "custom", Endpoint: "http://localhost:9000"},
		{ID: "agent-2", Name: "Agent 2", Protocol: "custom", Endpoint: "http://localhost:9001"},
	}

	registry := NewRegistry()
	registry.LoadAgents(agents)

	registry.UpdateStatus("agent-1", "healthy")
	registry.UpdateStatus("agent-2", "unhealthy")

	healthy := registry.GetHealthyAgents()
	assert.Len(t, healthy, 1)
	assert.Equal(t, "agent-1", healthy[0].ID)
}

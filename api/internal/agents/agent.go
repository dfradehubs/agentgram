package agents

import (
	"sync"

	"github.com/dfradehubs/agentgram-api/internal/models"
)

// AgentWrapper wraps an agent with runtime data
type AgentWrapper struct {
	Agent *models.Agent
	mu    sync.RWMutex
}

// NewAgentWrapper creates a new agent wrapper
func NewAgentWrapper(agent *models.Agent) *AgentWrapper {
	return &AgentWrapper{
		Agent: agent,
	}
}

// GetStatus gets the agent status in a thread-safe way
func (w *AgentWrapper) GetStatus() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.Agent.Status
}

// SetStatus sets the agent status in a thread-safe way
func (w *AgentWrapper) SetStatus(status string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Agent.Status = status
}

// IsHealthy checks if the agent is healthy
func (w *AgentWrapper) IsHealthy() bool {
	return w.GetStatus() == "healthy"
}

// GetAgent gets a copy of the agent
func (w *AgentWrapper) GetAgent() models.Agent {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return *w.Agent
}

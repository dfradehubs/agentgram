package agents

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

// Registry manages the agent registry with an in-memory cache
type Registry struct {
	agents    map[string]*AgentWrapper
	order     []string // preserves insertion order
	mu        sync.RWMutex
	agentRepo repository.AgentRepository // nil when running without DB
	logger    *zap.Logger
	stopCh    chan struct{}
}

// NewRegistry creates a new agent registry (without DB backing)
func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[string]*AgentWrapper),
	}
}

// NewDBRegistry creates a new DB-backed agent registry
func NewDBRegistry(agentRepo repository.AgentRepository, logger *zap.Logger) *Registry {
	return &Registry{
		agents:    make(map[string]*AgentWrapper),
		agentRepo: agentRepo,
		logger:    logger,
		stopCh:    make(chan struct{}),
	}
}

// LoadAgents loads agents into the registry preserving config order (legacy/non-DB path)
func (r *Registry) LoadAgents(agents []models.Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.agents = make(map[string]*AgentWrapper)
	r.order = make([]string, 0, len(agents))

	for i := range agents {
		agent := &agents[i]
		applyAgentDefaults(agent)
		r.agents[agent.ID] = NewAgentWrapper(agent)
		r.order = append(r.order, agent.ID)
	}

	return nil
}

// LoadFromDB loads agents from the database into the in-memory cache
func (r *Registry) LoadFromDB(ctx context.Context) error {
	if r.agentRepo == nil {
		return fmt.Errorf("no agent repository configured")
	}

	agents, err := r.agentRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("load agents from DB: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Preserve existing health statuses
	oldStatuses := make(map[string]string)
	for id, w := range r.agents {
		oldStatuses[id] = w.GetStatus()
	}

	r.agents = make(map[string]*AgentWrapper)
	r.order = make([]string, 0, len(agents))

	for _, agent := range agents {
		wrapper := NewAgentWrapper(agent)
		// Restore health status if agent existed before
		if status, ok := oldStatuses[agent.ID]; ok {
			wrapper.SetStatus(status)
		}
		r.agents[agent.ID] = wrapper
		r.order = append(r.order, agent.ID)
	}

	return nil
}

// Refresh reloads agents from DB (called after admin CRUD operations)
func (r *Registry) Refresh(ctx context.Context) error {
	return r.LoadFromDB(ctx)
}

// Get retrieves an agent by ID
func (r *Registry) Get(id string) (*models.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	wrapper, exists := r.agents[id]
	if !exists {
		return nil, fmt.Errorf("agent not found: %s", id)
	}

	agent := wrapper.GetAgent()
	return &agent, nil
}

// GetWrapper retrieves an agent wrapper by ID
func (r *Registry) GetWrapper(id string) (*AgentWrapper, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	wrapper, exists := r.agents[id]
	if !exists {
		return nil, fmt.Errorf("agent not found: %s", id)
	}

	return wrapper, nil
}

// List returns all agents in config order
func (r *Registry) List() []*models.Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*models.Agent, 0, len(r.order))
	for _, id := range r.order {
		if wrapper, ok := r.agents[id]; ok {
			agent := wrapper.GetAgent()
			agents = append(agents, &agent)
		}
	}

	return agents
}

// Count returns the number of registered agents
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}

// UpdateStatus updates an agent's status
func (r *Registry) UpdateStatus(id, status string) error {
	r.mu.RLock()
	wrapper, exists := r.agents[id]
	r.mu.RUnlock()

	if !exists {
		return fmt.Errorf("agent not found: %s", id)
	}

	wrapper.SetStatus(status)
	return nil
}

// GetHealthyAgents returns only healthy agents
func (r *Registry) GetHealthyAgents() []*models.Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*models.Agent, 0)
	for _, wrapper := range r.agents {
		if wrapper.IsHealthy() {
			agent := wrapper.GetAgent()
			agents = append(agents, &agent)
		}
	}

	return agents
}

// AllHealthy checks if all agents are healthy
func (r *Registry) AllHealthy() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, wrapper := range r.agents {
		if !wrapper.IsHealthy() {
			return false
		}
	}

	return true
}

// StartAutoRefresh starts a goroutine that reloads agents from the DB
// every 30 seconds. This ensures all pods in a multi-replica deployment
// pick up config changes made via the admin API on other pods.
func (r *Registry) StartAutoRefresh() {
	if r.agentRepo == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.stopCh:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := r.LoadFromDB(ctx); err != nil {
					r.logger.Error("auto-refresh agents failed", zap.Error(err))
				}
				cancel()
			}
		}
	}()
}

// StopAutoRefresh stops the auto-refresh goroutine.
func (r *Registry) StopAutoRefresh() {
	select {
	case <-r.stopCh:
		// already closed
	default:
		close(r.stopCh)
	}
}

// applyAgentDefaults fills zero-value fields with sensible defaults.
// Used for YAML-loaded agents that may omit optional fields.
// DB-loaded agents already have defaults from the SQL schema.
func applyAgentDefaults(agent *models.Agent) {
	if agent.MaxContextTokens == 0 {
		agent.MaxContextTokens = 200000
	}
	if agent.SummarizeThreshold == 0 {
		agent.SummarizeThreshold = 0.8
	}
}

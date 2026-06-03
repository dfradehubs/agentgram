package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/models"
)

// HealthHandler handles health endpoints
type HealthHandler struct {
	registry *agents.Registry
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(registry *agents.Registry) *HealthHandler {
	return &HealthHandler{
		registry: registry,
	}
}

// Liveness handles GET /health (liveness probe)
// @Summary Liveness probe
// @Description Returns ok if the service is running
// @Tags health
// @Produce json
// @Success 200 {object} models.HealthResponse
// @Router /health [get]
func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(models.HealthResponse{
		Status: "ok",
	})
}

// Readiness handles GET /health/ready (readiness probe)
// @Summary Readiness probe
// @Description Returns ready if at least one agent is healthy. Returns 503 if no agents are loaded or all are unhealthy.
// @Tags health
// @Produce json
// @Success 200 {object} models.HealthResponse
// @Failure 503 {object} models.HealthResponse
// @Router /health/ready [get]
func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Verify that we have agents loaded
	agentCount := h.registry.Count()
	if agentCount == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(models.HealthResponse{
			Status: "not ready",
			Details: map[string]interface{}{
				"reason": "no agents loaded",
			},
		})
		return
	}

	// Get agent status
	allAgents := h.registry.List()
	healthyCount := 0
	unhealthyAgents := []string{}

	for _, agent := range allAgents {
		if agent.Status == "healthy" {
			healthyCount++
		} else {
			unhealthyAgents = append(unhealthyAgents, agent.ID)
		}
	}

	details := map[string]interface{}{
		"total_agents":   agentCount,
		"healthy_agents": healthyCount,
	}

	// Consider "ready" if at least one agent is healthy
	// or if none have health check enabled (all in "unknown" status)
	allUnknown := true
	for _, agent := range allAgents {
		if agent.Status != "unknown" {
			allUnknown = false
			break
		}
	}

	if healthyCount > 0 || allUnknown {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(models.HealthResponse{
			Status:  "ready",
			Details: details,
		})
		return
	}

	// No healthy agents
	details["unhealthy_agents"] = unhealthyAgents
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(models.HealthResponse{
		Status:  "degraded",
		Details: details,
	})
}

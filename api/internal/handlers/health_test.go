package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthHandler_Liveness(t *testing.T) {
	registry := agents.NewRegistry()
	handler := NewHealthHandler(registry)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.Liveness(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Status)
}

func TestHealthHandler_Readiness_NoAgents(t *testing.T) {
	registry := agents.NewRegistry()
	handler := NewHealthHandler(registry)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	handler.Readiness(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp models.HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "not ready", resp.Status)
}

func TestHealthHandler_Readiness_WithAgents(t *testing.T) {
	agentsList := []models.Agent{
		{ID: "agent-1", Name: "Agent 1", Protocol: "custom", Endpoint: "http://localhost:9000"},
	}

	registry := agents.NewRegistry()
	registry.LoadAgents(agentsList)
	registry.UpdateStatus("agent-1", "healthy")

	handler := NewHealthHandler(registry)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	handler.Readiness(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "ready", resp.Status)
}

func TestHealthHandler_Readiness_Degraded(t *testing.T) {
	agentsList := []models.Agent{
		{ID: "agent-1", Name: "Agent 1", Protocol: "custom", Endpoint: "http://localhost:9000"},
	}

	registry := agents.NewRegistry()
	registry.LoadAgents(agentsList)
	registry.UpdateStatus("agent-1", "unhealthy")

	handler := NewHealthHandler(registry)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	handler.Readiness(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp models.HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "degraded", resp.Status)
}

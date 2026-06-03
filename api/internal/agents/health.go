package agents

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/metrics"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"go.uber.org/zap"
)

// HealthChecker periodically checks agent health
type HealthChecker struct {
	registry   *Registry
	httpClient *http.Client
	logger     *zap.Logger

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(registry *Registry, logger *zap.Logger) *HealthChecker {
	return &HealthChecker{
		registry: registry,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

// Start starts the health checker
func (h *HealthChecker) Start() {
	h.logger.Info("starting health checker")

	// Check initial health of all agents
	h.checkAll()

	// Start goroutines for each agent with its own interval
	agents := h.registry.List()
	for _, agent := range agents {
		if agent.HealthCheck != nil && agent.HealthCheck.Enabled {
			h.wg.Add(1)
			go h.watchAgent(agent.ID, agent.Endpoint, agent.HealthCheck)
		}
	}
}

// Stop stops the health checker
func (h *HealthChecker) Stop() {
	h.logger.Info("stopping health checker")
	close(h.stopCh)
	h.wg.Wait()
}

// checkAll checks the health of all agents
func (h *HealthChecker) checkAll() {
	agents := h.registry.List()
	for _, agent := range agents {
		if agent.HealthCheck != nil && agent.HealthCheck.Enabled {
			status := h.checkAgent(agent.Endpoint, agent.HealthCheck)
			_ = h.registry.UpdateStatus(agent.ID, status)
			reportHealthMetric(agent.ID, status)
		}
	}
}

// watchAgent monitors a specific agent
func (h *HealthChecker) watchAgent(agentID, endpoint string, config *models.HealthCheckConfig) {
	defer h.wg.Done()

	intervalSec := config.IntervalSeconds
	if intervalSec <= 0 {
		intervalSec = 30
	}
	interval := time.Duration(intervalSec) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopCh:
			return
		case <-ticker.C:
			status := h.checkAgent(endpoint, config)
			if err := h.registry.UpdateStatus(agentID, status); err != nil {
				h.logger.Error("failed to update agent status",
					zap.String("agent_id", agentID),
					zap.Error(err))
			}
			reportHealthMetric(agentID, status)
		}
	}
}

// checkAgent checks an agent's health
func (h *HealthChecker) checkAgent(endpoint string, config *models.HealthCheckConfig) string {
	timeoutSec := config.TimeoutSeconds
	if timeoutSec <= 0 {
		timeoutSec = 5
	}
	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(timeoutSec)*time.Second)
	defer cancel()

	// Use explicit URL if provided, otherwise build from endpoint + path
	healthURL := config.URL
	if healthURL == "" {
		healthURL = endpoint + config.Endpoint
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		h.logger.Debug("failed to create health check request",
			zap.String("url", healthURL),
			zap.Error(err))
		return "unhealthy"
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.logger.Debug("health check failed",
			zap.String("url", healthURL),
			zap.Error(err))
		return "unhealthy"
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return "healthy"
	}

	h.logger.Debug("health check returned non-2xx status",
		zap.String("url", healthURL),
		zap.Int("status", resp.StatusCode))
	return "unhealthy"
}

// reportHealthMetric updates the Prometheus gauge for a specific agent.
func reportHealthMetric(agentID, status string) {
	if !metrics.IsEnabled() {
		return
	}
	val := float64(0)
	if status == "healthy" {
		val = 1
	}
	metrics.AgentHealthStatus.WithLabelValues(agentID).Set(val)
}

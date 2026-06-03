package handlers

import (
	"context"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/metrics"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

// recordChatEvent inserts a ChatEvent into the repository asynchronously.
// It also records Prometheus metrics if enabled.
func recordChatEvent(repo repository.ChatEventRepository, event *models.ChatEvent, logger *zap.Logger) {
	if !metrics.IsEnabled() && repo == nil {
		return
	}

	resourceType := event.ResourceType
	resourceID := event.ResourceID

	// Prometheus metrics
	if metrics.IsEnabled() {
		metrics.ChatRequestsTotal.WithLabelValues(resourceType, resourceID, event.Status).Inc()
		metrics.ChatDurationSeconds.WithLabelValues(resourceType, resourceID).Observe(float64(event.DurationMs) / 1000.0)

		if event.TTFBMs != nil {
			metrics.ChatTTFBSeconds.WithLabelValues(resourceType, resourceID).Observe(float64(*event.TTFBMs) / 1000.0)
		}

		if event.Status == "error" && event.ErrorType != "" {
			metrics.ChatErrorsTotal.WithLabelValues(resourceType, resourceID, event.ErrorType).Inc()
		}

		for _, tc := range event.ToolCalls {
			metrics.ToolCallsTotal.WithLabelValues(resourceID, tc.Name).Inc()
		}

		if event.SessionRotated {
			metrics.ContextRotationsTotal.WithLabelValues(resourceID).Inc()
		}

		if event.TokenUsage != nil {
			if event.TokenUsage.Input > 0 {
				metrics.TokenUsageTotal.WithLabelValues(resourceID, "input").Add(float64(event.TokenUsage.Input))
			}
			if event.TokenUsage.Output > 0 {
				metrics.TokenUsageTotal.WithLabelValues(resourceID, "output").Add(float64(event.TokenUsage.Output))
			}
		}
	}

	// Async DB insert
	if repo != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := repo.Insert(ctx, event); err != nil && logger != nil {
				logger.Warn("failed to insert chat_event", zap.Error(err))
			}
		}()
	}
}

// classifyError returns an error_type string for chat_events based on the error message.
func classifyError(errMsg string) string {
	if errMsg == "" {
		return ""
	}
	switch {
	case contains(errMsg, "connect", "connection", "dial"):
		return "connection"
	case contains(errMsg, "status 4", "status 5"):
		return "http_status"
	case contains(errMsg, "stream"):
		return "stream"
	case contains(errMsg, "rpc", "json-rpc"):
		return "rpc"
	case contains(errMsg, "agent error", "task failed", "task rejected"):
		return "agent_error"
	case contains(errMsg, "llm", "model", "no chat models"):
		return "llm_error"
	case contains(errMsg, "tool"):
		return "tool_error"
	default:
		return "unknown"
	}
}

func contains(s string, substrs ...string) bool {
	lower := toLower(s)
	for _, sub := range substrs {
		if containsStr(lower, sub) {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func containsStr(s, substr string) bool {
	return len(substr) <= len(s) && (s == substr || len(s) > 0 && findSubstr(s, substr))
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

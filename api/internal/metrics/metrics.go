package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	enabled bool

	ChatRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agentgram_chat_requests_total",
			Help: "Total number of chat requests",
		},
		[]string{"resource_type", "resource_id", "status"},
	)

	ChatDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "agentgram_chat_duration_seconds",
			Help:    "Chat request duration in seconds",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300},
		},
		[]string{"resource_type", "resource_id"},
	)

	ChatTTFBSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "agentgram_chat_ttfb_seconds",
			Help:    "Time to first byte in seconds",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
		},
		[]string{"resource_type", "resource_id"},
	)

	ActiveStreams = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "agentgram_active_streams",
			Help: "Number of currently active SSE streams",
		},
		[]string{"resource_type", "resource_id"},
	)

	ChatErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agentgram_chat_errors_total",
			Help: "Total number of chat errors by type",
		},
		[]string{"resource_type", "resource_id", "error_type"},
	)

	ToolCallsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agentgram_tool_calls_total",
			Help: "Total number of tool calls",
		},
		[]string{"resource_id", "tool_name"},
	)

	ContextRotationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agentgram_context_rotations_total",
			Help: "Total number of ADK context rotations",
		},
		[]string{"resource_id"},
	)

	RateLimitRejectedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agentgram_rate_limit_rejected_total",
			Help: "Total number of rate-limited requests",
		},
		[]string{"resource_id"},
	)

	AgentHealthStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "agentgram_agent_health_status",
			Help: "Agent health status (1=healthy, 0=unhealthy)",
		},
		[]string{"agent_id"},
	)

	TokenUsageTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agentgram_token_usage_total",
			Help: "Total token usage",
		},
		[]string{"resource_id", "token_type"},
	)

	SharesCreatedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agentgram_shares_created_total",
			Help: "Total number of shared sessions created",
		},
		[]string{"resource_id"},
	)

	SharesAccessedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agentgram_shares_accessed_total",
			Help: "Total number of shared sessions accessed",
		},
		[]string{"resource_id"},
	)

	GroupsCreatedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "agentgram_groups_created_total",
			Help: "Total number of groups created",
		},
	)

	GroupsDeletedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "agentgram_groups_deleted_total",
			Help: "Total number of groups deleted",
		},
	)

	SessionsClonesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agentgram_sessions_clones_total",
			Help: "Total number of sessions cloned from shares",
		},
		[]string{"resource_id"},
	)
)

// Init registers all Prometheus metrics if metrics are enabled.
func Init(metricsEnabled bool) {
	enabled = metricsEnabled
	if !enabled {
		return
	}

	prometheus.MustRegister(
		ChatRequestsTotal,
		ChatDurationSeconds,
		ChatTTFBSeconds,
		ActiveStreams,
		ChatErrorsTotal,
		ToolCallsTotal,
		ContextRotationsTotal,
		RateLimitRejectedTotal,
		AgentHealthStatus,
		TokenUsageTotal,
		SharesCreatedTotal,
		SharesAccessedTotal,
		GroupsCreatedTotal,
		GroupsDeletedTotal,
		SessionsClonesTotal,
	)
}

// IsEnabled returns whether metrics collection is enabled.
func IsEnabled() bool {
	return enabled
}

// Handler returns the Prometheus HTTP handler for /metrics scraping.
func Handler() http.Handler {
	return promhttp.Handler()
}

package slack

import "github.com/prometheus/client_golang/prometheus"

var (
	SlackEventsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agentgram_slack_events_total",
			Help: "Slack events received",
		},
		[]string{"agent_id", "event_type"},
	)

	SlackMessagesForwardedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agentgram_slack_messages_forwarded_total",
			Help: "Messages forwarded to agents from Slack",
		},
		[]string{"agent_id", "status"},
	)

	SlackAPICallsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agentgram_slack_api_calls_total",
			Help: "Slack API calls made",
		},
		[]string{"agent_id", "method"},
	)

	SlackBotsActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "agentgram_slack_bots_active",
			Help: "Number of active Slack bot connections",
		},
	)
)

// RegisterMetrics registers Slack-specific Prometheus metrics.
func RegisterMetrics() {
	prometheus.MustRegister(
		SlackEventsTotal,
		SlackMessagesForwardedTotal,
		SlackAPICallsTotal,
		SlackBotsActive,
	)
}

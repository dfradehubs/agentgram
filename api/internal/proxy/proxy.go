package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// CapturedToolCall holds a tool call captured during SSE streaming
type CapturedToolCall struct {
	ID     string
	Name   string
	Args   string
	Result string
}

// ProxyResult contains the result of proxying a request to an agent
type ProxyResult struct {
	AssistantText  string             // Full accumulated assistant response text
	AgentSessionID string             // Session ID returned by the agent (if any)
	ToolCalls      []CapturedToolCall // Tool calls captured during streaming
	ContentParts   []ContentPart      // Ordered text/tool interleaving for reconstruction
	SessionRotated bool               // true if session was rotated due to context-limit
	Error          string             // Non-empty when the stream ended with an error (partial response)
}

// ContentPart represents an ordered segment in the streaming response
type ContentPart struct {
	Type      string                 // "text", "tool_use", or "chart"
	Text      string                 // For "text" parts
	ToolIndex int                    // For "tool_use", index into ToolCalls
	Chart     map[string]interface{} // For "chart" parts
}

// extractAgentErrorDetail builds a client-facing error from an agent HTTP error.
// It appends the first string value found in a JSON object, or the raw body
// (truncated) if parsing fails. This is agnostic to the agent's error schema.
func extractAgentErrorDetail(statusCode int, body []byte) string {
	base := fmt.Sprintf("agent returned status %d", statusCode)
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return base
	}
	// Try to extract the first string value from a JSON object
	var parsed map[string]json.RawMessage
	if json.Unmarshal(body, &parsed) == nil {
		for _, raw := range parsed {
			var s string
			if json.Unmarshal(raw, &s) == nil && s != "" {
				return fmt.Sprintf("%s: %s", base, s)
			}
		}
	}
	// Fallback: use raw body, truncated
	if len(trimmed) > 200 {
		trimmed = trimmed[:200] + "..."
	}
	return fmt.Sprintf("%s: %s", base, trimmed)
}

// Proxy is the main multiplexer that routes requests to agents
type Proxy struct {
	restProxy *RESTProxy
	a2aProxy  *A2AProxy
	adkProxy  *ADKProxy
	logger    *zap.Logger
}

// NewProxy creates a new multiplexer proxy
func NewProxy(logger *zap.Logger) *Proxy {
	return &Proxy{
		restProxy: NewRESTProxy(logger),
		a2aProxy:  NewA2AProxy(logger),
		adkProxy:  NewADKProxy(logger),
		logger:    logger,
	}
}

// HandleOptions configures optional parameters for Handle.
type HandleOptions struct {
	ThreadID          string
	SessionName       string                  // Session display name (sent in RUN_STARTED)
	Locale            string                  // "es", "en", etc. for localized messages
	RequestID         string                  // X-Request-ID for end-to-end correlation
	UserEmail         string                  // Calling user's email, for per-user outbound auth (bearer rules)
	UserGroups        []string                // Calling user's groups, for per-group outbound auth (bearer rules)
	OnEvent           func(event interface{}) // Called for each AG-UI event (for Pub/Sub broadcast)
	AgentID           string                  // Tag all stream events with this agent id (group debates)
	SuppressLifecycle bool                    // Skip this call's RUN_STARTED/RUN_FINISHED (outer run owns the lifecycle)
	AgentTimeout      time.Duration           // Max time to wait for the agent response; 0 = default (10m)
}

// defaultAgentTimeout bounds an agent call when the caller sets no AgentTimeout.
const defaultAgentTimeout = 10 * time.Minute

// resolveAgentTimeout returns the effective agent timeout for a call.
func resolveAgentTimeout(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return defaultAgentTimeout
}

// newDetachedAgentContext returns a context detached from the caller's
// cancellation (so the agent stream survives a client disconnect) but carrying
// the caller's trace span, GitHub token AND identity claims — the latter drive
// the outbound X-User-Email / X-User-Groups headers (identity.SetHeaders), so
// downstream agents receive the acting user's identity/tenant. Bounded by the
// resolved agent timeout.
func newDetachedAgentContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	deadline := time.Now().Add(resolveAgentTimeout(timeout))
	// Respect an earlier deadline already on the incoming ctx (e.g. the MCP
	// debate's absolute deadline): time spent before this call — resolving the
	// session, store ops — counts against the budget, and the timeout can't be
	// silently restarted from scratch.
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	c, cancel := context.WithDeadline(context.Background(), deadline)
	c = trace.ContextWithSpan(c, trace.SpanFromContext(ctx))
	if tok := middleware.GetGitHubTokenFromContext(ctx); tok != "" {
		c = context.WithValue(c, middleware.GitHubTokenContextKey, tok)
	}
	if claims := middleware.GetUserFromContext(ctx); claims != nil {
		c = context.WithValue(c, middleware.UserContextKey, claims)
	}
	return c, cancel
}

// Handle handles a request and routes it to the corresponding protocol.
func (p *Proxy) Handle(ctx context.Context, w http.ResponseWriter, agent *models.Agent, chatReq *models.ChatRequest, authHeader string, opts HandleOptions) (*ProxyResult, error) {
	ctx, span := tracing.Tracer().Start(ctx, "proxy."+agent.Protocol,
		trace.WithAttributes(
			attribute.String("agent.id", agent.ID),
			attribute.String("agent.endpoint", agent.Endpoint),
		),
	)
	defer span.End()

	p.logger.Debug("handling request",
		zap.String("agent_id", agent.ID),
		zap.String("protocol", agent.Protocol))

	// Resolve the outbound credential once for the whole call: the agent's
	// auth method (forward/bearer/none) plus the user's identity decide what
	// header — if any — is sent to the agent.
	auth := agents.ResolveOutboundAuth(agent, opts.UserEmail, opts.UserGroups, authHeader)

	sseCfg := SSEConfig{
		ThreadID:          opts.ThreadID,
		SessionName:       opts.SessionName,
		AgentID:           opts.AgentID,
		SuppressLifecycle: opts.SuppressLifecycle,
		OnEvent:           opts.OnEvent,
	}

	agentTimeout := resolveAgentTimeout(opts.AgentTimeout)

	switch agent.Protocol {
	case "custom":
		body, err := FormatRequestBody(agent, chatReq)
		if err != nil {
			return nil, err
		}
		return p.restProxy.Handle(ctx, w, agent, body, auth, opts.RequestID, sseCfg, agentTimeout)

	case "a2a":
		return p.a2aProxy.Handle(ctx, w, agent, chatReq, auth, opts.RequestID, sseCfg, agentTimeout)

	case "adk":
		return p.adkProxy.Handle(ctx, w, agent, chatReq, auth, opts.RequestID, opts.Locale, sseCfg, agentTimeout)

	default:
		return nil, fmt.Errorf("unknown protocol: %s", agent.Protocol)
	}
}

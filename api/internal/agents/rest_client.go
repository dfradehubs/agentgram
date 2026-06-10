package agents

import (
	"context"
	"io"
	"net/http"

	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/security"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// RESTClient HTTP client for REST agents
type RESTClient struct {
	httpClient *http.Client
}

// NewRESTClient creates a new REST client with SSRF-safe transport
func NewRESTClient() *RESTClient {
	return &RESTClient{
		httpClient: &http.Client{
			// No global Timeout: it includes body read time, which kills long SSE streams.
			// Use transport-level timeouts for connection and TLS handshake instead.
			// Uses SSRF-safe transport that rejects connections to private IPs.
			Transport: otelhttp.NewTransport(security.NewSafeTransport()),
		},
	}
}

// Request makes a request to the agent
func (c *RESTClient) Request(ctx context.Context, agent *models.Agent, body io.Reader, auth OutboundAuth, requestID string) (*http.Response, error) {
	// Use custom method if configured, default to POST
	method := http.MethodPost
	if agent.CustomFormat != nil && agent.CustomFormat.RequestMethod != "" {
		method = agent.CustomFormat.RequestMethod
	}

	req, err := http.NewRequestWithContext(ctx, method, agent.Endpoint, body)
	if err != nil {
		return nil, err
	}

	// Set Content-Type (custom or default application/json)
	contentType := "application/json"
	if agent.CustomFormat != nil && agent.CustomFormat.RequestContentType != "" {
		contentType = agent.CustomFormat.RequestContentType
	}
	req.Header.Set("Content-Type", contentType)

	// Pass agent ID so agents can scope sessions
	req.Header.Set("X-Agent-ID", agent.ID)

	// Propagate request ID for end-to-end tracing
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}

	// Add configured agent headers (filtered for SSRF safety)
	for key, value := range security.FilterHeaders(agent.Headers) {
		req.Header.Set(key, value)
	}

	// Forward Authorization if configured
	if auth.HeaderValue != "" {
		req.Header.Set(auth.HeaderName, auth.HeaderValue)
	}

	// Forward GitHub token only to agents that explicitly require it
	if agent.RequireGitHubToken {
		if githubToken := middleware.GetGitHubTokenFromContext(ctx); githubToken != "" {
			req.Header.Set("X-GitHub-Token", githubToken)
		}
	}

	return c.httpClient.Do(req)
}

// RequestWithHeaders makes a request with additional headers
func (c *RESTClient) RequestWithHeaders(ctx context.Context, url string, body io.Reader, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	for key, value := range security.FilterHeaders(headers) {
		req.Header.Set(key, value)
	}

	return c.httpClient.Do(req)
}

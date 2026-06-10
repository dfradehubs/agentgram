package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dfradehubs/agentgram-api/internal/adk"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/security"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// ADKClient client for ADK REST SSE agents
type ADKClient struct {
	httpClient *http.Client
	logger     *zap.Logger
}

// NewADKClient creates an ADK client with an OTel-instrumented transport.
// Uses otelhttp like REST and A2A clients — plain transports cause premature
// stream closure (~30s) likely due to Go's automatic HTTP/2 negotiation
// interfering with long-lived SSE connections.
func NewADKClient(logger *zap.Logger) *ADKClient {
	return &ADKClient{
		httpClient: &http.Client{
			// No global Timeout: it includes body read time, which kills long SSE streams.
			// Uses SSRF-safe transport that rejects connections to private IPs.
			Transport: otelhttp.NewTransport(security.NewSafeTransport()),
		},
		logger: logger.Named("adk-client"),
	}
}

// adkSessionResponse is the response from the ADK create session endpoint
type adkSessionResponse struct {
	ID      string `json:"id"`
	AppName string `json:"appName"`
	UserID  string `json:"userId"`
}

// resolveADKParams resolves appName and userID with defaults
func resolveADKParams(agent *models.Agent, userID string) (string, string) {
	appName := agent.ADKAppName
	if appName == "" {
		appName = agent.ID
	}
	if userID == "" {
		userID = agent.ADKUserID
	}
	if userID == "" {
		userID = "agentgram"
	}
	return appName, userID
}

// baseURL derives the ADK REST base URL from the run_sse endpoint.
// e.g. "http://host:8080/agent/run_sse" -> "http://host:8080/agent"
func baseURL(endpoint string) string {
	return strings.TrimSuffix(endpoint, "/run_sse")
}

// CreateSession creates a new session on the ADK agent.
// POST /apps/{appName}/users/{userId}/sessions
func (c *ADKClient) CreateSession(ctx context.Context, agent *models.Agent, userID string, auth OutboundAuth, requestID string) (string, error) {
	appName, userID := resolveADKParams(agent, userID)

	url := fmt.Sprintf("%s/apps/%s/users/%s/sessions", baseURL(agent.Endpoint), appName, userID)
	c.logger.Debug("ADK CreateSession", zap.String("url", url), zap.String("appName", appName), zap.String("userID", userID))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create session request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Propagate request ID for end-to-end tracing
	if requestID != "" {
		httpReq.Header.Set("X-Request-ID", requestID)
	}

	for key, value := range security.FilterHeaders(agent.Headers) {
		httpReq.Header.Set(key, value)
	}

	if auth.HeaderValue != "" {
		httpReq.Header.Set(auth.HeaderName, auth.HeaderValue)
	}

	// Forward GitHub token only to agents that explicitly require it
	if agent.RequireGitHubToken {
		if githubToken := middleware.GetGitHubTokenFromContext(ctx); githubToken != "" {
			httpReq.Header.Set("X-GitHub-Token", githubToken)
		}
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to create ADK session: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ADK create session returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	c.logger.Debug("ADK CreateSession response", zap.Int("status", resp.StatusCode), zap.String("body", string(bodyBytes)))

	var sessionResp adkSessionResponse
	if err := json.Unmarshal(bodyBytes, &sessionResp); err != nil {
		return "", fmt.Errorf("failed to decode session response: %w", err)
	}

	c.logger.Debug("ADK session created", zap.String("sessionID", sessionResp.ID))
	return sessionResp.ID, nil
}

// RunSSE sends a POST /run_sse request to an ADK agent and returns the raw
// HTTP response for SSE streaming. The caller is responsible for closing the response body.
// If sessionID is empty, a new session is created first.
// Attachments are sent as native ADK InlineData parts alongside the text.
// Returns the response and the sessionID used (which may be newly created).
func (c *ADKClient) RunSSE(ctx context.Context, agent *models.Agent, message string, sessionID string, userID string, auth OutboundAuth, requestID string, attachments []models.Attachment) (*http.Response, string, error) {
	appName, userID := resolveADKParams(agent, userID)

	// Create session if none provided
	if sessionID == "" {
		newSessionID, err := c.CreateSession(ctx, agent, userID, auth, requestID)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create session: %w", err)
		}
		sessionID = newSessionID
	}

	parts := []*adk.Part{{Text: message}}
	for _, att := range attachments {
		parts = append(parts, &adk.Part{
			InlineData: &adk.InlineData{MimeType: att.ContentType, Data: att.Data},
		})
	}

	req := adk.RunSSERequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
		NewMessage: adk.Content{
			Role:  "user",
			Parts: parts,
		},
		Streaming: true,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal ADK request: %w", err)
	}

	// Record the outgoing ADK request body as a span event
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		span.AddEvent("adk.request_body", trace.WithAttributes(
			attribute.String("body", string(body)),
		))
	}

	c.logger.Debug("ADK RunSSE", zap.String("url", agent.Endpoint), zap.String("sessionID", sessionID), zap.String("body", string(body)))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, agent.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create ADK request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	// Propagate request ID for end-to-end tracing
	if requestID != "" {
		httpReq.Header.Set("X-Request-ID", requestID)
	}

	for key, value := range security.FilterHeaders(agent.Headers) {
		httpReq.Header.Set(key, value)
	}

	if auth.HeaderValue != "" {
		httpReq.Header.Set(auth.HeaderName, auth.HeaderValue)
	}

	// Forward GitHub token only to agents that explicitly require it
	if agent.RequireGitHubToken {
		if githubToken := middleware.GetGitHubTokenFromContext(ctx); githubToken != "" {
			httpReq.Header.Set("X-GitHub-Token", githubToken)
		}
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, "", fmt.Errorf("failed to send ADK run_sse: %w", err)
	}

	c.logger.Debug("ADK RunSSE response", zap.Int("status", resp.StatusCode), zap.String("contentType", resp.Header.Get("Content-Type")))

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, "", fmt.Errorf("ADK agent returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return resp, sessionID, nil
}

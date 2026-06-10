package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/a2a"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/security"
	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// A2AClient client for A2A agents
type A2AClient struct {
	httpClient *http.Client
}

// NewA2AClient creates a new A2A client
func NewA2AClient() *A2AClient {
	return &A2AClient{
		httpClient: &http.Client{
			// No global Timeout: it includes body read time, which kills long SSE streams.
			// Uses SSRF-safe transport that rejects connections to private IPs.
			Transport: otelhttp.NewTransport(security.NewSafeTransport()),
		},
	}
}

// SendMessageStream sends a message/stream request to an A2A agent and returns
// the raw HTTP response for SSE streaming. The caller is responsible for closing the response body.
// Attachments are sent as native A2A file parts alongside the text.
func (c *A2AClient) SendMessageStream(ctx context.Context, agent *models.Agent, message string, contextID string, auth OutboundAuth, requestID string, attachments []models.Attachment) (*http.Response, error) {
	msgID := uuid.New().String()

	parts := []a2a.Part{{Kind: "text", Text: message}}
	for _, att := range attachments {
		parts = append(parts, a2a.Part{
			Kind: "file",
			File: &a2a.FileContent{Name: att.Filename, MimeType: att.ContentType, Bytes: att.Data},
		})
	}

	rpcReq := a2a.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "message/stream",
		Params: a2a.MessageSendParams{
			Message: a2a.Message{
				MessageID: msgID,
				Role:      "user",
				Parts:     parts,
				ContextID: contextID,
			},
		},
	}

	body, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Record the outgoing A2A request body as a span event
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		span.AddEvent("a2a.request_body", trace.WithAttributes(
			attribute.String("body", string(body)),
		))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, agent.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-Agent-ID", agent.ID)

	// Propagate request ID for end-to-end tracing
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}

	for key, value := range security.FilterHeaders(agent.Headers) {
		req.Header.Set(key, value)
	}

	if auth.HeaderValue != "" {
		req.Header.Set(auth.HeaderName, auth.HeaderValue)
	}

	// Forward GitHub token only to agents that explicitly require it
	if agent.RequireGitHubToken {
		if githubToken := middleware.GetGitHubTokenFromContext(ctx); githubToken != "" {
			req.Header.Set("X-GitHub-Token", githubToken)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send message/stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("agent returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
}

// FetchAgentCard fetches the agent-card.json
func (c *A2AClient) FetchAgentCard(ctx context.Context, agent *models.Agent) (*a2a.AgentCard, error) {
	cardURL := agent.Endpoint + agent.AgentCardPath

	httpClient := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cardURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, value := range security.FilterHeaders(agent.Headers) {
		req.Header.Set(key, value)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch agent card: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent card returned status %d", resp.StatusCode)
	}

	var card a2a.AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return nil, fmt.Errorf("failed to decode agent card: %w", err)
	}

	return &card, nil
}

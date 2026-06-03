package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/security"
	"go.uber.org/zap"
)

// Tool represents an MCP tool definition
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// ToolResult represents the result of a tool call
type ToolResult struct {
	Content []ToolResultContent `json:"content"`
	IsError bool                `json:"isError,omitempty"`
}

// ToolResultContent represents a single piece of content in a tool result
type ToolResultContent struct {
	Type string `json:"type"` // "text", "image", "resource"
	Text string `json:"text,omitempty"`
}

// Client is an MCP client that talks to remote MCP servers via HTTP Streamable transport
type Client struct {
	serverURL       string
	headers         map[string]string
	transport       string // "sse" or "http"
	toolCallTimeout time.Duration
	client          *http.Client
	logger          *zap.Logger
	sessionID       string // Mcp-Session-Id from server
	initialized     bool
	nextID          atomic.Int64
	mu              sync.Mutex
}

// NewClient creates a new MCP client.
// toolCallTimeout controls the max duration for a single tool call request.
// If zero, no per-call timeout is applied (context deadline from caller still applies).
func NewClient(serverURL, transport string, headers map[string]string, toolCallTimeout time.Duration, logger *zap.Logger) *Client {
	c := &Client{
		serverURL:       serverURL,
		headers:         headers,
		transport:       transport,
		toolCallTimeout: toolCallTimeout,
		client:          &http.Client{Transport: security.NewSafeTransport()},
		logger:          logger,
	}
	c.nextID.Store(0)
	return c
}

// InitializeWithHeaders performs the MCP protocol handshake with extra per-request headers
func (c *Client) InitializeWithHeaders(ctx context.Context, extraHeaders map[string]string) error {
	params := map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"clientInfo": map[string]string{
			"name":    "agentgram",
			"version": "1.0",
		},
		"capabilities": map[string]interface{}{},
	}

	resp, sessionID, err := c.jsonRPCRawWithHeaders(ctx, "initialize", params, extraHeaders)
	if err != nil {
		// If the server says it's already initialized, terminate the stale session
		// via DELETE and retry the initialize handshake.
		if strings.Contains(err.Error(), "already initialized") {
			c.logger.Info("MCP server already initialized, terminating stale session and retrying",
				zap.String("url", c.serverURL))
			c.terminateSession(ctx, extraHeaders)

			// Retry initialize after termination
			resp, sessionID, err = c.jsonRPCRawWithHeaders(ctx, "initialize", params, extraHeaders)
			if err != nil {
				return fmt.Errorf("MCP initialize failed after session termination: %w", err)
			}
		} else {
			return fmt.Errorf("MCP initialize failed: %w", err)
		}
	}

	c.mu.Lock()
	if sessionID != "" {
		c.sessionID = sessionID
	}
	c.initialized = true
	c.mu.Unlock()

	c.logger.Debug("MCP client initialized",
		zap.String("url", c.serverURL),
		zap.String("session_id", c.sessionID),
		zap.String("response", string(resp)))

	// Send notifications/initialized as required by the MCP spec
	if err := c.sendNotificationWithHeaders(ctx, "notifications/initialized", nil, extraHeaders); err != nil {
		c.logger.Warn("Failed to send initialized notification", zap.Error(err))
	}

	return nil
}

// Initialize performs the MCP protocol handshake with the server
func (c *Client) Initialize(ctx context.Context) error {
	return c.InitializeWithHeaders(ctx, nil)
}

// Reconnect resets the client state and re-initializes
func (c *Client) Reconnect(ctx context.Context) error {
	return c.ReconnectWithHeaders(ctx, nil)
}

// ReconnectWithHeaders resets the client state and re-initializes with extra headers.
// Creates a fresh HTTP client to avoid reusing connections with stale server-side state.
func (c *Client) ReconnectWithHeaders(ctx context.Context, extraHeaders map[string]string) error {
	// Terminate existing session before resetting
	c.terminateSession(ctx, extraHeaders)

	c.mu.Lock()
	c.initialized = false
	c.client = &http.Client{Transport: security.NewSafeTransport()}
	c.nextID.Store(0)
	c.mu.Unlock()

	return c.InitializeWithHeaders(ctx, extraHeaders)
}

// IsInitialized returns whether the client has been initialized
func (c *Client) IsInitialized() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.initialized
}

// ListTools returns the tools available on the MCP server
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	return c.ListToolsWithHeaders(ctx, nil)
}

// ListToolsWithHeaders returns the tools available on the MCP server with extra headers
func (c *Client) ListToolsWithHeaders(ctx context.Context, extraHeaders map[string]string) ([]Tool, error) {
	if !c.IsInitialized() {
		return nil, fmt.Errorf("MCP client not initialized, call Initialize() first")
	}

	resp, _, err := c.jsonRPCRawWithHeaders(ctx, "tools/list", nil, extraHeaders)
	if err != nil {
		return nil, err
	}

	var result struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse tools list: %w", err)
	}

	return result.Tools, nil
}

// CallTool calls a tool on the MCP server
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*ToolResult, error) {
	return c.CallToolWithHeaders(ctx, name, arguments, nil)
}

// CallToolWithHeaders calls a tool on the MCP server with extra per-request headers
func (c *Client) CallToolWithHeaders(ctx context.Context, name string, arguments map[string]interface{}, extraHeaders map[string]string) (*ToolResult, error) {
	if !c.IsInitialized() {
		return nil, fmt.Errorf("MCP client not initialized, call Initialize() first")
	}

	// Apply per-call timeout so long-running tool calls don't hang indefinitely
	if c.toolCallTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.toolCallTimeout)
		defer cancel()
	}

	params := map[string]interface{}{
		"name":      name,
		"arguments": arguments,
	}

	resp, _, err := c.jsonRPCRawWithHeaders(ctx, "tools/call", params, extraHeaders)
	if err != nil {
		return nil, err
	}

	var result ToolResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse tool result: %w", err)
	}

	return &result, nil
}

// terminateSession sends a DELETE request to the MCP server to end the current session.
// Per the MCP spec, clients use HTTP DELETE to terminate sessions.
func (c *Client) terminateSession(ctx context.Context, extraHeaders map[string]string) {
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.serverURL, nil)
	if err != nil {
		c.logger.Warn("Failed to create DELETE request for MCP session termination", zap.Error(err))
		return
	}

	c.mu.Lock()
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	c.sessionID = ""
	c.mu.Unlock()

	for k, v := range security.FilterHeaders(c.headers) {
		req.Header.Set(k, v)
	}
	for k, v := range security.FilterHeaders(extraHeaders) {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Warn("MCP session termination request failed", zap.Error(err))
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	c.logger.Info("MCP session terminated",
		zap.String("url", c.serverURL),
		zap.Int("status", resp.StatusCode))
}

// sendNotificationWithHeaders sends a JSON-RPC 2.0 notification (no id, no response expected)
func (c *Client) sendNotificationWithHeaders(ctx context.Context, method string, params interface{}, extraHeaders map[string]string) error {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		reqBody["params"] = params
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.serverURL, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	c.mu.Lock()
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	c.mu.Unlock()

	for k, v := range security.FilterHeaders(c.headers) {
		req.Header.Set(k, v)
	}
	for k, v := range security.FilterHeaders(extraHeaders) {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Notifications may return 200 or 202 (Accepted)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("MCP notification %s failed with status %d: %s", method, resp.StatusCode, string(body))
	}

	// Drain response body
	io.Copy(io.Discard, resp.Body)
	return nil
}

// jsonRPCRawWithHeaders sends a JSON-RPC 2.0 request with optional extra headers
func (c *Client) jsonRPCRawWithHeaders(ctx context.Context, method string, params interface{}, extraHeaders map[string]string) (json.RawMessage, string, error) {
	id := c.nextID.Add(1)

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		reqBody["params"] = params
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.serverURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	// Include Mcp-Session-Id if we have one
	c.mu.Lock()
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	c.mu.Unlock()

	for k, v := range security.FilterHeaders(c.headers) {
		req.Header.Set(k, v)
	}

	// Apply per-request extra headers (filtered for SSRF safety)
	for k, v := range security.FilterHeaders(extraHeaders) {
		req.Header.Set(k, v)
	}

	c.logger.Debug("MCP request sending",
		zap.String("method", method),
		zap.String("url", c.serverURL))

	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Warn("MCP request failed",
			zap.String("method", method),
			zap.String("url", c.serverURL),
			zap.Error(err))
		return nil, "", err
	}
	defer resp.Body.Close()

	c.logger.Debug("MCP response received",
		zap.String("method", method),
		zap.Int("status", resp.StatusCode),
		zap.String("content_type", resp.Header.Get("Content-Type")))

	// Extract Mcp-Session-Id from all responses (including errors)
	newSessionID := resp.Header.Get("Mcp-Session-Id")

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.logger.Warn("MCP server returned error",
			zap.String("method", method),
			zap.String("url", c.serverURL),
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(body)))
		return nil, newSessionID, fmt.Errorf("MCP server error %d: %s", resp.StatusCode, string(body))
	}

	// Handle SSE responses: parse "data: {json}" lines from text/event-stream
	contentType := resp.Header.Get("Content-Type")
	var responseBody []byte
	if strings.Contains(contentType, "text/event-stream") {
		responseBody = extractJSONFromSSE(resp.Body)
	} else {
		responseBody, _ = io.ReadAll(resp.Body)
	}

	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(responseBody, &rpcResp); err != nil {
		return nil, "", fmt.Errorf("failed to decode response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, "", fmt.Errorf("MCP error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, newSessionID, nil
}

// extractJSONFromSSE reads an SSE stream and returns the first JSON-RPC data payload.
// SSE format: lines of "event: message\ndata: {json}\n\n"
func extractJSONFromSSE(body io.Reader) []byte {
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if len(data) > 0 && data[0] == '{' {
				return []byte(data)
			}
		}
	}
	return nil
}

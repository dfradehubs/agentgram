package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// RESTProxy handles proxying to REST agents
type RESTProxy struct {
	client *agents.RESTClient
	logger *zap.Logger
}

// NewRESTProxy creates a new REST proxy
func NewRESTProxy(logger *zap.Logger) *RESTProxy {
	return &RESTProxy{
		client: agents.NewRESTClient(),
		logger: logger,
	}
}

// Handle handles a request to a REST agent using AG-UI protocol
func (p *RESTProxy) Handle(ctx context.Context, w http.ResponseWriter, agent *models.Agent, body io.Reader, authHeader string, requestID string, threadID string, sessionName string, onEvent func(interface{})) (*ProxyResult, error) {
	// Create SSE writer
	sse, err := NewSSEWriter(w)
	if err != nil {
		return nil, err
	}

	if onEvent != nil {
		sse.SetOnEvent(onEvent)
	}

	if threadID != "" {
		sse.SetThreadID(threadID)
	}
	if sessionName != "" {
		sse.SetSessionName(sessionName)
	}

	// Send run started event
	if err := sse.SendRunStarted(); err != nil {
		return nil, err
	}

	// Use a separate context for the agent request so the stream continues
	// even if the client disconnects. This lets us capture the full response
	// for session persistence.
	agentCtx, agentCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer agentCancel()

	// Propagate trace span into the detached context so child spans remain
	// connected to the original trace.
	agentCtx = trace.ContextWithSpan(agentCtx, trace.SpanFromContext(ctx))

	// Propagate GitHub token from the original request context
	if githubToken := middleware.GetGitHubTokenFromContext(ctx); githubToken != "" {
		agentCtx = context.WithValue(agentCtx, middleware.GitHubTokenContextKey, githubToken)
	}

	// Record the outgoing request body as a span event
	bodyBytes, _ := io.ReadAll(body)
	if span := trace.SpanFromContext(agentCtx); span.SpanContext().IsValid() {
		span.AddEvent("rest.request_body", trace.WithAttributes(
			attribute.String("body", string(bodyBytes)),
		))
	}
	// Forward request to agent with retry on connection errors (pre-content)
	const maxAgentRetries = 3
	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt < maxAgentRetries; attempt++ {
		if attempt > 0 {
			sse.SendKeepAlive()
			delay := time.Duration(1<<uint(attempt-1)) * time.Second // 1s, 2s
			p.logger.Warn("retrying agent connection",
				zap.String("agent_id", agent.ID),
				zap.Int("attempt", attempt+1),
				zap.Duration("delay", delay))
			time.Sleep(delay)
		}
		resp, lastErr = p.client.Request(agentCtx, agent, bytes.NewReader(bodyBytes), authHeader, requestID)
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		p.logger.Error("agent connection failed after retries",
			zap.String("agent_id", agent.ID),
			zap.Error(lastErr))
		sse.SendRunError("failed to connect to agent")
		return nil, lastErr
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		// Log full error details server-side but send sanitized message to client
		p.logger.Error("agent returned error",
			zap.String("agent_id", agent.ID),
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(bodyBytes)))
		clientMsg := extractAgentErrorDetail(resp.StatusCode, bodyBytes)
		sse.SendRunError(clientMsg)
		return nil, fmt.Errorf("agent returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	contentType := resp.Header.Get("Content-Type")

	// If agent responds with SSE, convert to AG-UI
	if strings.Contains(contentType, "text/event-stream") {
		return p.proxySSEToAGUI(ctx, sse, resp.Body)
	}

	// If agent responds with JSON, convert to AG-UI
	var contentPath string
	if agent.CustomFormat != nil {
		contentPath = agent.CustomFormat.ResponseContentPath
	}
	return p.convertJSONToAGUI(ctx, sse, resp.Body, contentPath)
}

// proxySSEToAGUI proxies SSE from agent to client using AG-UI protocol.
// ctx is the client's request context; when it's cancelled the proxy enters
// "drain" mode: it keeps reading from the agent stream (to capture the full
// response for session persistence) but stops writing to the client.
func (p *RESTProxy) proxySSEToAGUI(ctx context.Context, sse *SSEWriter, body io.Reader) (*ProxyResult, error) {
	reader := bufio.NewReader(body)

	messageStarted := false
	clientGone := false // true once the client has disconnected
	var accumulated strings.Builder
	var currentSegment strings.Builder // Text in the current segment (between tool calls)
	var agentSessionID string
	var currentEventName string // Tracks SSE event name from "event:" lines

	// Accumulate tool calls for session persistence
	var toolCalls []CapturedToolCall
	toolCallMap := make(map[string]int) // toolCallID -> index in toolCalls
	var contentParts []ContentPart
	var streamErr string // non-empty if stream ended with an error

	// sendToClient always invokes fn so onEvent (buffer/pub-sub) keeps firing even
	// in drain mode; SendAGUIEvent skips the actual client write once it's gone.
	sendToClient := func(fn func() error) {
		if err := fn(); err != nil && !clientGone {
			clientGone = true
			sse.MarkClientGone()
			p.logger.Debug("client disconnected, entering drain mode")
		}
	}

	for {
		// Detect client disconnect without blocking the read loop
		select {
		case <-ctx.Done():
			if !clientGone {
				clientGone = true
				sse.MarkClientGone()
				p.logger.Debug("client context cancelled, entering drain mode")
			}
		default:
		}

		lineBytes, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			p.logger.Error("error reading SSE stream", zap.Error(err))
			streamErr = fmt.Sprintf("stream error: %v", err)
			sendToClient(func() error { return sse.SendRunError(streamErr) })
			break // stream broken, finalize with what we have
		}

		line := strings.TrimSpace(string(lineBytes))

		// Parse SSE event name (e.g. "event: tool_start")
		if strings.HasPrefix(line, "event:") {
			currentEventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}

		// Parse SSE data
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		// Try to parse as JSON to extract content
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			// Reset currentEventName even if JSON parse fails
			currentEventName = ""
			// If not JSON, send as text content
			if !messageStarted {
				sendToClient(func() error { return sse.SendTextMessageStart() })
				messageStarted = true
			}
			accumulated.WriteString(data)
			currentSegment.WriteString(data)
			sendToClient(func() error { return sse.SendTextMessageContent(data) })
			continue
		}

		// Determine the effective event type: SSE event name takes priority, then JSON "type" field
		effectiveType := currentEventName
		if effectiveType == "" {
			effectiveType, _ = event["type"].(string)
		}
		currentEventName = ""

		p.logger.Debug("SSE event",
			zap.String("effective_type", effectiveType),
			zap.Any("data", event))

		// Helper: close open text message (used before tool calls)
		closeTextMessage := func() {
			if messageStarted {
				sendToClient(func() error { return sse.SendTextMessageEnd() })
				messageStarted = false
			}
			if seg := currentSegment.String(); seg != "" {
				contentParts = append(contentParts, ContentPart{Type: "text", Text: seg})
				currentSegment.Reset()
			}
		}

		// Helper: ensure text message is started
		ensureTextStarted := func() {
			if !messageStarted {
				sendToClient(func() error { return sse.SendTextMessageStart() })
				messageStarted = true
			}
		}

		// Helper: accumulate and send text content
		sendText := func(text string) {
			ensureTextStarted()
			accumulated.WriteString(text)
			currentSegment.WriteString(text)
			sendToClient(func() error { return sse.SendTextMessageContent(text) })
		}

		switch effectiveType {
		case "tool_start":
			closeTextMessage()
			toolID, _ := event["tool_id"].(string)
			toolName, _ := event["tool_name"].(string)
			toolIdx := len(toolCalls)
			contentParts = append(contentParts, ContentPart{Type: "tool_use", ToolIndex: toolIdx})
			toolCallMap[toolID] = toolIdx
			toolCalls = append(toolCalls, CapturedToolCall{ID: toolID, Name: toolName})
			sendToClient(func() error { return sse.SendToolCallStart(toolID, toolName) })

		case "tool_input":
			toolID, _ := event["tool_id"].(string)
			args := event["args"]
			argsJSON, _ := json.Marshal(args)
			if idx, ok := toolCallMap[toolID]; ok {
				toolCalls[idx].Args += string(argsJSON)
			}
			sendToClient(func() error { return sse.SendToolCallArgs(toolID, string(argsJSON)) })

		case "tool_result":
			toolID, _ := event["tool_id"].(string)
			result := extractToolResult(event)
			if idx, ok := toolCallMap[toolID]; ok {
				toolCalls[idx].Result = result
			}
			p.logger.Debug("tool_result extracted",
				zap.String("tool_id", toolID),
				zap.Int("result_len", len(result)))
			sendToClient(func() error { return sse.SendToolCallEnd(toolID, result) })

		case "TOOL_CALL_START":
			closeTextMessage()
			toolID, _ := event["toolCallId"].(string)
			toolName, _ := event["toolName"].(string)
			toolIdx := len(toolCalls)
			contentParts = append(contentParts, ContentPart{Type: "tool_use", ToolIndex: toolIdx})
			toolCallMap[toolID] = toolIdx
			toolCalls = append(toolCalls, CapturedToolCall{ID: toolID, Name: toolName})
			sendToClient(func() error { return sse.SendToolCallStart(toolID, toolName) })

		case "TOOL_CALL_ARGS":
			toolID, _ := event["toolCallId"].(string)
			delta, _ := event["delta"].(string)
			if idx, ok := toolCallMap[toolID]; ok {
				toolCalls[idx].Args += delta
			}
			sendToClient(func() error { return sse.SendToolCallArgs(toolID, delta) })

		case "TOOL_CALL_END":
			toolID, _ := event["toolCallId"].(string)
			result, _ := event["result"].(string)
			if idx, ok := toolCallMap[toolID]; ok {
				toolCalls[idx].Result = result
			}
			sendToClient(func() error { return sse.SendToolCallEnd(toolID, result) })

		case "CUSTOM":
			subType, _ := event["subType"].(string)
			data, _ := event["data"].(map[string]interface{})
			sendToClient(func() error { return sse.SendCustomEvent(subType, data) })
			if subType == "CHART" && data != nil {
				contentParts = append(contentParts, ContentPart{Type: "chart", Chart: data})
			}

		case "content_delta":
			if text, ok := event["text"].(string); ok {
				sendText(text)
			}

		case "start":
			if sid, ok := event["session_id"].(string); ok && sid != "" {
				agentSessionID = sid
			} else if sid, ok := event["conversation_id"].(string); ok && sid != "" {
				agentSessionID = sid
			}
			ensureTextStarted()

		case "chunk":
			if content, ok := event["content"].(string); ok {
				sendText(content)
			}

		case "end":
			if sid, ok := event["session_id"].(string); ok && sid != "" && agentSessionID == "" {
				agentSessionID = sid
			} else if sid, ok := event["conversation_id"].(string); ok && sid != "" && agentSessionID == "" {
				agentSessionID = sid
			}
			if messageStarted {
				sendToClient(func() error { return sse.SendTextMessageEnd() })
			}
			sendToClient(func() error { return sse.SendRunFinished() })
			if seg := currentSegment.String(); seg != "" {
				contentParts = append(contentParts, ContentPart{Type: "text", Text: seg})
			}
			return &ProxyResult{AssistantText: accumulated.String(), AgentSessionID: agentSessionID, ToolCalls: toolCalls, ContentParts: contentParts}, nil

		case "error":
			msg := "unknown error"
			if m, ok := event["message"].(string); ok {
				msg = m
			}
			if messageStarted {
				sendToClient(func() error { return sse.SendTextMessageEnd() })
			}
			sendToClient(func() error { return sse.SendRunError(msg) })
			errMsg := fmt.Sprintf("agent error: %s", msg)
			// Return accumulated content so partial responses are persisted
			if accumulated.Len() > 0 || len(toolCalls) > 0 {
				if seg := currentSegment.String(); seg != "" {
					contentParts = append(contentParts, ContentPart{Type: "text", Text: seg})
				}
				return &ProxyResult{
					AssistantText:  accumulated.String(),
					AgentSessionID: agentSessionID,
					ToolCalls:      toolCalls,
					ContentParts:   contentParts,
					Error:          errMsg,
				}, fmt.Errorf("%s", errMsg)
			}
			return nil, fmt.Errorf("%s", errMsg)

		default:
			// Try to capture session ID from any event
			if agentSessionID == "" {
				if sid, ok := event["session_id"].(string); ok && sid != "" {
					agentSessionID = sid
				} else if sid, ok := event["conversation_id"].(string); ok && sid != "" {
					agentSessionID = sid
				}
			}
			// Try to extract content from various formats
			if content, ok := event["content"].(string); ok {
				sendText(content)
			} else if text, ok := event["text"].(string); ok {
				sendText(text)
			} else if delta, ok := event["delta"].(map[string]interface{}); ok {
				if content, ok := delta["content"].(string); ok {
					sendText(content)
				}
			} else if contentObj, ok := event["content"].(map[string]interface{}); ok {
				// ADK format: content.parts[].text
				if parts, ok := contentObj["parts"].([]interface{}); ok {
					for _, p := range parts {
						if part, ok := p.(map[string]interface{}); ok {
							if text, ok := part["text"].(string); ok && text != "" {
								sendText(text)
							}
						}
					}
				}
			}
		}
	}

	// Ensure message and run are properly closed
	if messageStarted {
		sendToClient(func() error { return sse.SendTextMessageEnd() })
	}
	sendToClient(func() error { return sse.SendRunFinished() })
	if seg := currentSegment.String(); seg != "" {
		contentParts = append(contentParts, ContentPart{Type: "text", Text: seg})
	}
	return &ProxyResult{
		AssistantText:  accumulated.String(),
		AgentSessionID: agentSessionID,
		ToolCalls:      toolCalls,
		ContentParts:   contentParts,
		Error:          streamErr,
	}, nil
}

// extractToolResult extracts the result text from a tool_result event.
// Agents may send results in various formats:
//   - Direct string fields: "result", "output", "content", "message"
//   - Nested Anthropic/ADK style: "response.content[].text"
//   - Fallback: JSON-marshal of non-string fields
func extractToolResult(event map[string]interface{}) string {
	// 1. Try direct string fields (high-priority, explicit result)
	for _, field := range []string{"result", "output", "content"} {
		if s, ok := event[field].(string); ok && s != "" {
			return s
		}
	}
	// 2. Try nested response.content[].text (Anthropic/ADK style — full data)
	if resp, ok := event["response"].(map[string]interface{}); ok {
		if contentArr, ok := resp["content"].([]interface{}); ok {
			var texts []string
			for _, item := range contentArr {
				if block, ok := item.(map[string]interface{}); ok {
					if text, ok := block["text"].(string); ok && text != "" {
						texts = append(texts, text)
					}
				}
			}
			if len(texts) > 0 {
				return strings.Join(texts, "\n")
			}
		}
	}
	// 3. "message" field (often a short summary, lower priority than response)
	if s, ok := event["message"].(string); ok && s != "" {
		return s
	}
	// 4. Fallback: marshal non-string fields
	for _, field := range []string{"result", "output", "content", "response"} {
		if event[field] != nil {
			if resultBytes, err := json.Marshal(event[field]); err == nil {
				candidate := string(resultBytes)
				if candidate != "null" && candidate != "" {
					return candidate
				}
			}
		}
	}
	return ""
}

// convertJSONToAGUI converts a JSON response to AG-UI events.
// If contentPath is non-empty, it is used to extract the content via dot-notation before falling back to auto-detection.
func (p *RESTProxy) convertJSONToAGUI(ctx context.Context, sse *SSEWriter, body io.Reader, contentPath string) (*ProxyResult, error) {
	// Read full body
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		sse.SendRunError(fmt.Sprintf("failed to read response: %v", err))
		return nil, err
	}

	// Send text message start
	if err := sse.SendTextMessageStart(); err != nil {
		return nil, err
	}

	var assistantText string

	// Try to parse as JSON
	var response map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		// If not JSON, send as plain text
		assistantText = string(bodyBytes)
		if err := sse.SendTextMessageContent(assistantText); err != nil {
			return nil, err
		}
	} else {
		// Extract content from JSON: try configured path first, then auto-detection
		var content string
		if contentPath != "" {
			if extracted, ok := ExtractByPath(response, contentPath); ok {
				content = extracted
			}
		}
		if content == "" {
			content = extractContent(response)
		}
		if content != "" {
			assistantText = content
			if err := sse.SendTextMessageContent(content); err != nil {
				return nil, err
			}
		} else {
			// Send full JSON as content
			assistantText = string(bodyBytes)
			if err := sse.SendTextMessageContent(assistantText); err != nil {
				return nil, err
			}
		}
	}

	// Send text message end
	if err := sse.SendTextMessageEnd(); err != nil {
		return nil, err
	}

	// Send run finished
	if err := sse.SendRunFinished(); err != nil {
		return nil, err
	}
	return &ProxyResult{AssistantText: assistantText}, nil
}

// extractContent tries to extract content from a JSON response
func extractContent(response map[string]interface{}) string {
	// Try different common structures

	// OpenAI style: choices[0].message.content
	if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].(string); ok {
					return content
				}
			}
		}
	}

	// Anthropic style: content[0].text
	if content, ok := response["content"].([]interface{}); ok && len(content) > 0 {
		if block, ok := content[0].(map[string]interface{}); ok {
			if text, ok := block["text"].(string); ok {
				return text
			}
		}
	}

	// Simple: response, text, or content directly
	if resp, ok := response["response"].(string); ok {
		return resp
	}
	if text, ok := response["text"].(string); ok {
		return text
	}
	if content, ok := response["content"].(string); ok {
		return content
	}

	return ""
}

// FormatRequestBody formats the request body for REST agents.
// If the agent has a CustomFormat.RequestTemplate, it renders the Go template.
// Otherwise uses the standard format: query (last user message) + conversation_id.
func FormatRequestBody(agent *models.Agent, chatReq *models.ChatRequest) (io.Reader, error) {
	// Use custom template if configured
	if agent.CustomFormat != nil && agent.CustomFormat.RequestTemplate != "" {
		data := BuildTemplateData(chatReq)
		body, err := RenderRequestTemplate(agent.CustomFormat.RequestTemplate, data)
		if err != nil {
			return nil, fmt.Errorf("failed to render custom template: %w", err)
		}
		return bytes.NewReader(body), nil
	}

	// Default format
	payload := map[string]interface{}{}
	if chatReq.SessionID != "" {
		payload["conversation_id"] = chatReq.SessionID
	}
	// Collect all user messages and concatenate them into the query.
	// This preserves context messages prepended by PrepareMessagesForMultiAgent.
	var parts []string
	for _, msg := range chatReq.Messages {
		if msg.Role == "user" {
			parts = append(parts, msg.Content)
		}
	}
	if len(parts) > 0 {
		payload["query"] = strings.Join(parts, "\n\n")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	return bytes.NewReader(body), nil
}

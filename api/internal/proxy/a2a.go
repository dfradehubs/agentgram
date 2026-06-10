package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/a2a"
	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// A2AProxy handles proxying to A2A agents
type A2AProxy struct {
	client *agents.A2AClient
	logger *zap.Logger
}

// NewA2AProxy creates a new A2A proxy
func NewA2AProxy(logger *zap.Logger) *A2AProxy {
	return &A2AProxy{
		client: agents.NewA2AClient(),
		logger: logger,
	}
}

// Handle handles a request to an A2A agent using AG-UI protocol.
// It sends message/stream to the agent, reads SSE events, and converts them to AG-UI events.
func (p *A2AProxy) Handle(ctx context.Context, w http.ResponseWriter, agent *models.Agent, chatReq *models.ChatRequest, auth agents.OutboundAuth, requestID string, threadID string, sessionName string, onEvent func(interface{})) (*ProxyResult, error) {
	// Create SSE writer for AG-UI output
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

	// Collect all user messages (context + query) and concatenate them.
	// This preserves context messages prepended by PrepareMessagesForMultiAgent.
	// Also extract attachments from the last user message for native file support.
	var parts []string
	var attachments []models.Attachment
	for _, msg := range chatReq.Messages {
		if msg.Role == "user" {
			parts = append(parts, msg.Content)
		}
	}
	// Extract attachments from the last user message
	if len(chatReq.Messages) > 0 {
		lastMsg := chatReq.Messages[len(chatReq.Messages)-1]
		if lastMsg.Role == "user" && len(lastMsg.Attachments) > 0 {
			attachments = lastMsg.Attachments
		}
	}
	userMessage := strings.Join(parts, "\n\n")

	if userMessage == "" {
		sse.SendRunError("no user message found")
		return nil, fmt.Errorf("no user message found")
	}

	// Send RUN_STARTED
	if err := sse.SendRunStarted(); err != nil {
		return nil, err
	}

	// Use a detached context for upstream A2A streaming so frontend/client
	// disconnects do not abort the agent stream mid-run.
	a2aCtx, a2aCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer a2aCancel()

	// Propagate current trace span into the detached context.
	a2aCtx = trace.ContextWithSpan(a2aCtx, trace.SpanFromContext(ctx))

	// Preserve GitHub token for upstream forwarding logic.
	if githubToken := middleware.GetGitHubTokenFromContext(ctx); githubToken != "" {
		a2aCtx = context.WithValue(a2aCtx, middleware.GitHubTokenContextKey, githubToken)
	}

	// Send message/stream to A2A agent with retry on connection errors (pre-content)
	const maxAgentRetries = 3
	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt < maxAgentRetries; attempt++ {
		if attempt > 0 {
			sse.SendKeepAlive()
			delay := time.Duration(1<<uint(attempt-1)) * time.Second // 1s, 2s
			p.logger.Warn("retrying A2A agent connection",
				zap.String("agent_id", agent.ID),
				zap.Int("attempt", attempt+1),
				zap.Duration("delay", delay))
			time.Sleep(delay)
		}
		resp, lastErr = p.client.SendMessageStream(a2aCtx, agent, userMessage, chatReq.SessionID, auth, requestID, attachments)
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		p.logger.Error("A2A agent connection failed after retries",
			zap.String("agent_id", agent.ID),
			zap.Error(lastErr))
		sse.SendRunError(fmt.Sprintf("failed to send message: %v", lastErr))
		return nil, lastErr
	}
	defer resp.Body.Close()

	p.logger.Debug("message/stream connected to A2A agent",
		zap.String("agent_id", agent.ID))

	// Read SSE events from agent response
	messageStarted := false
	clientGone := false
	var accumulated strings.Builder
	var currentText strings.Builder
	var agentContextID string

	// Track tool calls for session persistence
	var toolCalls []CapturedToolCall
	toolCallMap := make(map[string]int)
	var contentParts []ContentPart

	flushText := func() {
		if currentText.Len() > 0 {
			contentParts = append(contentParts, ContentPart{Type: "text", Text: currentText.String()})
			currentText.Reset()
		}
	}

	// sendToClient always invokes fn so onEvent (buffer/pub-sub) keeps firing even
	// in drain mode; SendAGUIEvent skips the actual client write once it's gone.
	sendToClient := func(fn func() error) {
		if err := fn(); err != nil && !clientGone {
			clientGone = true
			sse.MarkClientGone()
			p.logger.Debug("client disconnected, entering drain mode")
		}
	}

	reader := bufio.NewReader(resp.Body)

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

		line, err := reader.ReadString('\n')
		if err != nil {
			// Stream ended
			if messageStarted {
				sendToClient(func() error { return sse.SendTextMessageEnd() })
			}
			// If we accumulated text or tool calls, it was a successful run
			if accumulated.Len() > 0 || len(toolCalls) > 0 {
				sendToClient(func() error { return sse.SendRunFinished() })
				flushText()
				return &ProxyResult{
					AssistantText:  accumulated.String(),
					AgentSessionID: agentContextID,
					ToolCalls:      toolCalls,
					ContentParts:   contentParts,
				}, nil
			}
			// Stream ended without content
			p.logger.Error("stream ended unexpectedly",
				zap.String("agent_id", agent.ID),
				zap.Error(err))
			sendToClient(func() error { sse.SendRunError("stream ended unexpectedly"); return nil })
			return nil, fmt.Errorf("stream ended: %w", err)
		}

		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// Parse SSE data lines
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimPrefix(line, "data:")
		data = strings.TrimSpace(data)

		if data == "" {
			continue
		}

		// Parse JSON-RPC response
		var rpcResp a2a.JSONRPCResponse
		if err := json.Unmarshal([]byte(data), &rpcResp); err != nil {
			p.logger.Warn("failed to parse SSE event",
				zap.String("agent_id", agent.ID),
				zap.String("data", data),
				zap.Error(err))
			continue
		}

		// Check for JSON-RPC error
		if rpcResp.Error != nil {
			p.logger.Error("agent returned JSON-RPC error",
				zap.String("agent_id", agent.ID),
				zap.Int("code", rpcResp.Error.Code),
				zap.String("message", rpcResp.Error.Message))
			if messageStarted {
				sendToClient(func() error { return sse.SendTextMessageEnd() })
			}
			sendToClient(func() error { sse.SendRunError(rpcResp.Error.Message); return nil })
			errMsg := fmt.Sprintf("agent error: %s", rpcResp.Error.Message)
			if accumulated.Len() > 0 || len(toolCalls) > 0 {
				flushText()
				return &ProxyResult{
					AssistantText:  accumulated.String(),
					AgentSessionID: agentContextID,
					ToolCalls:      toolCalls,
					ContentParts:   contentParts,
					Error:          errMsg,
				}, fmt.Errorf("%s", errMsg)
			}
			return nil, fmt.Errorf("%s", errMsg)
		}

		if rpcResp.Result == nil {
			continue
		}

		event := rpcResp.Result

		// Capture contextId from any event that has it
		if event.ContextID != "" {
			agentContextID = event.ContextID
		}

		switch event.Kind {
		case "status-update":
			if event.Status == nil {
				continue
			}

			p.logger.Debug("A2A status update",
				zap.String("agent_id", agent.ID),
				zap.String("state", event.Status.State),
				zap.String("task_id", event.TaskID),
				zap.Bool("final", event.Final))

			switch event.Status.State {
			case "completed":
				// Check if status message has text content
				if event.Status.Message != nil {
					for _, part := range event.Status.Message.Parts {
						if part.Kind == "text" && part.Text != "" && !part.IsThought() {
							if !messageStarted {
								sendToClient(func() error { return sse.SendTextMessageStart() })
								messageStarted = true
							}
							accumulated.WriteString(part.Text)
							currentText.WriteString(part.Text)
							sendToClient(func() error { return sse.SendTextMessageContent(part.Text) })
						}
					}
				}
				if messageStarted {
					sendToClient(func() error { return sse.SendTextMessageEnd() })
				}
				sendToClient(func() error { return sse.SendRunFinished() })
				flushText()
				return &ProxyResult{
					AssistantText:  accumulated.String(),
					AgentSessionID: agentContextID,
					ToolCalls:      toolCalls,
					ContentParts:   contentParts,
				}, nil

			case "failed":
				errMsg := "task failed"
				if event.Status.Message != nil {
					for _, part := range event.Status.Message.Parts {
						if part.Kind == "text" && part.Text != "" {
							errMsg = part.Text
							break
						}
					}
				}
				if messageStarted {
					sendToClient(func() error { return sse.SendTextMessageEnd() })
				}
				sendToClient(func() error { sse.SendRunError(errMsg); return nil })
				fullErr := fmt.Sprintf("task failed: %s", errMsg)
				if accumulated.Len() > 0 || len(toolCalls) > 0 {
					flushText()
					return &ProxyResult{
						AssistantText:  accumulated.String(),
						AgentSessionID: agentContextID,
						ToolCalls:      toolCalls,
						ContentParts:   contentParts,
						Error:          fullErr,
					}, fmt.Errorf("%s", fullErr)
				}
				return nil, fmt.Errorf("%s", fullErr)

			case "canceled":
				if messageStarted {
					sendToClient(func() error { return sse.SendTextMessageEnd() })
				}
				sendToClient(func() error { return sse.SendRunFinished() })
				flushText()
				return &ProxyResult{
					AssistantText:  accumulated.String(),
					AgentSessionID: agentContextID,
					ToolCalls:      toolCalls,
					ContentParts:   contentParts,
				}, nil

			case "working":
				// Working status messages contain intermediate steps:
				// text (thinking or streaming content) AND data parts (functionCall/functionResponse).
				if event.Status.Message != nil {
					for _, part := range event.Status.Message.Parts {
						switch part.Kind {
						case "text":
							if part.Text == "" {
								continue
							}
							// Only emit as thinking if explicitly marked as thought
							if part.IsThought() {
								p.emitThinkingDrain(sse, part.Text, &messageStarted, clientGone)
								continue
							}
							// Regular streaming content — accumulate and persist
							if !messageStarted {
								sendToClient(func() error { return sse.SendTextMessageStart() })
								messageStarted = true
							}
							accumulated.WriteString(part.Text)
							currentText.WriteString(part.Text)
							sendToClient(func() error { return sse.SendTextMessageContent(part.Text) })
						case "data":
							if part.Data != nil {
								p.captureToolCall(part.Data, sse, &messageStarted, &toolCalls, toolCallMap, &contentParts, &currentText, clientGone)
							}
						}
					}
				}

			case "submitted", "input-required":
				// These are intermediate states, continue reading

			case "rejected":
				errMsg := "request rejected by agent"
				if event.Status.Message != nil {
					for _, part := range event.Status.Message.Parts {
						if part.Kind == "text" && part.Text != "" {
							errMsg = part.Text
							break
						}
					}
				}
				if messageStarted {
					sendToClient(func() error { return sse.SendTextMessageEnd() })
				}
				sendToClient(func() error { sse.SendRunError(errMsg); return nil })
				fullErr := fmt.Sprintf("task rejected: %s", errMsg)
				if accumulated.Len() > 0 || len(toolCalls) > 0 {
					flushText()
					return &ProxyResult{
						AssistantText:  accumulated.String(),
						AgentSessionID: agentContextID,
						ToolCalls:      toolCalls,
						ContentParts:   contentParts,
						Error:          fullErr,
					}, fmt.Errorf("%s", fullErr)
				}
				return nil, fmt.Errorf("%s", fullErr)

			case "auth-required":
				errMsg := "authentication required by agent"
				if event.Status.Message != nil {
					for _, part := range event.Status.Message.Parts {
						if part.Kind == "text" && part.Text != "" {
							errMsg = part.Text
							break
						}
					}
				}
				if messageStarted {
					sendToClient(func() error { return sse.SendTextMessageEnd() })
				}
				sendToClient(func() error { sse.SendRunError(errMsg); return nil })
				fullErr := fmt.Sprintf("auth required: %s", errMsg)
				if accumulated.Len() > 0 || len(toolCalls) > 0 {
					flushText()
					return &ProxyResult{
						AssistantText:  accumulated.String(),
						AgentSessionID: agentContextID,
						ToolCalls:      toolCalls,
						ContentParts:   contentParts,
						Error:          fullErr,
					}, fmt.Errorf("%s", fullErr)
				}
				return nil, fmt.Errorf("%s", fullErr)
			}

		case "artifact-update":
			if event.Artifact == nil {
				continue
			}

			adkAuthor := ""
			if event.Metadata != nil {
				if author, ok := event.Metadata["adk_author"].(string); ok {
					adkAuthor = author
				}
			}

			p.logger.Debug("A2A artifact-update",
				zap.String("agent_id", agent.ID),
				zap.String("artifact_id", event.Artifact.ArtifactID),
				zap.String("adk_author", adkAuthor),
				zap.Bool("append", event.Artifact.Append),
				zap.Bool("last_chunk", event.Artifact.LastChunk),
				zap.Int("parts_count", len(event.Artifact.Parts)))

			isIntermediateAgent := agent.PipelineFinalAgent != "" && adkAuthor != "" && adkAuthor != agent.PipelineFinalAgent

			for _, part := range event.Artifact.Parts {
				switch part.Kind {
				case "text":
					if part.Text == "" {
						continue
					}
					// Treat as thinking only if explicitly marked or from intermediate agent
					if part.IsThought() || isIntermediateAgent {
						p.emitThinkingDrain(sse, part.Text, &messageStarted, clientGone)
						continue
					}
					// Regular streaming content
					if !messageStarted {
						sendToClient(func() error { return sse.SendTextMessageStart() })
						messageStarted = true
					}
					accumulated.WriteString(part.Text)
					currentText.WriteString(part.Text)
					sendToClient(func() error { return sse.SendTextMessageContent(part.Text) })

				case "data":
					// Structured data: capture as tool call for persistence
					if part.Data != nil {
						p.captureToolCall(part.Data, sse, &messageStarted, &toolCalls, toolCallMap, &contentParts, &currentText, clientGone)
					}

				case "file":
					// File content — log and skip for now (AG-UI doesn't support file streaming)
					p.logger.Debug("skipping file part in artifact",
						zap.String("agent_id", agent.ID),
						zap.String("artifact_id", event.Artifact.ArtifactID))
				}
			}
		}
	}
}

// emitThinkingDrain sends thinking text as its own TEXT_MESSAGE sequence.
// If clientGone is true, skips SSE writes but still processes events.
func (p *A2AProxy) emitThinkingDrain(sse *SSEWriter, text string, messageStarted *bool, clientGone bool) {
	if clientGone {
		*messageStarted = false
		return
	}
	if *messageStarted {
		if err := sse.SendTextMessageEnd(); err != nil {
			return
		}
		*messageStarted = false
	}
	if err := sse.SendTextMessageStartThinking(); err != nil {
		return
	}
	if err := sse.SendTextMessageContent(text); err != nil {
		return
	}
	sse.SendTextMessageEnd()
}

// captureToolCall extracts function_call and function_response from A2A data parts
// and persists them as CapturedToolCalls. Also emits AG-UI TOOL_CALL events to the client.
func (p *A2AProxy) captureToolCall(data map[string]any, sse *SSEWriter, messageStarted *bool, toolCalls *[]CapturedToolCall, toolCallMap map[string]int, contentParts *[]ContentPart, currentText *strings.Builder, clientGone bool) {
	send := func(fn func() error) {
		if !clientGone {
			fn()
		}
	}

	// Resolve the tool call/response map. Three formats are supported:
	// 1. Nested (ADK-via-A2A): {"functionCall": {name, args, id}}
	// 2. Nested snake_case:    {"function_call": {name, args, id}}
	// 3. Flat (Anthropic A2A):  {name, args, id} directly in root
	fc := extractMap(data, "functionCall", "function_call")
	if fc == nil {
		// Flat format: tool call has "name" and is NOT a response (no "response" key).
		// Args may be absent for tools without parameters.
		if _, hasName := data["name"].(string); hasName {
			if _, hasResponse := data["response"]; !hasResponse {
				fc = data // flat tool call (with or without args)
			}
		}
	}

	if fc != nil {
		toolCallID, _ := fc["id"].(string)
		toolName, _ := fc["name"].(string)
		if toolCallID == "" {
			toolCallID = fmt.Sprintf("tc_%d", len(*toolCalls))
		}

		// Close open text message
		if *messageStarted {
			send(func() error { return sse.SendTextMessageEnd() })
			*messageStarted = false
		}

		// Flush pending text
		if currentText.Len() > 0 {
			*contentParts = append(*contentParts, ContentPart{Type: "text", Text: currentText.String()})
			currentText.Reset()
		}

		var argsStr string
		if args, ok := fc["args"]; ok && args != nil {
			argsJSON, _ := json.Marshal(args)
			argsStr = string(argsJSON)
		}

		idx := len(*toolCalls)
		*toolCalls = append(*toolCalls, CapturedToolCall{ID: toolCallID, Name: toolName, Args: argsStr})
		toolCallMap[toolCallID] = idx
		*contentParts = append(*contentParts, ContentPart{Type: "tool_use", ToolIndex: idx})

		send(func() error { return sse.SendToolCallStart(toolCallID, toolName) })
		if argsStr != "" {
			send(func() error { return sse.SendToolCallArgs(toolCallID, argsStr) })
		}
		return
	}

	// Try to extract a function response.
	// Nested: {"functionResponse": {name, response, id}} or {"function_response": ...}
	// Flat:   {name, response, id} directly in root
	fr := extractMap(data, "functionResponse", "function_response")
	if fr == nil {
		if _, hasName := data["name"].(string); hasName {
			if _, hasResp := data["response"]; hasResp {
				fr = data // flat tool response
			}
		}
	}

	if fr != nil {
		toolCallID, _ := fr["id"].(string)
		if toolCallID == "" {
			if name, ok := fr["name"].(string); ok {
				// Try to find by name
				for id, idx := range toolCallMap {
					if (*toolCalls)[idx].Name == name && (*toolCalls)[idx].Result == "" {
						toolCallID = id
						break
					}
				}
			}
		}

		var resultStr string
		if resp, ok := fr["response"]; ok && resp != nil {
			resultJSON, _ := json.Marshal(resp)
			resultStr = string(resultJSON)
		}

		if idx, ok := toolCallMap[toolCallID]; ok {
			(*toolCalls)[idx].Result = resultStr
		}

		send(func() error { return sse.SendToolCallEnd(toolCallID, resultStr) })
		return
	}

	// Generic data part — unrecognized format. Skip empty objects.
	if len(data) == 0 {
		return
	}
	dataJSON, _ := json.Marshal(data)
	p.logger.Warn("A2A data part unrecognized format — emitting as thinking",
		zap.String("data", string(dataJSON)))
	p.emitThinkingDrain(sse, string(dataJSON), messageStarted, clientGone)
}

// extractMap looks up a nested map[string]any from data using multiple key names.
// Returns nil if none of the keys match.
func extractMap(data map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if v, ok := data[key].(map[string]any); ok {
			return v
		}
	}
	return nil
}

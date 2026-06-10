package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/adk"
	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	contextLimitOpenTag  = "<context-limit-summary>"
	contextLimitCloseTag = "</context-limit-summary>"
)

// adkStreamResult holds the outcome of streaming an ADK response.
type adkStreamResult struct {
	accumulated    string
	toolCalls      []CapturedToolCall
	contentParts   []ContentPart
	messageStarted bool
	// contextLimitSummary is non-empty when the agent returned a context-limit summary
	// instead of a normal response. The text between the tags is stored here.
	contextLimitSummary string
}

// ADKProxy handles proxying to ADK REST SSE agents
type ADKProxy struct {
	client *agents.ADKClient
	logger *zap.Logger
}

// NewADKProxy creates a new ADK proxy.
func NewADKProxy(logger *zap.Logger) *ADKProxy {
	return &ADKProxy{
		client: agents.NewADKClient(logger),
		logger: logger,
	}
}

// Handle handles a request to an ADK agent using AG-UI protocol.
// It sends POST /run_sse to the agent, reads SSE events, and converts them to AG-UI events.
// If the agent responds with a context-limit summary, it transparently creates a new session
// and retries the request with the summary as context.
func (p *ADKProxy) Handle(ctx context.Context, w http.ResponseWriter, agent *models.Agent, chatReq *models.ChatRequest, auth agents.OutboundAuth, requestID string, threadID string, sessionName string, locale string, onEvent func(interface{})) (*ProxyResult, error) {
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

	// Collect user messages and extract attachments from the last user message
	var parts []string
	var attachments []models.Attachment
	for _, msg := range chatReq.Messages {
		if msg.Role == "user" {
			parts = append(parts, msg.Content)
		}
	}
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

	if err := sse.SendRunStarted(); err != nil {
		return nil, err
	}

	// Keep the downstream SSE connection active while ADK is silent (e.g. during
	// long tool executions). Some intermediaries close idle streams around 30s.
	keepAliveStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := sse.SendKeepAlive(); err != nil {
					p.logger.Debug("failed to send ADK SSE keep-alive", zap.Error(err))
					return
				}
			case <-keepAliveStop:
				return
			}
		}
	}()
	defer close(keepAliveStop)

	// Determine user ID: prefer authenticated email for per-user session isolation,
	// fall back to agent config for backwards compatibility.
	userID := agent.ADKUserID
	if claims := middleware.GetUserFromContext(ctx); claims != nil && claims.Email != "" {
		userID = claims.Email
	}

	// Use a detached context for the ADK connection so that if the frontend
	// disconnects mid-stream, the backend can still finish reading the agent
	// response and persist it. The original ctx is only used to detect client
	// disconnection for early SSE write abort.
	adkCtx, adkCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer adkCancel()

	// Propagate trace span into the detached context
	adkCtx = trace.ContextWithSpan(adkCtx, trace.SpanFromContext(ctx))

	// Preserve GitHub token for upstream forwarding logic.
	if githubToken := middleware.GetGitHubTokenFromContext(ctx); githubToken != "" {
		adkCtx = context.WithValue(adkCtx, middleware.GitHubTokenContextKey, githubToken)
	}

	// Connect to ADK agent with retry on connection errors (pre-content)
	const maxAgentRetries = 3
	var resp *http.Response
	var adkSessionID string
	var lastErr error
	for attempt := 0; attempt < maxAgentRetries; attempt++ {
		if attempt > 0 {
			sse.SendKeepAlive()
			delay := time.Duration(1<<uint(attempt-1)) * time.Second // 1s, 2s
			p.logger.Warn("retrying ADK agent connection",
				zap.String("agent_id", agent.ID),
				zap.Int("attempt", attempt+1),
				zap.Duration("delay", delay))
			time.Sleep(delay)
		}
		resp, adkSessionID, lastErr = p.client.RunSSE(adkCtx, agent, userMessage, chatReq.SessionID, userID, auth, requestID, attachments)
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		p.logger.Error("ADK agent connection failed after retries",
			zap.String("agent_id", agent.ID),
			zap.Error(lastErr))
		sse.SendRunError(fmt.Sprintf("failed to send message: %v", lastErr))
		return nil, lastErr
	}
	defer resp.Body.Close()

	p.logger.Debug("run_sse connected to ADK agent",
		zap.String("agent_id", agent.ID),
		zap.String("adk_session_id", adkSessionID))

	// Stream first response (emits SSE normally, detects context-limit)
	streamRes, err := p.streamADKResponse(resp, sse, agent)
	if err != nil {
		// If we got partial content before the error, return it for persistence
		if streamRes != nil && (streamRes.accumulated != "" || len(streamRes.toolCalls) > 0) {
			return &ProxyResult{
				AssistantText:  streamRes.accumulated,
				AgentSessionID: adkSessionID,
				ToolCalls:      streamRes.toolCalls,
				ContentParts:   streamRes.contentParts,
				Error:          err.Error(),
			}, err
		}
		return nil, err
	}

	// If no context-limit, we're done (SSE already emitted in streaming)
	if streamRes.contextLimitSummary == "" {
		sse.SendRunFinished()
		return &ProxyResult{
			AssistantText:  streamRes.accumulated,
			AgentSessionID: adkSessionID,
			ToolCalls:      streamRes.toolCalls,
			ContentParts:   streamRes.contentParts,
		}, nil
	}

	// Context-limit detected: rotate session and retry
	_, rotationSpan := tracing.Tracer().Start(adkCtx, "adk.session_rotation",
		trace.WithAttributes(
			attribute.String("agent.id", agent.ID),
			attribute.String("old_session_id", adkSessionID),
			attribute.Int("summary_len", len(streamRes.contextLimitSummary)),
		),
	)

	p.logger.Info("context-limit detected, rotating ADK session",
		zap.String("agent_id", agent.ID),
		zap.String("old_session_id", adkSessionID),
		zap.Int("summary_len", len(streamRes.contextLimitSummary)),
		zap.String("summary_preview", truncateStr(streamRes.contextLimitSummary, 300)))

	rotationSpan.AddEvent("context_limit_summary", trace.WithAttributes(
		attribute.String("summary", streamRes.contextLimitSummary),
	))

	// Emit informational message to the frontend
	if err := sse.SendTextMessageStart(); err != nil {
		rotationSpan.RecordError(err)
		rotationSpan.SetStatus(codes.Error, err.Error())
		rotationSpan.End()
		return nil, err
	}
	agentName := agent.Name
	if agentName == "" {
		agentName = agent.ID
	}
	var infoMsg string
	if locale == "es" {
		infoMsg = fmt.Sprintf("\u23f3 Session limit reached for agent %s. Resuming with context...\n\n", agentName)
	} else {
		infoMsg = fmt.Sprintf("\u23f3 Session limit reached for agent %s. Resuming with context...\n\n", agentName)
	}
	if err := sse.SendTextMessageContent(infoMsg); err != nil {
		rotationSpan.RecordError(err)
		rotationSpan.SetStatus(codes.Error, err.Error())
		rotationSpan.End()
		return nil, err
	}
	if err := sse.SendTextMessageEnd(); err != nil {
		rotationSpan.RecordError(err)
		rotationSpan.SetStatus(codes.Error, err.Error())
		rotationSpan.End()
		return nil, err
	}

	// Create new session (use adkCtx — decoupled from frontend)
	newSessionID, err := p.client.CreateSession(adkCtx, agent, userID, auth, requestID)
	if err != nil {
		p.logger.Error("failed to create new ADK session for context-limit retry",
			zap.String("agent_id", agent.ID),
			zap.Error(err))
		sse.SendRunError(fmt.Sprintf("failed to create new session: %v", err))
		rotationSpan.RecordError(err)
		rotationSpan.SetStatus(codes.Error, err.Error())
		rotationSpan.End()
		return nil, err
	}

	rotationSpan.SetAttributes(attribute.String("new_session_id", newSessionID))

	p.logger.Info("new ADK session created for retry",
		zap.String("agent_id", agent.ID),
		zap.String("new_session_id", newSessionID))

	// Build retry message: summary + original user question
	retryMessage := streamRes.contextLimitSummary + "\n\n" + userMessage

	rotationSpan.AddEvent("retry_message", trace.WithAttributes(
		attribute.String("body", retryMessage),
	))

	p.logger.Info("sending retry with context",
		zap.String("agent_id", agent.ID),
		zap.String("new_session_id", newSessionID),
		zap.Int("retry_message_len", len(retryMessage)),
		zap.String("retry_message_preview", truncateStr(retryMessage, 500)))

	// Send retry request (use adkCtx — decoupled from frontend) — no attachments on retry
	retryResp, _, err := p.client.RunSSE(adkCtx, agent, retryMessage, newSessionID, userID, auth, requestID, nil)
	if err != nil {
		p.logger.Error("failed to send retry run_sse to ADK agent",
			zap.String("agent_id", agent.ID),
			zap.Error(err))
		sse.SendRunError(fmt.Sprintf("failed to retry message: %v", err))
		rotationSpan.RecordError(err)
		rotationSpan.SetStatus(codes.Error, err.Error())
		rotationSpan.End()
		return nil, err
	}
	defer retryResp.Body.Close()

	// Stream retry response (also detects context-limit)
	retryRes, err := p.streamADKResponse(retryResp, sse, agent)
	if err != nil {
		rotationSpan.RecordError(err)
		rotationSpan.SetStatus(codes.Error, err.Error())
		rotationSpan.End()
		if retryRes != nil && (retryRes.accumulated != "" || len(retryRes.toolCalls) > 0) {
			return &ProxyResult{
				AssistantText:  retryRes.accumulated,
				AgentSessionID: newSessionID,
				ToolCalls:      retryRes.toolCalls,
				ContentParts:   retryRes.contentParts,
				SessionRotated: true,
				Error:          err.Error(),
			}, err
		}
		return nil, err
	}

	// If retry also hit context-limit, emit error to frontend
	if retryRes.contextLimitSummary != "" {
		p.logger.Warn("context-limit detected on retry, giving up",
			zap.String("agent_id", agent.ID),
			zap.String("new_session_id", newSessionID))
		rotationSpan.SetAttributes(attribute.Bool("retry_context_limit", true))
		rotationSpan.End()

		if err := sse.SendTextMessageStart(); err != nil {
			return nil, err
		}
		var errMsg string
		if locale == "es" {
			errMsg = "❌ Agent context limit reached again. Please start a new conversation."
		} else {
			errMsg = "❌ Agent context limit reached again. Please start a new conversation."
		}
		sse.SendTextMessageContent(errMsg)
		sse.SendTextMessageEnd()
		sse.SendRunFinished()
		return &ProxyResult{
			AssistantText:  retryRes.accumulated,
			AgentSessionID: newSessionID,
			ToolCalls:      retryRes.toolCalls,
			ContentParts:   retryRes.contentParts,
			SessionRotated: true,
		}, nil
	}

	rotationSpan.End()
	sse.SendRunFinished()

	return &ProxyResult{
		AssistantText:  retryRes.accumulated,
		AgentSessionID: newSessionID,
		ToolCalls:      retryRes.toolCalls,
		ContentParts:   retryRes.contentParts,
		SessionRotated: true,
	}, nil
}

// streamADKResponse reads an ADK SSE stream and processes events.
// It always detects context-limit summaries: if the first text starts with <context-limit-summary>,
// no output is emitted to the client. Otherwise, events are streamed normally via SSE.
// The caller (Handle) is responsible for SendRunFinished.
func (p *ADKProxy) streamADKResponse(resp *http.Response, sse *SSEWriter, agent *models.Agent) (*adkStreamResult, error) {
	messageStarted := false
	var accumulated strings.Builder
	contextLimitMode := false
	var contextLimitBuffer strings.Builder

	// Track active tool calls by ID (not by name).
	// ADK agents may call the same tool multiple times in one run.
	activeToolCalls := make(map[string]struct{})
	activeToolCallsByName := make(map[string][]string)

	// Capture tool calls and content parts
	var toolCalls []CapturedToolCall
	toolCallMap := make(map[string]int)
	var contentParts []ContentPart
	var currentText strings.Builder

	flushText := func() {
		if currentText.Len() > 0 {
			contentParts = append(contentParts, ContentPart{Type: "text", Text: currentText.String()})
			currentText.Reset()
		}
	}

	markToolCallStarted := func(toolName, toolCallID string) {
		activeToolCalls[toolCallID] = struct{}{}
		if toolName != "" {
			activeToolCallsByName[toolName] = append(activeToolCallsByName[toolName], toolCallID)
		}
	}

	resolveToolCallIDFromName := func(toolName string) string {
		ids := activeToolCallsByName[toolName]
		if len(ids) == 0 {
			return ""
		}
		toolCallID := ids[0]
		if len(ids) == 1 {
			delete(activeToolCallsByName, toolName)
		} else {
			activeToolCallsByName[toolName] = ids[1:]
		}
		return toolCallID
	}

	markToolCallCompleted := func(toolName, toolCallID string) {
		delete(activeToolCalls, toolCallID)
		if toolName == "" {
			return
		}
		ids := activeToolCallsByName[toolName]
		if len(ids) == 0 {
			return
		}
		// Remove first matching ID if present (or trim stale entries).
		for i, id := range ids {
			if id == toolCallID {
				activeToolCallsByName[toolName] = append(ids[:i], ids[i+1:]...)
				if len(activeToolCallsByName[toolName]) == 0 {
					delete(activeToolCallsByName, toolName)
				}
				return
			}
		}
	}

	reader := bufio.NewReader(resp.Body)
	eventsReceived := 0
	// Track whether any partial=true text was emitted since the last tool call.
	// When true, partial=false events are duplicates and must be skipped.
	// Reset after tool calls so that a non-streamed response after tool execution is emitted.
	hadPartialText := false

	for {
		line, err := reader.ReadString('\n')
		// Process any received line even when ReadString returns EOF/UnexpectedEOF.
		// Some ADK servers close chunked streams without a trailing newline.
		if err != nil && line == "" {
			if !contextLimitMode && messageStarted {
				sse.SendTextMessageEnd()
			}
			isEOF := err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) ||
				strings.Contains(err.Error(), "unexpected EOF") ||
				strings.Contains(err.Error(), "context canceled")

			// Context-limit mode: we have the summary, stream is done.
			if isEOF && contextLimitMode {
				flushText()
				return &adkStreamResult{
					accumulated:         accumulated.String(),
					toolCalls:           toolCalls,
					contentParts:        contentParts,
					messageStarted:      messageStarted,
					contextLimitSummary: extractSummaryContent(contextLimitBuffer.String()),
				}, nil
			}

			// Stream interrupted with accumulated content: return what we have
			// (already streamed to frontend) but log warning about truncation.
			if isEOF && (accumulated.Len() > 0 || eventsReceived > 0) {
				p.logger.Warn("ADK stream ended without turnComplete — response may be truncated",
					zap.String("agent_id", agent.ID),
					zap.Int("events_received", eventsReceived),
					zap.Int("accumulated_len", accumulated.Len()),
					zap.Error(err))
				flushText()
				return &adkStreamResult{
					accumulated:    accumulated.String(),
					toolCalls:      toolCalls,
					contentParts:   contentParts,
					messageStarted: messageStarted,
				}, nil
			}

			// No content at all — real error
			p.logger.Error("ADK stream error",
				zap.String("agent_id", agent.ID),
				zap.Int("events_received", eventsReceived),
				zap.Error(err))
			sse.SendRunError("stream ended unexpectedly")
			return nil, fmt.Errorf("stream error: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// ADK sends errors as plain text (not SSE format)
		if !strings.HasPrefix(line, "data:") {
			p.logger.Warn("ADK non-SSE line received",
				zap.String("agent_id", agent.ID),
				zap.String("line", line))
			if strings.Contains(line, "Error") || strings.Contains(line, "error") {
				if !contextLimitMode && messageStarted {
					sse.SendTextMessageEnd()
					messageStarted = false
				}
				sse.SendRunError(line)
				errMsg := fmt.Sprintf("ADK agent error: %s", line)
				if accumulated.Len() > 0 || len(toolCalls) > 0 {
					flushText()
					return &adkStreamResult{
						accumulated:    accumulated.String(),
						toolCalls:      toolCalls,
						contentParts:   contentParts,
						messageStarted: messageStarted,
					}, fmt.Errorf("%s", errMsg)
				}
				return nil, fmt.Errorf("%s", errMsg)
			}
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}

		var event adk.Event
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			p.logger.Warn("failed to parse ADK SSE event",
				zap.String("agent_id", agent.ID),
				zap.String("data", data),
				zap.Error(err))
			continue
		}

		eventsReceived++

		// Log all events for diagnosis
		p.logger.Debug("ADK event received",
			zap.String("agent_id", agent.ID),
			zap.String("author", event.Author),
			zap.String("invocation_id", event.InvocationID),
			zap.Bool("partial", event.Partial),
			zap.Bool("turn_complete", event.TurnComplete),
			zap.Bool("has_content", event.Content != nil),
			zap.Int("accumulated_len", accumulated.Len()),
			zap.Bool("had_partial_text", hadPartialText))

		// Handle error events
		if event.ErrorCode != "" || event.ErrorMessage != "" {
			if !contextLimitMode && messageStarted {
				sse.SendTextMessageEnd()
				messageStarted = false
			}
			errMsg := event.ErrorMessage
			if errMsg == "" {
				errMsg = event.ErrorCode
			}
			sse.SendRunError(errMsg)
			fullErr := fmt.Sprintf("ADK agent error: %s: %s", event.ErrorCode, event.ErrorMessage)
			if accumulated.Len() > 0 || len(toolCalls) > 0 {
				flushText()
				return &adkStreamResult{
					accumulated:    accumulated.String(),
					toolCalls:      toolCalls,
					contentParts:   contentParts,
					messageStarted: messageStarted,
				}, fmt.Errorf("%s", fullErr)
			}
			return nil, fmt.Errorf("%s", fullErr)
		}

		if event.Content == nil {
			// turnComplete with no content and no pending tool calls = done
			if event.TurnComplete && len(activeToolCalls) == 0 {
				if !contextLimitMode && messageStarted {
					sse.SendTextMessageEnd()
				}
				flushText()
				summary := ""
				if contextLimitMode {
					summary = extractSummaryContent(contextLimitBuffer.String())
				}
				return &adkStreamResult{
					accumulated:         accumulated.String(),
					toolCalls:           toolCalls,
					contentParts:        contentParts,
					messageStarted:      messageStarted,
					contextLimitSummary: summary,
				}, nil
			}
			continue
		}

		// Determine if this is from an intermediate agent (pipeline thinking)
		isIntermediateAgent := agent.PipelineFinalAgent != "" && event.Author != "" && event.Author != agent.PipelineFinalAgent

		for _, part := range event.Content.Parts {
			if part == nil {
				continue
			}

			// Handle function calls -> TOOL_CALL_START + TOOL_CALL_ARGS
			if part.FunctionCall != nil {
				fc := part.FunctionCall
				toolCallID := fc.ID
				if toolCallID == "" {
					toolCallID = uuid.New().String()
				}

				if !contextLimitMode {
					// Close any open text message before tool call
					if messageStarted {
						if err := sse.SendTextMessageEnd(); err != nil {
							return nil, err
						}
						messageStarted = false
					}

					if err := sse.SendToolCallStart(toolCallID, fc.Name); err != nil {
						return nil, err
					}
				}

				var argsStr string
				if fc.Args != nil {
					argsJSON, _ := json.Marshal(fc.Args)
					argsStr = string(argsJSON)
					if !contextLimitMode {
						if err := sse.SendToolCallArgs(toolCallID, argsStr); err != nil {
							return nil, err
						}
					}
				}

				// Capture tool call for session persistence
				flushText()
				idx := len(toolCalls)
				toolCalls = append(toolCalls, CapturedToolCall{
					ID:   toolCallID,
					Name: fc.Name,
					Args: argsStr,
				})
				toolCallMap[toolCallID] = idx
				contentParts = append(contentParts, ContentPart{Type: "tool_use", ToolIndex: idx})

				markToolCallStarted(fc.Name, toolCallID)
				// Reset partial tracker: text after tool calls is a new segment
				hadPartialText = false
				continue
			}

			// Handle function responses -> TOOL_CALL_END
			if part.FunctionResponse != nil {
				fr := part.FunctionResponse
				toolCallID := fr.ID
				if toolCallID == "" {
					if id := resolveToolCallIDFromName(fr.Name); id != "" {
						toolCallID = id
					} else {
						toolCallID = uuid.New().String()
					}
				}

				resultJSON, _ := json.Marshal(fr.Response)
				resultStr := string(resultJSON)
				if !contextLimitMode {
					if err := sse.SendToolCallEnd(toolCallID, resultStr); err != nil {
						return nil, err
					}
				}

				// Capture tool result for session persistence
				if idx, ok := toolCallMap[toolCallID]; ok {
					toolCalls[idx].Result = resultStr
				}

				markToolCallCompleted(fr.Name, toolCallID)
				continue
			}

			// Handle text content
			if part.Text != "" {
				// Check for context-limit summary tag at the start of first text
				if !contextLimitMode && strings.HasPrefix(strings.TrimSpace(part.Text), contextLimitOpenTag) {
					contextLimitMode = true
					contextLimitBuffer.WriteString(part.Text)
					continue
				}

				// If in context-limit mode, keep buffering
				if contextLimitMode {
					contextLimitBuffer.WriteString(part.Text)
					continue
				}

				// Thought parts or intermediate agent text -> emit as thinking
				if part.Thought || isIntermediateAgent {
					if err := p.emitThinking(sse, part.Text, &messageStarted); err != nil {
						return nil, err
					}
					continue
				}

				// partial=true: streaming chunk — always emit.
				// partial=false: final accumulated event — only emit if no
				// partial=true text was sent since the last tool call, which
				// means this is new text the proxy hasn't streamed yet.
				if event.Partial {
					if !messageStarted {
						if err := sse.SendTextMessageStart(); err != nil {
							return nil, err
						}
						messageStarted = true
					}
					if err := sse.SendTextMessageContent(part.Text); err != nil {
						return nil, err
					}
					accumulated.WriteString(part.Text)
					currentText.WriteString(part.Text)
					hadPartialText = true
				} else if !hadPartialText {
					// No partial=true text was emitted for this segment
					// (e.g. after tool calls). Emit the non-streamed response.
					p.logger.Debug("ADK emitting non-streamed text",
						zap.String("agent_id", agent.ID),
						zap.Int("text_len", len(part.Text)),
						zap.String("text_preview", truncateStr(part.Text, 200)))
					if !messageStarted {
						if err := sse.SendTextMessageStart(); err != nil {
							return nil, err
						}
						messageStarted = true
					}
					if err := sse.SendTextMessageContent(part.Text); err != nil {
						return nil, err
					}
					accumulated.WriteString(part.Text)
					currentText.WriteString(part.Text)
				} else {
					// partial=false after partial=true: skip (already streamed)
					p.logger.Debug("ADK skipping duplicate accumulated text",
						zap.String("agent_id", agent.ID),
						zap.Int("text_len", len(part.Text)))
				}
			}
		}

		// Do NOT exit here based on turnComplete with content.
		// In ADK, turnComplete=true on a text event means "this text segment is done",
		// but the agent may continue with tool calls. The only reliable exit with content
		// is when the stream closes (EOF). The Content==nil + turnComplete check above
		// handles the explicit "done" signal.
	}
}

// extractSummaryContent extracts the text between <context-limit-summary> tags.
func extractSummaryContent(text string) string {
	startIdx := strings.Index(text, contextLimitOpenTag)
	if startIdx == -1 {
		return ""
	}
	startIdx += len(contextLimitOpenTag)

	endIdx := strings.Index(text, contextLimitCloseTag)
	if endIdx == -1 {
		// No closing tag — return everything after the opening tag
		return strings.TrimSpace(text[startIdx:])
	}

	return strings.TrimSpace(text[startIdx:endIdx])
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// emitThinking sends thinking text as its own TEXT_MESSAGE sequence.
func (p *ADKProxy) emitThinking(sse *SSEWriter, text string, messageStarted *bool) error {
	if *messageStarted {
		if err := sse.SendTextMessageEnd(); err != nil {
			return err
		}
		*messageStarted = false
	}
	if err := sse.SendTextMessageStartThinking(); err != nil {
		return err
	}
	if err := sse.SendTextMessageContent(text); err != nil {
		return err
	}
	return sse.SendTextMessageEnd()
}

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/langfuse"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// DefaultMaxToolCallRounds is the default maximum number of LLM ↔ tool iterations.
const DefaultMaxToolCallRounds = 10

// RunResult contains metrics about the RunLoop execution.
type RunResult struct {
	AssistantText string
	ToolCalls     []ToolCallMetric
	TokenUsage    *TokenUsageMetric
	Rounds        int
	Error         string
}

// ToolCallMetric holds analytics about a single tool call.
type ToolCallMetric struct {
	Name       string
	DurationMs int
}

// TokenUsageMetric holds accumulated token usage.
type TokenUsageMetric struct {
	Input  int
	Output int
	Total  int
}

// ToolHandler provides resolution and execution of tool calls.
type ToolHandler struct {
	// Resolve returns display metadata for a tool call.
	// displayName is the name shown in SSE events and persisted in the session.
	// serverID is the MCP server ID (empty for non-MCP tools).
	// If skip is true, the tool call is ignored (e.g., unknown tool).
	Resolve func(tc LLMToolCall) (displayName, serverID string, skip bool)

	// Execute runs the tool call and returns result text.
	Execute func(ctx context.Context, tc LLMToolCall) string
}

// LoopParams configures the LLM + tool calling loop.
type LoopParams struct {
	SessionID         string
	SessionStore      store.SessionStore
	LLMClient         *LLMClient
	Tools             []Tool
	Messages          []LLMMessage // Already converted to LLM format
	Handler           ToolHandler
	Parallel          bool         // Execute tool calls in parallel (true) or sequentially (false)
	MaxToolCallRounds int          // Max LLM ↔ tool iterations. 0 uses DefaultMaxToolCallRounds.
	Logger            *zap.Logger

	// OnStart is called after RUN_STARTED is emitted and before the tool loop begins.
	// Use this to emit additional SSE events (e.g., warnings about disconnected servers).
	OnStart func(w http.ResponseWriter, flusher http.Flusher)
}

// RunLoop executes the LLM + tool calling loop, emitting AG-UI SSE events.
//
// Callers are responsible for:
//   - Saving the user message to the session store (before calling RunLoop)
//   - Converting chat messages to LLM format (LoopParams.Messages)
//
// RunLoop handles:
//   - SSE setup and RUN_STARTED/RUN_FINISHED lifecycle events
//   - The tool-calling loop (LLM call → tool execution → feed back → repeat)
//   - Deferred persistence of the assistant message (including tool calls)
func RunLoop(ctx context.Context, w http.ResponseWriter, params LoopParams) (*RunResult, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}

	// Metrics tracking
	var toolMetrics []ToolCallMetric
	var tokenMetrics TokenUsageMetric
	SetupSSEHeaders(w)

	sessionID := params.SessionID
	runID := uuid.New().String()
	WriteSSE(w, flusher, models.NewAGUIRunStartedEvent(sessionID, runID))

	if params.OnStart != nil {
		params.OnStart(w, flusher)
	}

	llmMessages := params.Messages
	var finalText string
	var storedToolCalls []models.StoredToolCall
	var storedToolResults []models.StoredToolResult
	var contentParts []models.ContentPart

	// Defer save: persist whatever we have even if the client disconnects mid-stream
	defer func() {
		if params.SessionStore == nil {
			return
		}
		if finalText == "" && len(storedToolCalls) == 0 {
			return
		}
		saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer saveCancel()
		msg := models.ChatMessage{
			Role:         "assistant",
			Content:      finalText,
			ToolCalls:    storedToolCalls,
			ToolResults:  storedToolResults,
			ContentParts: contentParts,
		}
		if err := params.SessionStore.AddMessage(saveCtx, sessionID, msg); err != nil && params.Logger != nil {
			params.Logger.Warn("failed to persist assistant message", zap.Error(err))
		}
	}()

	maxRounds := params.MaxToolCallRounds
	if maxRounds <= 0 {
		maxRounds = DefaultMaxToolCallRounds
	}

	lastRound := 0
	for round := 0; round < maxRounds; round++ {
		lastRound = round
		resp, err := params.LLMClient.Chat(ctx, llmMessages, params.Tools)
		if err != nil {
			WriteSSE(w, flusher, models.NewAGUIRunErrorEvent(err.Error()))
			return &RunResult{AssistantText: finalText, ToolCalls: toolMetrics, TokenUsage: &tokenMetrics, Rounds: round + 1, Error: err.Error()}, err
		}

		// Accumulate token usage from LLM response
		if resp.InputTokens > 0 || resp.OutputTokens > 0 {
			tokenMetrics.Input += resp.InputTokens
			tokenMetrics.Output += resp.OutputTokens
			tokenMetrics.Total += resp.InputTokens + resp.OutputTokens
		}

		// Emit text before tool calls if present
		if resp.Text != "" && len(resp.ToolCalls) > 0 {
			contentParts = append(contentParts, models.ContentPart{Type: "text", Text: resp.Text})
			emitTextMessage(w, flusher, resp.Text)
		}

		// No tool calls → emit final text and finish
		if len(resp.ToolCalls) == 0 {
			if resp.Text != "" {
				finalText = resp.Text
				contentParts = append(contentParts, models.ContentPart{Type: "text", Text: resp.Text})
				emitTextMessage(w, flusher, resp.Text)
			}
			break
		}

		// Resolve and filter tool calls
		type resolvedTC struct {
			tc          LLMToolCall
			displayName string
			serverID    string
			toolCallID  string
		}
		var resolved []resolvedTC
		for _, tc := range resp.ToolCalls {
			displayName, serverID, skip := params.Handler.Resolve(tc)
			if skip {
				continue
			}
			resolved = append(resolved, resolvedTC{
				tc:          tc,
				displayName: displayName,
				serverID:    serverID,
				toolCallID:  uuid.New().String(),
			})
		}

		// Execute tool calls and collect results
		results := make([]string, len(resolved))

		if params.Parallel {
			// Parallel: emit all START+ARGS upfront, execute concurrently, emit END as each completes
			for _, r := range resolved {
				emitToolCallStart(w, flusher, r.toolCallID, r.displayName, r.serverID, r.tc.Arguments)
			}

			type execResultWithDur struct {
				index      int
				resultText string
				durationMs int
			}

			var wg sync.WaitGroup
			doneCh := make(chan execResultWithDur, len(resolved))
			lfTrace := langfuse.TraceFromContext(ctx)
			for i, r := range resolved {
				wg.Add(1)
				go func(idx int, r resolvedTC) {
					defer wg.Done()
					var lfSpan *langfuse.Span
					if lfTrace != nil {
						lfSpan = lfTrace.StartToolCall(r.displayName, r.tc.Arguments)
					}
					tcStart := time.Now()
					resultText := params.Handler.Execute(ctx, r.tc)
					if lfSpan != nil {
						lfSpan.End(resultText)
					}
					doneCh <- execResultWithDur{index: idx, resultText: resultText, durationMs: int(time.Since(tcStart).Milliseconds())}
				}(i, r)
			}

			// Emit TOOL_CALL_END as each tool finishes
			for range resolved {
				done := <-doneCh
				results[done.index] = done.resultText
				r := resolved[done.index]
				toolMetrics = append(toolMetrics, ToolCallMetric{Name: r.displayName, DurationMs: done.durationMs})
				WriteSSE(w, flusher, &models.AGUIToolCallEndEvent{
					Type:       models.AGUIEventToolCallEnd,
					ToolCallID: r.toolCallID,
					Result:     done.resultText,
				})
			}
			wg.Wait()
		} else {
			// Sequential: emit START+ARGS, execute, emit END per tool
			lfTrace := langfuse.TraceFromContext(ctx)
			for i, r := range resolved {
				emitToolCallStart(w, flusher, r.toolCallID, r.displayName, r.serverID, r.tc.Arguments)
				var lfSpan *langfuse.Span
				if lfTrace != nil {
					lfSpan = lfTrace.StartToolCall(r.displayName, r.tc.Arguments)
				}
				tcStart := time.Now()
				resultText := params.Handler.Execute(ctx, r.tc)
				if lfSpan != nil {
					lfSpan.End(resultText)
				}
				tcDur := int(time.Since(tcStart).Milliseconds())
				results[i] = resultText
				toolMetrics = append(toolMetrics, ToolCallMetric{Name: r.displayName, DurationMs: tcDur})
				WriteSSE(w, flusher, &models.AGUIToolCallEndEvent{
					Type:       models.AGUIEventToolCallEnd,
					ToolCallID: r.toolCallID,
					Result:     resultText,
				})
			}
		}

		// Persist tool calls and append to LLM conversation
		for i, r := range resolved {
			resultText := results[i]

			toolIdx := len(storedToolCalls)
			contentParts = append(contentParts, models.ContentPart{Type: "tool_use", ToolIndex: models.IntPtr(toolIdx)})
			storedToolCalls = append(storedToolCalls, models.StoredToolCall{
				ID:   r.toolCallID,
				Name: r.displayName,
				Args: r.tc.Arguments,
			})
			var parsed map[string]interface{}
			if json.Unmarshal([]byte(resultText), &parsed) != nil {
				parsed = map[string]interface{}{"text": resultText}
			}
			storedToolResults = append(storedToolResults, models.StoredToolResult{
				ID:       r.toolCallID,
				Name:     r.displayName,
				Response: parsed,
			})

			llmMessages = append(llmMessages, LLMMessage{
				Role: "assistant",
				Content: []map[string]interface{}{
					{
						"type":  "tool_use",
						"id":    r.tc.ID,
						"name":  r.tc.Name,
						"input": r.tc.Arguments,
					},
				},
			})
			llmMessages = append(llmMessages, LLMMessage{
				Role:       "user",
				ToolCallID: r.tc.ID,
				ToolName:   r.tc.Name,
				Content: []map[string]interface{}{
					{
						"type":        "tool_result",
						"tool_use_id": r.tc.ID,
						"content":     resultText,
					},
				},
			})
		}
	}

	if lastRound == maxRounds-1 && params.Logger != nil {
		params.Logger.Warn("max tool call rounds reached", zap.Int("max", maxRounds))
	}

	WriteSSE(w, flusher, models.NewAGUIRunFinishedEvent(sessionID, runID))
	return &RunResult{
		AssistantText: finalText,
		ToolCalls:     toolMetrics,
		TokenUsage:    &tokenMetrics,
		Rounds:        lastRound + 1,
	}, nil
}

// ResolveModel finds the LLM model by ID, falling back to the default chat model.
func ResolveModel(ctx context.Context, llmRepo repository.LLMModelRepository, modelID string) (*models.LLMModel, error) {
	if modelID != "" {
		m, err := llmRepo.Get(ctx, modelID)
		if err == nil && m.Enabled {
			return m, nil
		}
	}
	chatModels, err := llmRepo.ListByRole(ctx, "chat")
	if err != nil {
		return nil, fmt.Errorf("list chat models: %w", err)
	}
	for _, m := range chatModels {
		if m.IsDefault {
			return m, nil
		}
	}
	if len(chatModels) > 0 {
		return chatModels[0], nil
	}
	return nil, fmt.Errorf("no chat models configured")
}

// SetupSSEHeaders sets the standard SSE response headers.
func SetupSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}

// WriteSSE marshals an event to JSON and writes it as an SSE data frame.
func WriteSSE(w http.ResponseWriter, flusher http.Flusher, event interface{}) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// ConvertToLLMMessages converts ChatMessages to LLMMessages, handling image attachments
// as multipart content blocks for LLMs with vision support.
func ConvertToLLMMessages(messages []models.ChatMessage) []LLMMessage {
	llmMessages := make([]LLMMessage, 0, len(messages))
	for _, m := range messages {
		if m.Role == "system" {
			continue
		}
		if len(m.Attachments) > 0 && m.Role == "user" {
			var contentBlocks []map[string]interface{}
			for _, att := range m.Attachments {
				if isImageContentType(att.ContentType) {
					contentBlocks = append(contentBlocks, map[string]interface{}{
						"type": "image",
						"source": map[string]string{
							"type":       "base64",
							"media_type": att.ContentType,
							"data":       att.Data,
						},
					})
				}
			}
			if m.Content != "" {
				contentBlocks = append(contentBlocks, map[string]interface{}{
					"type": "text",
					"text": m.Content,
				})
			}
			llmMessages = append(llmMessages, LLMMessage{
				Role:    m.Role,
				Content: contentBlocks,
			})
		} else {
			llmMessages = append(llmMessages, LLMMessage{
				Role:    m.Role,
				Content: m.Content,
			})
		}
	}
	return llmMessages
}

// emitTextMessage emits a complete TEXT_MESSAGE (start + content + end) via SSE.
func emitTextMessage(w http.ResponseWriter, flusher http.Flusher, text string) {
	msgID := uuid.New().String()
	WriteSSE(w, flusher, models.NewAGUITextMessageStartEvent(msgID))
	WriteSSE(w, flusher, models.NewAGUITextMessageContentEvent(msgID, text))
	WriteSSE(w, flusher, models.NewAGUITextMessageEndEvent(msgID))
}

// emitToolCallStart emits TOOL_CALL_START and TOOL_CALL_ARGS events.
func emitToolCallStart(w http.ResponseWriter, flusher http.Flusher, toolCallID, toolName, serverID string, args map[string]interface{}) {
	WriteSSE(w, flusher, &models.AGUIToolCallStartEvent{
		Type:       models.AGUIEventToolCallStart,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		ServerID:   serverID,
	})
	argsJSON, _ := json.Marshal(args)
	WriteSSE(w, flusher, &models.AGUIToolCallArgsEvent{
		Type:       models.AGUIEventToolCallArgs,
		ToolCallID: toolCallID,
		Delta:      string(argsJSON),
	})
}

// isImageContentType checks if a content type is an image MIME type (case-insensitive per RFC 2045).
func isImageContentType(ct string) bool {
	return len(ct) >= 6 && strings.EqualFold(ct[:6], "image/")
}

package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/dfradehubs/agentgram-api/internal/langfuse"
	"github.com/dfradehubs/agentgram-api/internal/llm"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ChatMessage is used in chat request bodies
type ChatMessage = models.ChatMessage

// ChatOrchestrator handles the LLM + tool calling loop for MCP chat
type ChatOrchestrator struct {
	llmRepo           repository.LLMModelRepository
	maxToolCallRounds int
	logger            *zap.Logger
}

// NewChatOrchestrator creates a new chat orchestrator.
// maxToolCallRounds sets the limit for LLM ↔ tool iterations (0 uses default).
func NewChatOrchestrator(llmRepo repository.LLMModelRepository, maxToolCallRounds int, logger *zap.Logger) *ChatOrchestrator {
	return &ChatOrchestrator{
		llmRepo:           llmRepo,
		maxToolCallRounds: maxToolCallRounds,
		logger:            logger,
	}
}

// ChatRequest represents the request body for MCP chat
type ChatRequest struct {
	Messages  []ChatMessage `json:"messages"`
	ModelID   string        `json:"model_id"`
	SessionID string        `json:"session_id,omitempty"`
}

// MultiChatRequest represents the request body for multi-MCP chat
type MultiChatRequest struct {
	Messages  []ChatMessage `json:"messages"`
	ModelID   string        `json:"model_id"`
	ServerIDs []string      `json:"server_ids"`
	SessionID string        `json:"session_id,omitempty"`
}

// Chat handles the full chat loop: LLM -> tool calls -> LLM -> ... -> final response
// Emits AG-UI SSE events to the ResponseWriter.
// extraHeaders are passed to MCP tool calls (used for forward_auth servers).
func (o *ChatOrchestrator) Chat(ctx context.Context, w http.ResponseWriter, server *ServerInfo, req *ChatRequest, sessionID string, sessionStore store.SessionStore, extraHeaders map[string]string) (*RunResult, error) {
	// Check server connectivity before proceeding
	status, statusErr := server.GetStatus()
	if status != "connected" {
		flusher, ok := w.(http.Flusher)
		if !ok {
			return nil, fmt.Errorf("streaming not supported")
		}
		SetupSSEHeaders(w)
		errMsg := fmt.Sprintf("MCP server '%s' not connected", server.Config.Name)
		if statusErr != "" {
			errMsg += ": " + statusErr
		}
		WriteSSE(w, flusher, models.NewAGUIRunErrorEvent(errMsg))
		return nil, fmt.Errorf("%s", errMsg)
	}

	modelCfg, err := ResolveModel(ctx, o.llmRepo, req.ModelID)
	if err != nil {
		return nil, err
	}

	// Save user message to session
	if sessionStore != nil && len(req.Messages) > 0 {
		lastMsg := req.Messages[len(req.Messages)-1]
		if lastMsg.Role == "user" {
			_ = sessionStore.AddMessage(ctx, sessionID, lastMsg)
		}
	}

	handler := ToolHandler{
		Resolve: func(tc LLMToolCall) (string, string, bool) {
			return tc.Name, "", false
		},
		Execute: func(ctx context.Context, tc LLMToolCall) string {
			result, err := server.Client.CallToolWithHeaders(ctx, tc.Name, tc.Arguments, extraHeaders)
			if err != nil {
				o.logger.Warn("MCP tool call failed", zap.String("tool", tc.Name), zap.Error(err))
				return fmt.Sprintf("Error: %v", err)
			}
			var text string
			for _, c := range result.Content {
				if c.Type == "text" {
					text += c.Text
				}
			}
			return text
		},
	}

	return RunLoop(ctx, w, LoopParams{
		SessionID:         sessionID,
		SessionStore:      sessionStore,
		LLMClient:         newTracedLLMClient(modelCfg, "mcp-chat"),
		Tools:             server.GetTools(),
		Messages:          ConvertToLLMMessages(req.Messages),
		Handler:           handler,
		Parallel:          false,
		MaxToolCallRounds: o.maxToolCallRounds,
		Logger:            o.logger,
	})
}

// ChatMulti handles multi-server MCP chat: combines tools from N servers, routes tool calls
func (o *ChatOrchestrator) ChatMulti(ctx context.Context, w http.ResponseWriter, servers []*ServerInfo, req *MultiChatRequest, sessionID string, sessionStore store.SessionStore, extraHeaders map[string]string) (*RunResult, error) {
	// Separate connected vs disconnected servers
	var connectedServers []*ServerInfo
	var disconnectedNames []string
	for _, server := range servers {
		status, _ := server.GetStatus()
		if status == "connected" {
			connectedServers = append(connectedServers, server)
		} else {
			disconnectedNames = append(disconnectedNames, server.Config.Name)
		}
	}

	// If no servers are connected, emit error via SSE and return
	if len(connectedServers) == 0 {
		flusher, ok := w.(http.Flusher)
		if !ok {
			return nil, fmt.Errorf("streaming not supported")
		}
		SetupSSEHeaders(w)
		errMsg := "No MCP servers are connected"
		WriteSSE(w, flusher, models.NewAGUIRunErrorEvent(errMsg))
		return nil, fmt.Errorf("%s", errMsg)
	}

	modelCfg, err := ResolveModel(ctx, o.llmRepo, req.ModelID)
	if err != nil {
		return nil, err
	}

	// Combine tools from connected servers, prefix with serverID__
	var allTools []Tool
	toolServerMap := make(map[string]*ServerInfo) // prefixedName -> server
	for _, server := range connectedServers {
		for _, t := range server.GetTools() {
			prefixed := server.Config.ID + "__" + t.Name
			allTools = append(allTools, Tool{
				Name:        prefixed,
				Description: fmt.Sprintf("[%s] %s", server.Config.Name, t.Description),
				InputSchema: t.InputSchema,
			})
			toolServerMap[prefixed] = server
		}
	}

	// Save user message
	if sessionStore != nil && len(req.Messages) > 0 {
		lastMsg := req.Messages[len(req.Messages)-1]
		if lastMsg.Role == "user" {
			_ = sessionStore.AddMessage(ctx, sessionID, lastMsg)
		}
	}

	handler := ToolHandler{
		Resolve: func(tc LLMToolCall) (string, string, bool) {
			parts := strings.SplitN(tc.Name, "__", 2)
			if len(parts) == 2 {
				if _, ok := toolServerMap[tc.Name]; ok {
					return parts[1], parts[0], false
				}
			}
			o.logger.Warn("unknown tool server", zap.String("tool", tc.Name))
			return tc.Name, "", true
		},
		Execute: func(ctx context.Context, tc LLMToolCall) string {
			parts := strings.SplitN(tc.Name, "__", 2)
			targetServer := toolServerMap[tc.Name]
			if targetServer == nil || len(parts) < 2 {
				return "Error: unknown tool server"
			}
			realToolName := parts[1]
			serverID := parts[0]
			result, err := targetServer.Client.CallToolWithHeaders(ctx, realToolName, tc.Arguments, extraHeaders)
			if err != nil {
				o.logger.Warn("MCP tool call failed",
					zap.String("tool", realToolName),
					zap.String("server", serverID),
					zap.Error(err))
				return fmt.Sprintf("Error: %v", err)
			}
			var text string
			for _, c := range result.Content {
				if c.Type == "text" {
					text += c.Text
				}
			}
			return text
		},
	}

	var onStart func(http.ResponseWriter, http.Flusher)
	if len(disconnectedNames) > 0 {
		onStart = func(w http.ResponseWriter, flusher http.Flusher) {
			warnMsgID := uuid.New().String()
			warnText := fmt.Sprintf("⚠️ Disconnected servers: %s. Using tools from the available servers.", strings.Join(disconnectedNames, ", "))
			WriteSSE(w, flusher, models.NewAGUITextMessageStartEvent(warnMsgID))
			WriteSSE(w, flusher, models.NewAGUITextMessageContentEvent(warnMsgID, warnText))
			WriteSSE(w, flusher, models.NewAGUITextMessageEndEvent(warnMsgID))
		}
	}

	return RunLoop(ctx, w, LoopParams{
		SessionID:         sessionID,
		SessionStore:      sessionStore,
		LLMClient:         newTracedLLMClient(modelCfg, "mcp-chat-multi"),
		Tools:             allTools,
		Messages:          ConvertToLLMMessages(req.Messages),
		Handler:           handler,
		Parallel:          false,
		MaxToolCallRounds: o.maxToolCallRounds,
		Logger:            o.logger,
		OnStart:           onStart,
	})
}

// newTracedLLMClient creates an LLM client with Langfuse tracing wrapping.
// The TracedProvider checks for a trace in context at call time, so wrapping is safe even without an active trace.
func newTracedLLMClient(model *models.LLMModel, genName string) *LLMClient {
	provider, err := llm.NewProvider(model)
	if err != nil {
		return NewLLMClient(model)
	}
	traced := langfuse.WrapProvider(provider, genName, model.Model)
	return NewLLMClientWithProvider(traced)
}

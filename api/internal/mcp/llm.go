package mcp

import (
	"context"

	"github.com/dfradehubs/agentgram-api/internal/llm"
	"github.com/dfradehubs/agentgram-api/internal/models"
)

// LLMMessage is an alias for llm.Message used within the MCP package.
type LLMMessage = llm.Message

// LLMToolCall is an alias for llm.ToolCall used within the MCP package.
type LLMToolCall = llm.ToolCall

// LLMResponse is an alias for llm.Response used within the MCP package.
type LLMResponse = llm.Response

// LLMClient wraps an llm.Provider for backward compatibility within the MCP package.
type LLMClient struct {
	provider llm.Provider
}

// NewLLMClient creates a new LLM client for the given LLM model.
func NewLLMClient(model *models.LLMModel) *LLMClient {
	provider, err := llm.NewProvider(model)
	if err != nil {
		// Return a client that will error on Chat() calls
		return &LLMClient{}
	}
	return &LLMClient{provider: provider}
}

// NewLLMClientWithProvider creates an LLM client with a pre-configured provider (e.g. traced).
func NewLLMClientWithProvider(provider llm.Provider) *LLMClient {
	return &LLMClient{provider: provider}
}

// Chat sends messages to the LLM with tools and returns the response.
func (c *LLMClient) Chat(ctx context.Context, messages []LLMMessage, tools []Tool) (*LLMResponse, error) {
	// Convert MCP tools to llm.Tool
	llmTools := make([]llm.Tool, len(tools))
	for i, t := range tools {
		llmTools[i] = llm.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}

	return c.provider.GenerateContent(ctx, &llm.Request{
		Messages:  messages,
		Tools:     llmTools,
		MaxTokens: 4096,
	})
}

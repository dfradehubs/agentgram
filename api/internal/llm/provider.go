package llm

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/models"
)

// Message represents a message in an LLM conversation.
type Message struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content"` // string or []ContentBlock
	ToolCallID string      `json:"tool_call_id,omitempty"`
	ToolName   string      `json:"-"` // Function name for tool results (used by Gemini)
}

// Tool represents a tool/function definition for the LLM.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// ToolCall represents a tool invocation returned by the LLM.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]interface{}
}

// Request represents a request to an LLM provider.
type Request struct {
	Messages     []Message
	SystemPrompt string // Optional system prompt (Anthropic: separate field, OpenAI/Google: system message)
	Tools        []Tool // Optional tools for function calling
	MaxTokens    int
}

// Response represents a parsed LLM response.
type Response struct {
	Text         string
	ToolCalls    []ToolCall
	InputTokens  int // Token usage from the LLM (if available)
	OutputTokens int
}

// Provider is the common interface for LLM providers.
type Provider interface {
	GenerateContent(ctx context.Context, req *Request) (*Response, error)
}

// NewProvider creates a Provider for the given LLM model configuration.
func NewProvider(model *models.LLMModel) (Provider, error) {
	return NewProviderWithClient(model, nil)
}

// NewProviderWithClient creates a Provider with a custom HTTP client. If client is nil, a default client is used.
func NewProviderWithClient(model *models.LLMModel, client *http.Client) (Provider, error) {
	if model == nil || model.APIKey == "" {
		return nil, fmt.Errorf("invalid LLM model configuration")
	}

	if client == nil {
		// No global Timeout: it includes body read time, which causes
		// "Client.Timeout exceeded while awaiting headers" on long LLM
		// calls (large context, tool-use loops). Use transport-level
		// timeouts for connection, TLS, and first-header instead.
		client = &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 5 * time.Minute,
				IdleConnTimeout:       90 * time.Second,
			},
		}
	}

	switch model.Provider {
	case "anthropic":
		return &anthropicProvider{model: model, client: client}, nil
	case "openai":
		return &openaiProvider{model: model, client: client}, nil
	case "google":
		return &googleProvider{model: model, client: client}, nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", model.Provider)
	}
}

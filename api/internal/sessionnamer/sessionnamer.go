package sessionnamer

import (
	"context"
	"strings"
	"unicode"

	"github.com/dfradehubs/agentgram-api/internal/llm"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"go.uber.org/zap"
)

// Namer generates short session names using an LLM.
type Namer struct {
	provider llm.Provider
	logger   *zap.Logger
}

// New creates a new Namer if model is valid, otherwise returns nil.
func New(model *models.LLMModel, logger *zap.Logger) *Namer {
	if model == nil || !model.Enabled || model.APIKey == "" {
		return nil
	}
	provider, err := llm.NewProvider(model)
	if err != nil {
		logger.Warn("failed to create LLM provider for session namer", zap.Error(err))
		return nil
	}
	return &Namer{
		provider: provider,
		logger:   logger,
	}
}

// NewWithProvider creates a Namer with a pre-configured provider (e.g. traced).
func NewWithProvider(provider llm.Provider, logger *zap.Logger) *Namer {
	if provider == nil {
		return nil
	}
	return &Namer{
		provider: provider,
		logger:   logger,
	}
}

const systemPrompt = `Generate a short title (3 to 6 words) for a chat conversation.
The title must capture the main topic of the conversation.
Respond ONLY with the title, no quotes, no periods, no explanations.
Use the same language as the user's message.`

// GenerateName produces a short session title from the user message and an optional assistant preview.
func (n *Namer) GenerateName(ctx context.Context, userMessage, assistantPreview string) (string, error) {
	content := "User message: " + userMessage
	if assistantPreview != "" {
		preview := assistantPreview
		if len(preview) > 200 {
			preview = preview[:200]
		}
		content += "\nAssistant response: " + preview
	}

	resp, err := n.provider.GenerateContent(ctx, &llm.Request{
		SystemPrompt: systemPrompt,
		Messages:     []llm.Message{{Role: "user", Content: content}},
		MaxTokens:    30,
	})
	if err != nil {
		n.logger.Warn("session namer failed", zap.Error(err))
		return "", err
	}

	return cleanName(resp.Text), nil
}

// cleanName trims whitespace, removes surrounding quotes/dots, and truncates to 60 chars.
func cleanName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'`")
	s = strings.TrimRight(s, ".")
	s = strings.TrimSpace(s)

	// Truncate at 60 runes
	runes := []rune(s)
	if len(runes) > 60 {
		s = string(runes[:60])
	}

	// Capitalize first letter
	runes = []rune(s)
	if len(runes) > 0 {
		runes[0] = unicode.ToUpper(runes[0])
		s = string(runes)
	}

	return s
}

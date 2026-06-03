package summarizer

import (
	"context"
	"fmt"
	"strings"

	"github.com/dfradehubs/agentgram-api/internal/llm"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"go.uber.org/zap"
)

// Summarizer summarizes conversation context using an LLM
type Summarizer struct {
	provider llm.Provider
	logger   *zap.Logger
}

// New creates a new Summarizer if model is valid, otherwise returns nil
func New(model *models.LLMModel, logger *zap.Logger) *Summarizer {
	if model == nil || !model.Enabled || model.APIKey == "" {
		return nil
	}
	provider, err := llm.NewProvider(model)
	if err != nil {
		logger.Warn("failed to create LLM provider for summarizer", zap.Error(err))
		return nil
	}
	return &Summarizer{
		provider: provider,
		logger:   logger,
	}
}

// NewWithProvider creates a Summarizer with a pre-configured provider (e.g. traced).
func NewWithProvider(provider llm.Provider, logger *zap.Logger) *Summarizer {
	if provider == nil {
		return nil
	}
	return &Summarizer{
		provider: provider,
		logger:   logger,
	}
}

const summaryPrompt = `You are an expert assistant at summarizing multi-agent conversations. Your summary will be used as context for other agents to continue the work, so you MUST preserve all operational information.

Rules:
1. PRESERVE LITERALLY: names, URLs, IDs, file paths, code snippets, commands, configuration values and any concrete data. Do not paraphrase them.
2. DECISIONS: list what was decided, who decided it (which agent) and why.
3. CURRENT STATE: where the task stands, what has been completed and what is still pending.
4. USER PREFERENCES: explicit constraints, requirements or instructions from the user.
5. PER-AGENT CONTEXT: what each agent did, so the next one knows which agent has which information.
6. ERRORS: if there were errors, what failed and how it was resolved (or whether it remains unresolved).
7. Respond in the same language as the conversation.
8. Do not include greetings, metadata or explanations about the format.

Conversation:
%s

Structured summary:`

// Summarize takes conversation messages and returns a summarized context string.
// On error, returns empty string and the error (caller should fallback).
func (s *Summarizer) Summarize(ctx context.Context, messages []models.ChatMessage) (string, error) {
	if len(messages) == 0 {
		return "", nil
	}

	// Build conversation text
	var sb strings.Builder
	for _, msg := range messages {
		if msg.IsError {
			continue
		}

		var prefix string
		switch msg.Role {
		case "assistant":
			prefix = fmt.Sprintf("Agent[%s]", msg.AgentID)
		case "system":
			prefix = "System"
		default:
			prefix = "User"
		}

		if msg.Content != "" {
			fmt.Fprintf(&sb, "%s: %s\n", prefix, msg.Content)
		}
	}

	prompt := fmt.Sprintf(summaryPrompt, sb.String())

	resp, err := s.provider.GenerateContent(ctx, &llm.Request{
		Messages:  []llm.Message{{Role: "user", Content: prompt}},
		MaxTokens: 2048,
	})
	if err != nil {
		s.logger.Warn("summarizer failed", zap.Error(err))
		return "", err
	}

	return resp.Text, nil
}

// Package orchestrator implements the group-debate moderator: an LLM that
// decides which agent of a group speaks next, plus the surface-agnostic
// debate loop shared by the API (streaming SSE) and MCP (collected text)
// surfaces. The shared multi-agent session transcript is the communication
// channel between agents; this package only decides turn order.
package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/dfradehubs/agentgram-api/internal/llm"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"go.uber.org/zap"
)

// AgentBrief is the minimal agent info the moderator needs to route.
type AgentBrief struct {
	ID          string
	Name        string
	Description string
}

// TurnResult is the outcome of one agent turn in a debate.
type TurnResult struct {
	AgentID string
	Text    string
	Err     error // per-turn failure; the debate continues
}

// TurnRunner executes one agent turn and returns the agent's reply text.
// The API implements it by streaming SSE; MCP by collecting text. The runner
// is responsible for persisting the reply into the shared session so agents
// see each other's contributions via the existing context delta.
type TurnRunner func(ctx context.Context, agentID string) (string, error)

// Moderator decides which agent speaks next using an LLM.
type Moderator struct {
	provider llm.Provider
	logger   *zap.Logger
}

// New creates a Moderator if the model is usable, otherwise returns nil.
func New(model *models.LLMModel, logger *zap.Logger) *Moderator {
	if model == nil || !model.Enabled || model.APIKey == "" {
		return nil
	}
	provider, err := llm.NewProvider(model)
	if err != nil {
		logger.Warn("failed to create LLM provider for moderator", zap.Error(err))
		return nil
	}
	return &Moderator{provider: provider, logger: logger}
}

// NewWithProvider creates a Moderator with a pre-configured provider (e.g. traced).
func NewWithProvider(provider llm.Provider, logger *zap.Logger) *Moderator {
	if provider == nil {
		return nil
	}
	return &Moderator{provider: provider, logger: logger}
}

// ponytail: the prompt is the single tuning point for moderator behavior.
//
// Security note: agent replies and user text are interpolated verbatim into
// the transcript, so a malicious participant can try to steer turn selection
// (prompt injection). Blast radius is bounded by design: NextSpeaker output is
// validated against the roster (anything else means FINISH), turns are capped,
// and agents can already emit arbitrary text to the user directly.
const nextSpeakerPrompt = `You are the moderator of a group conversation between a user and several specialized AI agents, like a Telegram group. Your only job is to decide who speaks next.

Available agents:
%s

Conversation so far:
---
%s
---

Rules:
1. Pick the agent whose description best matches what is needed NOW (answer the user, or add to / verify another agent's reply).
2. An agent should only speak if it genuinely adds value. Do not force turns.
3. When the user's message is already well answered and no agent would add value, the conversation is over.
4. Do not pick the same agent twice in a row unless strictly necessary.

Respond with EXACTLY ONE of:
- The id of the next agent to speak (one of: %s)
- FINISH if nobody else should speak

No explanations, no punctuation, just the id or FINISH.`

const synthesisPrompt = `You are the moderator of a group conversation between a user and several AI agents. Multiple agents have contributed. Write a brief final synthesis for the user that consolidates their contributions: agreements, differences, and the actionable answer. Respond in the same language as the conversation. Do not repeat everything; only consolidate. If there is nothing meaningful to consolidate, respond with an empty message.

Conversation:
---
%s
---`

const moderatorMaxTokens = 64

// synthesisMaxTokens bounds the final consolidation. MUST be > 0: several
// providers (Anthropic, OpenAI) require a positive max_tokens and reject 0.
const synthesisMaxTokens = 1024

// NextSpeaker asks the LLM who should speak next. Returns done=true when the
// debate should end (explicit FINISH, empty answer, or an id not in the roster).
func (m *Moderator) NextSpeaker(ctx context.Context, roster []AgentBrief, transcript string) (string, bool, error) {
	var rosterDesc strings.Builder
	ids := make([]string, 0, len(roster))
	for _, a := range roster {
		fmt.Fprintf(&rosterDesc, "- %s (%s): %s\n", a.ID, a.Name, a.Description)
		ids = append(ids, a.ID)
	}

	prompt := fmt.Sprintf(nextSpeakerPrompt, rosterDesc.String(), transcript, strings.Join(ids, ", "))
	resp, err := m.provider.GenerateContent(ctx, &llm.Request{
		Messages:  []llm.Message{{Role: "user", Content: prompt}},
		MaxTokens: moderatorMaxTokens,
	})
	if err != nil {
		return "", false, fmt.Errorf("moderator next-speaker call failed: %w", err)
	}

	answer := strings.TrimSpace(resp.Text)
	if answer == "" || strings.EqualFold(answer, "FINISH") {
		return "", true, nil
	}
	for _, id := range ids {
		if answer == id {
			return id, false, nil
		}
	}
	// Unknown id: safe default is to end the debate rather than guess.
	m.logger.Warn("moderator returned unknown agent id", zap.String("answer", answer))
	return "", true, nil
}

// Debate runs the moderated turn loop: ask NextSpeaker, run the turn, append
// the reply to the moderator's transcript, repeat until FINISH or maxTurns.
// A failed turn is recorded (with its error visible to the moderator) and the
// debate continues. The initial transcript should already include the user's
// message.
func (m *Moderator) Debate(ctx context.Context, roster []AgentBrief, transcript string, run TurnRunner, maxTurns int) ([]TurnResult, error) {
	var results []TurnResult
	for turn := 0; turn < maxTurns; turn++ {
		// Stop when the caller's deadline expired: each turn's outbound agent
		// call runs on its own detached timeout (by design, to survive client
		// disconnects), so this check is what bounds the debate's total time
		// to the caller's deadline plus at most one turn.
		if ctx.Err() != nil {
			return results, ctx.Err()
		}
		agentID, done, err := m.NextSpeaker(ctx, roster, transcript)
		if err != nil {
			return results, err
		}
		if done {
			break
		}

		text, err := run(ctx, agentID)
		if err != nil {
			m.logger.Warn("debate turn failed",
				zap.String("agent_id", agentID),
				zap.Error(err))
			if text != "" {
				transcript += fmt.Sprintf("\nAgent[%s]: %s [error: %v]", agentID, text, err)
			} else {
				transcript += fmt.Sprintf("\nAgent[%s]: [error: %v]", agentID, err)
			}
			results = append(results, TurnResult{AgentID: agentID, Text: text, Err: err})
			continue
		}

		transcript += fmt.Sprintf("\nAgent[%s]: %s", agentID, text)
		results = append(results, TurnResult{AgentID: agentID, Text: text})
	}
	return results, nil
}

// Synthesize produces an optional final consolidation when several agents
// contributed. Callers decide whether to invoke it (typically ≥2 speakers).
func (m *Moderator) Synthesize(ctx context.Context, transcript string) (string, error) {
	resp, err := m.provider.GenerateContent(ctx, &llm.Request{
		Messages:  []llm.Message{{Role: "user", Content: fmt.Sprintf(synthesisPrompt, transcript)}},
		MaxTokens: synthesisMaxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("moderator synthesis call failed: %w", err)
	}
	return strings.TrimSpace(resp.Text), nil
}

// Transcript bounds: the moderator only needs recent context to route, and an
// unbounded transcript would eventually blow the moderator model's context
// window (breaking the group for that session) and grow cost quadratically.
const (
	transcriptMaxMessages = 30
	transcriptMaxMsgChars = 2000
)

// RenderTranscript renders session messages into the plain-text transcript
// format the moderator prompts expect (same shape the agents see via the
// multi-agent context builder). Bounded to the most recent messages with
// per-message truncation.
func RenderTranscript(messages []models.ChatMessage) string {
	start := 0
	if len(messages) > transcriptMaxMessages {
		start = len(messages) - transcriptMaxMessages
	}

	var sb strings.Builder
	for _, msg := range messages[start:] {
		if msg.Role == "system" || msg.IsError {
			continue
		}
		prefix := "User"
		if msg.Role == "assistant" {
			prefix = fmt.Sprintf("Agent[%s]", msg.AgentID)
		} else if msg.UserName != "" {
			prefix = fmt.Sprintf("User[%s]", msg.UserName)
		}
		content := msg.Content
		if len(content) > transcriptMaxMsgChars {
			content = content[:transcriptMaxMsgChars] + "…"
		}
		fmt.Fprintf(&sb, "%s: %s\n", prefix, content)
	}
	return sb.String()
}

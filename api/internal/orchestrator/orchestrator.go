// Package orchestrator implements the group-debate moderator: an LLM that
// decides which agent of a group speaks next, plus the surface-agnostic
// debate loop shared by the API (streaming SSE) and MCP (collected text)
// surfaces. The shared multi-agent session transcript is the communication
// channel between agents; this package only decides turn order.
package orchestrator

import (
	"context"
	"errors"
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
	AgentID      string
	Text         string
	Err          error // per-turn failure; the debate continues
	FailureKind  TurnFailureKind
	FailureKinds []TurnFailureKind
}

type TurnFailureKind string

const (
	TurnFailureAgent       TurnFailureKind = "agent"
	TurnFailurePersistence TurnFailureKind = "persistence"
	TurnFailureMapping     TurnFailureKind = "session_mapping"
)

type turnError struct {
	kind TurnFailureKind
	err  error
}

func (e *turnError) Error() string { return e.err.Error() }
func (e *turnError) Unwrap() error { return e.err }

// NewTurnError labels a per-turn failure without losing its underlying cause.
func NewTurnError(kind TurnFailureKind, err error) error {
	if err == nil {
		return nil
	}
	return &turnError{kind: kind, err: err}
}

func TurnErrorKind(err error) TurnFailureKind {
	if hasTurnFailure(err, TurnFailurePersistence) {
		return TurnFailurePersistence
	}
	if hasTurnFailure(err, TurnFailureMapping) {
		return TurnFailureMapping
	}
	if err != nil {
		return TurnFailureAgent
	}
	return ""
}

// TurnErrorKinds returns every independent failure carried by a joined turn
// error. A turn can fail at the agent and persistence layers simultaneously.
func TurnErrorKinds(err error) []TurnFailureKind {
	kinds := make([]TurnFailureKind, 0, 3)
	for _, kind := range []TurnFailureKind{TurnFailureAgent, TurnFailurePersistence, TurnFailureMapping} {
		if hasTurnFailure(err, kind) {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

// HasFailure reports whether a turn result contains a specific failure cause.
func (r TurnResult) HasFailure(kind TurnFailureKind) bool {
	for _, failure := range r.FailureKinds {
		if failure == kind {
			return true
		}
	}
	return r.FailureKind == kind
}

func hasTurnFailure(err error, kind TurnFailureKind) bool {
	if err == nil {
		return false
	}
	if tagged, ok := err.(*turnError); ok && tagged.kind == kind {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			if hasTurnFailure(child, kind) {
				return true
			}
		}
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return hasTurnFailure(wrapped.Unwrap(), kind)
	}
	return false
}

var (
	ErrMaxTurnsReached   = errors.New("maximum debate turns reached")
	ErrModeratorProtocol = errors.New("moderator returned an invalid next-speaker response")
)

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
// validated against the roster (invalid output aborts the debate), turns are capped,
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
2. A response does NOT mean the request is resolved merely because an agent spoke. Judge whether the user's latest request was actually answered.
3. If the latest agent says it lacks access, data, tools, capability, evidence, or certainty to verify the answer, or tells the user to check another system, treat the request as unresolved. Pick a different, untried agent whose description suggests it may verify or complete the answer.
4. Do not respond FINISH while an unresolved part of the user's latest request could be addressed by an untried agent.
5. An agent should only speak if it genuinely adds value. Do not force turns when no remaining agent can help.
6. Respond FINISH only when the request is actually answered, or no remaining agent can help.
7. Do not pick the same agent twice in a row unless strictly necessary.

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
// debate should end only for explicit FINISH. Empty or unknown output is a
// moderator protocol error so callers never mistake a model failure for success.
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
	if strings.EqualFold(answer, "FINISH") {
		return "", true, nil
	}
	if answer == "" {
		return "", false, fmt.Errorf("%w: empty response", ErrModeratorProtocol)
	}
	for _, id := range ids {
		if answer == id {
			return id, false, nil
		}
	}
	m.logger.Warn("moderator returned unknown agent id", zap.String("answer", answer))
	return "", false, fmt.Errorf("%w: unknown agent id %q", ErrModeratorProtocol, answer)
}

// Debate runs the moderated turn loop: ask NextSpeaker, run the turn, append
// the reply to the moderator's transcript, repeat until FINISH or maxTurns.
// A failed turn is recorded (with its error visible to the moderator) and the
// debate continues. The initial transcript should already include the user's
// message.
func (m *Moderator) Debate(ctx context.Context, roster []AgentBrief, transcript string, run TurnRunner, maxTurns int) ([]TurnResult, error) {
	var results []TurnResult
	attempted := make(map[string]bool)
	for turn := 0; turn < maxTurns; turn++ {
		// Stop when the caller's deadline expired: each turn's outbound agent
		// call survives client disconnects but is clamped to the same absolute
		// deadline, so this check prevents new work after the budget expires.
		if ctx.Err() != nil {
			return results, ctx.Err()
		}
		agentID, done, err := m.NextSpeaker(ctx, roster, transcript)
		if err != nil {
			return results, err
		}
		if done && replyNeedsHandoff(results) {
			remaining := make([]AgentBrief, 0, len(roster))
			for _, agent := range roster {
				if !attempted[agent.ID] {
					remaining = append(remaining, agent)
				}
			}
			if len(remaining) > 0 {
				forcedTranscript := transcript + "\nModerator routing safeguard: the latest agent could not access or verify the requested information. The request is unresolved; select the best remaining agent."
				agentID, done, err = m.NextSpeaker(ctx, remaining, forcedTranscript)
				if err != nil {
					return results, err
				}
				if done {
					// A limitation response must not silently end while another
					// agent remains. The reduced roster is already ordered by the
					// group's configured priority, so it is the safe fallback.
					agentID = remaining[0].ID
					done = false
				}
			}
		}
		if done {
			return results, nil
		}
		attempted[agentID] = true

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
			results = append(results, TurnResult{AgentID: agentID, Text: text, Err: err, FailureKind: TurnErrorKind(err), FailureKinds: TurnErrorKinds(err)})
			continue
		}

		transcript += fmt.Sprintf("\nAgent[%s]: %s", agentID, text)
		results = append(results, TurnResult{AgentID: agentID, Text: text})
	}
	return results, ErrMaxTurnsReached
}

var handoffMarkers = []string{
	"no tengo acceso",
	"no dispongo de acceso",
	"no puedo acceder",
	"no puedo confirmar",
	"no puedo verificar",
	"no puedo comprobar",
	"i do not have access",
	"i don't have access",
	"i cannot access",
	"i can't access",
	"i cannot confirm",
	"i can't confirm",
	"i cannot verify",
	"i can't verify",
}

func replyNeedsHandoff(results []TurnResult) bool {
	if len(results) == 0 {
		return false
	}
	text := strings.ToLower(results[len(results)-1].Text)
	for _, marker := range handoffMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
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

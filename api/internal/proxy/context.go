package proxy

import (
	"context"
	"fmt"
	"strings"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/summarizer"
)

const maxContextMessages = 10

// PrepareMessagesForAgent prepares the messages to send to an agent.
// If we already have a session with the agent, only the new message is sent.
// If not, a context summary of previous conversation is prepended.
func PrepareMessagesForAgent(
	session *models.Session,
	newMessage models.ChatMessage,
	hasAgentSession bool,
) []models.ChatMessage {
	// Already have a session with this agent - agent has context
	if hasAgentSession {
		return []models.ChatMessage{newMessage}
	}

	// No previous messages - just send the new one
	if session == nil || len(session.Messages) == 0 {
		return []models.ChatMessage{newMessage}
	}

	// Build context summary from previous messages
	contextMsg := buildContextMessage(session.Messages)
	if contextMsg == "" {
		return []models.ChatMessage{newMessage}
	}

	return []models.ChatMessage{
		{Role: "user", Content: contextMsg},
		newMessage,
	}
}

// ContextResult holds the prepared messages and whether context was sent
type ContextResult struct {
	Messages    []models.ChatMessage
	ContextSent bool
}

// estimateTokens provides a rough token count (1 token ≈ 4 chars).
func estimateTokens(messages []models.ChatMessage) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Content)
	}
	return total / 4
}

// PrepareMessagesForMultiAgent prepares messages for a multi-agent session.
// When sendContext is true, it calculates the delta: messages from other agents
// since the target agent's last interaction.
// When sendContext is false, only the new user message is sent.
// Summarization is triggered automatically when the estimated delta tokens exceed
// maxContextTokens * summarizeThreshold.
func PrepareMessagesForMultiAgent(
	session *models.Session,
	targetAgentID string,
	newMessage models.ChatMessage,
	hasAgentSession bool,
	sendContext bool,
	maxContextTokens int,
	summarizeThreshold float64,
	sum *summarizer.Summarizer,
	ctx context.Context,
) ContextResult {
	if !sendContext {
		return ContextResult{
			Messages:    []models.ChatMessage{newMessage},
			ContextSent: false,
		}
	}

	if session == nil || len(session.Messages) == 0 {
		return ContextResult{
			Messages:    []models.ChatMessage{newMessage},
			ContextSent: false,
		}
	}

	// When the caller has no agent session yet (e.g. a second user in a Slack
	// thread), the agent has never seen any of this conversation. Send the full
	// recent history instead of just the delta since the agent's last response,
	// which would be empty or incomplete from this user's perspective.
	var contextMessages []models.ChatMessage
	if !hasAgentSession {
		// Exclude the new message itself (already appended to session)
		contextMessages = allMessagesExceptLast(session.Messages, newMessage)
	} else {
		contextMessages = calculateDelta(session.Messages, targetAgentID)
	}

	if len(contextMessages) == 0 {
		return ContextResult{
			Messages:    []models.ChatMessage{newMessage},
			ContextSent: false,
		}
	}

	var contextMsg string

	// Auto-summarize when context exceeds threshold
	if sum != nil && maxContextTokens > 0 {
		threshold := int(float64(maxContextTokens) * summarizeThreshold)
		if estimateTokens(contextMessages) > threshold {
			summary, err := sum.Summarize(ctx, contextMessages)
			if err == nil && summary != "" {
				contextMsg = fmt.Sprintf("[Multi-agent session context summary]\n%s\n\nThe user is now talking to you. Keep the previous context in mind.", summary)
			}
		}
	}

	// Fallback to raw context if summarization wasn't triggered or failed
	if contextMsg == "" {
		contextMsg = buildMultiAgentContextMessage(contextMessages)
	}

	return ContextResult{
		Messages: []models.ChatMessage{
			{Role: "user", Content: contextMsg},
			newMessage,
		},
		ContextSent: true,
	}
}

// allMessagesExceptLast returns all non-system, non-error messages from the
// session excluding the most recent one that matches newMessage (which will be
// sent separately). This gives a new participant the full conversation history.
func allMessagesExceptLast(messages []models.ChatMessage, newMessage models.ChatMessage) []models.ChatMessage {
	// Find how many messages to consider (exclude the last one if it matches newMessage)
	end := len(messages)
	if end > 0 {
		last := messages[end-1]
		if last.Role == newMessage.Role && last.Content == newMessage.Content {
			end--
		}
	}

	var result []models.ChatMessage
	// Take last N messages for context (same limit as other context builders)
	start := 0
	if end > maxContextMessages {
		start = end - maxContextMessages
	}
	for i := start; i < end; i++ {
		msg := messages[i]
		if msg.Role == "system" || msg.IsError {
			continue
		}
		result = append(result, msg)
	}
	return result
}

// calculateDelta returns messages that happened after the target agent's last interaction.
// Includes messages from other agents and user messages not directed to the target.
func calculateDelta(messages []models.ChatMessage, targetAgentID string) []models.ChatMessage {
	// Find the last index where the target agent was involved.
	// An agent is "involved" if:
	// - The message has its agent_id directly
	// - It's a broadcast that included the agent in BroadcastAgentIDs
	lastTargetIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.IsError {
			continue
		}
		// Direct assistant response from this agent — definitely involved
		if msg.AgentID == targetAgentID {
			lastTargetIdx = i
			break
		}
		// Broadcast user message that included this agent — only count it if
		// the agent actually responded successfully after this point.
		// If the agent failed (400/500) or never responded, we must resend context.
		if containsAgent(msg.BroadcastAgentIDs, targetAgentID) {
			if hasSuccessfulResponse(messages, i, targetAgentID) {
				lastTargetIdx = i
				break
			}
			continue
		}
	}

	// Collect messages after that index
	startIdx := lastTargetIdx + 1
	var delta []models.ChatMessage
	for i := startIdx; i < len(messages); i++ {
		msg := messages[i]
		// Skip system messages (context labels, etc.) and error messages
		if msg.Role == "system" || msg.IsError {
			continue
		}
		// Include messages from other agents (assistant with different agent_id)
		if msg.AgentID != "" && msg.AgentID != targetAgentID {
			delta = append(delta, msg)
			continue
		}
		// Include user broadcast messages that did NOT include the target agent
		if msg.Role == "user" && msg.AgentID == "" && len(msg.BroadcastAgentIDs) > 0 && !containsAgent(msg.BroadcastAgentIDs, targetAgentID) {
			delta = append(delta, msg)
			continue
		}
		// Include user messages without any specific destination (general context)
		if msg.Role == "user" && msg.AgentID == "" && len(msg.BroadcastAgentIDs) == 0 {
			delta = append(delta, msg)
			continue
		}
	}

	return delta
}

// hasSuccessfulResponse checks if the target agent has a non-error assistant
// response after the given index. Used to verify that a broadcast message was
// actually processed by the agent (not just sent and failed with 400/500).
func hasSuccessfulResponse(messages []models.ChatMessage, afterIdx int, agentID string) bool {
	for j := afterIdx + 1; j < len(messages); j++ {
		if messages[j].AgentID == agentID && !messages[j].IsError {
			return true
		}
	}
	return false
}

// containsAgent checks if a slice of agent IDs contains the target
func containsAgent(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

// buildMultiAgentContextMessage creates a context summary from other agents' messages
func buildMultiAgentContextMessage(messages []models.ChatMessage) string {
	// Take last N messages
	start := 0
	if len(messages) > maxContextMessages {
		start = len(messages) - maxContextMessages
	}
	recent := messages[start:]

	var sb strings.Builder
	for _, msg := range recent {
		prefix := "User"
		if msg.Role == "assistant" {
			prefix = fmt.Sprintf("Agent[%s]", msg.AgentID)
		} else if msg.UserName != "" {
			prefix = fmt.Sprintf("User[%s]", msg.UserName)
		}
		fmt.Fprintf(&sb, "%s: %s\n", prefix, msg.Content)
	}

	return fmt.Sprintf("[Multi-agent session context]\nOther users and agents have been talking in this same session:\n---\n%s---\nThe user is now talking to you. Keep the previous context and the participants in mind.", sb.String())
}

// buildContextMessage creates a summary of previous conversation for context
func buildContextMessage(messages []models.ChatMessage) string {
	if len(messages) == 0 {
		return ""
	}

	// Take last N messages
	start := 0
	if len(messages) > maxContextMessages {
		start = len(messages) - maxContextMessages
	}
	recent := messages[start:]

	var sb strings.Builder
	for _, msg := range recent {
		prefix := "User"
		if msg.Role == "assistant" {
			prefix = "Agent"
		} else if msg.UserName != "" {
			prefix = fmt.Sprintf("User[%s]", msg.UserName)
		}
		fmt.Fprintf(&sb, "%s: %s\n", prefix, msg.Content)
	}

	return fmt.Sprintf("[Previous conversation context]\nThe user has had the following conversation with other agents:\n---\n%s---\nThe user is now talking to you. Keep the previous context in mind.", sb.String())
}

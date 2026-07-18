package proxy

import (
	"strings"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/models"
)

func TestAllMessagesExceptLastExcludesCurrentUserMessageAfterAgentReply(t *testing.T) {
	user := models.ChatMessage{Role: "user", Content: "is production healthy?"}
	messages := []models.ChatMessage{
		{Role: "user", Content: "earlier question"},
		user,
		{Role: "assistant", Content: "agent A says yes", AgentID: "agent-a"},
	}

	got := allMessagesExceptLast(messages, user)
	if len(got) != 2 || got[0].Content != "earlier question" || got[1].Content != "agent A says yes" {
		t.Fatalf("context = %#v; current user message should appear only as the separate new message", got)
	}
}

func TestPrepareMessagesForMultiAgentBuildsCanonicalGroupTurn(t *testing.T) {
	question := models.ChatMessage{
		Role: "user", Content: "Is production healthy?", UserName: "Daniel",
		BroadcastAgentIDs: []string{"agent-a", "agent-b"},
		Attachments:       []models.Attachment{{Filename: "chart.png"}},
	}
	session := &models.Session{Messages: []models.ChatMessage{
		question,
		{Role: "assistant", Content: "Logs show repeated 503s.", AgentID: "agent-a"},
	}}

	result := PrepareMessagesForMultiAgent(session, "agent-b", question, false, true, 0, 0, nil, t.Context())
	if !result.ContextSent || len(result.Messages) != 1 {
		t.Fatalf("result = %#v, want one canonical message with context", result)
	}
	content := result.Messages[0].Content
	if strings.Count(content, question.Content) != 1 {
		t.Fatalf("original question appears %d times:\n%s", strings.Count(content, question.Content), content)
	}
	if !strings.Contains(content, "Agent[agent-a]: Logs show repeated 503s.") {
		t.Fatalf("previous attributed contribution missing:\n%s", content)
	}
	if len(result.Messages[0].Attachments) != 1 {
		t.Fatal("original attachments were not preserved")
	}
}

func TestBuildMultiAgentContextMessageIsBoundedAndKeepsRecent(t *testing.T) {
	messages := make([]models.ChatMessage, 12)
	for i := range messages {
		messages[i] = models.ChatMessage{
			Role: "assistant", AgentID: "agent-" + string(rune('a'+i)),
			Content: strings.Repeat("x", maxGroupContributionRunes+500),
		}
	}
	context := buildMultiAgentContextMessage(messages)
	if len([]rune(context)) > maxGroupContextRunes {
		t.Fatalf("context has %d runes, max %d", len([]rune(context)), maxGroupContextRunes)
	}
	if strings.Contains(context, "Agent[agent-a]") || !strings.Contains(context, "Agent[agent-l]") {
		t.Fatalf("context should retain the most recent contributions:\n%s", context)
	}
	if !strings.Contains(context, "truncated") {
		t.Fatal("context should disclose truncation")
	}
}

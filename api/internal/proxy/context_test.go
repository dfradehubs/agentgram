package proxy

import (
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

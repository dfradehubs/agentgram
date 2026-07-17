package slack

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/proxy"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"go.uber.org/zap"
)

type partialResultStore struct {
	store.SessionStore
	messages []models.ChatMessage
}

func (s *partialResultStore) AddMessage(_ context.Context, _ string, msg models.ChatMessage) error {
	s.messages = append(s.messages, msg)
	return nil
}

func TestPersistAssistantResultPreservesPartialProxyError(t *testing.T) {
	sessionStore := &partialResultStore{}
	h := &MessageHandler{sessionStore: sessionStore, logger: zap.NewNop()}
	result := &proxy.ProxyResult{
		AssistantText: "partial answer from Slack agent",
		Error:         "agent stream ended before completion",
	}

	if err := h.persistAssistantResult(context.Background(), "session-1", "agent-1", result, errors.New("unexpected EOF")); err != nil {
		t.Fatalf("persistAssistantResult: %v", err)
	}
	if len(sessionStore.messages) != 1 {
		t.Fatalf("persisted messages = %d, want 1", len(sessionStore.messages))
	}
	msg := sessionStore.messages[0]
	if !msg.IsError || !strings.Contains(msg.Content, "partial answer from Slack agent") || !strings.Contains(msg.Content, "stream ended") {
		t.Fatalf("partial error message not preserved: %#v", msg)
	}
}

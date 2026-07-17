package proxy

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSSEWriterSuppressLifecycle(t *testing.T) {
	rec := httptest.NewRecorder()
	sse, err := NewSSEWriter(rec)
	if err != nil {
		t.Fatalf("NewSSEWriter: %v", err)
	}
	sse.Apply(SSEConfig{AgentID: "logs-agent", SuppressLifecycle: true})

	if err := sse.SendRunStarted(); err != nil {
		t.Fatalf("SendRunStarted: %v", err)
	}
	if err := sse.SendRunFinished(); err != nil {
		t.Fatalf("SendRunFinished: %v", err)
	}
	if err := sse.SendRunError("agent down"); err != nil {
		t.Fatalf("SendRunError: %v", err)
	}

	body := rec.Body.String()
	if strings.Contains(body, "RUN_STARTED") || strings.Contains(body, "RUN_FINISHED") {
		t.Errorf("suppressed lifecycle events were written:\n%s", body)
	}
	// A per-turn error must NOT abort the outer run: it becomes a scoped CUSTOM event
	if strings.Contains(body, "RUN_ERROR") {
		t.Errorf("RUN_ERROR leaked through a suppressed-lifecycle writer:\n%s", body)
	}
	if !strings.Contains(body, `"subType":"turn.error"`) || !strings.Contains(body, `"agentId":"logs-agent"`) {
		t.Errorf("expected agent-scoped turn.error CUSTOM event, got:\n%s", body)
	}
}

func TestSSEWriterTagsEventsWithAgentID(t *testing.T) {
	rec := httptest.NewRecorder()
	sse, err := NewSSEWriter(rec)
	if err != nil {
		t.Fatalf("NewSSEWriter: %v", err)
	}
	sse.Apply(SSEConfig{AgentID: "kube-agent"})

	_ = sse.SendTextMessageStart()
	_ = sse.SendTextMessageContent("hello")
	_ = sse.SendTextMessageEnd()
	_ = sse.SendToolCallStart("tc1", "SearchTool")

	body := rec.Body.String()
	if got := strings.Count(body, `"agentId":"kube-agent"`); got != 4 {
		t.Errorf("expected 4 agentId-tagged events, got %d:\n%s", got, body)
	}
}

func TestSSEWriterDefaultBehaviorUnchanged(t *testing.T) {
	// Without config, events carry no agentId and lifecycle events are emitted —
	// the exact pre-debate behavior for 1:1 chats.
	rec := httptest.NewRecorder()
	sse, err := NewSSEWriter(rec)
	if err != nil {
		t.Fatalf("NewSSEWriter: %v", err)
	}

	_ = sse.SendRunStarted()
	_ = sse.SendTextMessageStart()
	_ = sse.SendTextMessageContent("hi")
	_ = sse.SendRunError("boom")
	_ = sse.SendRunFinished()

	body := rec.Body.String()
	for _, want := range []string{"RUN_STARTED", "RUN_ERROR", "RUN_FINISHED"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in default-mode output:\n%s", want, body)
		}
	}
	if strings.Contains(body, "agentId") {
		t.Errorf("unexpected agentId tag in default-mode output:\n%s", body)
	}
}

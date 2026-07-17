package proxy

import (
	"errors"
	"strings"
	"testing"
)

func TestProxyResultHasPersistableContentIncludesStructuredOnlyResults(t *testing.T) {
	result := &ProxyResult{ContentParts: []ContentPart{{
		Type:  "chart",
		Chart: map[string]interface{}{"type": "bar"},
	}}}
	if !result.HasPersistableContent() {
		t.Fatal("chart-only result was treated as empty")
	}
}

func TestProxyResultToChatMessagePreservesStructuredContent(t *testing.T) {
	result := &ProxyResult{
		ContentParts: []ContentPart{{Type: "chart", Chart: map[string]interface{}{"type": "bar"}}},
		ToolCalls: []CapturedToolCall{{ID: "tc-1", Name: "metrics", Args: `{"range":"1h"}`, Result: `{"value":7}`}},
	}
	msg := result.ToChatMessage("agent-a", "")
	if len(msg.ContentParts) != 1 || len(msg.ToolCalls) != 1 || len(msg.ToolResults) != 1 {
		t.Fatalf("structured result was not preserved: %#v", msg)
	}
}

func TestPublicErrorMessageDoesNotExposeInternalDetails(t *testing.T) {
	got := PublicErrorMessage(errors.New("POST http://10.0.0.8:9000: secret upstream body"))
	if got == "" || strings.Contains(got, "10.0.0.8") || strings.Contains(got, "secret") {
		t.Fatalf("unsafe public error: %q", got)
	}
}

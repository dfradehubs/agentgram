package audit

import (
	"context"
	"strings"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/models"
)

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"under limit", "hello", 10, "hello"},
		{"exact limit", "hello", 5, "hello"},
		{"over limit", "hello world", 5, "hello…[truncated]"},
		{"zero max is no-op", "hello", 0, "hello"},
		{"multibyte not split", "áéíóú€", 3, "áéí…[truncated]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateRunes(tt.in, tt.max); got != tt.want {
				t.Errorf("truncateRunes(%q,%d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
		})
	}
}

// captureRepo records the last inserted event so we can assert on it.
type captureRepo struct{ last *models.AuditEvent }

func (c *captureRepo) Insert(_ context.Context, e *models.AuditEvent) error { c.last = e; return nil }
func (c *captureRepo) List(context.Context, models.AuditEventFilter) ([]*models.AuditEvent, int, error) {
	return nil, 0, nil
}
func (c *captureRepo) Cleanup(context.Context, int) (int64, error) { return 0, nil }

func TestRecordEventTruncatesPromptResponseAndToolCalls(t *testing.T) {
	// RecordEvent truncates synchronously (before the async insert), so we can
	// assert on the mutated event immediately.
	ev := &models.AuditEvent{
		Prompt:   strings.Repeat("a", 100),
		Response: strings.Repeat("b", 100),
		ToolCalls: []models.AuditToolCall{
			{Name: "t", Arguments: strings.Repeat("c", 100), Result: strings.Repeat("d", 100)},
		},
	}
	audit := &captureRepo{}
	RecordEvent(audit, ev, 10, nil)

	for _, f := range []struct {
		name string
		got  string
	}{
		{"prompt", ev.Prompt},
		{"response", ev.Response},
		{"tool args", ev.ToolCalls[0].Arguments},
		{"tool result", ev.ToolCalls[0].Result},
	} {
		if !strings.HasSuffix(f.got, "…[truncated]") || len([]rune(strings.TrimSuffix(f.got, "…[truncated]"))) != 10 {
			t.Errorf("%s not truncated to 10 runes: %q", f.name, f.got)
		}
	}
}

func TestRecordEventNoopGuards(t *testing.T) {
	// Must not panic on nil repo or nil event.
	RecordEvent(nil, &models.AuditEvent{Prompt: "x"}, 10, nil)
	RecordEvent(&captureRepo{}, nil, 10, nil)
}

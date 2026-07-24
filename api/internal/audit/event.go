package audit

import (
	"context"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

// auditInsertTimeout bounds the async DB write so a slow DB can't leak goroutines.
const auditInsertTimeout = 5 * time.Second

// RecordEvent truncates the event's prompt/response to maxContentChars and
// inserts it asynchronously, so auditing never blocks the request hot-path.
// A nil repo makes this a no-op (auditing disabled / not wired).
func RecordEvent(repo repository.AuditEventRepository, ev *models.AuditEvent, maxContentChars int, logger *zap.Logger) {
	if repo == nil || ev == nil {
		return
	}
	if maxContentChars > 0 {
		ev.Prompt = truncateRunes(ev.Prompt, maxContentChars)
		ev.Response = truncateRunes(ev.Response, maxContentChars)
		for i := range ev.ToolCalls {
			ev.ToolCalls[i].Arguments = truncateRunes(ev.ToolCalls[i].Arguments, maxContentChars)
			ev.ToolCalls[i].Result = truncateRunes(ev.ToolCalls[i].Result, maxContentChars)
		}
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), auditInsertTimeout)
		defer cancel()
		if err := repo.Insert(ctx, ev); err != nil && logger != nil {
			logger.Warn("failed to insert audit_event", zap.Error(err))
		}
	}()
}

// truncateRunes cuts s to at most max runes (not bytes) so UTF-8 stays valid,
// appending an ellipsis marker when it actually truncates.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…[truncated]"
}

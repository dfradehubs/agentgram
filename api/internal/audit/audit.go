package audit

import (
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ActionType represents the type of auditable action
type ActionType string

const (
	ActionChat            ActionType = "chat"
	ActionSessionDelete   ActionType = "session_delete"
	ActionBroadcast       ActionType = "broadcast"
	ActionMCPChat         ActionType = "mcp_chat"
	ActionCreate          ActionType = "create"
	ActionDelete          ActionType = "delete"
	ActionShareSession    ActionType = "share_session"
	ActionRevokeShare     ActionType = "revoke_share"
	ActionCloneShare      ActionType = "clone_shared_session"
)

// Logger provides structured audit logging to stdout
type Logger struct {
	logger *zap.Logger
}

// New creates a new audit Logger with structured JSON output
func New(baseLogger *zap.Logger) *Logger {
	return &Logger{
		logger: baseLogger.Named("audit"),
	}
}

// Log emits a structured audit log entry
func (l *Logger) Log(userEmail string, action ActionType, fields ...zapcore.Field) {
	allFields := make([]zapcore.Field, 0, len(fields)+3)
	allFields = append(allFields,
		zap.String("user_email", userEmail),
		zap.String("action_type", string(action)),
		zap.String("timestamp", time.Now().UTC().Format(time.RFC3339)),
	)
	allFields = append(allFields, fields...)
	l.logger.Info("audit", allFields...)
}

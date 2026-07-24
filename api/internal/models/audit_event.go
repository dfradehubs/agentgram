package models

import "time"

// Audit resource/source/action constants.
const (
	AuditResourceAgent = "agent"
	AuditResourceMCP   = "mcp"
	AuditResourceSkill = "skill"
	AuditResourceGroup = "group"

	AuditSourceWeb   = "web"
	AuditSourceMCP   = "mcp"
	AuditSourceSlack = "slack"

	AuditActionChat        = "chat"
	AuditActionMCPTool     = "mcp_tool"
	AuditActionSkillRead   = "skill_read"
	AuditActionGroupDebate = "group_debate"
)

// AuditToolCall is a tool invocation captured in an audit event, with arguments
// and result when available (truncation is applied to prompt/response only).
type AuditToolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
	Result    string `json:"result,omitempty"`
}

// AuditEvent is a detailed record of one user-facing interaction, including the
// prompt sent and the response returned (truncated to a configurable limit).
// Distinct from ChatEvent, which stores only aggregate metrics with no content.
type AuditEvent struct {
	ID           string          `json:"id"`
	UserEmail    string          `json:"user_email"`
	UserGroups   []string        `json:"user_groups,omitempty"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	ResourceName string          `json:"resource_name"`
	Source       string          `json:"source"`
	Client       string          `json:"client,omitempty"`      // MCP client / user-agent that made the call
	SessionID    string          `json:"session_id,omitempty"`  // correlate events in one conversation
	Action       string          `json:"action"`
	Prompt       string          `json:"prompt"`
	Response     string          `json:"response"`
	ToolCalls    []AuditToolCall `json:"tool_calls,omitempty"`
	TokenUsage   *TokenUsage     `json:"token_usage,omitempty"`
	LLMModel     string          `json:"llm_model,omitempty"`
	Status       string          `json:"status"`
	ErrorType    string          `json:"error_type,omitempty"`
	ErrorMsg     string          `json:"error_msg,omitempty"`
	DurationMs   int             `json:"duration_ms"`
	CreatedAt    time.Time       `json:"created_at"`
}

// AuditEventFilter narrows an audit-event list query. Zero values mean "no
// filter" for that field.
type AuditEventFilter struct {
	From         *time.Time
	To           *time.Time
	UserEmail    string
	Group        string
	ResourceType string
	SessionID    string
	Limit        int
	Offset       int
}

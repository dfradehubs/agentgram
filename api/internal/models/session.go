package models

// Session represents a chat session (managed by agents)
type Session struct {
	SessionID    string        `json:"session_id"`
	SessionName  string        `json:"session_name"`
	UserID       string        `json:"user_id"`
	AppName      string        `json:"app_name"`
	AgentIDs     []string      `json:"agent_ids,omitempty"`  // Participating agents
	IsMultiAgent bool          `json:"is_multi_agent"`       // Whether this is a multi-agent session
	GroupID       string        `json:"group_id,omitempty"`   // Agent group ID (for collaborative sessions)
	Source        string        `json:"source,omitempty"`     // Origin: "slack" or "" (web)
	SlackThreadID string       `json:"slack_thread_id,omitempty"` // For syncing sibling sessions in same thread
	CreatedAt    int64         `json:"created_at"`           // Unix timestamp
	LastActivity int64         `json:"last_activity"`        // Unix timestamp
	MessageCount int           `json:"message_count"`
	Messages     []ChatMessage `json:"messages,omitempty"`   // Only included when fetching single session
	ActiveRun    bool          `json:"active_run,omitempty"` // True when a run is in flight (ephemeral, resolved at read time)
}

// SessionListResponse response for listing sessions
type SessionListResponse struct {
	Sessions []Session `json:"sessions"`
}

// SessionGetResponse is the paginated response for getting a single session
type SessionGetResponse struct {
	Session    Session       `json:"session"`
	Messages   []ChatMessage `json:"messages"`
	HasMore    bool          `json:"has_more"`
	NextCursor int           `json:"next_cursor,omitempty"` // Message index to use as `before` for the next page
}

// SessionRenameRequest request for renaming a session
type SessionRenameRequest struct {
	SessionName string `json:"session_name"`
}


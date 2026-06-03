package models

// ChatRequest is the expected body for POST /api/agents/:agentId/chat
type ChatRequest struct {
	Messages    []ChatMessage `json:"messages"`
	SessionID   string        `json:"session_id,omitempty"`
	SendContext *bool         `json:"send_context,omitempty"`
	GroupID     string        `json:"group_id,omitempty"`
}

// ChatMessage represents a message in the conversation
type ChatMessage struct {
	Role              string             `json:"role"`                          // "user" | "assistant" | "system"
	Content           string             `json:"content"`
	AgentID           string             `json:"agent_id,omitempty"`            // Which agent sent/received this message (multi-agent sessions)
	UserName          string             `json:"user_name,omitempty"`           // Display name of the user who sent this message
	UserEmail         string             `json:"user_email,omitempty"`          // Email of the user who sent this message
	IsAdmin           bool               `json:"is_admin,omitempty"`            // Whether the user who sent this message is an admin
	IsError           bool               `json:"is_error,omitempty"`            // Whether this message represents an error response
	BroadcastAgentIDs []string           `json:"broadcast_agent_ids,omitempty"` // For broadcast messages: which agents it was sent to
	Attachments       []Attachment       `json:"attachments,omitempty"`         // File attachments (images, PDFs, etc.)
	ToolCalls         []StoredToolCall   `json:"tool_calls,omitempty"`          // Tool calls made during this message
	ToolResults       []StoredToolResult `json:"tool_results,omitempty"`        // Tool results for the tool calls
	ContentParts      []ContentPart      `json:"content_parts,omitempty"`       // Ordered text/tool interleaving for reconstruction
}

// ContentPart represents an ordered segment of an assistant message
type ContentPart struct {
	Type      string                 `json:"type"`                  // "text", "tool_use", or "chart"
	Text      string                 `json:"text,omitempty"`        // Text content (for type="text")
	ToolIndex *int                   `json:"tool_index,omitempty"`  // Index into ToolCalls (for type="tool_use"); pointer so 0 is not omitted
	Chart     map[string]interface{} `json:"chart,omitempty"`       // Chart data (for type="chart")
}

// StoredToolCall represents a tool call stored in session history
type StoredToolCall struct {
	ID   string                 `json:"id,omitempty"`
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

// StoredToolResult represents a tool result stored in session history
type StoredToolResult struct {
	ID       string                 `json:"id,omitempty"`
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

// IntPtr returns a pointer to an int value (useful for ContentPart.ToolIndex)
func IntPtr(i int) *int { return &i }

// Attachment represents a file attached to a message
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Data        string `json:"data"` // base64
}

package a2a

// ============== Standard A2A Protocol Types (a2a-go v0.3.x compatible) ==============

// Part represents a part of a message (standard A2A uses "kind" discriminator)
type Part struct {
	Kind     string         `json:"kind"`               // "text", "file", "data"
	Text     string         `json:"text,omitempty"`
	File     *FileContent   `json:"file,omitempty"`     // For kind: "file"
	Data     map[string]any `json:"data,omitempty"`     // For kind: "data" (structured data)
	Metadata map[string]any `json:"metadata,omitempty"`
}

// FileContent represents file content in an A2A Part (kind: "file")
type FileContent struct {
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Bytes    string `json:"bytes,omitempty"`    // Base64-encoded content
	URI      string `json:"uri,omitempty"`      // URI reference
}

// IsThought returns true if this part is marked as a thinking/reasoning step.
// ADK marks thought parts with metadata key "google.com/thought": true.
func (p Part) IsThought() bool {
	if p.Metadata == nil {
		return false
	}
	v, ok := p.Metadata["google.com/thought"]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

// Message represents an A2A message with standard fields
type Message struct {
	MessageID string `json:"messageId"`
	Role      string `json:"role"` // "user" or "agent"
	Parts     []Part `json:"parts"`
	ContextID string `json:"contextId,omitempty"`
}

// MessageSendConfig optional configuration for message/send
type MessageSendConfig struct {
	AcceptedOutputModes []string `json:"acceptedOutputModes,omitempty"`
	Blocking            bool     `json:"blocking,omitempty"`
}

// MessageSendParams parameters for message/send and message/stream
type MessageSendParams struct {
	Message       Message            `json:"message"`
	Configuration *MessageSendConfig `json:"configuration,omitempty"`
}

// JSONRPCRequest generic JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// JSONRPCResponse generic JSON-RPC 2.0 response (used for SSE streaming events)
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Result  *StreamingEvent `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError JSON-RPC error
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ============== Streaming Events (from message/stream SSE) ==============

// StreamingEvent represents an event in the message/stream SSE response.
// The "kind" field discriminates between event types.
type StreamingEvent struct {
	Kind string `json:"kind"` // "status-update" or "artifact-update"

	// Fields for status-update
	TaskID    string      `json:"taskId,omitempty"`
	Status    *TaskStatus `json:"status,omitempty"`
	ContextID string     `json:"contextId,omitempty"`
	Final     bool        `json:"final,omitempty"`

	// Fields for artifact-update
	Artifact *Artifact `json:"artifact,omitempty"`

	Metadata map[string]any `json:"metadata,omitempty"`
}

// TaskStatus status within a streaming event
type TaskStatus struct {
	State   string   `json:"state"` // "submitted", "working", "input-required", "completed", "failed", "canceled", "rejected", "auth-required"
	Message *Message `json:"message,omitempty"`
}

// Artifact represents an output artifact
type Artifact struct {
	ArtifactID string `json:"artifactId"`
	Name       string `json:"name,omitempty"`
	Parts      []Part `json:"parts"`
	Append     bool   `json:"append,omitempty"`    // Append to existing artifact
	LastChunk  bool   `json:"lastChunk,omitempty"` // Last chunk of incremental artifact
}

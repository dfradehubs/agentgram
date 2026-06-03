package adk

// RunSSERequest is the body for POST /run_sse (Google ADK REST SSE protocol)
type RunSSERequest struct {
	AppName    string  `json:"appName"`
	UserID     string  `json:"userId"`
	SessionID  string  `json:"sessionId,omitempty"`
	NewMessage Content `json:"newMessage"`
	Streaming  bool    `json:"streaming,omitempty"`
}

// Content represents a message with role and parts (matches genai.Content)
type Content struct {
	Role  string  `json:"role,omitempty"`
	Parts []*Part `json:"parts,omitempty"`
}

// Part represents a content part in ADK (matches genai.Part)
type Part struct {
	Text             string            `json:"text,omitempty"`
	FunctionCall     *FunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *FunctionResponse `json:"functionResponse,omitempty"`
	InlineData       *InlineData       `json:"inlineData,omitempty"`
	Thought          bool              `json:"thought,omitempty"`
}

// FunctionCall represents a function/tool call from the agent (matches genai.FunctionCall)
type FunctionCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name,omitempty"`
	Args map[string]any `json:"args,omitempty"`
}

// FunctionResponse represents the result of a function/tool call (matches genai.FunctionResponse)
type FunctionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name,omitempty"`
	Response map[string]any `json:"response,omitempty"`
}

// InlineData represents inline binary data (matches genai.Blob)
type InlineData struct {
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"` // Base64-encoded
}

// Event is an SSE event from the ADK /run_sse endpoint (matches adkrest/internal/models.Event)
type Event struct {
	ID                 string        `json:"id,omitempty"`
	Time               int64         `json:"time,omitempty"`
	InvocationID       string        `json:"invocationId,omitempty"`
	Branch             string        `json:"branch,omitempty"`
	Author             string        `json:"author,omitempty"`
	Partial            bool          `json:"partial,omitempty"`
	LongRunningToolIDs []string      `json:"longRunningToolIds,omitempty"`
	Content            *Content      `json:"content,omitempty"`
	TurnComplete       bool          `json:"turnComplete,omitempty"`
	Interrupted        bool          `json:"interrupted,omitempty"`
	ErrorCode          string        `json:"errorCode,omitempty"`
	ErrorMessage       string        `json:"errorMessage,omitempty"`
	Actions            *EventActions `json:"actions,omitempty"`
}

// EventActions contains action metadata in an ADK event
type EventActions struct {
	StateDelta    map[string]any    `json:"stateDelta,omitempty"`
	ArtifactDelta map[string]int64  `json:"artifactDelta,omitempty"`
}

package models

// AG-UI Protocol Event Types
// See: https://docs.copilotkit.ai/coagents/ag-ui

// AGUIEventType represents the type of AG-UI event
type AGUIEventType string

const (
	AGUIEventRunStarted         AGUIEventType = "RUN_STARTED"
	AGUIEventRunFinished        AGUIEventType = "RUN_FINISHED"
	AGUIEventRunError           AGUIEventType = "RUN_ERROR"
	AGUIEventTextMessageStart   AGUIEventType = "TEXT_MESSAGE_START"
	AGUIEventTextMessageContent AGUIEventType = "TEXT_MESSAGE_CONTENT"
	AGUIEventTextMessageEnd     AGUIEventType = "TEXT_MESSAGE_END"
	AGUIEventToolCallStart      AGUIEventType = "TOOL_CALL_START"
	AGUIEventToolCallArgs       AGUIEventType = "TOOL_CALL_ARGS"
	AGUIEventToolCallEnd        AGUIEventType = "TOOL_CALL_END"
	AGUIEventCustom             AGUIEventType = "CUSTOM"
)

// AGUIRunStartedEvent signals the start of a run
type AGUIRunStartedEvent struct {
	Type        AGUIEventType `json:"type"`
	ThreadID    string        `json:"threadId"`
	RunID       string        `json:"runId"`
	SessionName string        `json:"sessionName,omitempty"`
}

// AGUIRunFinishedEvent signals the end of a run
type AGUIRunFinishedEvent struct {
	Type     AGUIEventType `json:"type"`
	ThreadID string        `json:"threadId"`
	RunID    string        `json:"runId"`
}

// AGUIRunErrorEvent signals an error in the run
type AGUIRunErrorEvent struct {
	Type    AGUIEventType `json:"type"`
	Message string        `json:"message"`
	Code    string        `json:"code,omitempty"`
}

// AGUITextMessageStartEvent signals the start of a text message
type AGUITextMessageStartEvent struct {
	Type       AGUIEventType `json:"type"`
	MessageID  string        `json:"messageId"`
	Role       string        `json:"role"`                // "assistant"
	AgentID    string        `json:"agentId,omitempty"`   // Which agent is responding (broadcast)
	IsThinking bool          `json:"isThinking,omitempty"` // Marks intermediate thinking steps
}

// AGUITextMessageContentEvent contains a chunk of text content
type AGUITextMessageContentEvent struct {
	Type      AGUIEventType `json:"type"`
	MessageID string        `json:"messageId"`
	Delta     string        `json:"delta"`
	AgentID   string        `json:"agentId,omitempty"`   // Which agent is responding (broadcast)
}

// AGUITextMessageEndEvent signals the end of a text message
type AGUITextMessageEndEvent struct {
	Type      AGUIEventType `json:"type"`
	MessageID string        `json:"messageId"`
	AgentID   string        `json:"agentId,omitempty"`   // Which agent is responding (broadcast)
}

// NewAGUIRunStartedEvent creates a new run started event
func NewAGUIRunStartedEvent(threadID, runID string) *AGUIRunStartedEvent {
	return &AGUIRunStartedEvent{
		Type:     AGUIEventRunStarted,
		ThreadID: threadID,
		RunID:    runID,
	}
}

// NewAGUIRunFinishedEvent creates a new run finished event
func NewAGUIRunFinishedEvent(threadID, runID string) *AGUIRunFinishedEvent {
	return &AGUIRunFinishedEvent{
		Type:     AGUIEventRunFinished,
		ThreadID: threadID,
		RunID:    runID,
	}
}

// NewAGUIRunErrorEvent creates a new run error event
func NewAGUIRunErrorEvent(message string) *AGUIRunErrorEvent {
	return &AGUIRunErrorEvent{
		Type:    AGUIEventRunError,
		Message: message,
	}
}

// NewAGUITextMessageStartEvent creates a new text message start event
func NewAGUITextMessageStartEvent(messageID string) *AGUITextMessageStartEvent {
	return &AGUITextMessageStartEvent{
		Type:      AGUIEventTextMessageStart,
		MessageID: messageID,
		Role:      "assistant",
	}
}

// NewAGUITextMessageStartEventFull creates a text message start event with agent ID and isThinking flag
func NewAGUITextMessageStartEventFull(messageID, agentID string, isThinking bool) *AGUITextMessageStartEvent {
	return &AGUITextMessageStartEvent{
		Type:       AGUIEventTextMessageStart,
		MessageID:  messageID,
		Role:       "assistant",
		AgentID:    agentID,
		IsThinking: isThinking,
	}
}

// NewAGUITextMessageContentEvent creates a new text message content event
func NewAGUITextMessageContentEvent(messageID, delta string) *AGUITextMessageContentEvent {
	return &AGUITextMessageContentEvent{
		Type:      AGUIEventTextMessageContent,
		MessageID: messageID,
		Delta:     delta,
	}
}

// NewAGUITextMessageContentEventWithAgent creates a new text message content event with agent ID
func NewAGUITextMessageContentEventWithAgent(messageID, delta, agentID string) *AGUITextMessageContentEvent {
	return &AGUITextMessageContentEvent{
		Type:      AGUIEventTextMessageContent,
		MessageID: messageID,
		Delta:     delta,
		AgentID:   agentID,
	}
}

// NewAGUITextMessageEndEvent creates a new text message end event
func NewAGUITextMessageEndEvent(messageID string) *AGUITextMessageEndEvent {
	return &AGUITextMessageEndEvent{
		Type:      AGUIEventTextMessageEnd,
		MessageID: messageID,
	}
}

// NewAGUITextMessageEndEventWithAgent creates a new text message end event with agent ID
func NewAGUITextMessageEndEventWithAgent(messageID, agentID string) *AGUITextMessageEndEvent {
	return &AGUITextMessageEndEvent{
		Type:      AGUIEventTextMessageEnd,
		MessageID: messageID,
		AgentID:   agentID,
	}
}

// AGUICustomEvent is a generic custom event
type AGUICustomEvent struct {
	Type       AGUIEventType          `json:"type"`
	SubType    string                 `json:"subType"`
	Data       map[string]interface{} `json:"data,omitempty"`
}

// NewAGUIConversationStepEvent creates a CUSTOM event for conversation step progress
func NewAGUIConversationStepEvent(agentID string, stepIndex, totalSteps int, isUserTurn bool) *AGUICustomEvent {
	return &AGUICustomEvent{
		Type:    AGUIEventCustom,
		SubType: "CONVERSATION_STEP",
		Data: map[string]interface{}{
			"agentId":    agentID,
			"stepIndex":  stepIndex,
			"totalSteps": totalSteps,
			"isUserTurn": isUserTurn,
		},
	}
}

// AGUIToolCallStartEvent signals the start of a tool call
type AGUIToolCallStartEvent struct {
	Type       AGUIEventType `json:"type"`
	ToolCallID string        `json:"toolCallId"`
	ToolName   string        `json:"toolName"`
	ServerID   string        `json:"serverId,omitempty"` // MCP server that owns this tool (multi-MCP)
}

// AGUIToolCallArgsEvent contains the arguments for a tool call
type AGUIToolCallArgsEvent struct {
	Type       AGUIEventType `json:"type"`
	ToolCallID string        `json:"toolCallId"`
	Delta      string        `json:"delta"`
}

// AGUIToolCallEndEvent signals the end of a tool call
type AGUIToolCallEndEvent struct {
	Type       AGUIEventType `json:"type"`
	ToolCallID string        `json:"toolCallId"`
	Result     string        `json:"result,omitempty"`
}

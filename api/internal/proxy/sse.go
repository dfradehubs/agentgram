package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/google/uuid"
)

// SSEWriter writes SSE events to the client using AG-UI protocol
type SSEWriter struct {
	w           http.ResponseWriter
	flusher     http.Flusher
	threadID    string
	runID       string
	messageID   string
	sessionName string
	mu          sync.Mutex
	onEvent     func(event interface{}) // Optional callback for each event
}

// NewSSEWriter creates a new SSEWriter with AG-UI protocol support
func NewSSEWriter(w http.ResponseWriter) (*SSEWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}

	// Configure SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	return &SSEWriter{
		w:        w,
		flusher:  flusher,
		threadID: uuid.New().String(),
		runID:    uuid.New().String(),
	}, nil
}

// SetThreadID sets the thread ID for the session
func (s *SSEWriter) SetThreadID(threadID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.threadID = threadID
}

// SetSessionName sets the session name to include in RUN_STARTED
func (s *SSEWriter) SetSessionName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionName = name
}

// SetOnEvent sets an optional callback invoked for each AG-UI event (for Pub/Sub broadcast).
func (s *SSEWriter) SetOnEvent(fn func(event interface{})) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onEvent = fn
}

// SendAGUIEvent sends an AG-UI protocol event
func (s *SSEWriter) SendAGUIEvent(event interface{}) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal AG-UI event: %w", err)
	}

	s.mu.Lock()
	onEvent := s.onEvent
	s.mu.Unlock()

	// Broadcast to subscribers (fire-and-forget, before writing to client)
	if onEvent != nil {
		onEvent(event)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = fmt.Fprintf(s.w, "data: %s\n\n", data)
	if err != nil {
		return fmt.Errorf("failed to write AG-UI event: %w", err)
	}

	s.flusher.Flush()
	return nil
}

// SendRunStarted sends the AG-UI RUN_STARTED event
func (s *SSEWriter) SendRunStarted() error {
	s.mu.Lock()
	threadID := s.threadID
	runID := s.runID
	sessionName := s.sessionName
	s.mu.Unlock()
	event := models.NewAGUIRunStartedEvent(threadID, runID)
	event.SessionName = sessionName
	return s.SendAGUIEvent(event)
}

// SendRunFinished sends the AG-UI RUN_FINISHED event
func (s *SSEWriter) SendRunFinished() error {
	s.mu.Lock()
	threadID := s.threadID
	runID := s.runID
	s.mu.Unlock()
	return s.SendAGUIEvent(models.NewAGUIRunFinishedEvent(threadID, runID))
}

// SendRunError sends the AG-UI RUN_ERROR event
func (s *SSEWriter) SendRunError(message string) error {
	return s.SendAGUIEvent(models.NewAGUIRunErrorEvent(message))
}

// SendTextMessageStart sends the AG-UI TEXT_MESSAGE_START event
func (s *SSEWriter) SendTextMessageStart() error {
	messageID := uuid.New().String()
	s.mu.Lock()
	s.messageID = messageID
	s.mu.Unlock()
	return s.SendAGUIEvent(models.NewAGUITextMessageStartEvent(messageID))
}

// SendTextMessageStartThinking sends a TEXT_MESSAGE_START marked as thinking
func (s *SSEWriter) SendTextMessageStartThinking() error {
	messageID := uuid.New().String()
	s.mu.Lock()
	s.messageID = messageID
	s.mu.Unlock()
	event := models.NewAGUITextMessageStartEvent(messageID)
	event.IsThinking = true
	return s.SendAGUIEvent(event)
}

// SendTextMessageContent sends the AG-UI TEXT_MESSAGE_CONTENT event
func (s *SSEWriter) SendTextMessageContent(delta string) error {
	s.mu.Lock()
	messageID := s.messageID
	s.mu.Unlock()
	return s.SendAGUIEvent(models.NewAGUITextMessageContentEvent(messageID, delta))
}

// SendTextMessageEnd sends the AG-UI TEXT_MESSAGE_END event
func (s *SSEWriter) SendTextMessageEnd() error {
	s.mu.Lock()
	messageID := s.messageID
	s.mu.Unlock()
	return s.SendAGUIEvent(models.NewAGUITextMessageEndEvent(messageID))
}

// Flush forces pending data to be sent
func (s *SSEWriter) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flusher.Flush()
}

// SendKeepAlive writes an SSE comment to keep intermediaries from timing out
// idle streams while waiting for the next agent chunk.
func (s *SSEWriter) SendKeepAlive() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := fmt.Fprint(s.w, ": keep-alive\n\n"); err != nil {
		return fmt.Errorf("failed to write SSE keep-alive: %w", err)
	}
	s.flusher.Flush()
	return nil
}

// SendToolCallStart sends the AG-UI TOOL_CALL_START event
func (s *SSEWriter) SendToolCallStart(toolCallID, toolName string) error {
	return s.SendAGUIEvent(&models.AGUIToolCallStartEvent{
		Type:       models.AGUIEventToolCallStart,
		ToolCallID: toolCallID,
		ToolName:   toolName,
	})
}

// SendToolCallArgs sends the AG-UI TOOL_CALL_ARGS event
func (s *SSEWriter) SendToolCallArgs(toolCallID, delta string) error {
	return s.SendAGUIEvent(&models.AGUIToolCallArgsEvent{
		Type:       models.AGUIEventToolCallArgs,
		ToolCallID: toolCallID,
		Delta:      delta,
	})
}

// SendToolCallEnd sends the AG-UI TOOL_CALL_END event
func (s *SSEWriter) SendToolCallEnd(toolCallID, result string) error {
	return s.SendAGUIEvent(&models.AGUIToolCallEndEvent{
		Type:       models.AGUIEventToolCallEnd,
		ToolCallID: toolCallID,
		Result:     result,
	})
}

// SendCustomEvent sends an AG-UI CUSTOM event with the given subType and data
func (s *SSEWriter) SendCustomEvent(subType string, data map[string]interface{}) error {
	return s.SendAGUIEvent(&models.AGUICustomEvent{
		Type:    models.AGUIEventCustom,
		SubType: subType,
		Data:    data,
	})
}

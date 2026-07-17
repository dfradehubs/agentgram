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
	w                 http.ResponseWriter
	flusher           http.Flusher
	threadID          string
	runID             string
	messageID         string
	sessionName       string
	agentID           string // When set, TEXT_MESSAGE_*/TOOL_CALL_START events are tagged with it (group debates)
	suppressLifecycle bool   // When true, RUN_STARTED/RUN_FINISHED are no-ops (outer run owns the lifecycle)
	deferLifecycle    bool   // When true, all RUN_* events are owned by the caller
	mu                sync.Mutex
	onEvent           func(event interface{}) // Optional callback for each event
	clientGone        bool                    // true once writing to the client failed / it disconnected
}

// SSEConfig bundles the client-facing stream configuration shared by all
// protocol handlers.
type SSEConfig struct {
	ThreadID          string
	SessionName       string
	AgentID           string
	SuppressLifecycle bool
	DeferLifecycle    bool
	Writer            *SSEWriter
	OnEvent           func(event interface{})
}

// Apply sets the configuration on the writer.
func (s *SSEWriter) Apply(cfg SSEConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cfg.ThreadID != "" {
		s.threadID = cfg.ThreadID
	}
	if cfg.SessionName != "" {
		s.sessionName = cfg.SessionName
	}
	s.agentID = cfg.AgentID
	s.suppressLifecycle = cfg.SuppressLifecycle
	s.deferLifecycle = cfg.DeferLifecycle
	if cfg.OnEvent != nil {
		s.onEvent = cfg.OnEvent
	}
}

func resolveSSEWriter(w http.ResponseWriter, cfg SSEConfig) (*SSEWriter, error) {
	if cfg.Writer != nil {
		cfg.Writer.Apply(cfg)
		return cfg.Writer, nil
	}
	sse, err := NewSSEWriter(w)
	if err != nil {
		return nil, err
	}
	sse.Apply(cfg)
	return sse, nil
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

// MarkClientGone marks the client as disconnected. Subsequent events still fire
// onEvent (so buffering/broadcast keeps working in drain mode) but are no longer
// written to the client.
func (s *SSEWriter) MarkClientGone() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clientGone = true
}

// SendAGUIEvent sends an AG-UI protocol event
func (s *SSEWriter) SendAGUIEvent(event interface{}) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal AG-UI event: %w", err)
	}

	s.mu.Lock()
	onEvent := s.onEvent
	clientGone := s.clientGone
	s.mu.Unlock()

	// Always notify subscribers/buffer first — this must keep working even after
	// the client disconnects, so a reconnecting client can replay the run.
	if onEvent != nil {
		onEvent(event)
	}

	// Client gone: it was buffered/broadcast above; skip the (failing) write.
	if clientGone {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = fmt.Fprintf(s.w, "data: %s\n\n", data)
	if err != nil {
		s.clientGone = true
		return fmt.Errorf("failed to write AG-UI event: %w", err)
	}

	s.flusher.Flush()
	return nil
}

// SendRunStarted sends the AG-UI RUN_STARTED event.
// No-op when lifecycle is suppressed (an outer run owns RUN_STARTED/RUN_FINISHED).
func (s *SSEWriter) SendRunStarted() error {
	s.mu.Lock()
	threadID := s.threadID
	runID := s.runID
	sessionName := s.sessionName
	suppress := s.suppressLifecycle
	deferred := s.deferLifecycle
	s.mu.Unlock()
	if suppress || deferred {
		return nil
	}
	event := models.NewAGUIRunStartedEvent(threadID, runID)
	event.SessionName = sessionName
	return s.SendAGUIEvent(event)
}

// SendRunFinished sends the AG-UI RUN_FINISHED event.
// No-op when lifecycle is suppressed (an outer run owns RUN_STARTED/RUN_FINISHED).
func (s *SSEWriter) SendRunFinished() error {
	s.mu.Lock()
	threadID := s.threadID
	runID := s.runID
	suppress := s.suppressLifecycle
	deferred := s.deferLifecycle
	s.mu.Unlock()
	if suppress || deferred {
		return nil
	}
	return s.SendAGUIEvent(models.NewAGUIRunFinishedEvent(threadID, runID))
}

// SendRunError sends the AG-UI RUN_ERROR event.
// When lifecycle is suppressed (a debate turn inside an outer run), a bare
// RUN_ERROR would abort the whole run on the client even though the debate
// continues — emit an agent-scoped CUSTOM event instead. Persistence is owned
// by the surface handler and may independently fail.
func (s *SSEWriter) SendRunError(message string) error {
	s.mu.Lock()
	suppress := s.suppressLifecycle
	deferred := s.deferLifecycle
	agentID := s.agentID
	s.mu.Unlock()
	if deferred {
		return nil
	}
	message = PublicErrorMessage(fmt.Errorf("%s", message))
	if suppress {
		return s.SendCustomEvent("turn.error", map[string]interface{}{
			"agentId": agentID,
			"message": message,
		})
	}
	return s.SendAGUIEvent(models.NewAGUIRunErrorEvent(message))
}

// SendTextMessageStart sends the AG-UI TEXT_MESSAGE_START event
func (s *SSEWriter) SendTextMessageStart() error {
	messageID := uuid.New().String()
	s.mu.Lock()
	s.messageID = messageID
	agentID := s.agentID
	s.mu.Unlock()
	return s.SendAGUIEvent(models.NewAGUITextMessageStartEventFull(messageID, agentID, false))
}

// SendTextMessageStartThinking sends a TEXT_MESSAGE_START marked as thinking
func (s *SSEWriter) SendTextMessageStartThinking() error {
	messageID := uuid.New().String()
	s.mu.Lock()
	s.messageID = messageID
	agentID := s.agentID
	s.mu.Unlock()
	return s.SendAGUIEvent(models.NewAGUITextMessageStartEventFull(messageID, agentID, true))
}

// SendTextMessageContent sends the AG-UI TEXT_MESSAGE_CONTENT event
func (s *SSEWriter) SendTextMessageContent(delta string) error {
	s.mu.Lock()
	messageID := s.messageID
	agentID := s.agentID
	s.mu.Unlock()
	return s.SendAGUIEvent(models.NewAGUITextMessageContentEventWithAgent(messageID, delta, agentID))
}

// SendTextMessageEnd sends the AG-UI TEXT_MESSAGE_END event
func (s *SSEWriter) SendTextMessageEnd() error {
	s.mu.Lock()
	messageID := s.messageID
	agentID := s.agentID
	s.mu.Unlock()
	return s.SendAGUIEvent(models.NewAGUITextMessageEndEventWithAgent(messageID, agentID))
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
	s.mu.Lock()
	agentID := s.agentID
	s.mu.Unlock()
	return s.SendAGUIEvent(&models.AGUIToolCallStartEvent{
		Type:       models.AGUIEventToolCallStart,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		AgentID:    agentID,
	})
}

// SendToolCallArgs sends the AG-UI TOOL_CALL_ARGS event
func (s *SSEWriter) SendToolCallArgs(toolCallID, delta string) error {
	s.mu.Lock()
	agentID := s.agentID
	s.mu.Unlock()
	return s.SendAGUIEvent(&models.AGUIToolCallArgsEvent{
		Type:       models.AGUIEventToolCallArgs,
		ToolCallID: toolCallID,
		Delta:      delta,
		AgentID:    agentID,
	})
}

// SendToolCallEnd sends the AG-UI TOOL_CALL_END event
func (s *SSEWriter) SendToolCallEnd(toolCallID, result string) error {
	s.mu.Lock()
	agentID := s.agentID
	s.mu.Unlock()
	return s.SendAGUIEvent(&models.AGUIToolCallEndEvent{
		Type:       models.AGUIEventToolCallEnd,
		ToolCallID: toolCallID,
		Result:     result,
		AgentID:    agentID,
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

package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	slackapi "github.com/slack-go/slack"
	"go.uber.org/zap"
)

const (
	debounceInterval = 200 * time.Millisecond
	writingEmoji     = ":writing_hand:"
)

// StreamingWriter implements http.ResponseWriter and http.Flusher.
// It intercepts AG-UI SSE events from the proxy and sends them progressively to Slack.
// Tool calls are shown inline with their name at the exact point they occur.
type StreamingWriter struct {
	client    *slackapi.Client
	channelID string
	threadTS  string
	agentID   string
	formatter *Formatter
	logger    *zap.Logger

	mu              sync.Mutex
	messageTS       string          // TS of the current message being edited
	toolCount       int
	textBuf         strings.Builder // accumulated display text
	headerBuf       bytes.Buffer    // raw SSE buffer for parsing
	statusCode      int
	debounceTimer   *time.Timer
	lastError       string
	overflowPending bool            // true when current message hit max length
	msgStartOffset  int             // offset in textBuf where the current message's text starts
	lastSentLen     int             // length of fullText last successfully sent
}

func NewStreamingWriter(client *slackapi.Client, channelID, threadTS, agentID string, formatter *Formatter, logger *zap.Logger) *StreamingWriter {
	return &StreamingWriter{
		client:    client,
		channelID: channelID,
		threadTS:  threadTS,
		agentID:   agentID,
		formatter: formatter,
		logger:    logger,
	}
}

// PostInitialMessage posts the "Escribiendo..." message immediately.
func (sw *StreamingWriter) PostInitialMessage() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if sw.messageTS != "" {
		return
	}

	_, ts, err := sw.client.PostMessage(
		sw.channelID,
		slackapi.MsgOptionText(fmt.Sprintf("%s Escribiendo...", writingEmoji), false),
		slackapi.MsgOptionTS(sw.threadTS),
	)
	if err != nil {
		sw.logger.Warn("failed to post initial message", zap.String("agent_id", sw.agentID), zap.Error(err))
		return
	}
	sw.messageTS = ts
	SlackAPICallsTotal.WithLabelValues(sw.agentID, "chat.postMessage").Inc()
}

func (sw *StreamingWriter) Header() http.Header       { return http.Header{} }
func (sw *StreamingWriter) WriteHeader(code int)       { sw.statusCode = code }
func (sw *StreamingWriter) Flush()                     {}
func (sw *StreamingWriter) FullText() string           { sw.mu.Lock(); defer sw.mu.Unlock(); return sw.textBuf.String() }
func (sw *StreamingWriter) LastError() string          { sw.mu.Lock(); defer sw.mu.Unlock(); return sw.lastError }

// Write implements http.ResponseWriter.
func (sw *StreamingWriter) Write(data []byte) (int, error) {
	sw.mu.Lock()
	sw.headerBuf.Write(data)
	raw := sw.headerBuf.String()
	sw.headerBuf.Reset()
	sw.mu.Unlock()

	lines := strings.Split(raw, "\n")
	var incomplete string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "data: ") {
			sw.processEvent(strings.TrimPrefix(trimmed, "data: "))
		} else if trimmed != ":" {
			incomplete += line + "\n"
		}
	}

	if incomplete != "" {
		sw.mu.Lock()
		sw.headerBuf.WriteString(incomplete)
		sw.mu.Unlock()
	}
	return len(data), nil
}

// Finalize flushes pending updates and sends the final message.
// If the message exceeds Slack's limit, it splits into continuation messages.
func (sw *StreamingWriter) Finalize() {
	sw.mu.Lock()
	if sw.debounceTimer != nil {
		sw.debounceTimer.Stop()
	}
	sw.mu.Unlock()

	sw.sendUpdate(true)

	// If sendUpdate triggered overflow, post the remaining text as a new message.
	// Loop in case the continuation itself overflows (very long responses).
	for i := 0; i < 10; i++ {
		sw.mu.Lock()
		overflow := sw.overflowPending
		sw.mu.Unlock()
		if !overflow {
			break
		}
		sw.sendUpdate(true)
	}
}

func (sw *StreamingWriter) processEvent(jsonStr string) {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return
	}

	switch raw["type"] {
	case "TEXT_MESSAGE_CONTENT":
		delta, _ := raw["delta"].(string)
		if delta != "" {
			sw.mu.Lock()
			sw.textBuf.WriteString(delta)
			sw.mu.Unlock()
			sw.scheduleDebouncedUpdate()
		}

	case "TOOL_CALL_START":
		toolName, _ := raw["toolName"].(string)
		if toolName == "" {
			toolName, _ = raw["name"].(string)
		}
		if toolName == "" {
			toolName = "tool"
		}
		sw.mu.Lock()
		sw.toolCount++
		fmt.Fprintf(&sw.textBuf, "\n\n:gear: `%s`\n", toolName)
		sw.mu.Unlock()
		sw.scheduleDebouncedUpdate()

	case "RUN_ERROR":
		msg, _ := raw["message"].(string)
		sw.mu.Lock()
		sw.lastError = msg
		sw.mu.Unlock()
		sw.postError(msg)
	}
}

func (sw *StreamingWriter) scheduleDebouncedUpdate() {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if sw.debounceTimer != nil {
		sw.debounceTimer.Stop()
	}
	sw.debounceTimer = time.AfterFunc(debounceInterval, func() {
		sw.sendUpdate(false)
	})
}

func (sw *StreamingWriter) sendUpdate(isFinal bool) {
	sw.mu.Lock()
	ts := sw.messageTS
	fullText := sw.textBuf.String()
	overflow := sw.overflowPending
	offset := sw.msgStartOffset
	sw.mu.Unlock()

	if ts == "" || fullText == "" {
		return
	}

	// Text for the CURRENT message only (from offset onwards)
	currentText := fullText[offset:]
	if currentText == "" {
		return
	}

	if overflow {
		// Need a new message — post only the unsent portion
		var msg string
		if isFinal {
			msg = sw.formatter.ToSlackMrkdwn(currentText)
		} else {
			msg = currentText
		}
		_, newTS, err := sw.client.PostMessage(
			sw.channelID,
			slackapi.MsgOptionText(msg, false),
			slackapi.MsgOptionTS(sw.threadTS),
		)
		if err != nil {
			sw.logger.Warn("failed to post continuation message", zap.String("agent_id", sw.agentID), zap.Error(err))
			return
		}
		SlackAPICallsTotal.WithLabelValues(sw.agentID, "chat.postMessage").Inc()

		sw.mu.Lock()
		sw.messageTS = newTS
		sw.overflowPending = false
		sw.msgStartOffset = offset // this message starts from here
		sw.mu.Unlock()
		return
	}

	var msg string
	if isFinal {
		msg = sw.formatter.ToSlackMrkdwn(currentText)
	} else {
		msg = currentText
	}

	_, _, _, err := sw.client.UpdateMessage(sw.channelID, ts, slackapi.MsgOptionText(msg, false))
	if err != nil {
		if strings.Contains(err.Error(), "msg_too_long") {
			// Freeze current message. Next debounce creates a new one starting from current position.
			sw.mu.Lock()
			sw.overflowPending = true
			sw.msgStartOffset = sw.lastSentLen // new message starts where last successful update ended
			sw.mu.Unlock()
			return
		}
		if isFinal {
			sw.logger.Warn("failed to send final update", zap.String("agent_id", sw.agentID), zap.Error(err))
		}
	} else {
		sw.mu.Lock()
		sw.lastSentLen = len(fullText)
		sw.mu.Unlock()
	}
	SlackAPICallsTotal.WithLabelValues(sw.agentID, "chat.update").Inc()
}

func (sw *StreamingWriter) postError(msg string) {
	errorText := classifyError(fmt.Errorf("%s", msg))
	sw.mu.Lock()
	ts := sw.messageTS
	sw.mu.Unlock()

	if ts != "" {
		sw.client.UpdateMessage(sw.channelID, ts, slackapi.MsgOptionText(errorText, false))
		return
	}
	sw.client.PostMessage(sw.channelID, slackapi.MsgOptionText(errorText, false), slackapi.MsgOptionTS(sw.threadTS))
}

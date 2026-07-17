package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"go.uber.org/zap"
)

// a2aFrame writes one A2A JSON-RPC status-update SSE frame.
func a2aFrame(w http.ResponseWriter, fl http.Flusher, state, text string) {
	msg := ""
	if text != "" {
		msg = fmt.Sprintf(`,"message":{"messageId":"m","role":"agent","parts":[{"kind":"text","text":%q}]}`, text)
	}
	fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"kind\":\"status-update\",\"contextId\":\"ctx-1\",\"status\":{\"state\":%q%s}}}\n\n", state, msg)
	fl.Flush()
}

// Regression: an A2A task that emits partial content and then reports the
// explicit "canceled" state is incomplete, not a clean success. Handle must
// return an error and flag ProxyResult.Error so a group debate treats the turn
// as failed instead of presenting a truncated reply as final.
func TestA2ACanceledIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
		a2aFrame(w, fl, "working", "partial answer so far")
		a2aFrame(w, fl, "canceled", "")
	}))
	defer srv.Close()

	p := NewA2AProxy(zap.NewNop())
	agent := &models.Agent{ID: "a", Protocol: "a2a", Endpoint: srv.URL}
	rec := httptest.NewRecorder()
	res, err := p.Handle(context.Background(), rec, agent,
		&models.ChatRequest{Messages: []models.ChatMessage{{Role: "user", Content: "hi"}}},
		agents.OutboundAuth{}, "req-1", SSEConfig{}, time.Minute)

	if err == nil {
		t.Fatal("canceled task returned nil error (treated as success)")
	}
	if res == nil || res.Error == "" || !strings.Contains(res.AssistantText, "partial answer") {
		t.Errorf("canceled task did not preserve its partial result: %#v", res)
	}
}

// Regression: the agent streams partial content and then stalls; the agent
// timeout fires mid-stream. A context-driven read error must NOT be reported as
// a successful run just because content accumulated.
func TestA2ATimeoutMidStreamIsError(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
		a2aFrame(w, fl, "working", "partial before timeout")
		<-release // hang until the test lets go, forcing the agent timeout to fire
	}))
	defer srv.Close()
	defer close(release)

	p := NewA2AProxy(zap.NewNop())
	agent := &models.Agent{ID: "a", Protocol: "a2a", Endpoint: srv.URL}
	rec := httptest.NewRecorder()
	res, err := p.Handle(context.Background(), rec, agent,
		&models.ChatRequest{Messages: []models.ChatMessage{{Role: "user", Content: "hi"}}},
		agents.OutboundAuth{}, "req-1", SSEConfig{}, 200*time.Millisecond)

	if err == nil {
		t.Fatal("mid-stream timeout returned nil error (truncated reply treated as success)")
	}
	if res == nil || res.Error == "" || !strings.Contains(res.AssistantText, "partial before timeout") {
		t.Errorf("timeout did not preserve its partial result: %#v", res)
	}
}

// Regression: a transport EOF/reset before the protocol's explicit completed
// state is an interrupted response, even if the agent emitted partial content.
func TestA2AEOFBeforeCompletedIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
		a2aFrame(w, fl, "working", "partial before disconnect")
		// Return without a terminal status-update:completed frame.
	}))
	defer srv.Close()

	p := NewA2AProxy(zap.NewNop())
	agent := &models.Agent{ID: "a", Protocol: "a2a", Endpoint: srv.URL}
	rec := httptest.NewRecorder()
	res, err := p.Handle(context.Background(), rec, agent,
		&models.ChatRequest{Messages: []models.ChatMessage{{Role: "user", Content: "hi"}}},
		agents.OutboundAuth{}, "req-1", SSEConfig{}, time.Minute)

	if err == nil {
		t.Fatal("EOF before completed returned nil error")
	}
	if res == nil || res.Error == "" || !strings.Contains(res.AssistantText, "partial before disconnect") {
		t.Errorf("EOF did not return a partial error result: %#v", res)
	}
	if strings.Contains(rec.Body.String(), `"type":"RUN_FINISHED"`) {
		t.Error("EOF before completed emitted RUN_FINISHED")
	}
}

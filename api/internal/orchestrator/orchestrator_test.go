package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/llm"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"go.uber.org/zap"
)

// fakeProvider returns scripted responses in order, then FINISH.
type fakeProvider struct {
	responses []string
	prompts   []string
}

func (f *fakeProvider) GenerateContent(_ context.Context, req *llm.Request) (*llm.Response, error) {
	// Record the rendered prompt (last user message) for assertions
	var prompt string
	if len(req.Messages) > 0 {
		if s, ok := req.Messages[len(req.Messages)-1].Content.(string); ok {
			prompt = s
		}
	}
	f.prompts = append(f.prompts, prompt)

	if len(f.prompts)-1 >= len(f.responses) {
		return &llm.Response{Text: "FINISH"}, nil
	}
	return &llm.Response{Text: f.responses[len(f.prompts)-1]}, nil
}

var testRoster = []AgentBrief{
	{ID: "logs-agent", Name: "Logs", Description: "Searches logs"},
	{ID: "kube-agent", Name: "Kube", Description: "Kubernetes operations"},
}

func newTestModerator(responses ...string) (*Moderator, *fakeProvider) {
	fp := &fakeProvider{responses: responses}
	return NewWithProvider(fp, zap.NewNop()), fp
}

func TestDebate(t *testing.T) {
	tests := []struct {
		name          string
		moderatorSays []string
		maxTurns      int
		runErr        map[string]error // agentID -> error to return
		wantTurns     []string         // expected agent IDs run, in order
		wantErrTurns  int              // how many TurnResults carry an error
		wantDebateErr error
	}{
		{
			name:          "sequence then FINISH",
			moderatorSays: []string{"logs-agent", "kube-agent", "FINISH"},
			maxTurns:      6,
			wantTurns:     []string{"logs-agent", "kube-agent"},
		},
		{
			name:          "maxTurns cap respected without FINISH",
			moderatorSays: []string{"logs-agent", "logs-agent", "logs-agent", "logs-agent"},
			maxTurns:      2,
			wantTurns:     []string{"logs-agent", "logs-agent"},
			wantDebateErr: ErrMaxTurnsReached,
		},
		{
			name:          "unknown agent id treated as FINISH",
			moderatorSays: []string{"logs-agent", "nonexistent-agent"},
			maxTurns:      6,
			wantTurns:     []string{"logs-agent"},
		},
		{
			name:          "failed turn does not abort debate",
			moderatorSays: []string{"logs-agent", "kube-agent", "FINISH"},
			maxTurns:      6,
			runErr:        map[string]error{"logs-agent": errors.New("agent down")},
			wantTurns:     []string{"logs-agent", "kube-agent"},
			wantErrTurns:  1,
		},
		{
			name:          "immediate FINISH runs nobody",
			moderatorSays: []string{"FINISH"},
			maxTurns:      6,
			wantTurns:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mod, _ := newTestModerator(tt.moderatorSays...)

			var ran []string
			run := func(_ context.Context, agentID string) (string, error) {
				ran = append(ran, agentID)
				if err := tt.runErr[agentID]; err != nil {
					return "", err
				}
				return fmt.Sprintf("reply from %s", agentID), nil
			}

			results, err := mod.Debate(context.Background(), testRoster, "User: hello", run, tt.maxTurns)
			if !errors.Is(err, tt.wantDebateErr) {
				t.Fatalf("Debate error = %v, want %v", err, tt.wantDebateErr)
			}

			if len(ran) != len(tt.wantTurns) {
				t.Fatalf("ran %v, want %v", ran, tt.wantTurns)
			}
			for i, id := range tt.wantTurns {
				if ran[i] != id {
					t.Errorf("turn %d: ran %s, want %s", i, ran[i], id)
				}
			}

			errTurns := 0
			for _, r := range results {
				if r.Err != nil {
					errTurns++
				}
			}
			if errTurns != tt.wantErrTurns {
				t.Errorf("error turns = %d, want %d", errTurns, tt.wantErrTurns)
			}
		})
	}
}

func TestDebateTranscriptGrows(t *testing.T) {
	// The second NextSpeaker prompt must contain the first agent's reply,
	// so agent N's selection is informed by agent N-1's contribution.
	mod, fp := newTestModerator("logs-agent", "kube-agent", "FINISH")

	run := func(_ context.Context, agentID string) (string, error) {
		return fmt.Sprintf("UNIQUE-REPLY-%s", agentID), nil
	}

	if _, err := mod.Debate(context.Background(), testRoster, "User: hello", run, 6); err != nil {
		t.Fatalf("Debate returned error: %v", err)
	}

	if len(fp.prompts) < 2 {
		t.Fatalf("expected at least 2 moderator calls, got %d", len(fp.prompts))
	}
	if !strings.Contains(fp.prompts[1], "UNIQUE-REPLY-logs-agent") {
		t.Errorf("second moderator prompt does not contain first agent's reply:\n%s", fp.prompts[1])
	}
	// Failed-turn errors must also be visible to the moderator (covered in prompt)
	if !strings.Contains(fp.prompts[0], "logs-agent") || !strings.Contains(fp.prompts[0], "Searches logs") {
		t.Errorf("first prompt missing roster info:\n%s", fp.prompts[0])
	}
}

func TestDebatePreservesPartialTextOnError(t *testing.T) {
	mod, _ := newTestModerator("logs-agent", "FINISH")
	results, err := mod.Debate(context.Background(), testRoster, "User: hello", func(_ context.Context, _ string) (string, error) {
		return "partial answer", errors.New("stream interrupted")
	}, 2)
	if err != nil {
		t.Fatalf("Debate returned transport error: %v", err)
	}
	if len(results) != 1 || results[0].Text != "partial answer" || results[0].Err == nil {
		t.Fatalf("partial failed turn was not preserved: %#v", results)
	}
}

func TestTurnErrorKindPrioritizesPersistence(t *testing.T) {
	err := errors.Join(
		NewTurnError(TurnFailureAgent, errors.New("stream failed")),
		NewTurnError(TurnFailurePersistence, errors.New("store failed")),
	)
	if got := TurnErrorKind(err); got != TurnFailurePersistence {
		t.Fatalf("TurnErrorKind = %q, want persistence", got)
	}
}

func TestNextSpeakerParsing(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantID   string
		wantDone bool
	}{
		{"plain id", "logs-agent", "logs-agent", false},
		{"id with whitespace", "  kube-agent\n", "kube-agent", false},
		{"finish", "FINISH", "", true},
		{"finish lowercase", "finish", "", true},
		{"unknown id", "made-up-agent", "", true},
		{"empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mod, _ := newTestModerator(tt.response)
			id, done, err := mod.NextSpeaker(context.Background(), testRoster, "User: hi")
			if err != nil {
				t.Fatalf("NextSpeaker error: %v", err)
			}
			if id != tt.wantID || done != tt.wantDone {
				t.Errorf("got (%q, %v), want (%q, %v)", id, done, tt.wantID, tt.wantDone)
			}
		})
	}
}

func TestRenderTranscriptBounded(t *testing.T) {
	// Build a session far beyond the caps: 100 messages, one of them huge
	msgs := make([]models.ChatMessage, 0, 100)
	for i := 0; i < 100; i++ {
		msgs = append(msgs, models.ChatMessage{Role: "user", Content: fmt.Sprintf("message-%d", i)})
	}
	msgs[99].Content = strings.Repeat("x", 10*transcriptMaxMsgChars)

	out := RenderTranscript(msgs)

	if strings.Contains(out, "message-0") || strings.Contains(out, "message-69") {
		t.Error("transcript includes messages beyond the recency cap")
	}
	if !strings.Contains(out, "message-98") {
		t.Error("transcript missing recent messages")
	}
	// The huge message must be truncated, keeping the whole transcript bounded
	if len(out) > transcriptMaxMessages*(transcriptMaxMsgChars+64) {
		t.Errorf("transcript not bounded: %d bytes", len(out))
	}
}

func TestSynthesize(t *testing.T) {
	mod, fp := newTestModerator("A concise synthesis.")
	out, err := mod.Synthesize(context.Background(), "User: hi\nAgent[a]: foo\nAgent[b]: bar")
	if err != nil {
		t.Fatalf("Synthesize error: %v", err)
	}
	if out != "A concise synthesis." {
		t.Errorf("got %q", out)
	}
	if len(fp.prompts) != 1 || !strings.Contains(fp.prompts[0], "Agent[a]: foo") {
		t.Errorf("synthesis prompt missing transcript: %v", fp.prompts)
	}
}

package fileprocessor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/llm"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"go.uber.org/zap"
)

// newTestProcessor creates a Processor that talks to a local httptest server.
// The caller provides the handler and gets back the processor and a cleanup func.
func newTestProcessor(t *testing.T, provider string, handler http.HandlerFunc) *Processor {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	model := &models.LLMModel{
		Provider: provider,
		Model:    "test-model",
		APIKey:   "test-key",
		Enabled:  true,
	}

	// Create HTTP client that routes all requests to the test server
	client := srv.Client()
	client.Transport = &rewriteTransport{base: srv.Client().Transport, serverURL: srv.URL}

	p, err := llm.NewProviderWithClient(model, client)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	return &Processor{
		provider: p,
		model:    model,
		logger:   zap.NewNop(),
	}
}

// rewriteTransport redirects all requests to the test server URL.
type rewriteTransport struct {
	base      http.RoundTripper
	serverURL string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "http"
	// Parse the test server URL to get host
	req.URL.Host = strings.TrimPrefix(t.serverURL, "http://")
	return t.base.RoundTrip(req)
}

func anthropicResponse(text string) []byte {
	resp := map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

func openaiResponse(text string) []byte {
	resp := map[string]interface{}{
		"choices": []map[string]interface{}{
			{"message": map[string]string{"content": text}},
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

func TestProcessAttachments_ImageReformulated(t *testing.T) {
	callCount := 0
	p := newTestProcessor(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// Vision call - describe image
			w.Write(anthropicResponse("Screenshot showing error 'connection refused' on port 5432"))
		} else {
			// Reformulation call
			w.Write(anthropicResponse("I'm seeing a 'connection refused' error on PostgreSQL port 5432. Why is the connection failing?"))
		}
	})

	msg := &models.ChatMessage{
		Role:    "user",
		Content: "why is it failing?",
		Attachments: []models.Attachment{
			{Filename: "error.png", ContentType: "image/png", Data: "fakebase64data"},
		},
	}

	err := p.ProcessAttachments(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Attachments != nil {
		t.Errorf("expected attachments to be nil, got %v", msg.Attachments)
	}
	if callCount != 2 {
		t.Errorf("expected 2 LLM calls (vision + reformulation), got %d", callCount)
	}
	if !strings.Contains(msg.Content, "connection refused") {
		t.Errorf("expected reformulated content to contain image context, got: %s", msg.Content)
	}
}

func TestProcessAttachments_TextFileReformulated(t *testing.T) {
	p := newTestProcessor(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		// Only reformulation call (text files don't call vision)
		w.Write(anthropicResponse("The config.json file contains an incorrect port (3000 instead of 8080). I need to fix the server configuration."))
	})

	configJSON := base64.StdEncoding.EncodeToString([]byte(`{"port": 3000}`))
	msg := &models.ChatMessage{
		Role:    "user",
		Content: "is the config correct?",
		Attachments: []models.Attachment{
			{Filename: "config.json", ContentType: "application/json", Data: configJSON},
		},
	}

	err := p.ProcessAttachments(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Attachments != nil {
		t.Errorf("expected attachments to be nil, got %v", msg.Attachments)
	}
	if !strings.Contains(msg.Content, "config") {
		t.Errorf("expected reformulated content to reference config, got: %s", msg.Content)
	}
}

func TestProcessAttachments_MultipleFiles(t *testing.T) {
	callCount := 0
	p := newTestProcessor(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// Vision call for the image
			w.Write(anthropicResponse("Dashboard showing CPU at 95%"))
		} else {
			// Reformulation call
			w.Write(anthropicResponse("The dashboard shows CPU at 95% and the logs indicate multiple goroutine leaks. How can I optimize CPU usage?"))
		}
	})

	logsData := base64.StdEncoding.EncodeToString([]byte("goroutine leak detected\ngoroutine leak detected"))
	msg := &models.ChatMessage{
		Role:    "user",
		Content: "how do I optimize?",
		Attachments: []models.Attachment{
			{Filename: "dashboard.png", ContentType: "image/png", Data: "fakebase64"},
			{Filename: "logs.txt", ContentType: "text/plain", Data: logsData},
		},
	}

	err := p.ProcessAttachments(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Attachments != nil {
		t.Errorf("expected attachments to be nil")
	}
	// 1 vision call + 1 reformulation = 2
	if callCount != 2 {
		t.Errorf("expected 2 LLM calls, got %d", callCount)
	}
}

func TestProcessAttachments_ReformulationFallback(t *testing.T) {
	callCount := 0
	p := newTestProcessor(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// Vision call succeeds
			w.Write(anthropicResponse("Error screenshot"))
		} else {
			// Reformulation fails
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "overloaded"}`))
		}
	})

	msg := &models.ChatMessage{
		Role:    "user",
		Content: "what's going on?",
		Attachments: []models.Attachment{
			{Filename: "error.png", ContentType: "image/png", Data: "fakebase64"},
		},
	}

	err := p.ProcessAttachments(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fallback to prepend format
	if !strings.Contains(msg.Content, "[File: error.png]") {
		t.Errorf("expected fallback prepend format, got: %s", msg.Content)
	}
	if !strings.Contains(msg.Content, "what's going on?") {
		t.Errorf("expected original question preserved in fallback, got: %s", msg.Content)
	}
	if msg.Attachments != nil {
		t.Errorf("expected attachments to be nil even on fallback")
	}
}

func TestProcessAttachments_NoText(t *testing.T) {
	callCount := 0
	p := newTestProcessor(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Write(anthropicResponse("Screenshot of a deployment pipeline with failed stage"))
		} else {
			w.Write(anthropicResponse("I see a deployment pipeline with a failed stage. What is the error causing the pipeline failure?"))
		}
	})

	msg := &models.ChatMessage{
		Role:    "user",
		Content: "", // No text from user
		Attachments: []models.Attachment{
			{Filename: "pipeline.png", ContentType: "image/png", Data: "fakebase64"},
		},
	}

	err := p.ProcessAttachments(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Content == "" {
		t.Error("expected LLM to generate a question from the image, got empty content")
	}
	if msg.Attachments != nil {
		t.Errorf("expected attachments to be nil")
	}
}

func TestReformulate_Anthropic(t *testing.T) {
	p := newTestProcessor(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		// Verify request structure
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		if body["system"] == nil || body["system"] == "" {
			t.Error("expected system prompt in anthropic request")
		}
		msgs, ok := body["messages"].([]interface{})
		if !ok || len(msgs) != 1 {
			t.Error("expected exactly 1 user message")
		}

		// Verify headers
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected x-api-key header, got: %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("expected anthropic-version header")
		}

		w.Write(anthropicResponse("Reformulated question"))
	})

	files := []extractedFile{
		{filename: "test.txt", content: "some content"},
	}

	result, err := p.reformulate(context.Background(), files, "what is this?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Reformulated question" {
		t.Errorf("expected 'Reformulated question', got: %s", result)
	}
}

func TestReformulate_OpenAI(t *testing.T) {
	p := newTestProcessor(t, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		msgs, ok := body["messages"].([]interface{})
		if !ok || len(msgs) != 2 {
			t.Error("expected 2 messages (system + user) for openai")
		}

		// Verify auth header
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("expected Bearer auth header for openai")
		}

		w.Write(openaiResponse("OpenAI reformulated question"))
	})

	files := []extractedFile{
		{filename: "data.csv", content: "col1,col2\na,b"},
	}

	result, err := p.reformulate(context.Background(), files, "analyze this")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "OpenAI reformulated question" {
		t.Errorf("expected 'OpenAI reformulated question', got: %s", result)
	}
}

func TestProcessAttachments_NoAttachments(t *testing.T) {
	p := &Processor{logger: zap.NewNop()}
	msg := &models.ChatMessage{Content: "hello", Attachments: nil}
	err := p.ProcessAttachments(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Content != "hello" {
		t.Errorf("content should be unchanged, got: %s", msg.Content)
	}
}

func TestBuildPrependFallback(t *testing.T) {
	files := []extractedFile{
		{filename: "a.txt", content: "content A"},
		{filename: "b.txt", content: "content B"},
	}
	result := buildPrependFallback(files)
	if !strings.Contains(result, "[File: a.txt]") {
		t.Error("expected fallback to contain filename a.txt")
	}
	if !strings.Contains(result, "[File: b.txt]") {
		t.Error("expected fallback to contain filename b.txt")
	}
	if !strings.Contains(result, "content A") || !strings.Contains(result, "content B") {
		t.Error("expected fallback to contain file contents")
	}
}

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDemoChat_EchoesLastUserMessage(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"ping"}]}`
	req := httptest.NewRequest(http.MethodPost, "/demo/chat", strings.NewReader(body))
	rec := httptest.NewRecorder()
	DemoChat(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !strings.Contains(resp["content"], "ping") {
		t.Fatalf("content = %q", resp["content"])
	}
}

func TestDemoChat_QueryField(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/demo/chat", strings.NewReader(`{"query":"hello"}`))
	rec := httptest.NewRecorder()
	DemoChat(rec, req)
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if !strings.Contains(resp["content"], "hello") {
		t.Fatalf("content = %q", resp["content"])
	}
}

func TestDemoChat_RejectsGet(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/demo/chat", nil)
	rec := httptest.NewRecorder()
	DemoChat(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rec.Code)
	}
}

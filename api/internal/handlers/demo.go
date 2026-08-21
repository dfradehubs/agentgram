package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// DemoChat is a built-in echo agent so a fresh instance can chat without
// registering anything. Enabled when seed.demo_agent is true.
func DemoChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	msg := lastUserMessage(body)
	if msg == "" {
		msg = "(empty)"
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"content": "Hello from Agentgram. You said: " + msg +
			"\n\nThis is the built-in demo agent. Register your own REST, A2A, ADK or MCP endpoints from Admin → Agents.",
	})
}

func lastUserMessage(body []byte) string {
	var req struct {
		Query    string `json:"query"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return strings.TrimSpace(string(body))
	}
	if req.Query != "" {
		return req.Query
	}
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" && req.Messages[i].Content != "" {
			return req.Messages[i].Content
		}
	}
	return ""
}

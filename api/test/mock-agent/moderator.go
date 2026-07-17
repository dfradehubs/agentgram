package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
)

// handleOpenAICompletions is a tiny OpenAI-compatible mock LLM that plays the
// group-debate moderator deterministically:
//   - Synthesis prompts get a canned consolidation.
//   - Next-speaker prompts pick the first roster agent that has not spoken yet
//     in the transcript; when everyone has spoken it returns FINISH.
//
// Wire it as an LLM model: provider "openai", role "moderator",
// endpoint http://mock-agent:9000/v1/chat/completions, any api_key.
func handleOpenAICompletions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) == 0 {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	prompt := req.Messages[len(req.Messages)-1].Content

	answer := moderatorAnswer(prompt)
	log.Printf("Mock moderator: answering %q", answer)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"choices": []map[string]interface{}{
			{"message": map[string]interface{}{"role": "assistant", "content": answer}},
		},
		"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 5},
	})
}

var rosterRe = regexp.MustCompile(`one of: ([^)\n]+)`)

func moderatorAnswer(prompt string) string {
	// Synthesis prompt (see orchestrator.synthesisPrompt)
	if strings.Contains(prompt, "final synthesis") {
		return "Mock moderator synthesis: the agents above have covered the request; see their combined answers."
	}

	// Next-speaker prompt: extract the roster ids from "(one of: a, b, c)"
	m := rosterRe.FindStringSubmatch(prompt)
	if m == nil {
		return "FINISH"
	}
	ids := strings.Split(m[1], ",")

	// Only look at the current round: the transcript since the last user
	// message. Otherwise continued sessions would never debate again.
	round := prompt
	if idx := strings.LastIndex(prompt, "User"); idx >= 0 {
		round = prompt[idx:]
	}

	// Pick the first agent that hasn't spoken yet in this round
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if !strings.Contains(round, fmt.Sprintf("Agent[%s]:", id)) {
			return id
		}
	}
	return "FINISH"
}

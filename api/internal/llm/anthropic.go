package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/dfradehubs/agentgram-api/internal/models"
)

type anthropicProvider struct {
	model  *models.LLMModel
	client *http.Client
}

func (p *anthropicProvider) GenerateContent(ctx context.Context, req *Request) (*Response, error) {
	// Build messages, stripping tool_call_id (Anthropic doesn't accept it at top level)
	cleanMessages := make([]map[string]interface{}, len(req.Messages))
	for i, m := range req.Messages {
		cleanMessages[i] = map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		}
	}

	body := map[string]interface{}{
		"model":      p.model.Model,
		"max_tokens": req.MaxTokens,
		"messages":   cleanMessages,
	}
	if req.SystemPrompt != "" {
		body["system"] = req.SystemPrompt
	}

	// Convert tools to Anthropic format
	if len(req.Tools) > 0 {
		tools := make([]map[string]interface{}, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = map[string]interface{}{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": t.InputSchema,
			}
		}
		body["tools"] = tools
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.model.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text,omitempty"`
			ID    string          `json:"id,omitempty"`
			Name  string          `json:"name,omitempty"`
			Input json.RawMessage `json:"input,omitempty"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	response := &Response{
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
	}
	for _, block := range result.Content {
		switch block.Type {
		case "text":
			response.Text += block.Text
		case "tool_use":
			var args map[string]interface{}
			if err := json.Unmarshal(block.Input, &args); err != nil {
				args = map[string]interface{}{"raw": string(block.Input)}
			}
			response.ToolCalls = append(response.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: args,
			})
		}
	}

	return response, nil
}

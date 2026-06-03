package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/google/uuid"
)

type googleProvider struct {
	model  *models.LLMModel
	client *http.Client
}

func (p *googleProvider) GenerateContent(ctx context.Context, req *Request) (*Response, error) {
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", p.model.Model)

	// Convert messages to Gemini format
	var contents []map[string]interface{}
	for _, m := range req.Messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}

		// Tool result messages use "function" role in Gemini
		if m.ToolCallID != "" {
			var responseObj map[string]interface{}
			if s, ok := m.Content.(string); ok {
				var parsed interface{}
				if err := json.Unmarshal([]byte(s), &parsed); err != nil {
					responseObj = map[string]interface{}{"result": s}
				} else if obj, ok := parsed.(map[string]interface{}); ok {
					responseObj = obj
				} else {
					responseObj = map[string]interface{}{"result": parsed}
				}
			} else if mObj, ok := m.Content.(map[string]interface{}); ok {
				responseObj = mObj
			} else {
				b, _ := json.Marshal(m.Content)
				responseObj = map[string]interface{}{"result": string(b)}
			}
			funcName := m.ToolName
			if funcName == "" {
				funcName = m.ToolCallID
			}
			contents = append(contents, map[string]interface{}{
				"role": "function",
				"parts": []map[string]interface{}{
					{
						"functionResponse": map[string]interface{}{
							"name":     funcName,
							"response": responseObj,
						},
					},
				},
			})
			continue
		}

		var parts []map[string]interface{}
		switch v := m.Content.(type) {
		case string:
			parts = []map[string]interface{}{{"text": v}}
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("marshal message content: %w", err)
			}
			parts = []map[string]interface{}{{"text": string(b)}}
		}

		contents = append(contents, map[string]interface{}{
			"role":  role,
			"parts": parts,
		})
	}

	body := map[string]interface{}{
		"contents": contents,
	}

	// System instruction
	if req.SystemPrompt != "" {
		body["systemInstruction"] = map[string]interface{}{
			"parts": []map[string]string{
				{"text": req.SystemPrompt},
			},
		}
	}

	// Convert tools to Gemini format
	if len(req.Tools) > 0 {
		funcDecls := make([]map[string]interface{}, len(req.Tools))
		for i, t := range req.Tools {
			funcDecls[i] = map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  sanitizeSchemaForGemini(t.InputSchema),
			}
		}
		body["tools"] = []map[string]interface{}{
			{"functionDeclarations": funcDecls},
		}
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal google request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create google request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", p.model.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text         string `json:"text,omitempty"`
					FunctionCall *struct {
						Name string                 `json:"name"`
						Args map[string]interface{} `json:"args"`
					} `json:"functionCall,omitempty"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode google response: %w", err)
	}

	if len(result.Candidates) == 0 {
		return nil, fmt.Errorf("empty response from google")
	}

	response := &Response{
		InputTokens:  result.UsageMetadata.PromptTokenCount,
		OutputTokens: result.UsageMetadata.CandidatesTokenCount,
	}
	for _, part := range result.Candidates[0].Content.Parts {
		if part.Text != "" {
			response.Text += part.Text
		}
		if part.FunctionCall != nil {
			response.ToolCalls = append(response.ToolCalls, ToolCall{
				ID:        uuid.NewString(),
				Name:      part.FunctionCall.Name,
				Arguments: part.FunctionCall.Args,
			})
		}
	}

	return response, nil
}

// sanitizeSchemaForGemini removes JSON Schema fields that Gemini doesn't accept.
func sanitizeSchemaForGemini(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}

	clean := make(map[string]interface{}, len(schema))
	for k, v := range schema {
		switch k {
		case "$schema", "additionalProperties", "$id", "$ref", "$comment":
			continue
		case "properties":
			if props, ok := v.(map[string]interface{}); ok {
				cleanProps := make(map[string]interface{}, len(props))
				for pk, pv := range props {
					if pm, ok := pv.(map[string]interface{}); ok {
						cleanProps[pk] = sanitizeSchemaForGemini(pm)
					} else {
						cleanProps[pk] = pv
					}
				}
				clean[k] = cleanProps
			} else {
				clean[k] = v
			}
		case "items":
			if items, ok := v.(map[string]interface{}); ok {
				clean[k] = sanitizeSchemaForGemini(items)
			} else {
				clean[k] = v
			}
		default:
			clean[k] = v
		}
	}
	return clean
}

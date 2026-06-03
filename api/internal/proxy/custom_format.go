package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"github.com/dfradehubs/agentgram-api/internal/models"
)

// TemplateData holds the variables available in request templates
type TemplateData struct {
	Query       string // All user messages concatenated
	LastMessage string // Last user message only
	SessionID   string
	Messages    string // Full messages array as JSON
	UserEmail   string
}

// BuildTemplateData constructs TemplateData from a ChatRequest
func BuildTemplateData(chatReq *models.ChatRequest) TemplateData {
	var parts []string
	var lastMessage string
	var userEmail string

	for _, msg := range chatReq.Messages {
		if msg.Role == "user" {
			parts = append(parts, msg.Content)
			lastMessage = msg.Content
			if msg.UserEmail != "" {
				userEmail = msg.UserEmail
			}
		}
	}

	messagesJSON, _ := json.Marshal(chatReq.Messages)

	return TemplateData{
		Query:       strings.Join(parts, "\n\n"),
		LastMessage: lastMessage,
		SessionID:   chatReq.SessionID,
		Messages:    string(messagesJSON),
		UserEmail:   userEmail,
	}
}

// RenderRequestTemplate executes a Go text/template with the given data
func RenderRequestTemplate(tmplStr string, data TemplateData) ([]byte, error) {
	tmpl, err := template.New("request").Parse(tmplStr)
	if err != nil {
		return nil, fmt.Errorf("invalid template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("template execution failed: %w", err)
	}
	return buf.Bytes(), nil
}

// ValidateRequestTemplate checks that the template parses and can execute with empty data
func ValidateRequestTemplate(tmplStr string) error {
	tmpl, err := template.New("request").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("invalid template syntax: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, TemplateData{}); err != nil {
		return fmt.Errorf("template execution failed: %w", err)
	}
	return nil
}

// ExtractByPath extracts a value from a parsed JSON object using dot-notation with array access.
// Examples: "response", "choices[0].message.content", "data.items[2].text"
func ExtractByPath(obj map[string]interface{}, path string) (string, bool) {
	parts := splitPath(path)
	var current interface{} = obj

	for _, part := range parts {
		key, idx, hasIdx := parseArrayAccess(part)

		switch v := current.(type) {
		case map[string]interface{}:
			val, ok := v[key]
			if !ok {
				return "", false
			}
			if hasIdx {
				arr, ok := val.([]interface{})
				if !ok || idx < 0 || idx >= len(arr) {
					return "", false
				}
				current = arr[idx]
			} else {
				current = val
			}
		default:
			return "", false
		}
	}

	switch v := current.(type) {
	case string:
		return v, true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(v), true
	default:
		// For objects/arrays, marshal to JSON
		b, err := json.Marshal(v)
		if err != nil {
			return "", false
		}
		return string(b), true
	}
}

// splitPath splits a dot-notation path into parts, e.g. "choices[0].message.content" → ["choices[0]", "message", "content"]
func splitPath(path string) []string {
	return strings.Split(path, ".")
}

// parseArrayAccess parses "key[0]" into ("key", 0, true) or "key" into ("key", 0, false)
func parseArrayAccess(part string) (string, int, bool) {
	bracketIdx := strings.Index(part, "[")
	if bracketIdx == -1 {
		return part, 0, false
	}
	key := part[:bracketIdx]
	idxStr := strings.TrimSuffix(part[bracketIdx+1:], "]")
	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		return part, 0, false
	}
	return key, idx, true
}

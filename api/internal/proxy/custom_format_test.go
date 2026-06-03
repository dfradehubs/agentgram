package proxy

import (
	"encoding/json"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTemplateData(t *testing.T) {
	chatReq := &models.ChatRequest{
		Messages: []models.ChatMessage{
			{Role: "user", Content: "Hello", UserEmail: "user@example.com"},
			{Role: "assistant", Content: "Hi!"},
			{Role: "user", Content: "How are you?"},
		},
		SessionID: "sess-123",
	}

	data := BuildTemplateData(chatReq)

	assert.Equal(t, "Hello\n\nHow are you?", data.Query)
	assert.Equal(t, "How are you?", data.LastMessage)
	assert.Equal(t, "sess-123", data.SessionID)
	assert.Equal(t, "user@example.com", data.UserEmail)
	assert.NotEmpty(t, data.Messages)
}

func TestRenderRequestTemplate(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		data     TemplateData
		wantJSON map[string]interface{}
		wantErr  bool
	}{
		{
			name: "full template",
			tmpl: `{"query": "{{.Query}}", "session_id": "{{.SessionID}}", "model": "gpt-4"}`,
			data: TemplateData{Query: "hello world", SessionID: "sess-1"},
			wantJSON: map[string]interface{}{
				"query":      "hello world",
				"session_id": "sess-1",
				"model":      "gpt-4",
			},
		},
		{
			name:     "only query",
			tmpl:     `{"prompt": "{{.LastMessage}}"}`,
			data:     TemplateData{LastMessage: "test message"},
			wantJSON: map[string]interface{}{"prompt": "test message"},
		},
		{
			name:    "invalid syntax",
			tmpl:    `{"query": "{{.Invalid`,
			data:    TemplateData{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderRequestTemplate(tt.tmpl, tt.data)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			var got map[string]interface{}
			require.NoError(t, json.Unmarshal(result, &got))
			assert.Equal(t, tt.wantJSON, got)
		})
	}
}

func TestValidateRequestTemplate(t *testing.T) {
	assert.NoError(t, ValidateRequestTemplate(`{"query": "{{.Query}}"}`))
	assert.Error(t, ValidateRequestTemplate(`{"query": "{{.Invalid`))
}

func TestExtractByPath(t *testing.T) {
	tests := []struct {
		name    string
		obj     map[string]interface{}
		path    string
		want    string
		wantOK  bool
	}{
		{
			name:   "simple field",
			obj:    map[string]interface{}{"response": "hello"},
			path:   "response",
			want:   "hello",
			wantOK: true,
		},
		{
			name: "nested field",
			obj: map[string]interface{}{
				"data": map[string]interface{}{
					"text": "nested value",
				},
			},
			path:   "data.text",
			want:   "nested value",
			wantOK: true,
		},
		{
			name: "array access - OpenAI style",
			obj: map[string]interface{}{
				"choices": []interface{}{
					map[string]interface{}{
						"message": map[string]interface{}{
							"content": "GPT says hello",
						},
					},
				},
			},
			path:   "choices[0].message.content",
			want:   "GPT says hello",
			wantOK: true,
		},
		{
			name: "Anthropic style",
			obj: map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"text": "Claude says hello",
					},
				},
			},
			path:   "content[0].text",
			want:   "Claude says hello",
			wantOK: true,
		},
		{
			name:   "missing field",
			obj:    map[string]interface{}{"other": "value"},
			path:   "response",
			wantOK: false,
		},
		{
			name: "array index out of bounds",
			obj: map[string]interface{}{
				"choices": []interface{}{},
			},
			path:   "choices[0].message.content",
			wantOK: false,
		},
		{
			name:   "deep missing path",
			obj:    map[string]interface{}{"a": "b"},
			path:   "a.b.c",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ExtractByPath(tt.obj, tt.path)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

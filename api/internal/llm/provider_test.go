package llm

import (
	"net/http"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/models"
)

func TestCustomEndpointAllowsEmptyAPIKey(t *testing.T) {
	provider, err := NewProviderWithClient(&models.LLMModel{
		Provider: "openai", Model: "local", Endpoint: "https://example.com/v1/chat/completions",
	}, &http.Client{})
	if err != nil || provider == nil {
		t.Fatalf("custom provider = (%v, %v), want success", provider, err)
	}
}

func TestNativeProviderRejectsEmptyAPIKey(t *testing.T) {
	if _, err := NewProviderWithClient(&models.LLMModel{Provider: "openai", Model: "gpt"}, &http.Client{}); err == nil {
		t.Fatal("native provider accepted an empty API key")
	}
}

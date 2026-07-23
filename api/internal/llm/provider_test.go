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

func TestEffectiveMaxTokens(t *testing.T) {
	cases := []struct {
		configured, def, want int
	}{
		{0, 4096, 4096},  // auto → default
		{-1, 4096, 4096}, // negative → default
		{512, 4096, 512}, // override wins
		{4096, 64, 4096}, // override wins even above default
	}
	for _, c := range cases {
		if got := EffectiveMaxTokens(c.configured, c.def); got != c.want {
			t.Errorf("EffectiveMaxTokens(%d,%d)=%d want %d", c.configured, c.def, got, c.want)
		}
	}
}

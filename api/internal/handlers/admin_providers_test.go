package handlers

import (
	"strings"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/models"
)

func TestValidateProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider models.LLMProvider
		wantErr  bool
	}{
		{"native key required", models.LLMProvider{ProviderType: "openai"}, true},
		{"native valid", models.LLMProvider{ProviderType: "anthropic", APIKey: "secret"}, false},
		{"custom key optional", models.LLMProvider{ProviderType: "custom", Endpoint: "https://example.com/v1/chat/completions"}, false},
		{"custom endpoint required", models.LLMProvider{ProviderType: "custom"}, true},
		{"custom host required", models.LLMProvider{ProviderType: "custom", Endpoint: "http:/missing-host"}, true},
		{"custom metadata blocked", models.LLMProvider{ProviderType: "custom", Endpoint: "http://169.254.169.254/latest"}, true},
		{"custom credentials blocked", models.LLMProvider{ProviderType: "custom", Endpoint: "https://user:pass@example.com/v1"}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := validateProvider(&test.provider)
			if (message != "") != test.wantErr {
				t.Fatalf("validateProvider() = %q, wantErr=%v", message, test.wantErr)
			}
		})
	}
}

func TestMaskAPIKey(t *testing.T) {
	if got := maskAPIKey("sk-1234567890"); got == "sk-1234567890" || !strings.Contains(got, "****") {
		t.Fatalf("maskAPIKey leaked key: %q", got)
	}
	if got := maskAPIKey(""); got != "" {
		t.Fatalf("empty key mask = %q", got)
	}
}

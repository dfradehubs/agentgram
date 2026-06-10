package handlers

import (
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestValidateMCPAuth(t *testing.T) {
	rule := func(t, s, k string) models.MCPAPIKeyRule {
		return models.MCPAPIKeyRule{SubjectType: t, Subject: s, APIKey: k}
	}
	tests := []struct {
		name    string
		req     AdminMCPRequest
		wantErr string
	}{
		{"empty is valid", AdminMCPRequest{}, ""},
		{"custom header valid", AdminMCPRequest{AuthHeaderName: "X-API-Key"}, ""},
		{"blocked header rejected", AdminMCPRequest{AuthHeaderName: "Host"}, "invalid auth_header_name"},
		{"ssrf header rejected", AdminMCPRequest{AuthHeaderName: "X-Forwarded-For"}, "invalid auth_header_name"},
		{"valid rules", AdminMCPRequest{APIKeyRules: []models.MCPAPIKeyRule{rule("user", "a@x.com", "k"), rule("group", "/g/t", "k2")}}, ""},
		{"bad subject_type", AdminMCPRequest{APIKeyRules: []models.MCPAPIKeyRule{rule("robot", "x", "k")}}, "subject_type"},
		{"empty subject", AdminMCPRequest{APIKeyRules: []models.MCPAPIKeyRule{rule("user", "", "k")}}, "require subject and api_key"},
		{"duplicate", AdminMCPRequest{APIKeyRules: []models.MCPAPIKeyRule{rule("user", "a", "k"), rule("user", "a", "k2")}}, "duplicate"},
		{"same subject diff type ok", AdminMCPRequest{APIKeyRules: []models.MCPAPIKeyRule{rule("user", "x", "k"), rule("group", "x", "k2")}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateMCPAuth(&tt.req)
			if tt.wantErr == "" {
				assert.Empty(t, got)
			} else {
				assert.Contains(t, got, tt.wantErr)
			}
		})
	}
}

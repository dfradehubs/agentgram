package mcp

import (
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestResolveBearerHeader(t *testing.T) {
	cfg := func(headerName string, fallback string, rules ...models.MCPAPIKeyRule) MCPServerConfig {
		return MCPServerConfig{
			AuthType:       models.MCPAuthBearer,
			BearerToken:    fallback,
			AuthHeaderName: headerName,
			APIKeyRules:    rules,
		}
	}
	userRule := func(subject, key string) models.MCPAPIKeyRule {
		return models.MCPAPIKeyRule{SubjectType: "user", Subject: subject, APIKey: key}
	}
	groupRule := func(subject, key string) models.MCPAPIKeyRule {
		return models.MCPAPIKeyRule{SubjectType: "group", Subject: subject, APIKey: key}
	}

	tests := []struct {
		name       string
		cfg        MCPServerConfig
		email      string
		groups     []string
		wantHeader string
		wantValue  string
	}{
		{"fallback on Authorization gets Bearer prefix", cfg("", "fallback"), "u@x.com", nil, "Authorization", "Bearer fallback"},
		{"custom header sends key verbatim", cfg("X-API-Key", "fallback"), "u@x.com", nil, "X-API-Key", "fallback"},
		{"authorization case-insensitive keeps prefix", cfg("authorization", "fallback"), "u@x.com", nil, "Authorization", "Bearer fallback"},
		{"exact user rule wins", cfg("X-API-Key", "fallback", userRule("u@x.com", "user-key"), groupRule("/g/t", "team-key")), "u@x.com", []string{"/g/t"}, "X-API-Key", "user-key"},
		{"user rule case-insensitive", cfg("X-API-Key", "fb", userRule("U@X.com", "user-key")), "u@x.com", nil, "X-API-Key", "user-key"},
		{"user beats group regardless of order", cfg("X-API-Key", "fb", groupRule("/g/t", "team-key"), userRule("u@x.com", "user-key")), "u@x.com", []string{"/g/t"}, "X-API-Key", "user-key"},
		{"first group by position wins", cfg("X-API-Key", "fb", groupRule("/g/a", "a-key"), groupRule("/g/b", "b-key")), "u@x.com", []string{"/g/b", "/g/a"}, "X-API-Key", "a-key"},
		{"no match falls back", cfg("X-API-Key", "fallback", userRule("other@x.com", "k"), groupRule("/g/o", "k")), "u@x.com", []string{"/g/t"}, "X-API-Key", "fallback"},
		{"no match no fallback sends nothing", cfg("X-API-Key", "", userRule("other@x.com", "k")), "u@x.com", nil, "", ""},
		{"empty email skips empty-subject user rule", cfg("X-API-Key", "fb", userRule("", "empty")), "", nil, "X-API-Key", "fb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, value := ResolveBearerHeader(tt.cfg, tt.email, tt.groups)
			assert.Equal(t, tt.wantHeader, name)
			assert.Equal(t, tt.wantValue, value)
		})
	}
}

func TestIsBearerPerUser(t *testing.T) {
	withRules := MCPServerConfig{AuthType: models.MCPAuthBearer, APIKeyRules: []models.MCPAPIKeyRule{{SubjectType: "user", Subject: "a", APIKey: "k"}}}
	assert.True(t, withRules.IsBearerPerUser())

	staticBearer := MCPServerConfig{AuthType: models.MCPAuthBearer}
	assert.False(t, staticBearer.IsBearerPerUser())

	oauth := MCPServerConfig{AuthType: models.MCPAuthOAuth2}
	assert.False(t, oauth.IsBearerPerUser())
}

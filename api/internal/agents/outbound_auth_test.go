package agents

import (
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestResolveOutboundAuth(t *testing.T) {
	bearerAgent := func(headerName string, rules ...models.AgentAPIKeyRule) *models.Agent {
		return &models.Agent{
			AuthType:       models.AgentAuthBearer,
			BearerToken:    "fallback-key",
			AuthHeaderName: headerName,
			APIKeyRules:    rules,
		}
	}
	userRule := func(subject, key string) models.AgentAPIKeyRule {
		return models.AgentAPIKeyRule{SubjectType: "user", Subject: subject, APIKey: key}
	}
	groupRule := func(subject, key string) models.AgentAPIKeyRule {
		return models.AgentAPIKeyRule{SubjectType: "group", Subject: subject, APIKey: key}
	}

	tests := []struct {
		name       string
		agent      *models.Agent
		email      string
		groups     []string
		forwarded  string
		wantHeader string
		wantValue  string
	}{
		{
			name:      "none sends nothing",
			agent:     &models.Agent{AuthType: models.AgentAuthNone},
			forwarded: "Bearer user-jwt",
		},
		{
			name:       "forward relays the user's header",
			agent:      &models.Agent{AuthType: models.AgentAuthForward},
			forwarded:  "Bearer user-jwt",
			wantHeader: "Authorization",
			wantValue:  "Bearer user-jwt",
		},
		{
			name:  "forward without header sends nothing",
			agent: &models.Agent{AuthType: models.AgentAuthForward},
		},
		{
			name:       "legacy ForwardAuthorization flag still forwards",
			agent:      &models.Agent{ForwardAuthorization: true},
			forwarded:  "Bearer user-jwt",
			wantHeader: "Authorization",
			wantValue:  "Bearer user-jwt",
		},
		{
			name:       "bearer fallback token on Authorization gets Bearer prefix",
			agent:      bearerAgent(""),
			email:      "user@example.com",
			wantHeader: "Authorization",
			wantValue:  "Bearer fallback-key",
		},
		{
			name:       "bearer custom header sends key verbatim",
			agent:      bearerAgent("X-API-Key"),
			email:      "user@example.com",
			wantHeader: "X-API-Key",
			wantValue:  "fallback-key",
		},
		{
			name:       "explicit Authorization header name keeps Bearer prefix (case-insensitive)",
			agent:      bearerAgent("authorization"),
			email:      "user@example.com",
			wantHeader: "Authorization",
			wantValue:  "Bearer fallback-key",
		},
		{
			name:       "exact user rule wins",
			agent:      bearerAgent("X-API-Key", userRule("user@example.com", "user-key"), groupRule("/g/team", "team-key")),
			email:      "user@example.com",
			groups:     []string{"/g/team"},
			wantHeader: "X-API-Key",
			wantValue:  "user-key",
		},
		{
			name:       "user rule matches case-insensitively",
			agent:      bearerAgent("X-API-Key", userRule("User@Example.com", "user-key")),
			email:      "user@example.com",
			wantHeader: "X-API-Key",
			wantValue:  "user-key",
		},
		{
			name:       "user rule beats group rule even when listed after",
			agent:      bearerAgent("X-API-Key", groupRule("/g/team", "team-key"), userRule("user@example.com", "user-key")),
			email:      "user@example.com",
			groups:     []string{"/g/team"},
			wantHeader: "X-API-Key",
			wantValue:  "user-key",
		},
		{
			name:       "first matching group rule by order wins",
			agent:      bearerAgent("X-API-Key", groupRule("/g/alpha", "alpha-key"), groupRule("/g/beta", "beta-key")),
			email:      "user@example.com",
			groups:     []string{"/g/beta", "/g/alpha"},
			wantHeader: "X-API-Key",
			wantValue:  "alpha-key",
		},
		{
			name:       "no rule match falls back to bearer token",
			agent:      bearerAgent("X-API-Key", userRule("other@example.com", "other-key"), groupRule("/g/other", "other-key")),
			email:      "user@example.com",
			groups:     []string{"/g/team"},
			wantHeader: "X-API-Key",
			wantValue:  "fallback-key",
		},
		{
			name: "no match and no fallback sends nothing",
			agent: &models.Agent{
				AuthType:    models.AgentAuthBearer,
				APIKeyRules: []models.AgentAPIKeyRule{userRule("other@example.com", "k")},
			},
			email: "user@example.com",
		},
		{
			name:       "empty email does not match a user rule with empty subject",
			agent:      bearerAgent("X-API-Key", userRule("", "empty-key")),
			email:      "",
			wantHeader: "X-API-Key",
			wantValue:  "fallback-key",
		},
		{
			name:      "oauth2 (reserved) sends nothing",
			agent:     &models.Agent{AuthType: models.AgentAuthOAuth2, BearerToken: "k"},
			email:     "user@example.com",
			forwarded: "Bearer user-jwt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveOutboundAuth(tt.agent, tt.email, tt.groups, tt.forwarded)
			assert.Equal(t, tt.wantHeader, got.HeaderName)
			assert.Equal(t, tt.wantValue, got.HeaderValue)
		})
	}
}

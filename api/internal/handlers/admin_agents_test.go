package handlers

import (
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestValidateAgentAuth(t *testing.T) {
	rule := func(subjectType, subject, key string) models.AgentAPIKeyRule {
		return models.AgentAPIKeyRule{SubjectType: subjectType, Subject: subject, APIKey: key}
	}

	tests := []struct {
		name    string
		req     AdminAgentRequest
		wantErr string // substring; "" means valid
	}{
		{"empty auth_type is valid (legacy)", AdminAgentRequest{}, ""},
		{"none is valid", AdminAgentRequest{AuthType: models.AgentAuthNone}, ""},
		{"forward is valid", AdminAgentRequest{AuthType: models.AgentAuthForward}, ""},
		{"bearer is valid", AdminAgentRequest{AuthType: models.AgentAuthBearer}, ""},
		{"oauth2 is rejected as not yet supported", AdminAgentRequest{AuthType: models.AgentAuthOAuth2}, "not yet supported"},
		{"unknown auth_type is rejected", AdminAgentRequest{AuthType: "magic"}, "invalid auth_type"},
		{
			"custom auth header is valid",
			AdminAgentRequest{AuthType: models.AgentAuthBearer, AuthHeaderName: "X-API-Key"},
			"",
		},
		{
			"blocked auth header is rejected",
			AdminAgentRequest{AuthType: models.AgentAuthBearer, AuthHeaderName: "Host"},
			"invalid auth_header_name",
		},
		{
			"valid rules pass",
			AdminAgentRequest{
				AuthType: models.AgentAuthBearer,
				APIKeyRules: []models.AgentAPIKeyRule{
					rule("user", "a@example.com", "k1"),
					rule("group", "/g/team", "k2"),
				},
			},
			"",
		},
		{
			"invalid subject_type is rejected",
			AdminAgentRequest{APIKeyRules: []models.AgentAPIKeyRule{rule("robot", "x", "k")}},
			"subject_type",
		},
		{
			"empty subject is rejected",
			AdminAgentRequest{APIKeyRules: []models.AgentAPIKeyRule{rule("user", "", "k")}},
			"require subject and api_key",
		},
		{
			"empty api_key is rejected",
			AdminAgentRequest{APIKeyRules: []models.AgentAPIKeyRule{rule("user", "a@example.com", "")}},
			"require subject and api_key",
		},
		{
			"duplicate subject is rejected",
			AdminAgentRequest{APIKeyRules: []models.AgentAPIKeyRule{
				rule("user", "a@example.com", "k1"),
				rule("user", "a@example.com", "k2"),
			}},
			"duplicate",
		},
		{
			"same subject with different type is allowed",
			AdminAgentRequest{APIKeyRules: []models.AgentAPIKeyRule{
				rule("user", "x", "k1"),
				rule("group", "x", "k2"),
			}},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateAgentAuth(&tt.req)
			if tt.wantErr == "" {
				assert.Empty(t, got)
			} else {
				assert.Contains(t, got, tt.wantErr)
			}
		})
	}
}

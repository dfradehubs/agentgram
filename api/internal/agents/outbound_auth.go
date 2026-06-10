package agents

import (
	"strings"

	"github.com/dfradehubs/agentgram-api/internal/models"
)

// OutboundAuth is the resolved outbound credential for an agent call.
// An empty HeaderValue means "send no credential".
type OutboundAuth struct {
	HeaderName  string // "Authorization", "X-API-Key", ...
	HeaderValue string // includes the "Bearer " prefix when HeaderName is Authorization
}

// ResolveOutboundAuth resolves the credential agentgram sends to an agent,
// based on the agent's auth method and the calling user's identity:
//
//   - forward: the user's own Authorization header, untouched.
//   - bearer:  an API key picked by precedence — exact user rule, then first
//     group rule (by position) matching the user's groups, then the agent's
//     fallback BearerToken. Sent as "Bearer <key>" on the Authorization
//     header, or verbatim on a custom header (e.g. X-API-Key).
//   - none / oauth2 (phase 2) / no match: no credential.
func ResolveOutboundAuth(agent *models.Agent, userEmail string, userGroups []string, forwardedAuthHeader string) OutboundAuth {
	switch agent.GetAuthType() {
	case models.AgentAuthForward:
		if forwardedAuthHeader != "" {
			return OutboundAuth{HeaderName: "Authorization", HeaderValue: forwardedAuthHeader}
		}

	case models.AgentAuthBearer:
		key := resolveAPIKey(agent, userEmail, userGroups)
		if key == "" {
			return OutboundAuth{}
		}
		name := agent.AuthHeaderName
		if name == "" || strings.EqualFold(name, "Authorization") {
			return OutboundAuth{HeaderName: "Authorization", HeaderValue: "Bearer " + key}
		}
		return OutboundAuth{HeaderName: name, HeaderValue: key}
	}

	return OutboundAuth{}
}

// resolveAPIKey picks the API key for a user: exact user rule first, then the
// first group rule (rules are stored ordered by position) whose subject is in
// the user's groups, then the agent-level fallback token.
func resolveAPIKey(agent *models.Agent, userEmail string, userGroups []string) string {
	for _, rule := range agent.APIKeyRules {
		if rule.SubjectType == "user" && strings.EqualFold(rule.Subject, userEmail) && userEmail != "" {
			return rule.APIKey
		}
	}
	for _, rule := range agent.APIKeyRules {
		if rule.SubjectType != "group" {
			continue
		}
		for _, g := range userGroups {
			if strings.EqualFold(rule.Subject, g) {
				return rule.APIKey
			}
		}
	}
	return agent.BearerToken
}

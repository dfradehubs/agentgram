package mcp

import "strings"

// bearerHeader builds the (name, value) header pair for a resolved API key.
// The "Authorization" header (default) carries a "Bearer " prefix; any other
// header (e.g. "X-API-Key") carries the key verbatim. An empty key yields an
// empty pair (send nothing).
func bearerHeader(authHeaderName, key string) (string, string) {
	if key == "" {
		return "", ""
	}
	if authHeaderName == "" || strings.EqualFold(authHeaderName, "Authorization") {
		return "Authorization", "Bearer " + key
	}
	return authHeaderName, key
}

// resolveAPIKey picks the API key for a user in bearer mode: exact user rule
// first, then the first group rule (rules are ordered by position) whose
// subject is in the user's groups, then the server-level BearerToken fallback.
func resolveAPIKey(cfg MCPServerConfig, userEmail string, userGroups []string) string {
	for _, rule := range cfg.APIKeyRules {
		if rule.SubjectType == "user" && userEmail != "" && strings.EqualFold(rule.Subject, userEmail) {
			return rule.APIKey
		}
	}
	for _, rule := range cfg.APIKeyRules {
		if rule.SubjectType != "group" {
			continue
		}
		for _, g := range userGroups {
			if strings.EqualFold(rule.Subject, g) {
				return rule.APIKey
			}
		}
	}
	return cfg.BearerToken
}

// ResolveBearerHeader resolves the per-user bearer header (name, value) for an
// MCP server, applying the api_key_rules precedence and the configured header
// name. Returns empty strings when no key resolves (send nothing).
func ResolveBearerHeader(cfg MCPServerConfig, userEmail string, userGroups []string) (name, value string) {
	return bearerHeader(cfg.AuthHeaderName, resolveAPIKey(cfg, userEmail, userGroups))
}

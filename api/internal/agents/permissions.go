package agents

import (
	"strings"

	"github.com/dfradehubs/agentgram-api/internal/models"
)

// HasAccess checks if a user has access to an agent
func HasAccess(agent *models.Agent, userEmail string, userGroups []string) bool {
	// Check for wildcard "*" in allowed_users (allows everyone)
	for _, allowed := range agent.AllowedUsers {
		if allowed == "*" {
			return true
		}
		if strings.EqualFold(allowed, userEmail) {
			return true
		}
	}

	// Check allowed_groups (Google Workspace format: google-workspace/sre@example.com)
	groupSet := make(map[string]bool)
	for _, g := range userGroups {
		groupSet[strings.ToLower(g)] = true
	}

	for _, allowed := range agent.AllowedGroups {
		if groupSet[strings.ToLower(allowed)] {
			return true
		}
	}

	// If no restrictions (empty lists), deny access by default
	// This is more secure than allowing access to everyone
	return false
}

// HasAccessWithInherited checks agent access including inherited group permissions
func HasAccessWithInherited(agent *models.Agent, userEmail string, userGroups []string, inherited *models.InheritedPerms) bool {
	// Check direct agent permissions first
	if HasAccess(agent, userEmail, userGroups) {
		return true
	}

	if inherited == nil {
		return false
	}

	// Check inherited user permissions
	for _, u := range inherited.Users {
		if u.Value == "*" || strings.EqualFold(u.Value, userEmail) {
			return true
		}
	}

	// Check inherited group permissions
	groupSet := make(map[string]bool)
	for _, g := range userGroups {
		groupSet[strings.ToLower(g)] = true
	}
	for _, g := range inherited.Groups {
		if groupSet[strings.ToLower(g.Value)] {
			return true
		}
	}

	return false
}

// FilterAgentsByAccess filters a list of agents based on user permissions
func FilterAgentsByAccess(agents []*models.Agent, userEmail string, userGroups []string) []*models.Agent {
	var allowed []*models.Agent

	for _, agent := range agents {
		if HasAccess(agent, userEmail, userGroups) {
			allowed = append(allowed, agent)
		}
	}

	return allowed
}

// FilterAgentsByAccessWithInherited filters agents using both direct and inherited permissions
func FilterAgentsByAccessWithInherited(agents []*models.Agent, userEmail string, userGroups []string, inheritedMap map[string]*models.InheritedPerms) []*models.Agent {
	var allowed []*models.Agent

	for _, agent := range agents {
		inherited := inheritedMap[agent.ID]
		if HasAccessWithInherited(agent, userEmail, userGroups, inherited) {
			allowed = append(allowed, agent)
		}
	}

	return allowed
}

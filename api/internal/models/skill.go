package models

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// MaxSkillIDLength bounds skill IDs so they stay safe as MCP tool names.
const MaxSkillIDLength = 56

var validSkillID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,55}$`)

// ValidateSkillID keeps persisted IDs safe for URLs and MCP tool names.
func ValidateSkillID(id string) error {
	if !validSkillID.MatchString(id) {
		return fmt.Errorf("skill id must start with a letter or digit and contain at most %d ASCII letters, digits, underscores, or hyphens", MaxSkillIDLength)
	}
	return nil
}

// Skill is a reusable instruction document served to MCP clients as a tool.
// Its Content is returned verbatim when the tool is called.
type Skill struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Content       string    `json:"content"`
	AllowedUsers  []string  `json:"allowed_users,omitempty"`
	AllowedGroups []string  `json:"allowed_groups,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// HasAccess mirrors agents.HasAccess: wildcard/user match or group match wins,
// deny by default when no rule matches.
func (s *Skill) HasAccess(userEmail string, userGroups []string) bool {
	for _, allowed := range s.AllowedUsers {
		if allowed == "*" || strings.EqualFold(allowed, userEmail) {
			return true
		}
	}

	groupSet := make(map[string]bool, len(userGroups))
	for _, g := range userGroups {
		groupSet[strings.ToLower(g)] = true
	}
	for _, allowed := range s.AllowedGroups {
		if groupSet[strings.ToLower(allowed)] {
			return true
		}
	}

	return false
}

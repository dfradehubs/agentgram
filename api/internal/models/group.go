package models

import "time"

// AgentGroup represents a multi-agent group with permissions
type AgentGroup struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	AgentIDs      []string  `json:"agent_ids"`
	CreatedBy     string    `json:"created_by"`
	AllowedUsers  []string  `json:"allowed_users,omitempty"`
	AllowedGroups []string  `json:"allowed_groups,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// InheritedPermission represents a permission inherited from an agent group
type InheritedPermission struct {
	Value     string `json:"value"`
	FromGroup string `json:"from_group"`
}

// InheritedPerms holds inherited permissions for an agent from its groups
type InheritedPerms struct {
	Users  []InheritedPermission `json:"users"`
	Groups []InheritedPermission `json:"groups"`
}

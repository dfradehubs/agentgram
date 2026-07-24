package models

import "time"

// User represents a user with RBAC role
type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	Role         string     `json:"role"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastAccessAt *time.Time `json:"last_access_at"`
}

// Roles, ordered from least to most privileged. admin > editor > viewer > user.
// editor can create/edit agents, MCP servers and skills (not delete, not manage
// permissions or admin-only sections); viewer has no admin-panel access today.
const (
	RoleUser   = "user"
	RoleViewer = "viewer"
	RoleEditor = "editor"
	RoleAdmin  = "admin"
)

// roleRank maps a role to its privilege level for hierarchical comparisons.
// Unknown roles rank as user (lowest).
func roleRank(role string) int {
	switch role {
	case RoleAdmin:
		return 3
	case RoleEditor:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

// HasRole reports whether role satisfies (meets or exceeds) the minimum role.
func HasRole(role, min string) bool {
	return roleRank(role) >= roleRank(min)
}

// NormalizeRole returns the role if known, otherwise RoleUser.
func NormalizeRole(role string) string {
	switch role {
	case RoleAdmin, RoleEditor, RoleViewer:
		return role
	default:
		return RoleUser
	}
}

// IsValidRole reports whether role is one of the known roles.
func IsValidRole(role string) bool {
	switch role {
	case RoleAdmin, RoleEditor, RoleViewer, RoleUser:
		return true
	default:
		return false
	}
}

// IsAdmin returns true if the user has admin role
func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

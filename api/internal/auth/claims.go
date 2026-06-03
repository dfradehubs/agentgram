package auth

// Claims represents the claims extracted from the JWT
type Claims struct {
	Sub               string   `json:"sub"`
	Email             string   `json:"email"`
	Name              string   `json:"name"`
	PreferredUsername string   `json:"preferred_username"`
	Groups            []string `json:"groups"`
	Issuer            string   `json:"iss"`
}

// GetEmail returns the user's email
func (c *Claims) GetEmail() string {
	if c.Email != "" {
		return c.Email
	}
	return c.PreferredUsername
}

// GetDisplayName returns a human-readable display name for the user.
// Priority: name (full name from OIDC) > preferred_username > email prefix.
func (c *Claims) GetDisplayName() string {
	if c.Name != "" {
		return c.Name
	}
	if c.PreferredUsername != "" {
		return c.PreferredUsername
	}
	// Fallback: use part before @ in email
	email := c.GetEmail()
	if idx := len(email); idx > 0 {
		for i, ch := range email {
			if ch == '@' {
				return email[:i]
			}
		}
	}
	return email
}

// GetGroups returns the user's groups
func (c *Claims) GetGroups() []string {
	if c.Groups == nil {
		return []string{}
	}
	return c.Groups
}

// HasGroup checks if the user belongs to a specific group
func (c *Claims) HasGroup(group string) bool {
	for _, g := range c.Groups {
		if g == group {
			return true
		}
	}
	return false
}

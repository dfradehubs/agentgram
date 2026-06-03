package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClaims_GetEmail(t *testing.T) {
	// With email
	claims := &Claims{Email: "user@example.com", PreferredUsername: "user"}
	assert.Equal(t, "user@example.com", claims.GetEmail())

	// Without email, uses preferred_username
	claims = &Claims{Email: "", PreferredUsername: "user@example.com"}
	assert.Equal(t, "user@example.com", claims.GetEmail())
}

func TestClaims_GetGroups(t *testing.T) {
	// With groups
	claims := &Claims{Groups: []string{"group1", "group2"}}
	assert.Equal(t, []string{"group1", "group2"}, claims.GetGroups())

	// Without groups
	claims = &Claims{Groups: nil}
	assert.Equal(t, []string{}, claims.GetGroups())
}

func TestClaims_HasGroup(t *testing.T) {
	claims := &Claims{
		Groups: []string{
			"google-workspace/sre@company.com",
			"google-workspace/admins@company.com",
		},
	}

	assert.True(t, claims.HasGroup("google-workspace/sre@company.com"))
	assert.True(t, claims.HasGroup("google-workspace/admins@company.com"))
	assert.False(t, claims.HasGroup("google-workspace/other@company.com"))
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		expected  string
		expectErr bool
	}{
		{
			name:      "valid bearer token",
			header:    "Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
			expected:  "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
			expectErr: false,
		},
		{
			name:      "lowercase bearer",
			header:    "bearer token123",
			expected:  "token123",
			expectErr: false,
		},
		{
			name:      "missing header",
			header:    "",
			expected:  "",
			expectErr: true,
		},
		{
			name:      "invalid format",
			header:    "Basic token123",
			expected:  "",
			expectErr: true,
		},
		{
			name:      "only bearer",
			header:    "Bearer",
			expected:  "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := ExtractBearerToken(tt.header)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, token)
			}
		})
	}
}

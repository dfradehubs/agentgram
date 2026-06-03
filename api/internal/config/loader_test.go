package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveEnvVars(t *testing.T) {
	// Set test environment variables
	os.Setenv("TEST_VAR", "test-value")
	os.Setenv("ANOTHER_VAR", "another-value")
	defer func() {
		os.Unsetenv("TEST_VAR")
		os.Unsetenv("ANOTHER_VAR")
	}()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple variable",
			input:    "${ENV:TEST_VAR}",
			expected: "test-value",
		},
		{
			name:     "variable with prefix and suffix",
			input:    "prefix-${ENV:TEST_VAR}-suffix",
			expected: "prefix-test-value-suffix",
		},
		{
			name:     "multiple variables",
			input:    "${ENV:TEST_VAR}-${ENV:ANOTHER_VAR}",
			expected: "test-value-another-value",
		},
		{
			name:     "undefined variable",
			input:    "${ENV:UNDEFINED_VAR}",
			expected: "",
		},
		{
			name:     "no variables",
			input:    "plain text",
			expected: "plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveEnvVars(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveEnvVar(t *testing.T) {
	os.Setenv("MY_VAR", "my-value")
	defer os.Unsetenv("MY_VAR")

	result := ResolveEnvVar("Value: ${ENV:MY_VAR}")
	assert.Equal(t, "Value: my-value", result)
}

package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// logger is a package-level logger for config loading warnings.
var logger = zap.L()

// envVarRegex matches ${ENV:VARIABLE_NAME}
var envVarRegex = regexp.MustCompile(`\$\{ENV:([^}]+)\}`)

// LoadYAML loads a YAML file and resolves environment variables
func LoadYAML(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("error reading file %s: %w", path, err)
	}

	// Resolve environment variables in the content
	resolved := resolveEnvVars(string(data))

	if err := yaml.Unmarshal([]byte(resolved), target); err != nil {
		return fmt.Errorf("error parsing YAML %s: %w", path, err)
	}

	return nil
}

// resolveEnvVars replaces ${ENV:VARIABLE} with the environment variable value
func resolveEnvVars(content string) string {
	return envVarRegex.ReplaceAllStringFunc(content, func(match string) string {
		// Extract variable name
		varName := envVarRegex.FindStringSubmatch(match)[1]
		varName = strings.TrimSpace(varName)

		// Get environment variable value
		if value := os.Getenv(varName); value != "" {
			return value
		}

		logger.Warn("environment variable not set, using empty string", zap.String("var", varName))
		return ""
	})
}

// ResolveEnvVar resolves a single environment variable in a string
func ResolveEnvVar(value string) string {
	return resolveEnvVars(value)
}

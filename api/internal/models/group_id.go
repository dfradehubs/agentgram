package models

import (
	"fmt"
	"regexp"
)

const MaxGroupIDLength = 56

var validGroupID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,55}$`)

// ValidateGroupID keeps persisted IDs safe for URLs and MCP tool names.
func ValidateGroupID(id string) error {
	if !validGroupID.MatchString(id) {
		return fmt.Errorf("group id must start with a letter or digit and contain at most %d ASCII letters, digits, underscores, or hyphens", MaxGroupIDLength)
	}
	return nil
}

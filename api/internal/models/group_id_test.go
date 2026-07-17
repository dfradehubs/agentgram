package models

import "testing"

func TestValidateGroupIDForMCPToolName(t *testing.T) {
	valid := []string{"ops", "group-123", "a_b", "A1"}
	invalid := []string{"", "has space", "has/slash", "grüp", "-leading", "_leading", "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcde"}
	for _, id := range valid {
		if err := ValidateGroupID(id); err != nil {
			t.Errorf("ValidateGroupID(%q): %v", id, err)
		}
	}
	for _, id := range invalid {
		if err := ValidateGroupID(id); err == nil {
			t.Errorf("ValidateGroupID(%q) unexpectedly succeeded", id)
		}
	}
}

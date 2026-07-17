package handlers

import "testing"

func TestValidateLLMRole(t *testing.T) {
	valid := []string{"chat", "summarizer", "file_processor", "chart_extractor", "session_namer", "moderator"}
	for _, role := range valid {
		if got := validateLLMRole(role); got != "" {
			t.Errorf("validateLLMRole(%q) = %q, want valid", role, got)
		}
	}
	if got := validateLLMRole("modrator"); got == "" {
		t.Error("validateLLMRole accepted an unknown role")
	}
}

func TestBoolValue(t *testing.T) {
	if got := boolValue(nil, true); !got {
		t.Error("omitted create value should use the enabled-by-default fallback")
	}
	if got := boolValue(nil, false); got {
		t.Error("omitted update value should preserve a disabled fallback")
	}
	falseValue := false
	if got := boolValue(&falseValue, true); got {
		t.Error("an explicit false value must not be replaced by the fallback")
	}
}

package proxy

import (
	"testing"
	"time"
)

func TestResolveAgentTimeout(t *testing.T) {
	if got := resolveAgentTimeout(0); got != defaultAgentTimeout {
		t.Errorf("0 → %v, want default %v", got, defaultAgentTimeout)
	}
	if got := resolveAgentTimeout(-1); got != defaultAgentTimeout {
		t.Errorf("negative → %v, want default %v", got, defaultAgentTimeout)
	}
	if got := resolveAgentTimeout(90 * time.Second); got != 90*time.Second {
		t.Errorf("explicit value not honored: %v", got)
	}
}

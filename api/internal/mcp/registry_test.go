package mcp

import (
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/models"
)

func TestEffectiveHeaders(t *testing.T) {
	tests := []struct {
		name     string
		cfg      MCPServerConfig
		wantAuth string
		wantKeep map[string]string
	}{
		{
			name: "bearer auth injects Authorization header",
			cfg: MCPServerConfig{
				AuthType:    models.MCPAuthBearer,
				BearerToken: "abc123",
			},
			wantAuth: "Bearer abc123",
		},
		{
			name: "bearer preserves extra configured headers",
			cfg: MCPServerConfig{
				AuthType:    models.MCPAuthBearer,
				BearerToken: "abc123",
				Headers:     map[string]string{"X-Custom": "value"},
			},
			wantAuth: "Bearer abc123",
			wantKeep: map[string]string{"X-Custom": "value"},
		},
		{
			name: "explicit Authorization in Headers wins over BearerToken",
			cfg: MCPServerConfig{
				AuthType:    models.MCPAuthBearer,
				BearerToken: "abc123",
				Headers:     map[string]string{"Authorization": "Basic xyz"},
			},
			wantAuth: "Basic xyz",
		},
		{
			name: "bearer with empty token does not inject Authorization",
			cfg: MCPServerConfig{
				AuthType:    models.MCPAuthBearer,
				BearerToken: "",
				Headers:     map[string]string{"X-Custom": "value"},
			},
			wantAuth: "",
			wantKeep: map[string]string{"X-Custom": "value"},
		},
		{
			name: "non-bearer auth does not inject Authorization",
			cfg: MCPServerConfig{
				AuthType:    models.MCPAuthOAuth2,
				BearerToken: "abc123",
				Headers:     map[string]string{"X-Custom": "value"},
			},
			wantAuth: "",
			wantKeep: map[string]string{"X-Custom": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveHeaders(tt.cfg)

			if tt.wantAuth != "" {
				if got["Authorization"] != tt.wantAuth {
					t.Errorf("Authorization = %q, want %q", got["Authorization"], tt.wantAuth)
				}
			} else if _, present := got["Authorization"]; present {
				t.Errorf("Authorization should not be set, got %q", got["Authorization"])
			}

			for k, v := range tt.wantKeep {
				if got[k] != v {
					t.Errorf("header %q = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestMCPServerGetAuthType_Bearer(t *testing.T) {
	s := &models.MCPServer{AuthType: models.MCPAuthBearer}
	if got := s.GetAuthType(); got != models.MCPAuthBearer {
		t.Errorf("GetAuthType() = %q, want %q", got, models.MCPAuthBearer)
	}
}

func TestConfigIsBearer(t *testing.T) {
	cfg := MCPServerConfig{AuthType: models.MCPAuthBearer}
	if !cfg.IsBearer() {
		t.Error("IsBearer() = false, want true")
	}
	cfg.AuthType = models.MCPAuthNone
	if cfg.IsBearer() {
		t.Error("IsBearer() with none auth = true, want false")
	}
}

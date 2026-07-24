package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/service"
)

func TestGetSkillIDFromToolName(t *testing.T) {
	tests := []struct {
		name   string
		tool   string
		wantID string
		wantOK bool
	}{
		{"skill tool", "skill__debug-prod", "debug-prod", true},
		{"plain agent tool", "ask_logs-agent", "", false},
		{"group tool", "group__g1", "", false},
		{"bare prefix", "skill__", "", false},
		{"invalid id chars", "skill__bad id", "bad id", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := GetSkillIDFromToolName(tt.tool)
			if id != tt.wantID || ok != tt.wantOK {
				t.Errorf("got (%q, %v), want (%q, %v)", id, ok, tt.wantID, tt.wantOK)
			}
		})
	}
}

// fakeSkillRepo is a minimal in-memory SkillRepository for MCP surface tests.
type fakeSkillRepo struct {
	skills []*models.Skill
}

func (f *fakeSkillRepo) List(_ context.Context) ([]*models.Skill, error) { return f.skills, nil }
func (f *fakeSkillRepo) Get(_ context.Context, id string) (*models.Skill, error) {
	for _, s := range f.skills {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, context.Canceled // any error; handler maps it to "skill not found"
}
func (f *fakeSkillRepo) Create(context.Context, *models.Skill) error                 { return nil }
func (f *fakeSkillRepo) Update(context.Context, *models.Skill) error                 { return nil }
func (f *fakeSkillRepo) Delete(context.Context, string) error                        { return nil }
func (f *fakeSkillRepo) UpdatePermissions(context.Context, string, []string, []string) error {
	return nil
}

func newSkillTestServer(skills ...*models.Skill) *Server {
	userService := service.NewUserService(&mcpFakeUserRepo{}, nil, nil, nil, nil)
	return NewServer(agents.NewRegistry(), nil, userService, nil, &fakeSkillRepo{skills: skills}, zap.NewNop())
}

func TestToolsListFiltersSkillsByAccess(t *testing.T) {
	s := newSkillTestServer(
		&models.Skill{ID: "visible", Name: "Visible", AllowedGroups: []string{"sre"}},
		&models.Skill{ID: "hidden", Name: "Hidden", AllowedUsers: []string{"other@example.com"}},
	)

	resp, _, err := s.HandleMessage(
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`),
		"user@example.com", []string{"sre"}, "")
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	body := string(resp)
	if !strings.Contains(body, `"skill__visible"`) {
		t.Errorf("tools/list missing accessible skill:\n%s", body)
	}
	if strings.Contains(body, `"skill__hidden"`) {
		t.Errorf("tools/list leaked skill the user can't access:\n%s", body)
	}
}

func TestHandleSkillToolCall(t *testing.T) {
	skill := &models.Skill{ID: "debug-prod", Name: "Debug prod", Content: "1. Check logs\n2. Check metrics", AllowedGroups: []string{"sre"}}
	h := &Handler{server: newSkillTestServer(skill)}
	req := jsonRPCRequest{ID: json.RawMessage(`1`)}
	httpReq := httptest.NewRequest(http.MethodPost, "/mcp", nil)

	// With access → content is returned, not an error.
	rec := httptest.NewRecorder()
	h.handleSkillToolCall(rec, httpReq, req, "debug-prod", "user@example.com", []string{"sre"})
	if got := rec.Body.String(); !strings.Contains(got, "Check logs") || strings.Contains(got, `"isError":true`) {
		t.Errorf("expected skill content without error, got:\n%s", got)
	}

	// Without access → denied.
	rec = httptest.NewRecorder()
	h.handleSkillToolCall(rec, httpReq, req, "debug-prod", "nobody@example.com", nil)
	if got := rec.Body.String(); !strings.Contains(got, "Access denied") || !strings.Contains(got, `"isError":true`) {
		t.Errorf("expected access denied error, got:\n%s", got)
	}
}

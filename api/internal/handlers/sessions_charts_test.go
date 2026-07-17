package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/service"
)

// Use case (IMPORTANT): PatchCharts is owner-only — a group member cannot add
// charts to another member's personal session even with the UUID.
func TestPatchChartsCrossUserDenied(t *testing.T) {
	fs := newFakeSessionStore()
	fg := &fakeGroupRepo{groups: map[string]*models.AgentGroup{
		"g1": {ID: "g1", Name: "G", AgentIDs: []string{"agent-a", "agent-b"}, AllowedUsers: []string{"*"}},
	}}
	h := NewSessionsHandler(agents.NewRegistry(), fs, fg, service.NewUserService(&fakeUserRepo{}, nil, nil), nil, zap.NewNop())

	// Session owned by someone else, in a group the caller can access.
	s, _ := fs.CreateSession(context.Background(), "someone-else@example.com", "agent-a", "seed")
	s.GroupID = "g1"
	_ = fs.SaveSession(context.Background(), s)

	router := chi.NewRouter()
	router.Post("/api/agents/{agentId}/sessions/{sessionId}/charts", h.PatchCharts)

	req := httptest.NewRequest("POST", "/api/agents/agent-a/sessions/"+s.SessionID+"/charts",
		strings.NewReader(`{"charts":[{"chartType":"bar"}],"assistant_offset":0}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, &auth.Claims{Email: testUserEmail}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (cannot modify another user's session)", rec.Code)
	}
}

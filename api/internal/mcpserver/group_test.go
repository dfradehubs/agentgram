package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/llm"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/orchestrator"
	"github.com/dfradehubs/agentgram-api/internal/proxy"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/service"
	appsettings "github.com/dfradehubs/agentgram-api/internal/settings"
	"github.com/dfradehubs/agentgram-api/internal/store"
)

func TestGetGroupIDFromToolName(t *testing.T) {
	tests := []struct {
		name   string
		tool   string
		wantID string
		wantOK bool
	}{
		{"group tool", "ask_group_group-123-abc", "group-123-abc", true},
		{"plain agent tool", "ask_logs-agent", "", false},
		{"agent id starting with group_", "ask_group_x", "x", true}, // routing guard handled by groupExists
		{"bare prefix", "ask_group_", "", false},
		{"unrelated", "list_agents", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := GetGroupIDFromToolName(tt.tool)
			if id != tt.wantID || ok != tt.wantOK {
				t.Errorf("got (%q, %v), want (%q, %v)", id, ok, tt.wantID, tt.wantOK)
			}
		})
	}
}

// --- fakes (slim duplicates of the handlers test fakes; test packages can't share) ---

type mcpScriptedProvider struct {
	mu        sync.Mutex
	responses []string
	calls     int
}

func (f *mcpScriptedProvider) GenerateContent(_ context.Context, _ *llm.Request) (*llm.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls >= len(f.responses) {
		return &llm.Response{Text: "FINISH"}, nil
	}
	r := f.responses[f.calls]
	f.calls++
	return &llm.Response{Text: r}, nil
}

type mcpFakeSessionStore struct {
	store.SessionStore
	mu            sync.Mutex
	sessions      map[string]*models.Session
	agentSessions map[string]string
	failSave      bool // when true, SaveSession returns an error
}

func newMCPFakeSessionStore() *mcpFakeSessionStore {
	return &mcpFakeSessionStore{sessions: map[string]*models.Session{}, agentSessions: map[string]string{}}
}

func (f *mcpFakeSessionStore) CreateSession(_ context.Context, userID, agentID, sessionName string) (*models.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := &models.Session{SessionID: uuid.New().String(), SessionName: sessionName, UserID: userID, AgentIDs: []string{agentID}}
	f.sessions[s.SessionID] = s
	return s, nil
}

func (f *mcpFakeSessionStore) GetSession(_ context.Context, sessionID string) (*models.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[sessionID]
	if !ok {
		return nil, nil // absence, not error — mirrors the Redis store contract
	}
	cp := *s
	cp.Messages = append([]models.ChatMessage(nil), s.Messages...)
	return &cp, nil
}

func (f *mcpFakeSessionStore) SaveSession(_ context.Context, session *models.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failSave {
		return fmt.Errorf("simulated store failure")
	}
	f.sessions[session.SessionID] = session
	return nil
}

func (f *mcpFakeSessionStore) AddMessage(_ context.Context, sessionID string, msg models.ChatMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[sessionID]
	if !ok {
		return fmt.Errorf("not found")
	}
	s.Messages = append(s.Messages, msg)
	return nil
}

func (f *mcpFakeSessionStore) GetAgentSessionID(_ context.Context, sessionID, agentID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.agentSessions[sessionID+"|"+agentID], nil
}

func (f *mcpFakeSessionStore) SetAgentSessionID(_ context.Context, sessionID, agentID, agentSessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agentSessions[sessionID+"|"+agentID] = agentSessionID
	return nil
}

type mcpFakeGroupRepo struct {
	repository.GroupRepository
	groups map[string]*models.AgentGroup
}

func (f *mcpFakeGroupRepo) Get(_ context.Context, id string) (*models.AgentGroup, error) {
	g, ok := f.groups[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return g, nil
}

func (f *mcpFakeGroupRepo) ListAccessible(_ context.Context, _ string, _ []string) ([]*models.AgentGroup, error) {
	var out []*models.AgentGroup
	for _, g := range f.groups {
		out = append(out, g)
	}
	return out, nil
}

func (f *mcpFakeGroupRepo) GetAllInheritedPermissions(_ context.Context) (map[string]*models.InheritedPerms, error) {
	return map[string]*models.InheritedPerms{}, nil
}

func (f *mcpFakeGroupRepo) AddSession(_ context.Context, _, _ string) error { return nil }

type mcpFakeUserRepo struct{ repository.UserRepository }

func (f *mcpFakeUserRepo) GetByEmail(_ context.Context, _ string) (*models.User, error) {
	return nil, fmt.Errorf("not found")
}

// newGroupTestHandler wires a Handler with two mock "custom" agents and one group.
func newGroupTestHandler(t *testing.T, agentAStatus int, moderatorSays ...string) (*Handler, *models.AgentGroup) {
	t.Helper()

	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if agentAStatus != http.StatusOK {
			http.Error(w, "boom", agentAStatus)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "reply from agent-a")
	}))
	t.Cleanup(srvA.Close)
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "reply from agent-b")
	}))
	t.Cleanup(srvB.Close)

	registry := agents.NewRegistry()
	if err := registry.LoadAgents([]models.Agent{
		{ID: "agent-a", Name: "Agent A", Description: "Handles logs", Protocol: "custom", Endpoint: srvA.URL, AllowedUsers: []string{"*"}},
		{ID: "agent-b", Name: "Agent B", Description: "Handles kubernetes", Protocol: "custom", Endpoint: srvB.URL, AllowedUsers: []string{"*"}},
	}); err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}

	group := &models.AgentGroup{ID: "g1", Name: "Test Group", AgentIDs: []string{"agent-a", "agent-b"}, CreatedBy: "user@example.com"}
	groupRepo := &mcpFakeGroupRepo{groups: map[string]*models.AgentGroup{"g1": group}}
	userService := service.NewUserService(&mcpFakeUserRepo{}, nil, nil)

	h := &Handler{
		server:       NewServer(registry, nil, userService, groupRepo, zap.NewNop()),
		registry:     registry,
		proxy:        proxy.NewProxy(zap.NewNop()),
		sessionStore: newMCPFakeSessionStore(),
		userService:  userService,
		groupRepo:    groupRepo,
		moderator:    orchestrator.NewWithProvider(&mcpScriptedProvider{responses: moderatorSays}, zap.NewNop()),
		settings:     appsettings.New(nil, zap.NewNop()),
		logger:       zap.NewNop(),
	}
	return h, group
}

// Use case: MCP group call — the debate runs and the collected markdown
// contains each agent's contribution plus the synthesis.
func TestCallGroupCollectsDebate(t *testing.T) {
	h, group := newGroupTestHandler(t, http.StatusOK,
		"agent-a", "agent-b", "FINISH", "Consolidated answer.")

	roster, agentsByID := h.buildGroupRoster(context.Background(), group, "user@example.com", nil)
	if len(roster) != 2 {
		t.Fatalf("roster size = %d, want 2", len(roster))
	}

	text, sessionID, _, err := h.callGroup(context.Background(), group, roster, agentsByID, "is prod healthy?", "", "user@example.com", nil, nil)
	if err != nil {
		t.Fatalf("callGroup: %v", err)
	}
	if sessionID == "" {
		t.Error("expected a session id for continuity")
	}
	for _, want := range []string{"**[Agent A]**", "reply from agent-a", "**[Agent B]**", "reply from agent-b", "**[Moderator]**", "Consolidated answer."} {
		if !strings.Contains(text, want) {
			t.Errorf("collected text missing %q:\n%s", want, text)
		}
	}
}

// Use case: a failing agent turn is reported inline and the debate continues.
func TestCallGroupFailedTurnContinues(t *testing.T) {
	h, group := newGroupTestHandler(t, http.StatusInternalServerError,
		"agent-a", "agent-b", "FINISH")

	roster, agentsByID := h.buildGroupRoster(context.Background(), group, "user@example.com", nil)
	text, _, _, err := h.callGroup(context.Background(), group, roster, agentsByID, "check cluster", "", "user@example.com", nil, nil)
	if err != nil {
		t.Fatalf("callGroup: %v", err)
	}
	if !strings.Contains(text, "_error:") {
		t.Errorf("failed turn not reported inline:\n%s", text)
	}
	if !strings.Contains(text, "reply from agent-b") {
		t.Errorf("debate did not continue after failed turn:\n%s", text)
	}
}

// Use case: moderator says nobody applies.
func TestCallGroupNoAgentApplies(t *testing.T) {
	h, group := newGroupTestHandler(t, http.StatusOK, "FINISH")

	roster, agentsByID := h.buildGroupRoster(context.Background(), group, "user@example.com", nil)
	text, _, _, err := h.callGroup(context.Background(), group, roster, agentsByID, "write a poem", "", "user@example.com", nil, nil)
	if err != nil {
		t.Fatalf("callGroup: %v", err)
	}
	if !strings.Contains(text, "No agent in this group") {
		t.Errorf("missing no-agent notice:\n%s", text)
	}
}

// Use case: tools/list exposes accessible groups as ask_group_* tools.
func TestToolsListIncludesGroups(t *testing.T) {
	h, _ := newGroupTestHandler(t, http.StatusOK)

	resp, _, err := h.server.HandleMessage([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`), "user@example.com", nil, "")
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	body := string(resp)
	if !strings.Contains(body, "ask_group_g1") {
		t.Errorf("tools/list missing group tool:\n%s", body)
	}
	if !strings.Contains(body, "Test Group") || !strings.Contains(body, "Agent A, Agent B") {
		t.Errorf("group tool description missing name/members:\n%s", body)
	}
}

// Guard: a tool name matching the group prefix but with no such group must
// fall through to the agent path (agent IDs starting with "group_").
func TestGroupExistsGuard(t *testing.T) {
	h, _ := newGroupTestHandler(t, http.StatusOK)

	if !h.groupExists(context.Background(), "g1") {
		t.Error("g1 should exist")
	}
	if h.groupExists(context.Background(), "x") {
		t.Error("nonexistent group reported as existing")
	}
}

// Use case (CRITICAL): callGroup rejects resuming a session owned by another
// user, even within an accessible group.
func TestCallGroupCrossUserSessionDenied(t *testing.T) {
	h, group := newGroupTestHandler(t, http.StatusOK, "agent-a", "FINISH")
	roster, agentsByID := h.buildGroupRoster(context.Background(), group, "user@example.com", nil)

	// Seed a session owned by someone else in this group.
	other, _ := h.sessionStore.CreateSession(context.Background(), "someone-else@example.com", "agent-a", "seed")
	other.GroupID = group.ID
	_ = h.sessionStore.SaveSession(context.Background(), other)

	_, _, _, err := h.callGroup(context.Background(), group, roster, agentsByID, "leak?", other.SessionID, "user@example.com", nil, nil)
	if err == nil {
		t.Fatal("expected access denied resuming another user's session")
	}
}

// Use case (HIGH): an unknown session_id errors instead of forking a session.
func TestCallGroupUnknownSession(t *testing.T) {
	h, group := newGroupTestHandler(t, http.StatusOK, "agent-a", "FINISH")
	roster, agentsByID := h.buildGroupRoster(context.Background(), group, "user@example.com", nil)

	_, _, _, err := h.callGroup(context.Background(), group, roster, agentsByID, "hi", "does-not-exist", "user@example.com", nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown session_id")
	}
}

// Use case (HIGH): when every agent turn fails, callGroup signals isError so
// MCP clients don't treat the debate as a success.
func TestCallGroupAllFailedIsError(t *testing.T) {
	// agent-a errors (500); the mock moderator only ever picks agent-a here.
	h, group := newGroupTestHandler(t, http.StatusInternalServerError, "agent-a", "FINISH")
	roster, agentsByID := h.buildGroupRoster(context.Background(), group, "user@example.com", nil)

	text, _, isError, err := h.callGroup(context.Background(), group, roster, agentsByID, "status", "", "user@example.com", nil, nil)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !isError {
		t.Error("expected isError=true when no agent replied successfully")
	}
	if !strings.Contains(text, "_error:") {
		t.Errorf("expected per-agent error in text, got: %s", text)
	}
}

// Use case (IMPORTANT): if the group-session flags can't be persisted, callGroup
// errors instead of handing back a session_id the client can't reopen.
func TestCallGroupPersistFailure(t *testing.T) {
	h, group := newGroupTestHandler(t, http.StatusOK, "agent-a", "FINISH")
	h.sessionStore.(*mcpFakeSessionStore).failSave = true
	roster, agentsByID := h.buildGroupRoster(context.Background(), group, "user@example.com", nil)

	_, _, isError, err := h.callGroup(context.Background(), group, roster, agentsByID, "hi", "", "user@example.com", nil, nil)
	if err == nil {
		t.Fatal("expected an error when the session flags fail to persist")
	}
	if !isError {
		t.Error("expected isError=true on persist failure")
	}
}

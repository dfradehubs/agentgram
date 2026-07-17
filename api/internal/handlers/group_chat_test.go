package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/audit"
	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/llm"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/orchestrator"
	"github.com/dfradehubs/agentgram-api/internal/proxy"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/service"
	appsettings "github.com/dfradehubs/agentgram-api/internal/settings"
	"github.com/dfradehubs/agentgram-api/internal/store"
)

// --- fakes -----------------------------------------------------------------

// scriptedProvider returns moderator responses in order, then FINISH.
type scriptedProvider struct {
	mu            sync.Mutex
	responses     []string
	prompts       []string
	lastMaxTokens int
}

// deadlineAwareProvider makes the second routing decision slow enough that a
// shared debate deadline must expire between two otherwise-fast agent turns.
func (f *scriptedProvider) GenerateContent(_ context.Context, req *llm.Request) (*llm.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastMaxTokens = req.MaxTokens
	if len(req.Messages) > 0 {
		if s, ok := req.Messages[len(req.Messages)-1].Content.(string); ok {
			f.prompts = append(f.prompts, s)
		}
	}
	idx := len(f.prompts) - 1
	if idx >= len(f.responses) {
		return &llm.Response{Text: "FINISH"}, nil
	}
	return &llm.Response{Text: f.responses[idx]}, nil
}

// fakeSessionStore implements the subset of store.SessionStore the group chat
// flow touches. Unimplemented methods panic via the embedded nil interface.
type fakeSessionStore struct {
	store.SessionStore
	mu            sync.Mutex
	sessions      map[string]*models.Session
	agentSessions map[string]string
	order         []string
	counter       int
	failReplyAdd  bool
	failReplyAdds int
	failSetAgent  bool
	rejectCanceled bool
	runEvents      int
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{
		sessions:      make(map[string]*models.Session),
		agentSessions: make(map[string]string),
	}
}

func (f *fakeSessionStore) CreateSession(_ context.Context, userID, agentID, sessionName string) (*models.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counter++
	s := &models.Session{
		SessionID:   uuid.New().String(),
		SessionName: sessionName,
		UserID:      userID,
		AgentIDs:    []string{agentID},
	}
	f.sessions[s.SessionID] = s
	f.order = append(f.order, s.SessionID)
	return s, nil
}

// sessionID returns the ID of the n-th created session (0-based).
func (f *fakeSessionStore) sessionID(n int) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.order[n]
}

func (f *fakeSessionStore) GetSession(_ context.Context, sessionID string) (*models.Session, error) {
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

func (f *fakeSessionStore) SaveSession(_ context.Context, session *models.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.sessions[session.SessionID]
	if !ok {
		f.sessions[session.SessionID] = session
		return nil
	}
	msgs := existing.Messages
	cp := *session
	cp.Messages = msgs
	f.sessions[session.SessionID] = &cp
	return nil
}

func (f *fakeSessionStore) AddMessage(ctx context.Context, sessionID string, msg models.ChatMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rejectCanceled && ctx.Err() != nil {
		return fmt.Errorf("canceled persistence context: %w", ctx.Err())
	}
	if msg.Role == "assistant" && f.failReplyAdd {
		return fmt.Errorf("simulated reply persistence failure")
	}
	if msg.Role == "assistant" && f.failReplyAdds > 0 {
		f.failReplyAdds--
		return fmt.Errorf("simulated one-shot reply persistence failure")
	}
	s, ok := f.sessions[sessionID]
	if !ok {
		return fmt.Errorf("not found")
	}
	s.Messages = append(s.Messages, msg)
	return nil
}

func (f *fakeSessionStore) GetAgentSessionID(_ context.Context, sessionID, agentID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.agentSessions[sessionID+"|"+agentID], nil
}

func (f *fakeSessionStore) SetAgentSessionID(_ context.Context, sessionID, agentID, agentSessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failSetAgent {
		return fmt.Errorf("simulated mapping persistence failure")
	}
	f.agentSessions[sessionID+"|"+agentID] = agentSessionID
	return nil
}

func (f *fakeSessionStore) AppendRunEvent(ctx context.Context, _ string, _ []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rejectCanceled && ctx.Err() != nil {
		return fmt.Errorf("canceled event persistence context: %w", ctx.Err())
	}
	f.runEvents++
	return nil
}
func (f *fakeSessionStore) SetActiveRun(_ context.Context, _, _ string) error          { return nil }
func (f *fakeSessionStore) ClearActiveRun(_ context.Context, _, _ string) error        { return nil }

func (f *fakeSessionStore) messages(sessionID string) []models.ChatMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.sessions[sessionID]; ok {
		return append([]models.ChatMessage(nil), s.Messages...)
	}
	return nil
}

// fakeGroupRepo implements the subset of repository.GroupRepository used.
type fakeGroupRepo struct {
	repository.GroupRepository
	groups map[string]*models.AgentGroup
}

func (f *fakeGroupRepo) Get(_ context.Context, id string) (*models.AgentGroup, error) {
	g, ok := f.groups[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return g, nil
}

func (f *fakeGroupRepo) GetAllInheritedPermissions(_ context.Context) (map[string]*models.InheritedPerms, error) {
	return map[string]*models.InheritedPerms{}, nil
}

func (f *fakeGroupRepo) AddSession(_ context.Context, _, _ string) error    { return nil }
func (f *fakeGroupRepo) RemoveSession(_ context.Context, _, _ string) error { return nil }

// fakeUserRepo makes every user a non-admin (DB lookup misses).
type fakeUserRepo struct{ repository.UserRepository }

func (f *fakeUserRepo) GetByEmail(_ context.Context, _ string) (*models.User, error) {
	return nil, fmt.Errorf("not found")
}

// --- test harness ----------------------------------------------------------

const testUserEmail = "user@example.com"

// mockAgentServer returns an httptest server acting as a "custom" protocol
// agent that replies with plain text (converted to AG-UI by the REST proxy).
func mockAgentServer(t *testing.T, reply string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			http.Error(w, "agent exploded", status)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv
}

type groupChatFixture struct {
	handler  *ProxyHandler
	store    *fakeSessionStore
	provider *scriptedProvider
	router   chi.Router
}

// newGroupChatFixture wires a ProxyHandler with fakes and two mock agents
// (agent-a, agent-b) in group "g1". moderatorSays scripts the LLM moderator.
func newGroupChatFixture(t *testing.T, agentAStatus, agentBStatus int, moderatorSays ...string) *groupChatFixture {
	t.Helper()

	srvA := mockAgentServer(t, "reply from agent-a", agentAStatus)
	srvB := mockAgentServer(t, "reply from agent-b", agentBStatus)

	registry := agents.NewRegistry()
	if err := registry.LoadAgents([]models.Agent{
		{ID: "agent-a", Name: "Agent A", Description: "Handles logs", Protocol: "custom", Endpoint: srvA.URL, AllowedUsers: []string{"*"}},
		{ID: "agent-b", Name: "Agent B", Description: "Handles kubernetes", Protocol: "custom", Endpoint: srvB.URL, AllowedUsers: []string{"*"}},
	}); err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}

	fs := newFakeSessionStore()
	fg := &fakeGroupRepo{groups: map[string]*models.AgentGroup{
		"g1":       {ID: "g1", Name: "Test Group", AgentIDs: []string{"agent-a", "agent-b"}, CreatedBy: testUserEmail},
		"g-denied": {ID: "g-denied", Name: "Private", AgentIDs: []string{"agent-a", "agent-b"}, CreatedBy: "someone-else@example.com"},
	}}
	provider := &scriptedProvider{responses: moderatorSays}

	h := &ProxyHandler{
		registry:    registry,
		userService: service.NewUserService(&fakeUserRepo{}, nil, nil),
		groupRepo:   fg,
		proxy:       proxy.NewProxy(zap.NewNop()),
		store:       fs,
		moderator:   orchestrator.NewWithProvider(provider, zap.NewNop()),
		settings:    appsettings.New(nil, zap.NewNop()),
		audit:       audit.New(zap.NewNop()),
		logger:      zap.NewNop(),
	}

	router := chi.NewRouter()
	router.Post("/api/groups/{groupId}/chat", h.GroupChat)

	return &groupChatFixture{handler: h, store: fs, provider: provider, router: router}
}

func (fx *groupChatFixture) post(t *testing.T, groupID string, body string) *httptest.ResponseRecorder {
	return fx.postAs(t, testUserEmail, groupID, body)
}

func (fx *groupChatFixture) postAs(t *testing.T, email, groupID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/groups/"+groupID+"/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	claims := &auth.Claims{Email: email}
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, claims))
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	return rec
}

// seedGroupSession inserts a group session owned by ownerEmail.
func (fx *groupChatFixture) seedGroupSession(ownerEmail, groupID string) string {
	s, _ := fx.store.CreateSession(context.Background(), ownerEmail, "agent-a", "seed")
	s.IsMultiAgent = true
	s.GroupID = groupID
	_ = fx.store.SaveSession(context.Background(), s)
	return s.SessionID
}

// parseSSEEvents extracts the JSON events from an SSE body.
func parseSSEEvents(t *testing.T, body string) []map[string]interface{} {
	t.Helper()
	var events []map[string]interface{}
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev map[string]interface{}
		if err := json.Unmarshal([]byte(line[6:]), &ev); err != nil {
			t.Fatalf("invalid SSE event %q: %v", line, err)
		}
		events = append(events, ev)
	}
	return events
}

func countEvents(events []map[string]interface{}, eventType string) int {
	n := 0
	for _, ev := range events {
		if ev["type"] == eventType {
			n++
		}
	}
	return n
}

func contentByAgent(events []map[string]interface{}) map[string]string {
	out := make(map[string]string)
	for _, ev := range events {
		if ev["type"] == "TEXT_MESSAGE_CONTENT" {
			agentID, _ := ev["agentId"].(string)
			delta, _ := ev["delta"].(string)
			out[agentID] += delta
		}
	}
	return out
}

func incompleteReasonFromEvents(events []map[string]interface{}) string {
	for _, ev := range events {
		if ev["type"] != "CUSTOM" || ev["subType"] != "debate.incomplete" {
			continue
		}
		data, _ := ev["data"].(map[string]interface{})
		reason, _ := data["reason"].(string)
		return reason
	}
	return ""
}

// --- use-case tests ---------------------------------------------------------

// Use case: the user sends a message and the moderator picks two agents in
// sequence; each reply is streamed tagged per agent in ONE outer run, a
// synthesis is added, and everything is persisted.
func TestGroupChatModeratedDebate(t *testing.T) {
	fx := newGroupChatFixture(t, http.StatusOK, http.StatusOK,
		"agent-a", "agent-b", "FINISH", "Both agents agree: all good.")

	rec := fx.post(t, "g1", `{"messages":[{"role":"user","content":"is prod healthy?"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	events := parseSSEEvents(t, rec.Body.String())

	// Exactly one outer lifecycle pair, no nested runs, no RUN_ERROR
	if got := countEvents(events, "RUN_STARTED"); got != 1 {
		t.Errorf("RUN_STARTED count = %d, want 1", got)
	}
	if got := countEvents(events, "RUN_FINISHED"); got != 1 {
		t.Errorf("RUN_FINISHED count = %d, want 1", got)
	}
	if got := countEvents(events, "RUN_ERROR"); got != 0 {
		t.Errorf("RUN_ERROR count = %d, want 0", got)
	}

	// Both agents' replies streamed, tagged with their agentId
	byAgent := contentByAgent(events)
	if !strings.Contains(byAgent["agent-a"], "reply from agent-a") {
		t.Errorf("missing agent-a tagged content: %v", byAgent)
	}
	if !strings.Contains(byAgent["agent-b"], "reply from agent-b") {
		t.Errorf("missing agent-b tagged content: %v", byAgent)
	}
	// Synthesis streamed as the special "moderator" speaker
	if !strings.Contains(byAgent["moderator"], "Both agents agree") {
		t.Errorf("missing moderator synthesis: %v", byAgent)
	}

	// Moderator turn selections announced
	selects := 0
	for _, ev := range events {
		if ev["type"] == "CUSTOM" && ev["subType"] == "moderator.select" {
			selects++
		}
	}
	if selects != 2 {
		t.Errorf("moderator.select count = %d, want 2", selects)
	}

	// Persistence: user msg + 2 agent replies + synthesis, tagged
	msgs := fx.store.messages(fx.store.sessionID(0))
	var agentIDs []string
	for _, m := range msgs {
		if m.Role == "assistant" {
			agentIDs = append(agentIDs, m.AgentID)
		}
	}
	want := []string{"agent-a", "agent-b", "moderator"}
	if fmt.Sprint(agentIDs) != fmt.Sprint(want) {
		t.Errorf("persisted assistant agent ids = %v, want %v", agentIDs, want)
	}

	// The second moderator prompt must include agent-a's reply (agents build
	// on each other via the shared transcript)
	if len(fx.provider.prompts) < 2 || !strings.Contains(fx.provider.prompts[1], "reply from agent-a") {
		t.Errorf("second moderator prompt missing first agent's reply")
	}

	// The session must register the whole roster as participants so clients
	// recognize it as a multi-agent group session (regression: only the first
	// agent was registered, breaking the web UI's multi-agent detection)
	sess, _ := fx.store.GetSession(context.Background(), fx.store.sessionID(0))
	if fmt.Sprint(sess.AgentIDs) != fmt.Sprint([]string{"agent-a", "agent-b"}) {
		t.Errorf("session AgentIDs = %v, want the full roster", sess.AgentIDs)
	}
	if !sess.IsMultiAgent || sess.GroupID != "g1" {
		t.Errorf("session flags: IsMultiAgent=%v GroupID=%q", sess.IsMultiAgent, sess.GroupID)
	}
}

// Use case: a turn fails (agent down) — the client sees a scoped turn.error,
// no RUN_ERROR, and the debate continues with the next agent.
func TestGroupChatFailedTurnContinues(t *testing.T) {
	fx := newGroupChatFixture(t, http.StatusInternalServerError, http.StatusOK,
		"agent-a", "agent-b", "FINISH")

	rec := fx.post(t, "g1", `{"messages":[{"role":"user","content":"check the cluster"}]}`)
	events := parseSSEEvents(t, rec.Body.String())

	if got := countEvents(events, "RUN_ERROR"); got != 0 {
		t.Errorf("RUN_ERROR leaked into the debate stream (count=%d)", got)
	}
	turnErrors := 0
	for _, ev := range events {
		if ev["type"] == "CUSTOM" && ev["subType"] == "turn.error" {
			turnErrors++
			if data, _ := ev["data"].(map[string]interface{}); data["agentId"] != "agent-a" {
				t.Errorf("turn.error not scoped to agent-a: %v", ev)
			}
		}
	}
	if turnErrors == 0 {
		t.Error("expected a scoped turn.error CUSTOM event")
	}
	// Debate continued: agent-b replied and the run finished normally
	if !strings.Contains(contentByAgent(events)["agent-b"], "reply from agent-b") {
		t.Error("debate did not continue after failed turn")
	}
	if got := countEvents(events, "RUN_FINISHED"); got != 1 {
		t.Errorf("RUN_FINISHED count = %d, want 1", got)
	}
}

// Regression: every selected agent fails but the moderator still says FINISH
// (so debateErr is nil). The run must not look like a clean success — a
// programmatic debate.incomplete{reason:all_agents_failed} is emitted alongside
// the human notice.
func TestGroupChatAllAgentsFailed(t *testing.T) {
	fx := newGroupChatFixture(t, http.StatusInternalServerError, http.StatusInternalServerError,
		"agent-a", "agent-b", "FINISH")

	rec := fx.post(t, "g1", `{"messages":[{"role":"user","content":"check the cluster"}]}`)
	events := parseSSEEvents(t, rec.Body.String())

	// No RUN_ERROR (partial content exists as scoped turn.errors), but the run
	// carries a programmatic incomplete signal.
	if got := countEvents(events, "RUN_ERROR"); got != 0 {
		t.Errorf("RUN_ERROR leaked (count=%d)", got)
	}
	var reason string
	for _, ev := range events {
		if ev["type"] == "CUSTOM" && ev["subType"] == "debate.incomplete" {
			if data, _ := ev["data"].(map[string]interface{}); data != nil {
				reason, _ = data["reason"].(string)
			}
		}
	}
	if reason != "all_agents_failed" {
		t.Errorf("debate.incomplete reason = %q, want all_agents_failed", reason)
	}
	// The user still sees a visible failure notice.
	var allText string
	for _, ev := range events {
		if ev["type"] == "TEXT_MESSAGE_CONTENT" {
			delta, _ := ev["delta"].(string)
			allText += delta
		}
	}
	if !strings.Contains(allText, "All selected agents failed") {
		t.Errorf("missing all-failed notice, got: %q", allText)
	}
}

func TestDebateIncompleteReasonHasSinglePrecedence(t *testing.T) {
	wrappedTimeout := fmt.Errorf("moderator failed: %w", context.DeadlineExceeded)
	if got := debateIncompleteReason(wrappedTimeout, 0, 2); got != "timeout" {
		t.Fatalf("wrapped timeout reason = %q, want timeout", got)
	}
	if got := debateIncompleteReason(errors.New("provider down"), 0, 2); got != "moderator_error" {
		t.Fatalf("moderator failure reason = %q, want moderator_error", got)
	}
	if got := debateIncompleteReason(nil, 0, 2); got != "all_agents_failed" {
		t.Fatalf("all-failed reason = %q, want all_agents_failed", got)
	}
	if got := debateIncompleteReason(nil, 1, 2); got != "" {
		t.Fatalf("successful debate reason = %q, want empty", got)
	}
}

type groupSettingsRepo struct{ vals map[string]string }

func (r groupSettingsRepo) GetAll(context.Context) (map[string]string, error) { return r.vals, nil }
func (r groupSettingsRepo) SetMany(context.Context, map[string]string) error  { return nil }

func TestGroupChatDeadlineBoundsAllTurns(t *testing.T) {
	fx := newGroupChatFixture(t, http.StatusOK, http.StatusOK, "agent-a", "agent-b", "FINISH")
	fast := mockAgentServer(t, "reply from agent-a", http.StatusOK)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(2 * time.Second):
			fmt.Fprint(w, "reply from agent-b")
		}
	}))
	t.Cleanup(slow.Close)
	if err := fx.handler.registry.LoadAgents([]models.Agent{
		{ID: "agent-a", Name: "Agent A", Protocol: "custom", Endpoint: fast.URL, AllowedUsers: []string{"*"}},
		{ID: "agent-b", Name: "Agent B", Protocol: "custom", Endpoint: slow.URL, AllowedUsers: []string{"*"}},
	}); err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	fx.handler.settings = appsettings.New(groupSettingsRepo{vals: map[string]string{
		appsettings.KeyGroupDebateTimeout: "250ms",
	}}, zap.NewNop())

	rec := fx.post(t, "g1", `{"messages":[{"role":"user","content":"check both agents"}]}`)
	events := parseSSEEvents(t, rec.Body.String())
	byAgent := contentByAgent(events)
	if !strings.Contains(byAgent["agent-a"], "reply from agent-a") {
		t.Fatalf("first turn did not complete before the shared deadline: %v", byAgent)
	}
	if byAgent["agent-b"] != "" {
		t.Fatalf("second turn restarted the timeout and completed: %v", byAgent)
	}
	if got := incompleteReasonFromEvents(events); got != "timeout" {
		t.Fatalf("incomplete reason = %q, want timeout", got)
	}
}

func TestGroupChatPersistenceFailuresAreIncomplete(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*fakeSessionStore)
	}{
		{"reply", func(s *fakeSessionStore) { s.failReplyAdd = true }},
		{"mapping", func(s *fakeSessionStore) { s.failSetAgent = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newGroupChatFixture(t, http.StatusOK, http.StatusOK, "agent-a", "FINISH")
			tt.configure(fx.store)

			rec := fx.post(t, "g1", `{"messages":[{"role":"user","content":"status"}]}`)
			events := parseSSEEvents(t, rec.Body.String())
			if got := incompleteReasonFromEvents(events); got != "persistence_error" {
				t.Fatalf("persistence failure ended as clean success; reason = %q", got)
			}
			if !strings.Contains(contentByAgent(events)["agent-a"], "reply from agent-a") {
				t.Fatal("partial reply was not preserved in the stream")
			}
		})
	}
}

func TestGroupChatPersistenceFailureStaysIncompleteAfterLaterSuccess(t *testing.T) {
	fx := newGroupChatFixture(t, http.StatusOK, http.StatusOK, "agent-a", "agent-b", "FINISH")
	fx.store.failReplyAdds = 1

	rec := fx.post(t, "g1", `{"messages":[{"role":"user","content":"status"}]}`)
	events := parseSSEEvents(t, rec.Body.String())
	if got := incompleteReasonFromEvents(events); got != "persistence_error" {
		t.Fatalf("mixed persistence/success result reason = %q, want persistence_error", got)
	}
	if !strings.Contains(contentByAgent(events)["agent-b"], "reply from agent-b") {
		t.Fatal("later successful turn was lost")
	}
}

func TestGroupChatClientCancellationDoesNotCancelFinalization(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		fmt.Fprint(w, "reply after disconnect")
	}))
	t.Cleanup(slow.Close)

	fx := newGroupChatFixture(t, http.StatusOK, http.StatusOK, "agent-a", "FINISH")
	if err := fx.handler.registry.LoadAgents([]models.Agent{
		{ID: "agent-a", Name: "Agent A", Protocol: "custom", Endpoint: slow.URL, AllowedUsers: []string{"*"}},
		{ID: "agent-b", Name: "Agent B", Protocol: "custom", Endpoint: slow.URL, AllowedUsers: []string{"*"}},
	}); err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	fx.store.rejectCanceled = true

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("POST", "/api/groups/g1/chat", strings.NewReader(`{"messages":[{"role":"user","content":"status"}]}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, &auth.Claims{Email: testUserEmail}))
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		fx.router.ServeHTTP(rec, req)
		close(done)
	}()
	<-started
	cancel()
	close(release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("group request did not finalize after client cancellation")
	}

	msgs := fx.store.messages(fx.store.sessionID(0))
	if len(msgs) < 2 || !strings.Contains(msgs[len(msgs)-1].Content, "reply after disconnect") {
		t.Fatalf("assistant reply was not finalized after disconnect: %#v", msgs)
	}
	if fx.store.runEvents == 0 {
		t.Fatal("run events were not buffered after disconnect")
	}
}

// Use case: the moderator decides nobody applies — the user gets an explicit
// notice instead of silence.
func TestGroupChatNoAgentApplies(t *testing.T) {
	fx := newGroupChatFixture(t, http.StatusOK, http.StatusOK, "FINISH")

	rec := fx.post(t, "g1", `{"messages":[{"role":"user","content":"write me a poem"}]}`)
	events := parseSSEEvents(t, rec.Body.String())

	var allText string
	for _, ev := range events {
		if ev["type"] == "TEXT_MESSAGE_CONTENT" {
			delta, _ := ev["delta"].(string)
			allText += delta
		}
	}
	if !strings.Contains(allText, "No agent in this group") {
		t.Errorf("missing no-agent notice, got: %q", allText)
	}
	if got := countEvents(events, "RUN_FINISHED"); got != 1 {
		t.Errorf("RUN_FINISHED count = %d, want 1", got)
	}
}

// Use case: @mention — agent_ids restricts the roster the moderator can pick from.
func TestGroupChatRosterOverride(t *testing.T) {
	// Moderator tries agent-a first, but the override only allows agent-b, so
	// "agent-a" is an unknown id → treated as FINISH after agent-b's turn.
	fx := newGroupChatFixture(t, http.StatusOK, http.StatusOK, "agent-b", "agent-a")

	rec := fx.post(t, "g1", `{"messages":[{"role":"user","content":"kube status"}],"agent_ids":["agent-b"]}`)
	events := parseSSEEvents(t, rec.Body.String())

	byAgent := contentByAgent(events)
	if byAgent["agent-a"] != "" {
		t.Errorf("agent-a spoke despite roster override: %v", byAgent)
	}
	if !strings.Contains(byAgent["agent-b"], "reply from agent-b") {
		t.Errorf("agent-b missing: %v", byAgent)
	}
	// The moderator prompt must only offer agent-b
	if len(fx.provider.prompts) == 0 || strings.Contains(fx.provider.prompts[0], "Handles logs") {
		t.Error("moderator prompt offered agents outside the override roster")
	}
}

// Use case: continuing a conversation — session_id reuses the group session
// and the next debate sees prior messages.
func TestGroupChatSessionContinuity(t *testing.T) {
	fx := newGroupChatFixture(t, http.StatusOK, http.StatusOK,
		"agent-a", "FINISH", // first request
		"agent-b", "FINISH") // second request

	rec1 := fx.post(t, "g1", `{"messages":[{"role":"user","content":"first question"}]}`)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request: %d", rec1.Code)
	}

	firstSession := fx.store.sessionID(0)
	rec2 := fx.post(t, "g1", fmt.Sprintf(`{"messages":[{"role":"user","content":"second question"}],"session_id":"%s"}`, firstSession))
	if rec2.Code != http.StatusOK {
		t.Fatalf("second request: %d, body: %s", rec2.Code, rec2.Body.String())
	}

	msgs := fx.store.messages(firstSession)
	var userMsgs int
	for _, m := range msgs {
		if m.Role == "user" {
			userMsgs++
		}
	}
	if userMsgs != 2 {
		t.Errorf("expected both user messages in the same session, got %d (msgs=%d)", userMsgs, len(msgs))
	}

	// The second debate's moderator prompt includes the first exchange
	prompts := fx.provider.prompts
	last := prompts[len(prompts)-1]
	if !strings.Contains(last, "first question") {
		t.Errorf("second debate prompt missing prior conversation:\n%s", last)
	}
}

// Use case: no moderator LLM configured — clear config error, not a stream.
func TestGroupChatNoModeratorConfigured(t *testing.T) {
	fx := newGroupChatFixture(t, http.StatusOK, http.StatusOK)
	fx.handler.moderator = nil

	rec := fx.post(t, "g1", `{"messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "moderator") {
		t.Errorf("error should mention the moderator role: %s", rec.Body.String())
	}
}

// Use case: access control — a user outside the group's permissions is rejected.
func TestGroupChatAccessDenied(t *testing.T) {
	fx := newGroupChatFixture(t, http.StatusOK, http.StatusOK, "agent-a", "FINISH")

	rec := fx.post(t, "g-denied", `{"messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// Use case: a session from another group cannot be hijacked via session_id.
func TestGroupChatSessionGroupMismatch(t *testing.T) {
	fx := newGroupChatFixture(t, http.StatusOK, http.StatusOK, "agent-a", "FINISH")

	// Create a session that belongs to no group
	s, _ := fx.store.CreateSession(context.Background(), testUserEmail, "agent-a", "other")
	rec := fx.post(t, "g1", fmt.Sprintf(`{"messages":[{"role":"user","content":"hi"}],"session_id":"%s"}`, s.SessionID))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (session belongs to another group)", rec.Code)
	}
}

// Use case (CRITICAL): a group member cannot resume another member's personal
// session even though both can access the group.
func TestGroupChatCrossUserSessionDenied(t *testing.T) {
	fx := newGroupChatFixture(t, http.StatusOK, http.StatusOK, "agent-a", "FINISH")

	// A session owned by someone else, in the same accessible group.
	otherSession := fx.seedGroupSession("someone-else@example.com", "g1")

	rec := fx.postAs(t, testUserEmail, "g1",
		fmt.Sprintf(`{"messages":[{"role":"user","content":"leak?"}],"session_id":"%s"}`, otherSession))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (cannot resume another user's session)", rec.Code)
	}
	// And the debate must not have run against that session.
	msgs := fx.store.messages(otherSession)
	for _, m := range msgs {
		if m.Content == "leak?" {
			t.Error("attacker's message was appended to another user's session")
		}
	}
}

// Use case (HIGH): an unknown session_id is a 404, never a silently forked session.
func TestGroupChatUnknownSessionNotFound(t *testing.T) {
	fx := newGroupChatFixture(t, http.StatusOK, http.StatusOK, "agent-a", "FINISH")

	rec := fx.post(t, "g1", `{"messages":[{"role":"user","content":"hi"}],"session_id":"11111111-1111-1111-1111-111111111111"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (unknown session_id)", rec.Code)
	}
	// No new session should have been created as a side effect.
	if len(fx.store.order) != 0 {
		t.Errorf("a session was created for an unknown session_id (%d sessions)", len(fx.store.order))
	}
}

// Use case (HIGH): the synthesis LLM call carries a positive MaxTokens so
// providers that reject max_tokens:0 don't fail.
func TestGroupChatSynthesisMaxTokens(t *testing.T) {
	fx := newGroupChatFixture(t, http.StatusOK, http.StatusOK,
		"agent-a", "agent-b", "FINISH", "the synthesis")

	rec := fx.post(t, "g1", `{"messages":[{"role":"user","content":"both please"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	// The synthesis prompt is the last moderator call; it must set MaxTokens > 0.
	if got := fx.provider.lastMaxTokens; got <= 0 {
		t.Errorf("synthesis MaxTokens = %d, want > 0", got)
	}
}

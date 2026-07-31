package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/service"
	"go.uber.org/zap"
)

// adminRoleUserRepo returns a user with a fixed DB role, so ResolveRole can be
// steered per test.
type adminRoleUserRepo struct {
	repository.UserRepository
	role string
}

func (f *adminRoleUserRepo) GetByEmail(_ context.Context, email string) (*models.User, error) {
	if f.role == "" {
		return nil, fmt.Errorf("not found")
	}
	return &models.User{Email: email, Role: f.role}, nil
}

// capturedRequest is what a stub admin router saw.
type capturedRequest struct {
	method  string
	path    string
	query   string
	body    string
	surface bool
}

// stubAdminRouter records the request and replies with the given status/body.
type stubAdminRouter struct {
	status int
	body   string
	// getBody, when set, answers GET requests (used for the secret-restore path).
	getBody string
	seen    []capturedRequest
}

func (s *stubAdminRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	mcp, _ := r.Context().Value(middleware.SurfaceMCPContextKey).(bool)
	s.seen = append(s.seen, capturedRequest{
		method:  r.Method,
		path:    r.URL.Path,
		query:   r.URL.RawQuery,
		body:    string(body),
		surface: mcp,
	})

	if r.Method == http.MethodGet && s.getBody != "" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(s.getBody))
		return
	}
	w.WriteHeader(s.status)
	w.Write([]byte(s.body))
}

// newAdminTestHandler builds a Handler whose admin surface is the given router,
// with the caller resolving to dbRole.
func newAdminTestHandler(router http.Handler, dbRole string) *Handler {
	userService := service.NewUserService(&adminRoleUserRepo{role: dbRole}, nil, nil, nil, nil)
	h := &Handler{
		server:      NewServer(agents.NewRegistry(), nil, userService, nil, nil, zap.NewNop()),
		userService: userService,
		logger:      zap.NewNop(),
	}
	h.SetAdminRouter(router)
	return h
}

// callAdminTool drives a tools/call through the public entry point.
func callAdminTool(t *testing.T, h *Handler, name string, args map[string]interface{}) (string, bool) {
	t.Helper()

	rawArgs, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, name, rawArgs)

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	claims := &auth.Claims{Email: "u@example.com", Groups: []string{"eng"}}
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, claims))
	rec := httptest.NewRecorder()

	h.HandleMCP(rec, req)

	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response %s: %v", rec.Body.String(), err)
	}
	if resp.Error != nil {
		return resp.Error.Message, true
	}
	if len(resp.Result.Content) == 0 {
		t.Fatalf("no content in response: %s", rec.Body.String())
	}
	return resp.Result.Content[0].Text, resp.Result.IsError
}

// adminToolNames lists the admin tools advertised to a user with the given role.
func adminToolNames(t *testing.T, h *Handler, email string, groups []string) []string {
	t.Helper()

	resp, _, err := h.server.HandleMessage([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`), email, groups, "")
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}

	var out struct {
		Result struct {
			Tools []struct {
				Name        string                 `json:"name"`
				Description string                 `json:"description"`
				Annotations map[string]interface{} `json:"annotations"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		t.Fatalf("unmarshal tools/list: %v", err)
	}

	var names []string
	for _, tool := range out.Result.Tools {
		if strings.HasPrefix(tool.Name, adminToolPrefix) {
			names = append(names, tool.Name)
		}
	}
	return names
}

// Use case: an ordinary user must not even see the admin tools, an editor sees
// the CRUD subset, and an admin sees everything.
func TestToolsListFiltersAdminToolsByRole(t *testing.T) {
	editorCount := 0
	for _, tool := range adminTools {
		if tool.minRole == models.RoleEditor {
			editorCount++
		}
	}

	tests := []struct {
		name  string
		role  string
		count int
	}{
		{"plain user", "", 0},
		{"viewer", models.RoleViewer, 0},
		{"editor", models.RoleEditor, editorCount},
		{"admin", models.RoleAdmin, len(adminTools)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newAdminTestHandler(&stubAdminRouter{status: http.StatusOK, body: "{}"}, tt.role)
			got := adminToolNames(t, h, "u@example.com", nil)
			if len(got) != tt.count {
				t.Errorf("advertised %d admin tools (%v), want %d", len(got), got, tt.count)
			}
		})
	}
}

// Use case: without an admin router (admin API disabled) nothing is advertised.
func TestToolsListHidesAdminToolsWhenRouterMissing(t *testing.T) {
	userService := service.NewUserService(&adminRoleUserRepo{role: models.RoleAdmin}, nil, nil, nil, nil)
	h := &Handler{
		server:      NewServer(agents.NewRegistry(), nil, userService, nil, nil, zap.NewNop()),
		userService: userService,
		logger:      zap.NewNop(),
	}

	if got := adminToolNames(t, h, "u@example.com", nil); len(got) != 0 {
		t.Errorf("expected no admin tools, got %v", got)
	}
}

// Use case: write tools must carry the annotations that make a client ask the
// user before running them, and a warning in the description.
func TestAdminToolAnnotations(t *testing.T) {
	for _, tool := range adminTools {
		t.Run(tool.name, func(t *testing.T) {
			ann := tool.definition()["annotations"].(map[string]interface{})
			if ann["title"] == "" {
				t.Error("missing title")
			}
			if ann["readOnlyHint"] != tool.readOnly {
				t.Errorf("readOnlyHint = %v, want %v", ann["readOnlyHint"], tool.readOnly)
			}
			if tool.readOnly && tool.destructive {
				t.Error("a read-only tool cannot be destructive")
			}
			if tool.method != http.MethodGet && tool.readOnly {
				t.Errorf("%s is not a GET but claims readOnlyHint", tool.method)
			}
			if !tool.readOnly && !strings.Contains(tool.description, "Do NOT call it unless") &&
				!strings.Contains(tool.description, "Only call it when the user has explicitly asked") {
				t.Errorf("write tool lacks an explicit-authorization warning: %s", tool.description)
			}
		})
	}
}

// Use case: the tool must reach the admin router as the right HTTP request, and
// be tagged as MCP-originated so the audit log can tell the surfaces apart.
func TestAdminToolDispatchesToRouter(t *testing.T) {
	tests := []struct {
		name       string
		tool       string
		args       map[string]interface{}
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		{
			name: "list", tool: "admin_list_agents", args: nil,
			wantMethod: http.MethodGet, wantPath: "/agents",
		},
		{
			name: "get by id", tool: "admin_get_agent", args: map[string]interface{}{"id": "kube-agent"},
			wantMethod: http.MethodGet, wantPath: "/agents/kube-agent",
		},
		{
			name: "create", tool: "admin_create_agent",
			args:       map[string]interface{}{"body": map[string]interface{}{"id": "a1", "name": "A1"}},
			wantMethod: http.MethodPost, wantPath: "/agents", wantBody: `{"id":"a1","name":"A1"}`,
		},
		{
			name: "delete", tool: "admin_delete_mcp_server", args: map[string]interface{}{"id": "example-mcp"},
			wantMethod: http.MethodDelete, wantPath: "/mcp/example-mcp",
		},
		{
			name: "audit with filters", tool: "admin_query_audit",
			args:       map[string]interface{}{"resource_type": "admin", "limit": 10},
			wantMethod: http.MethodGet, wantPath: "/audit", wantQuery: "limit=10&resource_type=admin",
		},
		{
			name: "metrics global stats", tool: "admin_metrics",
			args:       map[string]interface{}{"scope": "global", "view": "stats", "interval": "1h"},
			wantMethod: http.MethodGet, wantPath: "/metrics/overview", wantQuery: "interval=1h",
		},
		{
			name: "metrics per agent timeline", tool: "admin_metrics",
			args:       map[string]interface{}{"scope": "agent", "view": "timeline", "id": "kube-agent"},
			wantMethod: http.MethodGet, wantPath: "/metrics/agents/kube-agent/timeline",
		},
		{
			name: "settings update", tool: "admin_update_settings",
			args:       map[string]interface{}{"values": map[string]interface{}{"group_max_turns_mcp": "4"}},
			wantMethod: http.MethodPut, wantPath: "/settings", wantBody: `{"group_max_turns_mcp":"4"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := &stubAdminRouter{status: http.StatusOK, body: `{"ok":true}`}
			h := newAdminTestHandler(router, models.RoleAdmin)

			text, isErr := callAdminTool(t, h, tt.tool, tt.args)
			if isErr {
				t.Fatalf("unexpected error result: %s", text)
			}
			if len(router.seen) != 1 {
				t.Fatalf("expected 1 request, got %d", len(router.seen))
			}

			got := router.seen[0]
			if got.method != tt.wantMethod {
				t.Errorf("method = %s, want %s", got.method, tt.wantMethod)
			}
			if got.path != tt.wantPath {
				t.Errorf("path = %s, want %s", got.path, tt.wantPath)
			}
			if got.query != tt.wantQuery {
				t.Errorf("query = %q, want %q", got.query, tt.wantQuery)
			}
			if tt.wantBody != "" && got.body != tt.wantBody {
				t.Errorf("body = %s, want %s", got.body, tt.wantBody)
			}
			if !got.surface {
				t.Error("request not tagged as MCP-originated")
			}
		})
	}
}

// Use case: an editor calling an admin-only tool must be refused by the real role
// gate, not by a duplicated check in the MCP layer.
func TestAdminToolForbiddenComesFromRoleGate(t *testing.T) {
	userService := service.NewUserService(&adminRoleUserRepo{role: models.RoleEditor}, nil, nil, nil, nil)

	adminRouter := chi.NewRouter()
	adminRouter.Group(func(r chi.Router) {
		r.Use(middleware.RequireRole(userService, models.RoleAdmin, zap.NewNop()).Handler)
		r.Delete("/agents/{id}", func(w http.ResponseWriter, _ *http.Request) {
			t.Error("handler must not be reached")
			w.WriteHeader(http.StatusOK)
		})
	})
	adminRouter.Group(func(r chi.Router) {
		r.Use(middleware.RequireRole(userService, models.RoleEditor, zap.NewNop()).Handler)
		r.Get("/agents", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(`{"agents":[]}`))
		})
	})

	h := &Handler{
		server:      NewServer(agents.NewRegistry(), nil, userService, nil, nil, zap.NewNop()),
		userService: userService,
		logger:      zap.NewNop(),
	}
	h.SetAdminRouter(adminRouter)

	text, isErr := callAdminTool(t, h, "admin_delete_agent", map[string]interface{}{"id": "kube-agent"})
	if !isErr {
		t.Fatalf("expected an error result, got %s", text)
	}
	if !strings.Contains(text, "403") || !strings.Contains(text, "forbidden") {
		t.Errorf("expected a 403 forbidden result, got %s", text)
	}

	// The editor-level tool on the same router still works, proving the loopback
	// routes correctly rather than failing for everything.
	if text, isErr := callAdminTool(t, h, "admin_list_agents", nil); isErr {
		t.Errorf("editor tool failed: %s", text)
	}
}

// Use case: reading an agent must never hand a credential to the MCP client.
func TestAdminToolRedactsSecrets(t *testing.T) {
	router := &stubAdminRouter{
		status: http.StatusOK,
		body:   `{"id":"a1","bearer_token":"ghp_secret","headers":{"Authorization":"Bearer leak"}}`,
	}
	h := newAdminTestHandler(router, models.RoleAdmin)

	text, isErr := callAdminTool(t, h, "admin_get_agent", map[string]interface{}{"id": "a1"})
	if isErr {
		t.Fatalf("unexpected error: %s", text)
	}
	if strings.Contains(text, "ghp_secret") || strings.Contains(text, "Bearer leak") {
		t.Errorf("tool leaked a secret: %s", text)
	}
	if !strings.Contains(text, `"bearer_token": "***"`) {
		t.Errorf("expected a redacted token: %s", text)
	}
}

// Use case: get → edit → update round-trip. The client sends back the redaction
// sentinel for the token it never saw; the stored token must survive.
func TestAdminUpdatePreservesRedactedSecrets(t *testing.T) {
	router := &stubAdminRouter{
		status:  http.StatusOK,
		body:    `{"ok":true}`,
		getBody: `{"id":"a1","name":"Old","bearer_token":"ghp_stored"}`,
	}
	h := newAdminTestHandler(router, models.RoleAdmin)

	text, isErr := callAdminTool(t, h, "admin_update_agent", map[string]interface{}{
		"id":   "a1",
		"body": map[string]interface{}{"id": "a1", "name": "New", "bearer_token": "***"},
	})
	if isErr {
		t.Fatalf("unexpected error: %s", text)
	}

	// One internal GET to read the stored secrets, then the PUT.
	if len(router.seen) != 2 {
		t.Fatalf("expected a GET followed by a PUT, got %d requests", len(router.seen))
	}
	if router.seen[0].method != http.MethodGet || router.seen[1].method != http.MethodPut {
		t.Fatalf("unexpected request order: %s then %s", router.seen[0].method, router.seen[1].method)
	}
	put := router.seen[1].body
	if !strings.Contains(put, `"bearer_token":"ghp_stored"`) {
		t.Errorf("stored token not restored, PUT body was %s", put)
	}
	if !strings.Contains(put, `"name":"New"`) {
		t.Errorf("edit not applied, PUT body was %s", put)
	}
}

// Use case: a delete answers 204 with no body. An empty tool result would read as
// a failure to the model, so it gets an explicit confirmation.
func TestAdminDeleteReportsSuccessWithoutBody(t *testing.T) {
	router := &stubAdminRouter{status: http.StatusNoContent, body: ""}
	h := newAdminTestHandler(router, models.RoleAdmin)

	text, isErr := callAdminTool(t, h, "admin_delete_agent", map[string]interface{}{"id": "a1"})
	if isErr {
		t.Fatalf("unexpected error: %s", text)
	}
	if !strings.Contains(text, "204") || !strings.Contains(strings.ToLower(text), "done") {
		t.Errorf("expected an explicit confirmation, got %q", text)
	}
}

// Use case: a create has nothing stored to restore from, so no extra read.
func TestAdminCreateDoesNotReadFirst(t *testing.T) {
	router := &stubAdminRouter{status: http.StatusCreated, body: `{"id":"a1"}`, getBody: `{"unused":true}`}
	h := newAdminTestHandler(router, models.RoleAdmin)

	callAdminTool(t, h, "admin_create_agent", map[string]interface{}{
		"body": map[string]interface{}{"id": "a1", "bearer_token": "***"},
	})

	if len(router.seen) != 1 || router.seen[0].method != http.MethodPost {
		t.Errorf("expected a single POST, got %v", router.seen)
	}
}

// Use case: the audit log holds every user's conversations; the MCP surface must
// only show enough to identify one.
func TestAdminQueryAuditTruncatesContent(t *testing.T) {
	long := strings.Repeat("x", auditContentLimit+200)
	router := &stubAdminRouter{
		status: http.StatusOK,
		body:   fmt.Sprintf(`{"events":[{"user_email":"user@example.com","prompt":%q,"response":%q}],"total":1}`, long, "short"),
	}
	h := newAdminTestHandler(router, models.RoleAdmin)

	text, isErr := callAdminTool(t, h, "admin_query_audit", nil)
	if isErr {
		t.Fatalf("unexpected error: %s", text)
	}
	if !strings.Contains(text, "truncated") {
		t.Errorf("long prompt was not truncated: %s", text[:200])
	}
	if strings.Contains(text, strings.Repeat("x", auditContentLimit+1)) {
		t.Error("full prompt leaked through")
	}
	if !strings.Contains(text, "short") {
		t.Error("a short response should be preserved verbatim")
	}
}

// Use case: a model guesses a scope/view pair that has no endpoint. It must get a
// usable error instead of a 404 from a made-up path.
func TestAdminMetricsUnsupportedCombination(t *testing.T) {
	router := &stubAdminRouter{status: http.StatusOK, body: "{}"}
	h := newAdminTestHandler(router, models.RoleAdmin)

	text, isErr := callAdminTool(t, h, "admin_metrics", map[string]interface{}{"scope": "user", "view": "errors"})
	if !isErr {
		t.Fatalf("expected an error result, got %s", text)
	}
	if !strings.Contains(text, "unsupported") || !strings.Contains(text, "global/stats") {
		t.Errorf("error should list the supported combinations: %s", text)
	}
	if len(router.seen) != 0 {
		t.Error("no request should reach the admin router")
	}
}

// Use case: an id must never be able to steer the loopback to another endpoint.
func TestAdminToolRejectsUnsafeID(t *testing.T) {
	for _, id := range []string{"../users/victim@example.com/role", "a/b", "a b", ""} {
		t.Run(fmt.Sprintf("id=%q", id), func(t *testing.T) {
			router := &stubAdminRouter{status: http.StatusOK, body: "{}"}
			h := newAdminTestHandler(router, models.RoleAdmin)

			text, isErr := callAdminTool(t, h, "admin_get_agent", map[string]interface{}{"id": id})
			if !isErr {
				t.Fatalf("expected an error result, got %s", text)
			}
			if len(router.seen) != 0 {
				t.Errorf("request reached the router: %v", router.seen)
			}
		})
	}
}

// Use case: a missing or malformed body must fail before touching the router.
func TestAdminToolRejectsBadBody(t *testing.T) {
	tests := []struct {
		name string
		args map[string]interface{}
	}{
		{"missing body", map[string]interface{}{}},
		{"body is a string", map[string]interface{}{"body": "not an object"}},
		{"body is null", map[string]interface{}{"body": nil}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := &stubAdminRouter{status: http.StatusOK, body: "{}"}
			h := newAdminTestHandler(router, models.RoleAdmin)

			text, isErr := callAdminTool(t, h, "admin_create_agent", tt.args)
			if !isErr {
				t.Fatalf("expected an error result, got %s", text)
			}
			if len(router.seen) != 0 {
				t.Errorf("request reached the router: %v", router.seen)
			}
		})
	}
}

// Use case: admin tool names live in their own namespace, so they can never be
// mistaken for an agent, group, skill or MCP-server tool.
func TestAdminToolNamesDoNotCollide(t *testing.T) {
	for _, tool := range adminTools {
		if _, ok := GetAgentIDFromToolName(tool.name); ok {
			t.Errorf("%s parses as an agent tool", tool.name)
		}
		if _, ok := GetGroupIDFromToolName(tool.name); ok {
			t.Errorf("%s parses as a group tool", tool.name)
		}
		if _, ok := GetSkillIDFromToolName(tool.name); ok {
			t.Errorf("%s parses as a skill tool", tool.name)
		}
		if _, _, ok := GetMCPToolFromName(tool.name); ok {
			t.Errorf("%s parses as an MCP server tool", tool.name)
		}
		if !strings.HasPrefix(tool.name, adminToolPrefix) {
			t.Errorf("%s is missing the %s prefix", tool.name, adminToolPrefix)
		}
	}
}

// Use case: an unknown admin_* tool must be reported, not silently routed.
func TestUnknownAdminToolIsRejected(t *testing.T) {
	h := newAdminTestHandler(&stubAdminRouter{status: http.StatusOK, body: "{}"}, models.RoleAdmin)

	text, isErr := callAdminTool(t, h, "admin_drop_database", nil)
	if !isErr || !strings.Contains(text, "unknown tool") {
		t.Errorf("expected an unknown-tool error, got %q (isError=%v)", text, isErr)
	}
}

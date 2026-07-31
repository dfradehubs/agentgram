package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"go.uber.org/zap"
)

// recordingAuditRepo captures inserted events. RecordEvent inserts async, so the
// repo signals each insert on a channel the tests wait on.
type recordingAuditRepo struct {
	mu     sync.Mutex
	events []*models.AuditEvent
	got    chan struct{}
}

func newRecordingAuditRepo() *recordingAuditRepo {
	return &recordingAuditRepo{got: make(chan struct{}, 8)}
}

func (r *recordingAuditRepo) Insert(_ context.Context, ev *models.AuditEvent) error {
	r.mu.Lock()
	r.events = append(r.events, ev)
	r.mu.Unlock()
	r.got <- struct{}{}
	return nil
}

func (r *recordingAuditRepo) List(_ context.Context, _ models.AuditEventFilter) ([]*models.AuditEvent, int, error) {
	return nil, 0, nil
}

func (r *recordingAuditRepo) Cleanup(_ context.Context, _ int) (int64, error) {
	return 0, nil
}

// waitForEvent blocks until one event is inserted, or fails the test.
func (r *recordingAuditRepo) waitForEvent(t *testing.T) *models.AuditEvent {
	t.Helper()
	select {
	case <-r.got:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for audit event")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.events[len(r.events)-1]
}

func (r *recordingAuditRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

// newAuditTestRouter mounts the middleware on a chi router shaped like the admin
// one, with claims injected the way MCPAuth/Auth do.
func newAuditTestRouter(repo *recordingAuditRepo, status int) chi.Router {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			claims := &auth.Claims{Email: "editor@example.com", Groups: []string{"eng"}}
			next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), UserContextKey, claims)))
		})
	})
	r.Use(NewAdminAudit(repo, nil, zap.NewNop()).Handler)

	handler := func(w http.ResponseWriter, req *http.Request) {
		body, _ := readAll(req)
		w.WriteHeader(status)
		w.Write([]byte(`{"id":"a1","bearer_token":"ghp_stored","echo":` + string(quoteJSON(body)) + `}`))
	}
	r.Get("/agents", handler)
	r.Post("/agents", handler)
	r.Put("/agents/{id}", handler)
	r.Delete("/agents/{id}", handler)
	r.Put("/settings", handler)
	return r
}

func readAll(r *http.Request) ([]byte, error) {
	buf := make([]byte, 0, 512)
	tmp := make([]byte, 256)
	for {
		n, err := r.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			return buf, nil
		}
	}
}

func quoteJSON(b []byte) []byte {
	return []byte(`"` + strings.ReplaceAll(string(b), `"`, `\"`) + `"`)
}

// Use case: only mutations belong in the audit log; listing agents does not.
func TestAdminAuditRecordsMutationsOnly(t *testing.T) {
	tests := []struct {
		method     string
		path       string
		wantAction string
		wantEvent  bool
	}{
		{http.MethodGet, "/agents", "", false},
		{http.MethodPost, "/agents", models.AuditActionAdminCreate, true},
		{http.MethodPut, "/agents/kube-agent", models.AuditActionAdminUpdate, true},
		{http.MethodDelete, "/agents/kube-agent", models.AuditActionAdminDelete, true},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			repo := newRecordingAuditRepo()
			router := newAuditTestRouter(repo, http.StatusOK)

			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{"name":"x"}`))
			router.ServeHTTP(httptest.NewRecorder(), req)

			if !tt.wantEvent {
				// Nothing to wait for; a spurious insert would be visible immediately.
				time.Sleep(50 * time.Millisecond)
				if repo.count() != 0 {
					t.Fatalf("expected no audit event, got %d", repo.count())
				}
				return
			}

			ev := repo.waitForEvent(t)
			if ev.Action != tt.wantAction {
				t.Errorf("action = %q, want %q", ev.Action, tt.wantAction)
			}
			if ev.ResourceType != models.AuditResourceAdmin {
				t.Errorf("resource_type = %q, want %q", ev.ResourceType, models.AuditResourceAdmin)
			}
			if ev.UserEmail != "editor@example.com" {
				t.Errorf("user_email = %q", ev.UserEmail)
			}
			if ev.Status != "ok" {
				t.Errorf("status = %q, want ok", ev.Status)
			}
		})
	}
}

// Use case: an admin change made from the MCP tools must be distinguishable from
// one made in the web admin.
func TestAdminAuditSourceFromContext(t *testing.T) {
	tests := []struct {
		name    string
		mcp     bool
		want    string
		wantRes string
	}{
		{"web admin", false, models.AuditSourceWeb, "agents:kube-agent"},
		{"mcp tools", true, models.AuditSourceMCP, "agents:kube-agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRecordingAuditRepo()
			router := newAuditTestRouter(repo, http.StatusOK)

			req := httptest.NewRequest(http.MethodPut, "/agents/kube-agent", strings.NewReader(`{"name":"x"}`))
			if tt.mcp {
				req = req.WithContext(WithSurfaceMCP(req.Context()))
			}
			router.ServeHTTP(httptest.NewRecorder(), req)

			ev := repo.waitForEvent(t)
			if ev.Source != tt.want {
				t.Errorf("source = %q, want %q", ev.Source, tt.want)
			}
			if ev.ResourceID != tt.wantRes {
				t.Errorf("resource_id = %q, want %q", ev.ResourceID, tt.wantRes)
			}
		})
	}
}

// Use case: a create has no ID in its path, but the audit entry still has to say
// which resource was created.
func TestAdminAuditResourceIDForCreate(t *testing.T) {
	repo := newRecordingAuditRepo()
	router := newAuditTestRouter(repo, http.StatusCreated)

	req := httptest.NewRequest(http.MethodPost, "/agents", strings.NewReader(`{"id":"new-agent","name":"New"}`))
	router.ServeHTTP(httptest.NewRecorder(), req)

	ev := repo.waitForEvent(t)
	// The stub handler echoes id "a1"; the response is preferred over the request
	// because it reflects what was actually stored.
	if ev.ResourceID != "agents:a1" {
		t.Errorf("resource_id = %q, want agents:a1", ev.ResourceID)
	}
}

// Use case: a denied attempt is exactly what an auditor wants to see.
func TestAdminAuditRecordsForbidden(t *testing.T) {
	repo := newRecordingAuditRepo()
	router := newAuditTestRouter(repo, http.StatusForbidden)

	req := httptest.NewRequest(http.MethodDelete, "/agents/kube-agent", nil)
	router.ServeHTTP(httptest.NewRecorder(), req)

	ev := repo.waitForEvent(t)
	if ev.Status != "error" {
		t.Errorf("status = %q, want error", ev.Status)
	}
	if ev.ErrorType != http.StatusText(http.StatusForbidden) {
		t.Errorf("error_type = %q", ev.ErrorType)
	}
}

// Use case: the audit log must never become a place to read credentials from.
func TestAdminAuditRedactsSecrets(t *testing.T) {
	repo := newRecordingAuditRepo()
	router := newAuditTestRouter(repo, http.StatusOK)

	body := `{"id":"a1","bearer_token":"ghp_supersecret","headers":{"Authorization":"Bearer leak"}}`
	req := httptest.NewRequest(http.MethodPut, "/agents/a1", strings.NewReader(body))
	router.ServeHTTP(httptest.NewRecorder(), req)

	ev := repo.waitForEvent(t)
	if strings.Contains(ev.Prompt, "ghp_supersecret") || strings.Contains(ev.Prompt, "Bearer leak") {
		t.Errorf("prompt leaked a secret: %s", ev.Prompt)
	}
	if !strings.Contains(ev.Prompt, `"bearer_token":"***"`) {
		t.Errorf("prompt not redacted: %s", ev.Prompt)
	}
	if strings.Contains(ev.Response, "ghp_stored") {
		t.Errorf("response leaked a secret: %s", ev.Response)
	}
}

// Use case: the handler must still be able to read the body the middleware buffered.
func TestAdminAuditLeavesBodyReadable(t *testing.T) {
	repo := newRecordingAuditRepo()
	router := newAuditTestRouter(repo, http.StatusOK)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/agents", strings.NewReader(`{"name":"readable"}`))
	router.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "readable") {
		t.Errorf("handler did not see the body: %s", rec.Body.String())
	}
	repo.waitForEvent(t)
}

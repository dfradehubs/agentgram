package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
)

// fakeRepo serves a fixed list of MCP servers; only List is used by the registry.
type fakeRepo struct {
	repository.MCPServerRepository
	mu      sync.Mutex
	servers []*models.MCPServer
}

func (f *fakeRepo) List(context.Context) ([]*models.MCPServer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*models.MCPServer, 0, len(f.servers))
	for _, s := range f.servers {
		c := *s
		out = append(out, &c)
	}
	return out, nil
}

// fakeMCP is a JSON-RPC MCP server whose tool list can change at runtime. It records the
// X-Channel-Key header of the last tools/list call.
type fakeMCP struct {
	tools   atomic.Value // []string
	lastKey atomic.Value // string
	lists   atomic.Int32
	inits   atomic.Int32
	delay   time.Duration // slows initialize down, to overlap it with reloads
}

func newFakeMCP(t *testing.T, tools ...string) (*fakeMCP, *httptest.Server) {
	return newSlowFakeMCP(t, 0, tools...)
}

func newSlowFakeMCP(t *testing.T, delay time.Duration, tools ...string) (*fakeMCP, *httptest.Server) {
	f := &fakeMCP{delay: delay}
	f.tools.Store(tools)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var result any
		switch req.Method {
		case "initialize":
			f.inits.Add(1)
			time.Sleep(f.delay)
			result = map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{},
				"serverInfo": map[string]any{"name": "fake", "version": "1"}}
		case "tools/list":
			f.lists.Add(1)
			f.lastKey.Store(r.Header.Get("X-Channel-Key"))
			list := []map[string]any{}
			for _, n := range f.tools.Load().([]string) {
				list = append(list, map[string]any{"name": n, "inputSchema": map[string]any{"type": "object"}})
			}
			result = map[string]any{"tools": list}
		default: // notifications
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Regression: every API pod kept the tool list it fetched when it first connected, so tools
// added upstream (or by a deploy) never showed up until the pod restarted, and two replicas
// served different catalogs.
func TestLoadFromDBRefreshesStaleTools(t *testing.T) {
	up, srv := newFakeMCP(t, "a", "b")
	repo := &fakeRepo{servers: []*models.MCPServer{{ID: "s1", Name: "s1", Transport: "http", URL: srv.URL,
		Headers: map[string]string{"X-Channel-Key": "k1"}, AuthType: "none"}}}
	r := NewDBRegistry(repo, func() time.Duration { return 5 * time.Second }, zap.NewNop())
	r.toolsTTL = 0 // every refresh re-lists

	if err := r.LoadFromDB(context.Background()); err != nil {
		t.Fatal(err)
	}
	info, _ := r.Get("s1")
	waitFor(t, "initial tools", func() bool { return len(info.GetTools()) == 2 })

	up.tools.Store([]string{"a", "b", "c"})
	if err := r.LoadFromDB(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "new tool", func() bool { return len(info.GetTools()) == 3 })
	if same, _ := r.Get("s1"); same != info {
		t.Fatal("an unchanged server must keep its connection")
	}
}

func TestLoadFromDBKeepsFreshToolsWithinTTL(t *testing.T) {
	up, srv := newFakeMCP(t, "a")
	repo := &fakeRepo{servers: []*models.MCPServer{{ID: "s1", Name: "s1", Transport: "http", URL: srv.URL, AuthType: "none"}}}
	r := NewDBRegistry(repo, func() time.Duration { return 5 * time.Second }, zap.NewNop())
	_ = r.LoadFromDB(context.Background())
	info, _ := r.Get("s1")
	waitFor(t, "initial tools", func() bool { return len(info.GetTools()) == 1 })
	for i := 0; i < 5; i++ {
		_ = r.LoadFromDB(context.Background())
	}
	time.Sleep(100 * time.Millisecond)
	if n := up.lists.Load(); n != 1 {
		t.Fatalf("tools/list called %d times within the TTL", n)
	}
}

// Regression: a changed header (e.g. a rotated channel key) or auth setting kept the old
// client, so the new value was only used after a restart.
func TestLoadFromDBReconnectsWhenHeadersChange(t *testing.T) {
	up, srv := newFakeMCP(t, "a")
	repo := &fakeRepo{servers: []*models.MCPServer{{ID: "s1", Name: "s1", Transport: "http", URL: srv.URL,
		Headers: map[string]string{"X-Channel-Key": "old"}, AuthType: "none"}}}
	r := NewDBRegistry(repo, func() time.Duration { return 5 * time.Second }, zap.NewNop())
	_ = r.LoadFromDB(context.Background())
	first, _ := r.Get("s1")
	waitFor(t, "initial tools", func() bool { return len(first.GetTools()) == 1 })

	repo.mu.Lock()
	repo.servers[0].Headers = map[string]string{"X-Channel-Key": "new"}
	repo.mu.Unlock()
	_ = r.LoadFromDB(context.Background())
	second, _ := r.Get("s1")
	if second == first {
		t.Fatal("a server whose headers changed must get a new client")
	}
	waitFor(t, "reconnect with the new key", func() bool { k, _ := up.lastKey.Load().(string); return k == "new" })
}

// Bugbot: a new server's list starts stale, so a reload during its first initialize started a
// second, overlapping handshake that could end the session or flip the status to error.
func TestReloadDuringFirstInitDoesNotInitTwice(t *testing.T) {
	up, srv := newSlowFakeMCP(t, 300*time.Millisecond, "a")
	repo := &fakeRepo{servers: []*models.MCPServer{{ID: "s1", Name: "s1", Transport: "http", URL: srv.URL, AuthType: "none"}}}
	r := NewDBRegistry(repo, func() time.Duration { return 5 * time.Second }, zap.NewNop())
	r.toolsTTL = 0
	_ = r.LoadFromDB(context.Background())
	for i := 0; i < 5; i++ { // reloads while the first initialize is still running
		_ = r.LoadFromDB(context.Background())
		time.Sleep(20 * time.Millisecond)
	}
	info, _ := r.Get("s1")
	waitFor(t, "tools", func() bool { return len(info.GetTools()) == 1 })
	if n := up.inits.Load(); n != 1 {
		t.Fatalf("initialize ran %d times", n)
	}
	if st, _ := info.GetStatus(); st != "connected" {
		t.Fatalf("status %s", st)
	}
}

// Bugbot: per-user bearer rules decide whether the handshake must run with the caller's key, so
// adding or removing them must give a new client.
func TestConnectionKeyCoversPerUserBearerRules(t *testing.T) {
	base := MCPServerConfig{URL: "https://x", Transport: "http", AuthType: "bearer"}
	withRules := base
	withRules.APIKeyRules = []models.MCPAPIKeyRule{{SubjectType: "group", Subject: "/google-workspace/dev@freepik.com", APIKey: "k"}}
	if connectionKey(base) == connectionKey(withRules) {
		t.Fatal("adding per-user bearer rules must change the connection key")
	}
}

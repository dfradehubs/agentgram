package webui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMount_SPAFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("INDEX"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("JS"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Get("/api/agents", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("AGENTS"))
	})
	Mount(r, dir)

	t.Run("api still works", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Body.String() != "AGENTS" {
			t.Fatalf("got %q", rec.Body.String())
		}
	})
	t.Run("static file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Body.String() != "JS" {
			t.Fatalf("got %q", rec.Body.String())
		}
	})
	t.Run("spa fallback", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/chat/abc", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Body.String() != "INDEX" {
			t.Fatalf("got %q", rec.Body.String())
		}
	})
}

func TestMount_EmptyDirNoop(t *testing.T) {
	r := chi.NewRouter()
	Mount(r, "")
	Mount(r, "/does/not/exist")
}

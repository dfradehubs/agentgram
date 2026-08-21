package webui

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Mount serves a static UI directory on unmatched GET routes (SPA fallback to index.html).
// API, auth, MCP, health and swagger routes must be registered first.
func Mount(r chi.Router, dir string) {
	if dir == "" {
		return
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return
	}
	fileServer := http.FileServer(http.Dir(dir))
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			http.NotFound(w, req)
			return
		}
		serveSPA(w, req, dir, fileServer)
	})
}

func serveSPA(w http.ResponseWriter, r *http.Request, dir string, fileServer http.Handler) {
	clean := path.Clean(r.URL.Path)
	if strings.HasPrefix(clean, "/api/") || strings.HasPrefix(clean, "/auth/") ||
		strings.HasPrefix(clean, "/mcp") || strings.HasPrefix(clean, "/health") ||
		strings.HasPrefix(clean, "/swagger") || strings.HasPrefix(clean, "/demo/") {
		http.NotFound(w, r)
		return
	}

	full := path.Join(dir, clean)
	if st, err := os.Stat(full); err == nil && !st.IsDir() {
		fileServer.ServeHTTP(w, r)
		return
	}
	index := path.Join(dir, "index.html")
	if _, err := os.Stat(index); err != nil {
		if err == fs.ErrNotExist || os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
	}
	http.ServeFile(w, r, index)
}

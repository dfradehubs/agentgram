package middleware

import (
	"net/http"
	"strconv"
	"strings"
)

// CORSConfig holds the configuration for CORS middleware.
type CORSConfig struct {
	AllowedOrigins []string
}

// CORS returns middleware that handles Cross-Origin Resource Sharing.
// If no origins are configured, no CORS headers are set (same-origin only).
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	allowedSet := make(map[string]struct{}, len(cfg.AllowedOrigins))
	allowAll := false
	for _, o := range cfg.AllowedOrigins {
		if o == "*" {
			allowAll = true
		}
		allowedSet[o] = struct{}{}
	}

	allowMethods := strings.Join([]string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
	}, ", ")
	allowHeaders := "Authorization, Content-Type, X-Request-ID"
	exposeHeaders := "X-Request-ID"
	maxAge := strconv.Itoa(3600)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				// Same-origin request, no CORS headers needed.
				next.ServeHTTP(w, r)
				return
			}

			// Check if origin is allowed.
			allowed := false
			if allowAll {
				allowed = true
			} else if _, ok := allowedSet[origin]; ok {
				allowed = true
			}

			if !allowed {
				// Origin not in allow list; skip CORS headers.
				next.ServeHTTP(w, r)
				return
			}

			// Set CORS headers. Use the actual origin (not "*") to support credentials.
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Expose-Headers", exposeHeaders)
			w.Header().Set("Vary", "Origin")

			// Handle preflight requests.
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", allowMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				w.Header().Set("Access-Control-Max-Age", maxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

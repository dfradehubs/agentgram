package middleware

import (
	"net/http"
)

// BodyLimit returns middleware that limits the size of the request body.
// maxBytes specifies the maximum number of bytes allowed (e.g., 1<<20 for 1MB).
func BodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

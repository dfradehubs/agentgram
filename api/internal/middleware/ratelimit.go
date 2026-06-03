package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/dfradehubs/agentgram-api/internal/metrics"
	"golang.org/x/time/rate"
)

// limiterEntry wraps a rate.Limiter with a last-used timestamp for cleanup.
type limiterEntry struct {
	limiter  *rate.Limiter
	lastUsed time.Time
}

// RateLimiter rate limiting middleware per agent
type RateLimiter struct {
	limiters map[string]map[string]*limiterEntry // agentID -> userID -> entry
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	r := rate.Limit(float64(requestsPerMinute) / 60.0)
	burst := requestsPerMinute / 10 // 10% of rate per minute as burst
	if burst < 1 {
		burst = 1
	}
	rl := &RateLimiter{
		limiters: make(map[string]map[string]*limiterEntry),
		rate:     r,
		burst:    burst,
	}
	go rl.cleanup()
	return rl
}

// cleanup periodically removes limiter entries unused for over 10 minutes.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-10 * time.Minute)
		for agentID, users := range rl.limiters {
			for userID, entry := range users {
				if entry.lastUsed.Before(cutoff) {
					delete(users, userID)
				}
			}
			if len(users) == 0 {
				delete(rl.limiters, agentID)
			}
		}
		rl.mu.Unlock()
	}
}

// getLimiter gets or creates a limiter for an agent/user
func (rl *RateLimiter) getLimiter(agentID, userID string) *rate.Limiter {
	now := time.Now()

	rl.mu.RLock()
	if agentLimiters, ok := rl.limiters[agentID]; ok {
		if entry, ok := agentLimiters[userID]; ok {
			entry.lastUsed = now
			rl.mu.RUnlock()
			return entry.limiter
		}
	}
	rl.mu.RUnlock()

	// Create new limiter
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if _, ok := rl.limiters[agentID]; !ok {
		rl.limiters[agentID] = make(map[string]*limiterEntry)
	}

	entry := &limiterEntry{
		limiter:  rate.NewLimiter(rl.rate, rl.burst),
		lastUsed: now,
	}
	rl.limiters[agentID][userID] = entry

	return entry.limiter
}

// Allow checks if a request is allowed
func (rl *RateLimiter) Allow(agentID, userID string) bool {
	limiter := rl.getLimiter(agentID, userID)
	return limiter.Allow()
}

// Handler returns a middleware that limits by agent
func (rl *RateLimiter) Handler(agentID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetUserFromContext(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			userID := claims.Sub
			if userID == "" {
				userID = claims.Email
			}

			if !rl.Allow(agentID, userID) {
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// IPHandler returns middleware that limits by remote IP address (for public endpoints).
func (rl *RateLimiter) IPHandler(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				ip = forwarded
			}

			if !rl.Allow(scope, ip) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "60")
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ChatHandler returns middleware that extracts agentId from chi URL params
// and applies per-agent per-user rate limiting.
func (rl *RateLimiter) ChatHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentID := chi.URLParam(r, "agentId")
		if agentID == "" {
			agentID = chi.URLParam(r, "id")
		}
		if agentID == "" {
			next.ServeHTTP(w, r)
			return
		}

		claims := GetUserFromContext(r.Context())
		if claims == nil {
			next.ServeHTTP(w, r)
			return
		}

		userID := claims.Sub
		if userID == "" {
			userID = claims.Email
		}

		if !rl.Allow(agentID, userID) {
			if metrics.IsEnabled() {
				metrics.RateLimitRejectedTotal.WithLabelValues(agentID).Inc()
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "60")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}


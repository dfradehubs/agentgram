package slack

import (
	"context"
	"fmt"
	"sync"
	"time"

	slackapi "github.com/slack-go/slack"
)

type cachedEmail struct {
	email  string
	expiry time.Time
}

// EmailResolver resolves Slack user IDs to email addresses with caching.
type EmailResolver struct {
	cache    map[string]cachedEmail
	mu       sync.Mutex
	ttl      time.Duration
	maxItems int
}

// NewEmailResolver creates a new EmailResolver with TTL-based caching.
func NewEmailResolver(ttl time.Duration, maxItems int) *EmailResolver {
	return &EmailResolver{
		cache:    make(map[string]cachedEmail),
		ttl:      ttl,
		maxItems: maxItems,
	}
}

// Resolve returns the email address for a Slack user ID, using cache when possible.
func (r *EmailResolver) Resolve(ctx context.Context, client *slackapi.Client, userID string) (string, error) {
	r.mu.Lock()
	if c, ok := r.cache[userID]; ok && time.Now().Before(c.expiry) {
		r.mu.Unlock()
		return c.email, nil
	}
	r.mu.Unlock()

	info, err := client.GetUserInfoContext(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("get user info for %s: %w", userID, err)
	}
	email := info.Profile.Email
	if email == "" {
		return "", fmt.Errorf("no email for Slack user %s (check users:read.email scope)", userID)
	}

	r.mu.Lock()
	// Evict oldest if at capacity
	if len(r.cache) >= r.maxItems {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range r.cache {
			if oldestKey == "" || v.expiry.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.expiry
			}
		}
		delete(r.cache, oldestKey)
	}
	r.cache[userID] = cachedEmail{email: email, expiry: time.Now().Add(r.ttl)}
	r.mu.Unlock()

	return email, nil
}

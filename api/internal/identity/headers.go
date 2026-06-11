// Package identity builds the outbound headers that tell downstream services
// (agents, MCP servers) which user agentgram is acting for.
package identity

import (
	"context"
	"net/http"
	"strings"

	"github.com/dfradehubs/agentgram-api/internal/middleware"
)

// Headers carrying the calling user's identity on every outbound request.
// These are plain, unsigned assertions: downstream services must trust them
// only when the caller is provably agentgram (private network and/or the
// bearer API key). For cryptographic proof use auth_type "forward", where the
// service receives the user's signed JWT and can verify it itself.
const (
	HeaderUserEmail  = "X-User-Email"
	HeaderUserGroups = "X-User-Groups"
)

// maxGroupsHeaderBytes caps X-User-Groups: Workspace group lists can be long
// and proxies commonly reject headers past ~8KB. Whole groups are dropped
// past the cap rather than truncating mid-value.
const maxGroupsHeaderBytes = 4096

// Headers returns the identity headers for the authenticated user in ctx
// (email and groups from validated JWT/session claims), or nil when there is
// no authenticated user (e.g. health checks, background refresh loops).
func Headers(ctx context.Context) map[string]string {
	claims := middleware.GetUserFromContext(ctx)
	if claims == nil {
		return nil
	}

	headers := make(map[string]string, 2)
	if email := sanitizeHeaderValue(claims.GetEmail()); email != "" {
		headers[HeaderUserEmail] = email
	}
	if groups := joinGroups(claims.Groups); groups != "" {
		headers[HeaderUserGroups] = groups
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

// SetHeaders adds the identity headers to an outbound request. No-op when
// there is no authenticated user in ctx.
func SetHeaders(ctx context.Context, req *http.Request) {
	for k, v := range Headers(ctx) {
		req.Header.Set(k, v)
	}
}

// Merge adds the identity headers into an existing non-nil header map and
// returns it. A nil map is returned unchanged so callers' "no credential
// resolved" nil-checks keep working.
func Merge(ctx context.Context, headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	for k, v := range Headers(ctx) {
		headers[k] = v
	}
	return headers
}

// joinGroups builds the comma-separated X-User-Groups value, skipping empty
// entries and stopping before the size cap.
func joinGroups(groups []string) string {
	var b strings.Builder
	for _, g := range groups {
		g = sanitizeHeaderValue(g)
		if g == "" {
			continue
		}
		extra := len(g)
		if b.Len() > 0 {
			extra++ // separator
		}
		if b.Len()+extra > maxGroupsHeaderBytes {
			break
		}
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(g)
	}
	return b.String()
}

// sanitizeHeaderValue strips control characters (CR/LF injection) and
// surrounding whitespace from a header value.
func sanitizeHeaderValue(v string) string {
	v = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, v)
	return strings.TrimSpace(v)
}

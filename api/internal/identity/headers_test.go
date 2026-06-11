package identity

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
)

func ctxWithClaims(claims *auth.Claims) context.Context {
	return context.WithValue(context.Background(), middleware.UserContextKey, claims)
}

func newTestRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "http://downstream.local/chat", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	return req
}

func TestSetHeaders(t *testing.T) {
	tests := []struct {
		name       string
		ctx        context.Context
		wantEmail  string
		wantGroups string
	}{
		{
			name:       "no claims in context sends nothing",
			ctx:        context.Background(),
			wantEmail:  "",
			wantGroups: "",
		},
		{
			name: "email and groups forwarded",
			ctx: ctxWithClaims(&auth.Claims{
				Email:  "user@example.com",
				Groups: []string{"devs@example.com", "sre@example.com"},
			}),
			wantEmail:  "user@example.com",
			wantGroups: "devs@example.com,sre@example.com",
		},
		{
			name: "preferred_username fallback when email empty",
			ctx: ctxWithClaims(&auth.Claims{
				PreferredUsername: "user@example.com",
			}),
			wantEmail:  "user@example.com",
			wantGroups: "",
		},
		{
			name: "control characters stripped (header injection)",
			ctx: ctxWithClaims(&auth.Claims{
				Email:  "user@example.com\r\nX-Injected: 1",
				Groups: []string{"devs\r\n@example.com"},
			}),
			wantEmail:  "user@example.comX-Injected: 1",
			wantGroups: "devs@example.com",
		},
		{
			name: "empty groups skipped",
			ctx: ctxWithClaims(&auth.Claims{
				Email:  "user@example.com",
				Groups: []string{"", "devs@example.com", "  "},
			}),
			wantEmail:  "user@example.com",
			wantGroups: "devs@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newTestRequest(t)

			SetHeaders(tt.ctx, req)

			if got := req.Header.Get(HeaderUserEmail); got != tt.wantEmail {
				t.Errorf("X-User-Email = %q, want %q", got, tt.wantEmail)
			}
			if got := req.Header.Get(HeaderUserGroups); got != tt.wantGroups {
				t.Errorf("X-User-Groups = %q, want %q", got, tt.wantGroups)
			}
		})
	}
}

func TestSetHeadersGroupsCap(t *testing.T) {
	// Build a group list that exceeds the cap; whole groups past the limit
	// must be dropped, never truncated mid-value.
	group := "group-" + strings.Repeat("x", 94) // 100 bytes each
	groups := make([]string, 60)                // ~6KB total > 4KB cap
	for i := range groups {
		groups[i] = group
	}

	req := newTestRequest(t)
	SetHeaders(ctxWithClaims(&auth.Claims{Email: "u@example.com", Groups: groups}), req)

	got := req.Header.Get(HeaderUserGroups)
	if len(got) > maxGroupsHeaderBytes {
		t.Errorf("X-User-Groups length = %d, want <= %d", len(got), maxGroupsHeaderBytes)
	}
	for _, g := range strings.Split(got, ",") {
		if g != group {
			t.Errorf("found truncated group entry %q", g)
		}
	}
}

func TestMerge(t *testing.T) {
	ctx := ctxWithClaims(&auth.Claims{Email: "user@example.com", Groups: []string{"devs@example.com"}})

	t.Run("nil map stays nil so credential checks keep working", func(t *testing.T) {
		if got := Merge(ctx, nil); got != nil {
			t.Errorf("Merge(ctx, nil) = %v, want nil", got)
		}
	})

	t.Run("identity added to credential map", func(t *testing.T) {
		headers := Merge(ctx, map[string]string{"Authorization": "Bearer key"})
		want := map[string]string{
			"Authorization":  "Bearer key",
			HeaderUserEmail:  "user@example.com",
			HeaderUserGroups: "devs@example.com",
		}
		for k, v := range want {
			if headers[k] != v {
				t.Errorf("headers[%q] = %q, want %q", k, headers[k], v)
			}
		}
	})

	t.Run("no claims leaves map untouched", func(t *testing.T) {
		headers := Merge(context.Background(), map[string]string{"Authorization": "Bearer key"})
		if len(headers) != 1 {
			t.Errorf("headers = %v, want only Authorization", headers)
		}
	})
}

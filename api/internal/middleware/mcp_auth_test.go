package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/config"
	"go.uber.org/zap"
)

func TestMatchStaticToken_NoTokensConfigured(t *testing.T) {
	m := NewMCPAuth(nil, zap.NewNop(), "agentgram.example.com", nil)
	if _, _, ok := m.matchStaticToken("anything"); ok {
		t.Fatalf("expected no match when no static tokens are configured")
	}
}

func TestMatchStaticToken_HappyPath(t *testing.T) {
	m := NewMCPAuth(nil, zap.NewNop(), "agentgram.example.com", []config.StaticToken{
		{
			Name:   "magec",
			Token:  "s3cr3t",
			Email:  "magec@example.com",
			Groups: []string{"/google-workspace/sysadmin@example.com"},
		},
	})

	claims, name, ok := m.matchStaticToken("s3cr3t")
	if !ok {
		t.Fatalf("expected match, got none")
	}
	if name != "magec" {
		t.Fatalf("expected name=magec, got %q", name)
	}
	if claims.Email != "magec@example.com" {
		t.Fatalf("expected email=magec@example.com, got %q", claims.Email)
	}
	if claims.Sub != "static:magec" {
		t.Fatalf("expected sub=static:magec, got %q", claims.Sub)
	}
	if len(claims.Groups) != 1 || claims.Groups[0] != "/google-workspace/sysadmin@example.com" {
		t.Fatalf("expected one sysadmin group, got %v", claims.Groups)
	}
}

func TestMatchStaticToken_DoesNotMatchUnknownToken(t *testing.T) {
	m := NewMCPAuth(nil, zap.NewNop(), "agentgram.example.com", []config.StaticToken{
		{Name: "magec", Token: "s3cr3t", Email: "magec@example.com"},
	})
	if _, _, ok := m.matchStaticToken("wrong"); ok {
		t.Fatalf("expected unknown token to not match")
	}
}

func TestMatchStaticToken_SkipsEmptyTokenEntries(t *testing.T) {
	m := NewMCPAuth(nil, zap.NewNop(), "agentgram.example.com", []config.StaticToken{
		{Name: "broken", Token: "", Email: "broken@example.com"},
		{Name: "magec", Token: "s3cr3t", Email: "magec@example.com"},
	})
	if _, _, ok := m.matchStaticToken(""); ok {
		t.Fatalf("empty token must never match (silent loop should not collapse to true)")
	}
	if _, name, ok := m.matchStaticToken("s3cr3t"); !ok || name != "magec" {
		t.Fatalf("expected magec to still match after skipping the empty entry; got ok=%v name=%q", ok, name)
	}
}

func TestMatchStaticToken_ClaimsGroupsAreCopiedFromCaller(t *testing.T) {
	groups := []string{"/google-workspace/sysadmin@example.com"}
	m := NewMCPAuth(nil, zap.NewNop(), "agentgram.example.com", []config.StaticToken{
		{Name: "magec", Token: "s3cr3t", Email: "magec@example.com", Groups: groups},
	})
	groups[0] = "tampered"

	claims, _, _ := m.matchStaticToken("s3cr3t")
	if claims.Groups[0] != "/google-workspace/sysadmin@example.com" {
		t.Fatalf("claims.Groups must not alias caller slice; got %v", claims.Groups)
	}
}

func TestMatchStaticToken_ReturnsFreshClaimsPerCall(t *testing.T) {
	m := NewMCPAuth(nil, zap.NewNop(), "agentgram.example.com", []config.StaticToken{
		{
			Name:   "magec",
			Token:  "s3cr3t",
			Email:  "magec@example.com",
			Groups: []string{"/google-workspace/sysadmin@example.com"},
		},
	})

	first, _, _ := m.matchStaticToken("s3cr3t")
	second, _, _ := m.matchStaticToken("s3cr3t")
	if first == second {
		t.Fatalf("matchStaticToken must return distinct *Claims per call; got the same pointer twice (concurrent requests would share state)")
	}
	if &first.Groups[0] == &second.Groups[0] {
		t.Fatalf("matchStaticToken must return a fresh Groups slice per call; got aliasing backing array")
	}

	// Simulate a downstream mutation on the first response and check the
	// second one stays clean.
	first.Groups[0] = "tampered"
	first.Email = "tampered@example.com"
	if second.Groups[0] != "/google-workspace/sysadmin@example.com" {
		t.Fatalf("downstream mutation on one Claims leaked into another: got %q", second.Groups[0])
	}
	if second.Email != "magec@example.com" {
		t.Fatalf("downstream mutation on one Claims.Email leaked: got %q", second.Email)
	}

	// Third call must still see the original configured values, proving the
	// stored entry was not corrupted by the earlier tamper.
	third, _, _ := m.matchStaticToken("s3cr3t")
	if third.Email != "magec@example.com" || third.Groups[0] != "/google-workspace/sysadmin@example.com" {
		t.Fatalf("stored entry was mutated through a returned Claims pointer: got %#v", third)
	}
}

func TestMatchStaticToken_WrongTokenSameLengthDoesNotMatch(t *testing.T) {
	// Regression coverage for the "constant-length comparison" property:
	// a wrong token that has exactly the same byte length as the configured
	// one must still be rejected. With raw-token comparison this test would
	// also pass; the value is that the SHA-256-based comparison cannot leak
	// the configured length through early-exit on length mismatch.
	m := NewMCPAuth(nil, zap.NewNop(), "agentgram.example.com", []config.StaticToken{
		{Name: "magec", Token: "abcdef", Email: "magec@example.com"},
	})
	if _, _, ok := m.matchStaticToken("abcdeg"); ok {
		t.Fatalf("token of identical length but different value must not match")
	}
	if _, _, ok := m.matchStaticToken("zzzzzz"); ok {
		t.Fatalf("token of identical length but different value must not match")
	}
	if _, _, ok := m.matchStaticToken("longer-than-configured"); ok {
		t.Fatalf("token of different length must not match")
	}
	if _, _, ok := m.matchStaticToken("abc"); ok {
		t.Fatalf("shorter token must not match")
	}
}

func TestHandler_StaticTokenBypassesJWTValidator(t *testing.T) {
	m := NewMCPAuth(nil, zap.NewNop(), "agentgram.example.com", []config.StaticToken{
		{
			Name:   "magec",
			Token:  "s3cr3t",
			Email:  "magec@example.com",
			Groups: []string{"/google-workspace/sysadmin@example.com"},
		},
	})

	called := false
	var seen *auth.Claims
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if c, ok := r.Context().Value(UserContextKey).(*auth.Claims); ok {
			seen = c
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer s3cr3t")
	rec := httptest.NewRecorder()

	m.Handler(next).ServeHTTP(rec, req)

	if !called {
		t.Fatalf("expected next handler to be called when static token matches")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if seen == nil || seen.Email != "magec@example.com" {
		t.Fatalf("expected synthetic claims for magec in context, got %#v", seen)
	}
}

func TestHandler_NoAuthorizationHeaderReturns401(t *testing.T) {
	m := NewMCPAuth(nil, zap.NewNop(), "agentgram.example.com", []config.StaticToken{
		{Name: "magec", Token: "s3cr3t", Email: "magec@example.com"},
	})

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next handler must not run when there's no token")
	})

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	rec := httptest.NewRecorder()

	m.Handler(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("expected WWW-Authenticate header to be set on 401")
	}
}

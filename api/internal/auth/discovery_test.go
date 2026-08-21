package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscover_UsesWellKnown(t *testing.T) {
	t.Cleanup(ResetDiscoveryCache)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 server.URL,
			"authorization_endpoint": server.URL + "/oauth/v2/authorize",
			"token_endpoint":         server.URL + "/oauth/v2/token",
			"jwks_uri":               server.URL + "/oauth/v2/keys",
			"revocation_endpoint":    server.URL + "/oauth/v2/revoke",
			"end_session_endpoint":   server.URL + "/oauth/v2/logout",
		})
	}))
	defer server.Close()

	meta, err := Discover(t.Context(), server.URL)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if meta.AuthorizationEndpoint != server.URL+"/oauth/v2/authorize" {
		t.Fatalf("auth endpoint = %q", meta.AuthorizationEndpoint)
	}
	if meta.TokenEndpoint != server.URL+"/oauth/v2/token" {
		t.Fatalf("token endpoint = %q", meta.TokenEndpoint)
	}
	if meta.JWKSURI != server.URL+"/oauth/v2/keys" {
		t.Fatalf("jwks = %q", meta.JWKSURI)
	}
}

func TestDiscover_FallsBackToKeycloakPaths(t *testing.T) {
	t.Cleanup(ResetDiscoveryCache)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer server.Close()

	meta, err := Discover(t.Context(), server.URL)
	if err == nil {
		t.Fatal("expected discovery error")
	}
	if meta.AuthorizationEndpoint != server.URL+"/protocol/openid-connect/auth" {
		t.Fatalf("fallback auth = %q", meta.AuthorizationEndpoint)
	}
	if meta.JWKSURI != server.URL+"/protocol/openid-connect/certs" {
		t.Fatalf("fallback jwks = %q", meta.JWKSURI)
	}
}

func TestDiscover_EmptyIssuer(t *testing.T) {
	_, err := Discover(t.Context(), "")
	if err == nil {
		t.Fatal("expected error")
	}
}

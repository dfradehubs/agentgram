package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/config"
)

// TestSupportedScopes_AppendsExtraScopes guards the audience fix: deployments
// add a Keycloak client scope carrying an audience mapper (e.g. "mcp:custom-audience")
// via mcp_server.extra_scopes, and it MUST be advertised on top of the required
// base set so strict clients like Claude request it. Without the advertised
// scope the upstream agent (ADK) rejects the forwarded token with 401.
func TestSupportedScopes_AppendsExtraScopes(t *testing.T) {
	h := &Handler{cfg: &config.Config{}}
	h.cfg.MCPServer.ExtraScopes = []string{"mcp:custom-audience", "email"} // "email" is a dup of a base scope

	got := h.supportedScopes()

	// Base scopes are always present.
	for _, base := range mcpBaseScopes {
		if !contains(got, base) {
			t.Errorf("supportedScopes() = %v, missing required base scope %q", got, base)
		}
	}
	// The extra scope is advertised.
	if !contains(got, "mcp:custom-audience") {
		t.Errorf("supportedScopes() = %v, missing extra scope %q", got, "mcp:custom-audience")
	}
	// Duplicates are dropped: "email" must appear exactly once.
	if n := count(got, "email"); n != 1 {
		t.Errorf("scope %q appears %d times, want 1 (duplicates must be dropped)", "email", n)
	}
}

// TestSupportedScopes_NilConfig guards against a nil cfg (used by lightweight
// handler tests) panicking on the extra-scopes lookup.
func TestSupportedScopes_NilConfig(t *testing.T) {
	h := &Handler{}
	if got := h.supportedScopes(); len(got) != len(mcpBaseScopes) {
		t.Errorf("supportedScopes() with nil cfg = %v, want the base set %v", got, mcpBaseScopes)
	}
}

func contains(xs []string, x string) bool {
	return count(xs, x) > 0
}

func count(xs []string, x string) int {
	n := 0
	for _, v := range xs {
		if v == x {
			n++
		}
	}
	return n
}

// TestHandleResourceMetadata_AdvertisesSelfAsAuthorizationServer guards the DCR
// fix: oauth-protected-resource MUST advertise this server's own host as the
// authorization server, never the Keycloak issuer. Pointing clients at Keycloak
// makes them perform Dynamic Client Registration against Keycloak's
// clients-registrations endpoint (Istio RBACs it to 403), breaking auth for
// MCP clients like Claude Code. Clients must instead discover our AS metadata
// facade, whose registration_endpoint is our static /register handler.
func TestHandleResourceMetadata_AdvertisesSelfAsAuthorizationServer(t *testing.T) {
	const host = "agentgram.example.com"
	wantResource := "https://" + host

	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "https://"+host+"/.well-known/oauth-protected-resource", nil)
	req.Host = host
	rec := httptest.NewRecorder()

	h.HandleResourceMetadata(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
		ScopesSupported      []string `json:"scopes_supported"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if got.Resource != wantResource {
		t.Errorf("resource = %q, want %q", got.Resource, wantResource)
	}

	if len(got.AuthorizationServers) != 1 || got.AuthorizationServers[0] != wantResource {
		t.Errorf("authorization_servers = %v, want [%q] (must be self, not the Keycloak issuer)",
			got.AuthorizationServers, wantResource)
	}

	if len(got.ScopesSupported) == 0 {
		t.Error("scopes_supported is empty, want the advertised MCP scope set")
	}
}

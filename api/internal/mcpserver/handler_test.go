package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

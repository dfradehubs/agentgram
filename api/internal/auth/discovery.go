package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ProviderMetadata is a subset of OpenID Provider Metadata (OIDC Discovery 1.0).
type ProviderMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	RevocationEndpoint    string `json:"revocation_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
}

// Discover fetches {issuer}/.well-known/openid-configuration.
// On failure it returns Keycloak-compatible path fallbacks so existing
// deployments keep working without a reachable discovery document.
func Discover(ctx context.Context, issuer string) (*ProviderMetadata, error) {
	issuer = strings.TrimRight(issuer, "/")
	if issuer == "" {
		return nil, fmt.Errorf("oidc issuer is empty")
	}
	fallback := keycloakFallback(issuer)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return fallback, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fallback, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fallback, fmt.Errorf("oidc discovery returned %d", resp.StatusCode)
	}

	var meta ProviderMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return fallback, err
	}
	if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" {
		return fallback, fmt.Errorf("oidc discovery missing authorization_endpoint or token_endpoint")
	}
	if meta.Issuer == "" {
		meta.Issuer = issuer
	}
	if meta.JWKSURI == "" {
		meta.JWKSURI = fallback.JWKSURI
	}
	if meta.RevocationEndpoint == "" {
		meta.RevocationEndpoint = fallback.RevocationEndpoint
	}
	if meta.EndSessionEndpoint == "" {
		meta.EndSessionEndpoint = fallback.EndSessionEndpoint
	}
	return &meta, nil
}

func keycloakFallback(issuer string) *ProviderMetadata {
	return &ProviderMetadata{
		Issuer:                issuer,
		AuthorizationEndpoint: issuer + "/protocol/openid-connect/auth",
		TokenEndpoint:         issuer + "/protocol/openid-connect/token",
		JWKSURI:               issuer + "/protocol/openid-connect/certs",
		RevocationEndpoint:    issuer + "/protocol/openid-connect/revoke",
		EndSessionEndpoint:    issuer + "/protocol/openid-connect/logout",
	}
}

// metadataCache holds discovered endpoints per issuer for the process lifetime.
type metadataCache struct {
	mu    sync.Mutex
	byIss map[string]*ProviderMetadata
}

func (c *metadataCache) get(issuer string) *ProviderMetadata {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byIss == nil {
		return nil
	}
	return c.byIss[issuer]
}

func (c *metadataCache) set(issuer string, meta *ProviderMetadata) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byIss == nil {
		c.byIss = map[string]*ProviderMetadata{}
	}
	c.byIss[issuer] = meta
}

var discovered = &metadataCache{}

// ResolveMetadata returns cached discovery or fetches it (with Keycloak fallback).
func ResolveMetadata(ctx context.Context, issuer string) *ProviderMetadata {
	issuer = strings.TrimRight(issuer, "/")
	if meta := discovered.get(issuer); meta != nil {
		return meta
	}
	meta, _ := Discover(ctx, issuer)
	if meta != nil {
		discovered.set(issuer, meta)
	}
	return meta
}

// ResetDiscoveryCache is for tests.
func ResetDiscoveryCache() {
	discovered.mu.Lock()
	discovered.byIss = nil
	discovered.mu.Unlock()
}

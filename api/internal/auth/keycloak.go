package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
)

// KeycloakProvider manages JWKS fetching and caching from Keycloak
type KeycloakProvider struct {
	issuer   string
	cacheTTL time.Duration

	jwksCache jwk.Set
	cacheMu   sync.RWMutex
	cacheTime time.Time
}

// NewKeycloakProvider creates a new Keycloak provider
func NewKeycloakProvider(issuer string, cacheTTL time.Duration) *KeycloakProvider {
	return &KeycloakProvider{
		issuer:   issuer,
		cacheTTL: cacheTTL,
	}
}

// GetJWKS retrieves the JWKS (with caching)
func (k *KeycloakProvider) GetJWKS(ctx context.Context) (jwk.Set, error) {
	k.cacheMu.RLock()
	if k.jwksCache != nil && time.Since(k.cacheTime) < k.cacheTTL {
		defer k.cacheMu.RUnlock()
		return k.jwksCache, nil
	}
	k.cacheMu.RUnlock()

	// Need to refresh the cache
	return k.refreshJWKS(ctx)
}

// refreshJWKS refreshes the JWKS cache
func (k *KeycloakProvider) refreshJWKS(ctx context.Context) (jwk.Set, error) {
	k.cacheMu.Lock()
	defer k.cacheMu.Unlock()

	// Double-check after acquiring the lock
	if k.jwksCache != nil && time.Since(k.cacheTime) < k.cacheTTL {
		return k.jwksCache, nil
	}

	jwksURL := k.getJWKSURL()
	set, err := jwk.Fetch(ctx, jwksURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS from %s: %w", jwksURL, err)
	}

	k.jwksCache = set
	k.cacheTime = time.Now()

	return set, nil
}

// getJWKSURL builds the JWKS endpoint URL
func (k *KeycloakProvider) getJWKSURL() string {
	// Keycloak standard JWKS endpoint
	return fmt.Sprintf("%s/protocol/openid-connect/certs", k.issuer)
}

// GetIssuer returns the configured issuer
func (k *KeycloakProvider) GetIssuer() string {
	return k.issuer
}

// InvalidateCache invalidates the JWKS cache
func (k *KeycloakProvider) InvalidateCache() {
	k.cacheMu.Lock()
	defer k.cacheMu.Unlock()
	k.jwksCache = nil
	k.cacheTime = time.Time{}
}

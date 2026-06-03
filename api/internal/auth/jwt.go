package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/lestrrat-go/jwx/v2/jwt"
)

// JWTValidator validates JWT tokens
type JWTValidator struct {
	keycloak *KeycloakProvider
	clientID string
}

// NewJWTValidator creates a new JWT validator with audience validation
func NewJWTValidator(keycloak *KeycloakProvider, clientID string) *JWTValidator {
	return &JWTValidator{
		keycloak: keycloak,
		clientID: clientID,
	}
}

// ValidateToken validates a JWT token and extracts the claims
func (v *JWTValidator) ValidateToken(ctx context.Context, tokenString string) (*Claims, error) {
	// Get JWKS
	jwks, err := v.keycloak.GetJWKS(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get JWKS: %w", err)
	}

	// Parse and validate token (issuer + audience)
	opts := []jwt.ParseOption{
		jwt.WithKeySet(jwks),
		jwt.WithValidate(true),
		jwt.WithIssuer(v.keycloak.GetIssuer()),
	}
	if v.clientID != "" {
		opts = append(opts, jwt.WithAudience(v.clientID))
	}
	token, err := jwt.Parse(
		[]byte(tokenString),
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	// Extract claims
	claims := &Claims{
		Sub:    token.Subject(),
		Issuer: token.Issuer(),
	}

	// Extract email
	if email, ok := token.Get("email"); ok {
		if s, ok := email.(string); ok {
			claims.Email = s
		}
	}

	// Extract name (full name from OIDC profile)
	if name, ok := token.Get("name"); ok {
		if s, ok := name.(string); ok {
			claims.Name = s
		}
	}

	// Extract preferred_username
	if username, ok := token.Get("preferred_username"); ok {
		if s, ok := username.(string); ok {
			claims.PreferredUsername = s
		}
	}

	// Extract groups (can be in different locations depending on Keycloak config)
	claims.Groups = v.extractGroups(token)

	return claims, nil
}

// extractGroups extracts groups from the token
func (v *JWTValidator) extractGroups(token jwt.Token) []string {
	var groups []string

	// Try to get from "groups" directly
	if g, ok := token.Get("groups"); ok {
		groups = append(groups, toStringSlice(g)...)
	}

	// Try to get from realm_access.roles
	if ra, ok := token.Get("realm_access"); ok {
		if raMap, ok := ra.(map[string]interface{}); ok {
			if roles, ok := raMap["roles"]; ok {
				groups = append(groups, toStringSlice(roles)...)
			}
		}
	}

	// Try to get from resource_access.<client>.roles
	if resAccess, ok := token.Get("resource_access"); ok {
		if raMap, ok := resAccess.(map[string]interface{}); ok {
			for _, clientAccess := range raMap {
				if caMap, ok := clientAccess.(map[string]interface{}); ok {
					if roles, ok := caMap["roles"]; ok {
						groups = append(groups, toStringSlice(roles)...)
					}
				}
			}
		}
	}

	return groups
}

// toStringSlice converts an interface{} to []string
func toStringSlice(v interface{}) []string {
	var result []string

	switch val := v.(type) {
	case []interface{}:
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
	case []string:
		result = val
	}

	return result
}

// ExtractBearerToken extracts the token from the Authorization header
func ExtractBearerToken(authHeader string) (string, error) {
	if authHeader == "" {
		return "", fmt.Errorf("missing authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", fmt.Errorf("invalid authorization header format")
	}

	return parts[1], nil
}

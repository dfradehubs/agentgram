package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/config"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// TokenResponse represents the OIDC token endpoint response
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// OIDCClient handles communication with Keycloak OIDC endpoints
type OIDCClient struct {
	issuer       string
	clientID     string
	clientSecret string
	redirectURI  string
	postLogoutURI string
	httpClient   *http.Client
	keycloak     *KeycloakProvider
}

// NewOIDCClient creates a new OIDC client from config
func NewOIDCClient(keycloakCfg config.KeycloakConfig, keycloak *KeycloakProvider) *OIDCClient {
	return &OIDCClient{
		issuer:        keycloakCfg.Issuer,
		clientID:      keycloakCfg.ClientID,
		clientSecret:  keycloakCfg.ClientSecret,
		redirectURI:   keycloakCfg.RedirectURI,
		postLogoutURI: keycloakCfg.PostLogoutURI,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		keycloak:      keycloak,
	}
}

// ClientID returns the OIDC client ID.
func (c *OIDCClient) ClientID() string { return c.clientID }

// Issuer returns the OIDC issuer URL.
func (c *OIDCClient) Issuer() string { return c.issuer }

// ExchangeCodeWithRedirect exchanges an authorization code for tokens using a custom redirect URI.
func (c *OIDCClient) ExchangeCodeWithRedirect(ctx context.Context, code, redirectURI string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}
	return c.tokenRequest(ctx, data)
}

// GetAuthorizationURL builds the Keycloak authorization URL
func (c *OIDCClient) GetAuthorizationURL(state, nonce string) string {
	params := url.Values{
		"response_type": {"code"},
		"client_id":     {c.clientID},
		"redirect_uri":  {c.redirectURI},
		"scope":         {"openid profile email groups"},
		"state":         {state},
		"nonce":         {nonce},
	}
	return fmt.Sprintf("%s/protocol/openid-connect/auth?%s", c.issuer, params.Encode())
}

// ExchangeCode exchanges an authorization code for tokens
func (c *OIDCClient) ExchangeCode(ctx context.Context, code string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {c.redirectURI},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}
	return c.tokenRequest(ctx, data)
}

// RefreshToken refreshes an access token using a refresh token
func (c *OIDCClient) RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}
	return c.tokenRequest(ctx, data)
}

// RevokeToken revokes a token at Keycloak (best-effort)
func (c *OIDCClient) RevokeToken(ctx context.Context, token string) error {
	data := url.Values{
		"token":         {token},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}
	endpoint := fmt.Sprintf("%s/protocol/openid-connect/revoke", c.issuer)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("revoke request failed: %w", err)
	}
	defer resp.Body.Close()

	// Best-effort: don't fail if revocation endpoint returns error
	return nil
}

// GetLogoutURL returns the Keycloak logout URL
func (c *OIDCClient) GetLogoutURL(idTokenHint string) string {
	params := url.Values{
		"client_id":                {c.clientID},
		"post_logout_redirect_uri": {c.postLogoutURI},
	}
	if idTokenHint != "" {
		params.Set("id_token_hint", idTokenHint)
	}
	return fmt.Sprintf("%s/protocol/openid-connect/logout?%s", c.issuer, params.Encode())
}

// ParseIDTokenClaims parses and validates the ID token, extracting claims.
// The nonce parameter is validated against the token's nonce claim to prevent
// token substitution attacks (OpenID Connect Core 3.1.3.7).
func (c *OIDCClient) ParseIDTokenClaims(ctx context.Context, idToken string, expectedNonce string) (*Claims, error) {
	jwks, err := c.keycloak.GetJWKS(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get JWKS: %w", err)
	}

	token, err := jwt.Parse(
		[]byte(idToken),
		jwt.WithKeySet(jwks),
		jwt.WithValidate(true),
		jwt.WithIssuer(c.issuer),
		jwt.WithAudience(c.clientID),
	)
	if err != nil {
		return nil, fmt.Errorf("invalid ID token: %w", err)
	}

	// Validate nonce claim against the expected value from the OIDC state
	if expectedNonce != "" {
		tokenNonce, ok := token.Get("nonce")
		if !ok {
			return nil, fmt.Errorf("ID token missing nonce claim")
		}
		if nonceStr, ok := tokenNonce.(string); !ok || nonceStr != expectedNonce {
			return nil, fmt.Errorf("ID token nonce mismatch")
		}
	}

	claims := &Claims{
		Sub:    token.Subject(),
		Issuer: token.Issuer(),
	}

	if email, ok := token.Get("email"); ok {
		if s, ok := email.(string); ok {
			claims.Email = s
		}
	}
	if name, ok := token.Get("name"); ok {
		if s, ok := name.(string); ok {
			claims.Name = s
		}
	}
	if username, ok := token.Get("preferred_username"); ok {
		if s, ok := username.(string); ok {
			claims.PreferredUsername = s
		}
	}

	// Extract groups using the same logic as JWT validator
	validator := &JWTValidator{}
	claims.Groups = validator.extractGroups(token)

	return claims, nil
}

// GetUserGroups queries Keycloak Admin API to get a user's groups by email.
// Uses client credentials grant to obtain a service account token.
func (c *OIDCClient) GetUserGroups(ctx context.Context, email string) ([]string, error) {
	// Get service account token
	tokenData := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}
	tokenResp, err := c.tokenRequest(ctx, tokenData)
	if err != nil {
		return nil, fmt.Errorf("failed to get service account token: %w", err)
	}

	// Derive admin API base from issuer (e.g. https://kc.example.com/realms/master → https://kc.example.com/admin/realms/master)
	adminBase := strings.Replace(c.issuer, "/realms/", "/admin/realms/", 1)

	// Look up user by email
	lookupURL := fmt.Sprintf("%s/users?email=%s&exact=true", adminBase, url.QueryEscape(email))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, lookupURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user lookup request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("user lookup request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("user lookup returned %d: %s", resp.StatusCode, string(body))
	}

	var users []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, fmt.Errorf("failed to parse user lookup response: %w", err)
	}
	if len(users) == 0 {
		return nil, nil
	}

	// Get user groups
	groupsURL := fmt.Sprintf("%s/users/%s/groups", adminBase, users[0].ID)
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, groupsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create groups request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)

	resp2, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("groups request failed: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		return nil, fmt.Errorf("groups endpoint returned %d: %s", resp2.StatusCode, string(body))
	}

	var groups []struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&groups); err != nil {
		return nil, fmt.Errorf("failed to parse groups response: %w", err)
	}

	result := make([]string, len(groups))
	for i, g := range groups {
		result[i] = g.Path
	}
	return result, nil
}

// GetServiceToken obtains a service account token using client_credentials grant.
// The token has azp=agentgram (this client's ID), which agents accept.
func (c *OIDCClient) GetServiceToken(ctx context.Context) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}
	return c.tokenRequest(ctx, data)
}


func (c *OIDCClient) tokenRequest(ctx context.Context, data url.Values) (*TokenResponse, error) {
	endpoint := fmt.Sprintf("%s/protocol/openid-connect/token", c.issuer)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &tokenResp, nil
}

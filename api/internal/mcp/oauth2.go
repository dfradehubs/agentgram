package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/security"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// AuthServerMetadata holds discovered OAuth2 authorization server metadata (RFC 8414).
type AuthServerMetadata struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	RegistrationEndpoint  string   `json:"registration_endpoint,omitempty"`
	ScopesSupported       []string `json:"scopes_supported,omitempty"`
	ResponseTypesSupported []string `json:"response_types_supported,omitempty"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported,omitempty"`
}

// OAuth2Manager handles the full OAuth2.1 lifecycle for MCP servers:
// metadata discovery, authorization URL generation with PKCE, code exchange,
// token refresh, scope resolution per user group, and token injection.
type OAuth2Manager struct {
	tokenStore    *store.MCPTokenStore
	rdb           *redis.Client
	httpClient    *http.Client
	logger        *zap.Logger
	callbackURL   string
	metadataCache sync.Map
}

// NewOAuth2Manager creates a new OAuth2 manager.
// callbackURL is the base redirect URI (e.g. "https://agentgram.example.com/auth/mcp-oauth/callback").
func NewOAuth2Manager(tokenStore *store.MCPTokenStore, rdb *redis.Client, callbackURL string, logger *zap.Logger) *OAuth2Manager {
	return &OAuth2Manager{
		tokenStore:  tokenStore,
		rdb:         rdb,
		httpClient:  &http.Client{Timeout: 10 * time.Second, Transport: security.NewSafeTransport()},
		logger:      logger,
		callbackURL: callbackURL,
	}
}

// DiscoverMetadata fetches and caches the OAuth2 authorization server metadata.
// Implements the full fallback chain per RFC 8414 Section 3.1 and OIDC Discovery:
//  1. Path-based RFC 8414: {origin}/.well-known/oauth-authorization-server{path}
//  2. Root RFC 8414:        {origin}/.well-known/oauth-authorization-server
//  3. Path-appended OIDC:   {authServerURL}/.well-known/openid-configuration
//  4. Path-based OIDC:      {origin}/.well-known/openid-configuration{path}
func (m *OAuth2Manager) DiscoverMetadata(ctx context.Context, authServerURL string) (*AuthServerMetadata, error) {
	if cached, ok := m.metadataCache.Load(authServerURL); ok {
		return cached.(*AuthServerMetadata), nil
	}

	parsed, err := url.Parse(strings.TrimRight(authServerURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse auth server URL: %w", err)
	}

	origin := parsed.Scheme + "://" + parsed.Host
	pathComponent := strings.TrimRight(parsed.Path, "/")

	var candidates []string
	if pathComponent != "" {
		candidates = append(candidates, origin+"/.well-known/oauth-authorization-server"+pathComponent)
	}
	candidates = append(candidates, origin+"/.well-known/oauth-authorization-server")
	if pathComponent != "" {
		candidates = append(candidates, authServerURL+"/.well-known/openid-configuration")
		candidates = append(candidates, origin+"/.well-known/openid-configuration"+pathComponent)
	} else {
		candidates = append(candidates, origin+"/.well-known/openid-configuration")
	}

	var lastErr error
	for _, candidateURL := range candidates {
		metadata, fetchErr := m.fetchMetadata(ctx, candidateURL)
		if fetchErr != nil {
			lastErr = fetchErr
			m.logger.Debug("metadata discovery attempt failed",
				zap.String("url", candidateURL),
				zap.Error(fetchErr))
			continue
		}

		if metadata.AuthorizationEndpoint == "" {
			baseURL := strings.TrimRight(authServerURL, "/")
			metadata.AuthorizationEndpoint = baseURL + "/authorize"
		}
		if metadata.TokenEndpoint == "" {
			baseURL := strings.TrimRight(authServerURL, "/")
			metadata.TokenEndpoint = baseURL + "/token"
		}

		m.metadataCache.Store(authServerURL, metadata)
		m.logger.Debug("OAuth2 metadata discovered",
			zap.String("auth_server", authServerURL),
			zap.String("discovered_from", candidateURL),
			zap.String("token_endpoint", metadata.TokenEndpoint))

		return metadata, nil
	}

	return nil, fmt.Errorf("all metadata discovery attempts failed for %s: %w", authServerURL, lastErr)
}

// fetchMetadata fetches and parses a single metadata URL.
func (m *OAuth2Manager) fetchMetadata(ctx context.Context, metadataURL string) (*AuthServerMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("MCP-Protocol-Version", "2025-03-26")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", metadataURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, metadataURL, string(body))
	}

	var metadata AuthServerMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("parse response from %s: %w", metadataURL, err)
	}

	return &metadata, nil
}

// pkceChallenge generates a PKCE code verifier and challenge (S256).
type pkceChallenge struct {
	Verifier  string
	Challenge string
	Method    string
}

func generatePKCE() (*pkceChallenge, error) {
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, fmt.Errorf("generate verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	return &pkceChallenge{
		Verifier:  verifier,
		Challenge: challenge,
		Method:    "S256",
	}, nil
}

// OAuthState holds the PKCE state stored in Redis during the authorization flow.
type OAuthState struct {
	MCPServerID string `json:"mcp_server_id"`
	UserEmail   string `json:"user_email"`
	Verifier    string `json:"verifier"`
	Scopes      string `json:"scopes"`
	ReturnURL   string `json:"return_url"`
}

// GetAuthorizationURL builds the OAuth2 authorization URL with PKCE for a user+server.
// Returns (authURL, state, error). The state is stored in Redis for validation on callback.
func (m *OAuth2Manager) GetAuthorizationURL(ctx context.Context, server *models.MCPServer, userEmail string, scopes string, returnURL string) (string, error) {
	metadata, err := m.DiscoverMetadata(ctx, server.OAuth2AuthServerURL)
	if err != nil {
		return "", fmt.Errorf("discover metadata: %w", err)
	}

	pkce, err := generatePKCE()
	if err != nil {
		return "", err
	}

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	oauthState := OAuthState{
		MCPServerID: server.ID,
		UserEmail:   userEmail,
		Verifier:    pkce.Verifier,
		Scopes:      scopes,
		ReturnURL:   returnURL,
	}
	stateJSON, _ := json.Marshal(oauthState)
	stateKey := fmt.Sprintf("mcp_oauth_state:%s", state)
	if err := m.rdb.Set(ctx, stateKey, stateJSON, 10*time.Minute).Err(); err != nil {
		return "", fmt.Errorf("save state: %w", err)
	}

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {server.OAuth2ClientID},
		"redirect_uri":          {m.callbackURL},
		"state":                 {state},
		"code_challenge":        {pkce.Challenge},
		"code_challenge_method": {pkce.Method},
	}
	if scopes != "" {
		params.Set("scope", scopes)
	}

	authURL := metadata.AuthorizationEndpoint + "?" + params.Encode()
	return authURL, nil
}

// PeekStateServerID reads the MCP server ID from a stored OAuth state without consuming it.
func (m *OAuth2Manager) PeekStateServerID(ctx context.Context, state string) (string, error) {
	stateKey := fmt.Sprintf("mcp_oauth_state:%s", state)
	stateJSON, err := m.rdb.Get(ctx, stateKey).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("invalid or expired state")
	}
	if err != nil {
		return "", fmt.Errorf("get state: %w", err)
	}
	var oauthState OAuthState
	if err := json.Unmarshal([]byte(stateJSON), &oauthState); err != nil {
		return "", fmt.Errorf("parse state: %w", err)
	}
	return oauthState.MCPServerID, nil
}

// ExchangeCode exchanges an authorization code for tokens using PKCE.
func (m *OAuth2Manager) ExchangeCode(ctx context.Context, server *models.MCPServer, code, state string) (*models.MCPOAuth2Token, string, error) {
	stateKey := fmt.Sprintf("mcp_oauth_state:%s", state)
	stateJSON, err := m.rdb.Get(ctx, stateKey).Result()
	if err == redis.Nil {
		return nil, "", fmt.Errorf("invalid or expired state")
	}
	if err != nil {
		return nil, "", fmt.Errorf("get state: %w", err)
	}
	m.rdb.Del(ctx, stateKey)

	var oauthState OAuthState
	if err := json.Unmarshal([]byte(stateJSON), &oauthState); err != nil {
		return nil, "", fmt.Errorf("parse state: %w", err)
	}

	if oauthState.MCPServerID != server.ID {
		return nil, "", fmt.Errorf("state server mismatch")
	}

	metadata, err := m.DiscoverMetadata(ctx, server.OAuth2AuthServerURL)
	if err != nil {
		return nil, "", fmt.Errorf("discover metadata for exchange: %w", err)
	}

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {m.callbackURL},
		"client_id":     {server.OAuth2ClientID},
		"code_verifier": {oauthState.Verifier},
	}
	if server.OAuth2ClientSecret != "" {
		data.Set("client_secret", server.OAuth2ClientSecret)
	}

	token, err := m.tokenRequest(ctx, metadata.TokenEndpoint, data)
	if err != nil {
		return nil, "", err
	}
	token.Scopes = oauthState.Scopes

	if err := m.tokenStore.Save(ctx, oauthState.UserEmail, server.ID, token); err != nil {
		return nil, "", fmt.Errorf("save token: %w", err)
	}

	m.logger.Info("MCP OAuth2 token obtained",
		zap.String("server_id", server.ID),
		zap.String("user_email", oauthState.UserEmail),
		zap.String("scopes", oauthState.Scopes))

	return token, oauthState.ReturnURL, nil
}

// ExchangeClientCredentials obtains a service-account token using client_credentials grant.
func (m *OAuth2Manager) ExchangeClientCredentials(ctx context.Context, server *models.MCPServer, scopes string) (*models.MCPOAuth2Token, error) {
	metadata, err := m.DiscoverMetadata(ctx, server.OAuth2AuthServerURL)
	if err != nil {
		return nil, fmt.Errorf("discover metadata: %w", err)
	}

	data := url.Values{
		"grant_type": {"client_credentials"},
		"client_id":  {server.OAuth2ClientID},
	}
	if server.OAuth2ClientSecret != "" {
		data.Set("client_secret", server.OAuth2ClientSecret)
	}
	if scopes != "" {
		data.Set("scope", scopes)
	}

	token, err := m.tokenRequest(ctx, metadata.TokenEndpoint, data)
	if err != nil {
		return nil, err
	}
	token.Scopes = scopes

	serviceEmail := "__service__"
	if err := m.tokenStore.Save(ctx, serviceEmail, server.ID, token); err != nil {
		return nil, fmt.Errorf("save service token: %w", err)
	}

	m.logger.Info("MCP OAuth2 service token obtained",
		zap.String("server_id", server.ID),
		zap.String("scopes", scopes))

	return token, nil
}

// GetToken retrieves a valid token for a user+server, refreshing if necessary.
// Returns nil, nil if no token exists (user needs to authorize).
func (m *OAuth2Manager) GetToken(ctx context.Context, server *models.MCPServer, userEmail string) (*models.MCPOAuth2Token, error) {
	token, err := m.tokenStore.Get(ctx, userEmail, server.ID)
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, nil
	}

	if !store.IsTokenExpired(token) {
		return token, nil
	}

	if token.RefreshToken == "" {
		m.tokenStore.Delete(ctx, userEmail, server.ID)
		return nil, nil
	}

	refreshed, err := m.refreshToken(ctx, server, token.RefreshToken)
	if err != nil {
		m.logger.Warn("MCP OAuth2 refresh failed, clearing token",
			zap.String("server_id", server.ID),
			zap.String("user_email", userEmail),
			zap.Error(err))
		m.tokenStore.Delete(ctx, userEmail, server.ID)
		return nil, nil
	}

	refreshed.Scopes = token.Scopes
	if err := m.tokenStore.Save(ctx, userEmail, server.ID, refreshed); err != nil {
		m.logger.Error("failed to save refreshed token", zap.Error(err))
	}

	return refreshed, nil
}

// GetServiceToken retrieves the service account token, refreshing or creating if needed.
func (m *OAuth2Manager) GetServiceToken(ctx context.Context, server *models.MCPServer, scopes string) (*models.MCPOAuth2Token, error) {
	serviceEmail := "__service__"
	token, err := m.tokenStore.Get(ctx, serviceEmail, server.ID)
	if err != nil {
		return nil, err
	}

	if token != nil && !store.IsTokenExpired(token) {
		return token, nil
	}

	return m.ExchangeClientCredentials(ctx, server, scopes)
}

// ResolveScopes computes the effective OAuth2 scopes for a user based on their groups
// and the MCP server's scope mappings.
func ResolveScopes(baseScopes string, userGroups []string, mappings []models.MCPOAuth2ScopeMapping) string {
	scopeSet := make(map[string]struct{})
	for _, s := range strings.Fields(baseScopes) {
		scopeSet[s] = struct{}{}
	}

	groupSet := make(map[string]bool)
	for _, g := range userGroups {
		groupSet[strings.ToLower(g)] = true
	}

	for _, mapping := range mappings {
		if groupSet[strings.ToLower(mapping.GroupName)] {
			for _, s := range strings.Fields(mapping.Scopes) {
				scopeSet[s] = struct{}{}
			}
		}
	}

	scopes := make([]string, 0, len(scopeSet))
	for s := range scopeSet {
		scopes = append(scopes, s)
	}
	return strings.Join(scopes, " ")
}

// DisconnectUser removes a user's OAuth2 token for an MCP server.
func (m *OAuth2Manager) DisconnectUser(ctx context.Context, userEmail, mcpServerID string) error {
	return m.tokenStore.Delete(ctx, userEmail, mcpServerID)
}

// refreshToken uses the refresh_token grant to get new tokens.
func (m *OAuth2Manager) refreshToken(ctx context.Context, server *models.MCPServer, refreshToken string) (*models.MCPOAuth2Token, error) {
	metadata, err := m.DiscoverMetadata(ctx, server.OAuth2AuthServerURL)
	if err != nil {
		return nil, fmt.Errorf("discover metadata for refresh: %w", err)
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {server.OAuth2ClientID},
	}
	if server.OAuth2ClientSecret != "" {
		data.Set("client_secret", server.OAuth2ClientSecret)
	}

	return m.tokenRequest(ctx, metadata.TokenEndpoint, data)
}

// tokenRequest performs a token endpoint request and parses the response.
func (m *OAuth2Manager) tokenRequest(ctx context.Context, endpoint string, data url.Values) (*models.MCPOAuth2Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("oauth2 error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("empty access token")
	}

	var expiresAt int64
	if tokenResp.ExpiresIn > 0 {
		expiresAt = time.Now().Unix() + int64(tokenResp.ExpiresIn)
	}

	return &models.MCPOAuth2Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    expiresAt,
		Scopes:       tokenResp.Scope,
	}, nil
}

// ResourceMetadata holds the response from /.well-known/oauth-protected-resource (RFC 9728).
type ResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported,omitempty"`
}

// DiscoveryResult holds everything auto-discovered from an MCP server URL.
type DiscoveryResult struct {
	AuthServerURL  string
	Scopes         string
	ClientID       string
	ClientSecret   string
}

// DiscoverFromMCP performs the full RFC 9728 + RFC 8414 + RFC 7591 discovery chain:
//  1. Probe MCP URL → receive 401 with WWW-Authenticate header
//  2. Parse resource_metadata URL from WWW-Authenticate
//  3. Fetch resource metadata → extract authorization_servers[0]
//  4. Discover auth server metadata (RFC 8414) → extract scopes_supported
//  5. If registration_endpoint exists → Dynamic Client Registration (RFC 7591)
//
// All fields in the result are best-effort: if a step fails, we continue with what we have.
func (m *OAuth2Manager) DiscoverFromMCP(ctx context.Context, mcpURL string) (*DiscoveryResult, error) {
	result := &DiscoveryResult{}

	resourceMetadataURL, err := m.probeForResourceMetadata(ctx, mcpURL)
	if err != nil {
		return nil, fmt.Errorf("probe MCP URL: %w", err)
	}

	if resourceMetadataURL == "" {
		parsed, _ := url.Parse(mcpURL)
		if parsed != nil {
			origin := parsed.Scheme + "://" + parsed.Host
			pathComponent := strings.TrimRight(parsed.Path, "/")
			candidates := []string{}
			if pathComponent != "" {
				candidates = append(candidates, origin+"/.well-known/oauth-protected-resource"+pathComponent)
			}
			candidates = append(candidates, origin+"/.well-known/oauth-protected-resource")

			for _, candidate := range candidates {
				rm, fetchErr := m.fetchResourceMetadata(ctx, candidate)
				if fetchErr == nil && len(rm.AuthorizationServers) > 0 {
					result.AuthServerURL = rm.AuthorizationServers[0]
					if len(rm.ScopesSupported) > 0 {
						result.Scopes = strings.Join(rm.ScopesSupported, " ")
					}
					break
				}
			}
		}
	} else {
		rm, err := m.fetchResourceMetadata(ctx, resourceMetadataURL)
		if err != nil {
			m.logger.Warn("failed to fetch resource metadata", zap.String("url", resourceMetadataURL), zap.Error(err))
		} else {
			if len(rm.AuthorizationServers) > 0 {
				result.AuthServerURL = rm.AuthorizationServers[0]
			}
			if len(rm.ScopesSupported) > 0 {
				result.Scopes = strings.Join(rm.ScopesSupported, " ")
			}
		}
	}

	if result.AuthServerURL == "" {
		return result, fmt.Errorf("could not discover authorization server from %s", mcpURL)
	}

	m.logger.Info("discovered auth server from MCP",
		zap.String("mcp_url", mcpURL),
		zap.String("auth_server", result.AuthServerURL))

	authMeta, err := m.DiscoverMetadata(ctx, result.AuthServerURL)
	if err != nil {
		return result, fmt.Errorf("discover auth server metadata: %w", err)
	}

	if result.Scopes == "" && len(authMeta.ScopesSupported) > 0 {
		result.Scopes = strings.Join(authMeta.ScopesSupported, " ")
	}

	if authMeta.RegistrationEndpoint != "" {
		dcr, err := m.RegisterClient(ctx, result.AuthServerURL)
		if err != nil {
			m.logger.Warn("DCR failed during auto-discovery (client_id must be set manually)",
				zap.String("auth_server", result.AuthServerURL),
				zap.Error(err))
		} else {
			result.ClientID = dcr.ClientID
			result.ClientSecret = dcr.ClientSecret
		}
	}

	return result, nil
}

// probeForResourceMetadata sends a request to the MCP URL and parses the
// WWW-Authenticate header from the 401 response to extract the resource_metadata URL.
func (m *OAuth2Manager) probeForResourceMetadata(ctx context.Context, mcpURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mcpURL, strings.NewReader(`{"jsonrpc":"2.0","id":0,"method":"initialize"}`))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("MCP-Protocol-Version", "2025-03-26")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusUnauthorized {
		return "", nil
	}

	wwwAuth := resp.Header.Get("WWW-Authenticate")
	if wwwAuth == "" {
		return "", nil
	}

	return parseResourceMetadataURL(wwwAuth), nil
}

// parseResourceMetadataURL extracts the resource_metadata URL from a WWW-Authenticate header.
// Format: Bearer resource_metadata="https://example.com/.well-known/oauth-protected-resource"
func parseResourceMetadataURL(wwwAuth string) string {
	const key = "resource_metadata=\""
	idx := strings.Index(wwwAuth, key)
	if idx == -1 {
		return ""
	}
	rest := wwwAuth[idx+len(key):]
	end := strings.Index(rest, "\"")
	if end == -1 {
		return ""
	}
	return rest[:end]
}

// fetchResourceMetadata fetches and parses an OAuth2 Protected Resource Metadata document (RFC 9728).
func (m *OAuth2Manager) fetchResourceMetadata(ctx context.Context, metadataURL string) (*ResourceMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var rm ResourceMetadata
	if err := json.NewDecoder(resp.Body).Decode(&rm); err != nil {
		return nil, err
	}
	return &rm, nil
}

// InvalidateMetadataCache removes a cached metadata entry (useful after admin config changes).
func (m *OAuth2Manager) InvalidateMetadataCache(authServerURL string) {
	m.metadataCache.Delete(authServerURL)
}

// DCRResponse holds the response from a Dynamic Client Registration request (RFC 7591).
type DCRResponse struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	ClientName   string `json:"client_name,omitempty"`
}

// RegisterClient performs Dynamic Client Registration (RFC 7591) with the
// authorization server. Returns the registered client_id and optional secret.
// If the server does not support DCR (no registration_endpoint), returns an error.
func (m *OAuth2Manager) RegisterClient(ctx context.Context, authServerURL string) (*DCRResponse, error) {
	metadata, err := m.DiscoverMetadata(ctx, authServerURL)
	if err != nil {
		return nil, fmt.Errorf("discover metadata: %w", err)
	}

	if metadata.RegistrationEndpoint == "" {
		return nil, fmt.Errorf("authorization server does not support dynamic client registration (no registration_endpoint in metadata)")
	}

	suffix := make([]byte, 4)
	rand.Read(suffix)
	clientName := fmt.Sprintf("agentgram_%x", suffix)

	regReq := map[string]interface{}{
		"client_name":                clientName,
		"redirect_uris":             []string{m.callbackURL},
		"grant_types":               []string{"authorization_code", "refresh_token"},
		"response_types":            []string{"code"},
		"token_endpoint_auth_method": "none",
		"application_type":          "web",
		"scope":                     strings.Join(metadata.ScopesSupported, " "),
	}

	body, err := json.Marshal(regReq)
	if err != nil {
		return nil, fmt.Errorf("marshal registration request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, metadata.RegistrationEndpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("create registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("MCP-Protocol-Version", "2025-03-26")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("registration request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read registration response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("registration endpoint returned %d: %s", resp.StatusCode, string(respBody))
	}

	var dcr DCRResponse
	if err := json.Unmarshal(respBody, &dcr); err != nil {
		return nil, fmt.Errorf("parse registration response: %w", err)
	}

	if dcr.ClientID == "" {
		return nil, fmt.Errorf("registration response missing client_id")
	}

	m.logger.Info("OAuth2 dynamic client registration successful",
		zap.String("auth_server", authServerURL),
		zap.String("client_id", dcr.ClientID),
		zap.String("client_name", dcr.ClientName))

	return &dcr, nil
}

// EnsureClientRegistered checks if an OAuth2 MCP server has a client_id.
// If not, attempts Dynamic Client Registration and returns the updated fields.
// Returns (client_id, client_secret, error). If DCR is not supported or not needed,
// returns the existing values unchanged.
func (m *OAuth2Manager) EnsureClientRegistered(ctx context.Context, server *models.MCPServer) (string, string, error) {
	if server.OAuth2ClientID != "" {
		return server.OAuth2ClientID, server.OAuth2ClientSecret, nil
	}

	if server.OAuth2AuthServerURL == "" {
		return "", "", fmt.Errorf("no OAuth2 auth server URL configured")
	}

	dcr, err := m.RegisterClient(ctx, server.OAuth2AuthServerURL)
	if err != nil {
		return "", "", err
	}

	return dcr.ClientID, dcr.ClientSecret, nil
}

package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/config"
	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	authCookieName     = "auth_session"
	githubCookieMaxAge = 30 * 24 * 60 * 60 // 30 days
)

// AuthHandler handles OIDC authentication endpoints
type AuthHandler struct {
	oidc         *auth.OIDCClient
	github       *auth.GitHubOAuthClient
	sessionStore store.AuthSessionStore
	cookieCrypto *auth.CookieCrypto
	cfg          *config.Config
	logger       *zap.Logger
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(oidc *auth.OIDCClient, github *auth.GitHubOAuthClient, sessionStore store.AuthSessionStore, cookieCrypto *auth.CookieCrypto, cfg *config.Config, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		oidc:         oidc,
		github:       github,
		sessionStore: sessionStore,
		cookieCrypto: cookieCrypto,
		cfg:          cfg,
		logger:       logger,
	}
}

// Login redirects the user to Keycloak for authentication
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	state := uuid.New().String()
	nonce := uuid.New().String()

	if err := h.sessionStore.SaveState(r.Context(), state, nonce); err != nil {
		h.logger.Error("failed to save OIDC state", zap.Error(err))
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	authURL := h.oidc.GetAuthorizationURL(state, nonce)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback handles the OIDC callback from Keycloak
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		h.logger.Warn("missing code or state in OIDC callback")
		http.Error(w, `{"error":"missing code or state"}`, http.StatusBadRequest)
		return
	}

	// Check for error from Keycloak
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		errDesc := r.URL.Query().Get("error_description")
		h.logger.Warn("OIDC callback error",
			zap.String("error", errParam),
			zap.String("description", errDesc))
		http.Error(w, `{"error":"authentication failed"}`, http.StatusUnauthorized)
		return
	}

	// Validate CSRF state and recover nonce
	nonce, err := h.sessionStore.ValidateState(r.Context(), state)
	if err != nil {
		h.logger.Warn("invalid OIDC state", zap.Error(err))
		http.Error(w, `{"error":"invalid state"}`, http.StatusBadRequest)
		return
	}

	// Exchange code for tokens
	tokenResp, err := h.oidc.ExchangeCode(r.Context(), code)
	if err != nil {
		h.logger.Error("failed to exchange code", zap.Error(err))
		http.Error(w, `{"error":"token exchange failed"}`, http.StatusInternalServerError)
		return
	}

	// Parse ID token to extract claims (validates nonce to prevent token substitution)
	claims, err := h.oidc.ParseIDTokenClaims(r.Context(), tokenResp.IDToken, nonce)
	if err != nil {
		h.logger.Error("failed to parse ID token", zap.Error(err))
		http.Error(w, `{"error":"invalid ID token"}`, http.StatusInternalServerError)
		return
	}

	// Generate session ID
	sessionID, err := store.GenerateSessionID()
	if err != nil {
		h.logger.Error("failed to generate session ID", zap.Error(err))
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	// Create auth session
	now := time.Now()
	authSession := &store.AuthSession{
		SessionID:    sessionID,
		Email:        claims.GetEmail(),
		Name:         claims.Name,
		Sub:          claims.Sub,
		Groups:       claims.GetGroups(),
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		ExpiresAt:    now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Unix(),
		CreatedAt:    now.Unix(),
	}

	if err := h.sessionStore.Create(r.Context(), authSession); err != nil {
		h.logger.Error("failed to create auth session", zap.Error(err))
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	// Set session cookie
	maxAge := h.cfg.Auth.SessionMaxAge
	if maxAge == 0 {
		maxAge = 86400
	}

	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.cfg.Auth.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	h.logger.Info("user authenticated",
		zap.String("email", authSession.Email),
		zap.String("sub", authSession.Sub))

	// Redirect to frontend
	http.Redirect(w, r, "/", http.StatusFound)
}

// Logout clears the auth session and returns Keycloak logout URL
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(authCookieName)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		return
	}

	// Get session to retrieve tokens for revocation
	session, err := h.sessionStore.Get(r.Context(), cookie.Value)
	if err != nil {
		h.logger.Error("failed to get auth session for logout", zap.Error(err))
	}

	var logoutURL string
	if session != nil {
		// Revoke tokens (best-effort)
		if session.RefreshToken != "" {
			_ = h.oidc.RevokeToken(r.Context(), session.RefreshToken)
		}
		logoutURL = h.oidc.GetLogoutURL(session.IDToken)

		// Delete session from Redis
		if err := h.sessionStore.Delete(r.Context(), cookie.Value); err != nil {
			h.logger.Error("failed to delete auth session", zap.Error(err))
		}
	} else {
		logoutURL = h.oidc.GetLogoutURL("")
	}

	// Clear auth session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cfg.Auth.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	// Clear GitHub token cookie
	h.clearGitHubTokenCookie(w)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":         true,
		"logout_url": logoutURL,
	})
}

// GetSession returns the current user's session info
func (h *AuthHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cookie, err := r.Cookie(authCookieName)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": false,
		})
		return
	}

	session, err := h.sessionStore.Get(r.Context(), cookie.Value)
	if err != nil || session == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": false,
		})
		return
	}

	// Check if access token is expired (or about to expire) and refresh if needed
	secondsUntilExpiry := session.ExpiresAt - time.Now().Unix()
	if secondsUntilExpiry <= middleware.TokenRefreshThreshold && session.RefreshToken != "" && h.oidc != nil {
		h.logger.Debug("session check: proactive token refresh",
			zap.String("email", session.Email),
			zap.Int64("seconds_until_expiry", secondsUntilExpiry))

		tokenResp, refreshErr := h.oidc.RefreshToken(r.Context(), session.RefreshToken)
		if refreshErr == nil {
			session.AccessToken = tokenResp.AccessToken
			session.RefreshToken = tokenResp.RefreshToken
			session.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Unix()
			if tokenResp.IDToken != "" {
				session.IDToken = tokenResp.IDToken
			}
			_ = h.sessionStore.Update(r.Context(), session)

			// Rolling session: extend cookie lifetime on token refresh
			maxAge := h.cfg.Auth.SessionMaxAge
			if maxAge == 0 {
				maxAge = 86400
			}
			http.SetCookie(w, &http.Cookie{
				Name:     authCookieName,
				Value:    cookie.Value,
				Path:     "/",
				MaxAge:   maxAge,
				HttpOnly: true,
				Secure:   h.cfg.Auth.CookieSecure,
				SameSite: http.SameSiteLaxMode,
			})
			h.logger.Debug("session check: refreshed access token", zap.String("email", session.Email))
		} else {
			h.logger.Warn("session check: failed to refresh token",
				zap.String("email", session.Email),
				zap.Error(refreshErr))
			// If the access token is already expired and refresh failed, session is invalid
			if secondsUntilExpiry <= 0 {
				h.logger.Info("session check: access token expired and refresh failed",
					zap.String("email", session.Email))
				json.NewEncoder(w).Encode(map[string]interface{}{
					"authenticated": false,
				})
				return
			}
		}
	}

	resp := map[string]interface{}{
		"authenticated": true,
		"email":         session.Email,
		"name":          session.Name,
		"groups":        session.Groups,
	}

	// Read GitHub token from encrypted cookie
	githubToken := h.readGitHubTokenCookie(r)
	if githubToken != "" {
		resp["github_connected"] = true
		if h.github != nil {
			username, err := h.github.GetUser(r.Context(), githubToken)
			if errors.Is(err, auth.ErrGitHubTokenExpired) {
				h.logger.Info("github token expired, clearing cookie")
				h.clearGitHubTokenCookie(w)
				resp["github_connected"] = false
			} else if err == nil {
				resp["github_username"] = username
			}
		}
	} else {
		resp["github_connected"] = false
	}

	json.NewEncoder(w).Encode(resp)
}

// GitHubLogin redirects the user to GitHub for OAuth authorization
func (h *AuthHandler) GitHubLogin(w http.ResponseWriter, r *http.Request) {
	if h.github == nil {
		http.Error(w, `{"error":"github oauth not configured"}`, http.StatusNotFound)
		return
	}

	state := uuid.New().String()
	if err := h.sessionStore.SaveState(r.Context(), state, "github"); err != nil {
		h.logger.Error("failed to save GitHub OAuth state", zap.Error(err))
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	authURL := h.github.GetAuthorizationURL(state)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// GitHubCallback handles the GitHub OAuth callback
func (h *AuthHandler) GitHubCallback(w http.ResponseWriter, r *http.Request) {
	if h.github == nil {
		http.Error(w, `{"error":"github oauth not configured"}`, http.StatusNotFound)
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		h.logger.Warn("missing code or state in GitHub callback")
		http.Error(w, `{"error":"missing code or state"}`, http.StatusBadRequest)
		return
	}

	// Validate CSRF state
	nonce, err := h.sessionStore.ValidateState(r.Context(), state)
	if err != nil {
		h.logger.Warn("invalid GitHub OAuth state", zap.Error(err))
		http.Error(w, `{"error":"invalid state"}`, http.StatusBadRequest)
		return
	}
	if nonce != "github" {
		h.logger.Warn("state nonce mismatch for GitHub OAuth", zap.String("nonce", nonce))
		http.Error(w, `{"error":"invalid state"}`, http.StatusBadRequest)
		return
	}

	// Exchange code for access token
	token, err := h.github.ExchangeCode(r.Context(), code)
	if err != nil {
		h.logger.Error("failed to exchange GitHub code", zap.Error(err))
		http.Error(w, `{"error":"github token exchange failed"}`, http.StatusInternalServerError)
		return
	}

	// Verify user has an active auth session
	cookie, err := r.Cookie(authCookieName)
	if err != nil {
		h.logger.Warn("no auth session cookie for GitHub callback")
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	session, err := h.sessionStore.Get(r.Context(), cookie.Value)
	if err != nil || session == nil {
		h.logger.Warn("invalid auth session for GitHub callback")
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	// Store GitHub token in encrypted cookie
	if err := h.setGitHubTokenCookie(w, token); err != nil {
		h.logger.Error("failed to set github token cookie", zap.Error(err))
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	h.logger.Info("github account connected", zap.String("email", session.Email))

	// If opened in popup (from multi-agent chat), close popup and notify parent
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html><html><head><title>GitHub Connected</title></head><body>
<script>
if (window.opener) {
  window.opener.postMessage({type: "github_connected"}, "*");
  window.close();
} else {
  window.location.href = "/";
}
</script>
<p>GitHub conectado. Puedes cerrar esta ventana.</p>
</body></html>`)
}

// GitHubStatus returns the current GitHub connection status
func (h *AuthHandler) GitHubStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	githubToken := h.readGitHubTokenCookie(r)
	if githubToken == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"connected": false})
		return
	}

	resp := map[string]interface{}{"connected": true}
	if h.github != nil {
		username, err := h.github.GetUser(r.Context(), githubToken)
		if errors.Is(err, auth.ErrGitHubTokenExpired) {
			h.logger.Info("github token expired, clearing cookie")
			h.clearGitHubTokenCookie(w)
			json.NewEncoder(w).Encode(map[string]interface{}{"connected": false})
			return
		}
		if err == nil {
			resp["username"] = username
		}
	}
	json.NewEncoder(w).Encode(resp)
}

// GitHubDisconnect removes the GitHub token cookie
func (h *AuthHandler) GitHubDisconnect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	h.clearGitHubTokenCookie(w)

	h.logger.Info("github account disconnected")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// setGitHubTokenCookie encrypts the token and sets it as an HttpOnly cookie.
func (h *AuthHandler) setGitHubTokenCookie(w http.ResponseWriter, token string) error {
	encrypted, err := h.cookieCrypto.Encrypt(token)
	if err != nil {
		return fmt.Errorf("encrypt github token: %w", err)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.GitHubCookieName,
		Value:    encrypted,
		Path:     "/",
		MaxAge:   githubCookieMaxAge,
		HttpOnly: true,
		Secure:   h.cfg.Auth.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// clearGitHubTokenCookie removes the GitHub token cookie.
func (h *AuthHandler) clearGitHubTokenCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.GitHubCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cfg.Auth.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// readGitHubTokenCookie reads and decrypts the GitHub token from the cookie.
func (h *AuthHandler) readGitHubTokenCookie(r *http.Request) string {
	cookie, err := r.Cookie(auth.GitHubCookieName)
	if err != nil || cookie.Value == "" {
		return ""
	}

	token, err := h.cookieCrypto.Decrypt(cookie.Value)
	if err != nil {
		h.logger.Debug("failed to decrypt github token cookie", zap.Error(err))
		return ""
	}

	return token
}

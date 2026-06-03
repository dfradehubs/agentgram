package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"go.uber.org/zap"
)

// ContextKey type for context keys
type ContextKey string

const (
	// UserContextKey key for user claims in context
	UserContextKey ContextKey = "user"
	// AuthHeaderContextKey key for authorization header
	AuthHeaderContextKey ContextKey = "authHeader"
	// GitHubTokenContextKey key for GitHub token in context
	GitHubTokenContextKey ContextKey = "githubToken"
	// TokenRefreshThreshold seconds before expiry to proactively refresh
	TokenRefreshThreshold int64 = 60

)

// Auth JWT authentication middleware with dual auth (cookie + bearer)
type Auth struct {
	validator    *auth.JWTValidator
	authStore    store.AuthSessionStore
	oidcClient   *auth.OIDCClient
	cookieCrypto *auth.CookieCrypto
	logger       *zap.Logger
	maxAge       int
	cookieSecure bool
}

// NewAuth creates a new authentication middleware
func NewAuth(validator *auth.JWTValidator, authStore store.AuthSessionStore, oidcClient *auth.OIDCClient, cookieCrypto *auth.CookieCrypto, logger *zap.Logger, maxAge int, cookieSecure bool) *Auth {
	return &Auth{
		validator:    validator,
		authStore:    authStore,
		oidcClient:   oidcClient,
		cookieCrypto: cookieCrypto,
		logger:       logger,
		maxAge:       maxAge,
		cookieSecure: cookieSecure,
	}
}

// Handler returns the HTTP middleware
func (a *Auth) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Attempt 1: Cookie-based auth (OIDC session)
		if a.authStore != nil {
			if cookie, err := r.Cookie("auth_session"); err == nil {
				session, err := a.authStore.Get(r.Context(), cookie.Value)
				if err == nil && session != nil {
					// Check if access token is expired (or about to expire) and refresh if needed
					secondsUntilExpiry := session.ExpiresAt - time.Now().Unix()
					if secondsUntilExpiry <= TokenRefreshThreshold && session.RefreshToken != "" && a.oidcClient != nil {
						a.logger.Debug("proactive token refresh",
							zap.String("email", session.Email),
							zap.Int64("seconds_until_expiry", secondsUntilExpiry))

						if tokenResp, err := a.oidcClient.RefreshToken(r.Context(), session.RefreshToken); err == nil {
							session.AccessToken = tokenResp.AccessToken
							session.RefreshToken = tokenResp.RefreshToken
							session.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Unix()
							if tokenResp.IDToken != "" {
								session.IDToken = tokenResp.IDToken
								// Update profile claims from refreshed ID token (backfills Name for old sessions)
								if idClaims, err := a.oidcClient.ParseIDTokenClaims(r.Context(), tokenResp.IDToken, ""); err == nil {
									if idClaims.Name != "" {
										session.Name = idClaims.Name
									}
								}
							}
							_ = a.authStore.Update(r.Context(), session)
							// Rolling session: extend cookie lifetime on token refresh
							http.SetCookie(w, &http.Cookie{
								Name:     "auth_session",
								Value:    cookie.Value,
								Path:     "/",
								MaxAge:   a.maxAge,
								HttpOnly: true,
								Secure:   a.cookieSecure,
								SameSite: http.SameSiteLaxMode,
							})
							a.logger.Debug("refreshed access token", zap.String("email", session.Email))
						} else {
							a.logger.Warn("failed to refresh token",
								zap.String("email", session.Email),
								zap.Error(err))
							// If the access token is already expired, return 401 but do NOT
							// delete the Redis session. The user can re-authenticate and
							// their session data (minus expired tokens) is preserved.
							if secondsUntilExpiry <= 0 {
								a.logger.Info("access token expired and refresh failed",
									zap.String("email", session.Email))
								http.Error(w, `{"error":"session expired"}`, http.StatusUnauthorized)
								return
							}
						}
					}

					claims := &auth.Claims{
						Sub:    session.Sub,
						Email:  session.Email,
						Name:   session.Name,
						Groups: session.Groups,
					}

					// Backfill name from access token if not stored in session
					if claims.Name == "" && session.AccessToken != "" && a.validator != nil {
						if tokenClaims, err := a.validator.ValidateToken(r.Context(), session.AccessToken); err == nil && tokenClaims.Name != "" {
							claims.Name = tokenClaims.Name
							session.Name = tokenClaims.Name
							_ = a.authStore.Update(r.Context(), session)
						}
					}

					ctx := context.WithValue(r.Context(), UserContextKey, claims)
					ctx = context.WithValue(ctx, AuthHeaderContextKey, fmt.Sprintf("Bearer %s", session.AccessToken))

					// Read GitHub token from encrypted cookie
					if a.cookieCrypto != nil {
						if ghCookie, err := r.Cookie(auth.GitHubCookieName); err == nil && ghCookie.Value != "" {
							if token, err := a.cookieCrypto.Decrypt(ghCookie.Value); err == nil {
								ctx = context.WithValue(ctx, GitHubTokenContextKey, token)
							}
						}
					}

					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}

		// Attempt 2: Bearer token auth (existing JWT flow)
		authHeader := r.Header.Get("Authorization")
		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			a.logger.Debug("no valid auth method found",
				zap.String("path", r.URL.Path),
				zap.Error(err))
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		claims, err := a.validator.ValidateToken(r.Context(), token)
		if err != nil {
			a.logger.Debug("invalid token",
				zap.String("path", r.URL.Path),
				zap.Error(err))
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, claims)
		ctx = context.WithValue(ctx, AuthHeaderContextKey, authHeader)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserFromContext retrieves user claims from context
func GetUserFromContext(ctx context.Context) *auth.Claims {
	if claims, ok := ctx.Value(UserContextKey).(*auth.Claims); ok {
		return claims
	}
	return nil
}

// GetAuthHeaderFromContext retrieves authorization header from context
func GetAuthHeaderFromContext(ctx context.Context) string {
	if header, ok := ctx.Value(AuthHeaderContextKey).(string); ok {
		return header
	}
	return ""
}

// GetGitHubTokenFromContext retrieves GitHub token from context
func GetGitHubTokenFromContext(ctx context.Context) string {
	if token, ok := ctx.Value(GitHubTokenContextKey).(string); ok {
		return token
	}
	return ""
}

// NoAuth middleware that allows all requests (for when auth is disabled)
// Sets empty claims in context so handlers still work
func NoAuth(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger.Debug("auth disabled, allowing request",
				zap.String("path", r.URL.Path))

			// Set empty claims so handlers can still access user info
			claims := &auth.Claims{
				Sub:    "anonymous",
				Email:  "anonymous@localhost",
				Groups: []string{},
			}
			ctx := context.WithValue(r.Context(), UserContextKey, claims)
			ctx = context.WithValue(ctx, AuthHeaderContextKey, "")

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

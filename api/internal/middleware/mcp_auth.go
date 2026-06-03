package middleware

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/config"
	"go.uber.org/zap"
)

// MCPAuth is a JWT authentication middleware specialized for the MCP endpoint.
// It returns a WWW-Authenticate header with resource_metadata on 401, which
// allows MCP clients (like Claude Code) to discover the OAuth2 authorization server.
//
// In addition to Keycloak-signed JWTs, MCPAuth honors an optional list of
// static service-account tokens supplied via configuration. They are matched
// in constant time before falling back to JWT validation and map to synthetic
// claims (Email + Groups) so downstream authorisation continues to work the
// same way as for a real Keycloak user.
type MCPAuth struct {
	validator    *auth.JWTValidator
	logger       *zap.Logger
	host         string // public hostname for resource_metadata URL
	staticTokens []staticTokenEntry
}

// staticTokenEntry stores a configured service-account credential.
// tokenHash is the SHA-256 digest of the configured secret. Storing the hash
// (instead of the raw token) lets us compare against a fixed-length digest of
// the incoming token: subtle.ConstantTimeCompare returns immediately when the
// slice lengths differ, so comparing the raw values would still leak the
// configured token length via timing analysis even though the values
// themselves are compared in constant time.
type staticTokenEntry struct {
	name      string
	tokenHash [sha256.Size]byte
	claims    auth.Claims
}

// NewMCPAuth creates a new MCP authentication middleware.
// staticTokens is optional — pass nil to keep pre-existing behaviour.
func NewMCPAuth(validator *auth.JWTValidator, logger *zap.Logger, host string, staticTokens []config.StaticToken) *MCPAuth {
	entries := make([]staticTokenEntry, 0, len(staticTokens))
	for _, st := range staticTokens {
		if st.Token == "" {
			logger.Warn("MCP auth: skipping static_tokens entry with empty token", zap.String("name", st.Name))
			continue
		}
		entries = append(entries, staticTokenEntry{
			name:      st.Name,
			tokenHash: sha256.Sum256([]byte(st.Token)),
			claims: auth.Claims{
				Sub:    "static:" + st.Name,
				Email:  st.Email,
				Groups: append([]string(nil), st.Groups...),
			},
		})
	}
	return &MCPAuth{
		validator:    validator,
		logger:       logger,
		host:         host,
		staticTokens: entries,
	}
}

// Handler returns the HTTP middleware
func (m *MCPAuth) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			m.writeUnauthorized(w)
			return
		}

		// Try static service-account tokens first. We always run through every
		// entry so the comparison time does not leak which token (if any) matched.
		if claims, name, ok := m.matchStaticToken(token); ok {
			m.logger.Info("MCP auth: static token accepted",
				zap.String("name", name),
				zap.String("email", claims.Email))
			ctx := context.WithValue(r.Context(), UserContextKey, claims)
			ctx = context.WithValue(ctx, AuthHeaderContextKey, authHeader)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		claims, err := m.validator.ValidateToken(r.Context(), token)
		if err != nil {
			m.logger.Debug("MCP auth: invalid token",
				zap.String("path", r.URL.Path),
				zap.Error(err))
			m.writeUnauthorized(w)
			return
		}

		// DEBUG: log token audience for troubleshooting agent auth forwarding
		if parts := strings.SplitN(token, ".", 3); len(parts) == 3 {
			if payload, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
				var tokenClaims map[string]interface{}
				if json.Unmarshal(payload, &tokenClaims) == nil {
					m.logger.Info("MCP auth: token claims",
						zap.String("email", claims.Email),
						zap.Any("aud", tokenClaims["aud"]),
						zap.Any("azp", tokenClaims["azp"]))
				}
			}
		}

		ctx := context.WithValue(r.Context(), UserContextKey, claims)
		ctx = context.WithValue(ctx, AuthHeaderContextKey, authHeader)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// matchStaticToken returns a freshly-allocated Claims associated with a
// configured static token, using constant-time comparison over SHA-256 digests
// to avoid timing oracles on token value, length, or position in the list.
//
// A new Claims (with a defensive copy of Groups) is returned per call so a
// hypothetical downstream mutation cannot race or leak across concurrent
// requests — matching the JWT path, which also yields a fresh Claims each
// validation.
func (m *MCPAuth) matchStaticToken(presented string) (*auth.Claims, string, bool) {
	if len(m.staticTokens) == 0 {
		return nil, "", false
	}
	presentedHash := sha256.Sum256([]byte(presented))
	var (
		matchedEntry *staticTokenEntry
		matched      bool
	)
	for i := range m.staticTokens {
		entry := &m.staticTokens[i]
		if subtle.ConstantTimeCompare(presentedHash[:], entry.tokenHash[:]) == 1 {
			matchedEntry = entry
			matched = true
		}
	}
	if !matched {
		return nil, "", false
	}
	cloned := matchedEntry.claims
	cloned.Groups = append([]string(nil), matchedEntry.claims.Groups...)
	return &cloned, matchedEntry.name, true
}

// writeUnauthorized sends a 401 with the MCP-standard WWW-Authenticate header
// that tells the client where to find OAuth2 authorization server metadata.
func (m *MCPAuth) writeUnauthorized(w http.ResponseWriter) {
	host := m.host
	if host == "" {
		host = "localhost"
	}
	resourceMetadataURL := fmt.Sprintf("https://%s/.well-known/oauth-protected-resource", host)
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s"`, resourceMetadataURL))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	// JSON-RPC 2.0 error shape. Some MCP clients (e.g. Cursor 2.6.x) parse the
	// 401 response body with a strict JSON-RPC schema before honoring the HTTP
	// status and WWW-Authenticate header — if the body doesn't match, they
	// abort with a Zod validation error and never start the OAuth flow.
	// Returning a well-formed JSON-RPC error keeps those clients happy and
	// doesn't change behavior for well-behaved clients that only look at the
	// status code.
	w.Write([]byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32001,"message":"Unauthorized"}}`))
}

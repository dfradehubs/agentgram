package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/crypto"
	"github.com/redis/go-redis/v9"

	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"go.uber.org/zap"
)

// SlackLinkHandler handles Slack account linking via OIDC.
type SlackLinkHandler struct {
	linkRepo     *SlackUserLinkRepo
	slackRepo    repository.SlackIntegrationRepository
	oidc         *auth.OIDCClient
	github       *auth.GitHubOAuthClient
	sessionStore store.AuthSessionStore
	cipher       *crypto.AESCrypto
	rdb          *redis.Client
	hostURL      string
	logger       *zap.Logger
}

// SlackUserLinkRepo wraps the repository to avoid import cycle.
type SlackUserLinkRepo = repository.SlackUserLinkRepository

// NewSlackLinkHandler creates a new SlackLinkHandler.
func NewSlackLinkHandler(
	linkRepo repository.SlackUserLinkRepository,
	slackRepo repository.SlackIntegrationRepository,
	oidc *auth.OIDCClient,
	github *auth.GitHubOAuthClient,
	sessionStore store.AuthSessionStore,
	cipher *crypto.AESCrypto,
	rdb *redis.Client,
	hostURL string,
	logger *zap.Logger,
) *SlackLinkHandler {
	return &SlackLinkHandler{
		linkRepo:     &linkRepo,
		slackRepo:    slackRepo,
		oidc:         oidc,
		github:       github,
		sessionStore: sessionStore,
		cipher:       cipher,
		rdb:          rdb,
		hostURL:      hostURL,
		logger:       logger,
	}
}

// linkState is encrypted into the OIDC state parameter.
type linkState struct {
	SlackUserID string `json:"s"`
	Nonce       string `json:"n"`
	ChannelID   string `json:"c,omitempty"`
	ThreadTS    string `json:"t,omitempty"`
	AgentID     string `json:"a,omitempty"`
	Text        string `json:"q,omitempty"`
}

// LinkStart initiates the OIDC login flow for Slack account linking.
// GET /auth/slack/link?slack_user_id=<encrypted_slack_user_id>
func (h *SlackLinkHandler) LinkStart(w http.ResponseWriter, r *http.Request) {
	encSlackUserID := r.URL.Query().Get("slack_user_id")
	if encSlackUserID == "" {
		http.Error(w, "missing slack_user_id parameter", http.StatusBadRequest)
		return
	}

	// Decrypt the payload (JSON with slack user ID + thread context)
	decrypted, err := h.cipher.Decrypt(encSlackUserID)
	if err != nil {
		h.logger.Warn("invalid slack_user_id parameter", zap.Error(err))
		http.Error(w, "invalid or expired link", http.StatusBadRequest)
		return
	}

	// Parse payload: {"u":"slack_user_id","c":"channel_id","t":"thread_ts","a":"agent_id","q":"text"}
	var payload map[string]string
	slackUserID := decrypted
	var channelID, threadTS, agentID, originalText string
	if json.Unmarshal([]byte(decrypted), &payload) == nil {
		slackUserID = payload["u"]
		channelID = payload["c"]
		threadTS = payload["t"]
		agentID = payload["a"]
		originalText = payload["q"]
	}

	nonce := uuid.New().String()
	state := uuid.New().String()

	ls := linkState{SlackUserID: slackUserID, Nonce: nonce, ChannelID: channelID, ThreadTS: threadTS, AgentID: agentID, Text: originalText}
	lsJSON, _ := json.Marshal(ls)
	encState, err := h.cipher.Encrypt(string(lsJSON))
	if err != nil {
		h.logger.Error("failed to encrypt link state", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Save in auth session store for CSRF validation
	if err := h.sessionStore.SaveState(r.Context(), state, encState); err != nil {
		h.logger.Error("failed to save link state", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Build OIDC auth URL with offline_access scope
	redirectURI := h.hostURL + "/auth/slack/callback"
	params := url.Values{
		"response_type": {"code"},
		"client_id":     {h.oidc.ClientID()},
		"redirect_uri":  {redirectURI},
		"scope":         {"openid profile email groups offline_access"},
		"state":         {state},
		"nonce":         {nonce},
	}
	authURL := fmt.Sprintf("%s/protocol/openid-connect/auth?%s", h.oidc.Issuer(), params.Encode())
	http.Redirect(w, r, authURL, http.StatusFound)
}

// LinkCallback handles the OIDC callback after user authenticates.
// GET /auth/slack/callback?code=...&state=...
func (h *SlackLinkHandler) LinkCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		http.Error(w, "missing code or state", http.StatusBadRequest)
		return
	}

	// Validate CSRF state
	encState, err := h.sessionStore.ValidateState(r.Context(), state)
	if err != nil {
		h.logger.Warn("invalid OIDC state for slack link", zap.Error(err))
		http.Error(w, "invalid or expired state", http.StatusBadRequest)
		return
	}

	// Decrypt link state
	lsJSON, err := h.cipher.Decrypt(encState)
	if err != nil {
		h.logger.Error("failed to decrypt link state", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var ls linkState
	if err := json.Unmarshal([]byte(lsJSON), &ls); err != nil {
		h.logger.Error("failed to parse link state", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Exchange code for tokens (with offline_access, this returns an offline refresh token)
	redirectURI := h.hostURL + "/auth/slack/callback"
	tokens, err := h.oidc.ExchangeCodeWithRedirect(r.Context(), code, redirectURI)
	if err != nil {
		h.logger.Error("failed to exchange code for tokens", zap.Error(err))
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}

	// Parse ID token to get email
	claims, err := h.oidc.ParseIDTokenClaims(r.Context(), tokens.IDToken, ls.Nonce)
	if err != nil {
		h.logger.Error("failed to parse ID token", zap.Error(err))
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}

	email := claims.GetEmail()
	if email == "" {
		http.Error(w, "no email in token", http.StatusBadRequest)
		return
	}

	// Store the link
	link := &models.SlackUserLink{
		SlackUserID:  ls.SlackUserID,
		Email:        email,
		RefreshToken: tokens.RefreshToken,
	}
	if err := (*h.linkRepo).Upsert(r.Context(), link); err != nil {
		h.logger.Error("failed to save slack user link", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.logger.Info("slack user linked",
		zap.String("slack_user_id", ls.SlackUserID),
		zap.String("email", email))

	// Publish pending message for reprocessing by the BotManager
	if ls.ChannelID != "" && ls.ThreadTS != "" && ls.AgentID != "" {
		reprocessPayload, _ := json.Marshal(map[string]string{
			"slack_user_id": ls.SlackUserID,
			"channel_id":    ls.ChannelID,
			"thread_ts":     ls.ThreadTS,
			"agent_id":      ls.AgentID,
			"text":          ls.Text,
		})
		if err := h.rdb.Publish(r.Context(), "slack:reprocess", string(reprocessPayload)).Err(); err != nil {
			h.logger.Warn("failed to publish reprocess message", zap.Error(err))
		}
	}

	// Show success page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>Account linked</title>
<style>body{font-family:system-ui;display:flex;justify-content:center;align-items:center;min-height:100vh;background:#111;color:#fff}
.card{text-align:center;padding:2rem;border-radius:12px;background:#1a1a1a;max-width:400px}</style></head>
<body><div class="card">
<h2>Account linked successfully</h2>
<p>Your Slack account has been linked to <strong>%s</strong>.</p>
<p>Processing your message...</p>
</div></body></html>`, html.EscapeString(email))
}

// --- Admin endpoints ---

type slackLinkResponse struct {
	SlackUserID string `json:"slack_user_id"`
	Email       string `json:"email"`
	HasGitHub   bool   `json:"has_github"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// AdminListLinks lists all Slack user links (admin only).
func (h *SlackLinkHandler) AdminListLinks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	links, err := h.listAllLinks(ctx)
	if err != nil {
		h.logger.Error("failed to list slack user links", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links)
}

// AdminRevokeLink revokes a Slack user link (admin only).
func (h *SlackLinkHandler) AdminRevokeLink(w http.ResponseWriter, r *http.Request) {
	slackUserID := r.URL.Query().Get("slack_user_id")
	if slackUserID == "" {
		http.Error(w, "missing slack_user_id", http.StatusBadRequest)
		return
	}

	// Get link to find refresh token for revocation
	link, err := (*h.linkRepo).GetBySlackUserID(r.Context(), slackUserID)
	if err != nil || link == nil {
		http.Error(w, "link not found", http.StatusNotFound)
		return
	}

	// Revoke the refresh token in Keycloak (best-effort)
	if h.oidc != nil && link.RefreshToken != "" {
		_ = h.oidc.RevokeToken(r.Context(), link.RefreshToken)
	}

	// Delete from DB
	if err := (*h.linkRepo).Delete(r.Context(), slackUserID); err != nil {
		h.logger.Error("failed to delete slack user link", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	adminEmail := ""
	if claims := middleware.GetUserFromContext(r.Context()); claims != nil {
		adminEmail = claims.GetEmail()
	}
	h.logger.Info("slack user link revoked by admin",
		zap.String("slack_user_id", slackUserID),
		zap.String("email", link.Email),
		zap.String("admin", adminEmail))

	w.WriteHeader(http.StatusNoContent)
}

// --- GitHub OAuth for Slack ---

// GitHubLinkStart initiates GitHub OAuth for a Slack user.
// GET /auth/slack/github?slack_user_id=<encrypted>
func (h *SlackLinkHandler) GitHubLinkStart(w http.ResponseWriter, r *http.Request) {
	if h.github == nil {
		http.Error(w, "GitHub OAuth not configured", http.StatusNotFound)
		return
	}

	encSlackUserID := r.URL.Query().Get("slack_user_id")
	if encSlackUserID == "" {
		http.Error(w, "missing slack_user_id", http.StatusBadRequest)
		return
	}

	slackUserID, err := h.cipher.Decrypt(encSlackUserID)
	if err != nil {
		http.Error(w, "invalid or expired link", http.StatusBadRequest)
		return
	}

	state := uuid.New().String()

	// Save state → slackUserID for the callback
	encState, err := h.cipher.Encrypt(slackUserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := h.sessionStore.SaveState(r.Context(), state, encState); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	redirectURI := h.hostURL + "/auth/slack/github/callback"
	authURL := h.github.GetAuthorizationURLWithRedirect(state, redirectURI)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// GitHubLinkCallback handles the GitHub OAuth callback for Slack.
// GET /auth/slack/github/callback?code=...&state=...
func (h *SlackLinkHandler) GitHubLinkCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		http.Error(w, "missing code or state", http.StatusBadRequest)
		return
	}

	encState, err := h.sessionStore.ValidateState(r.Context(), state)
	if err != nil {
		http.Error(w, "invalid or expired state", http.StatusBadRequest)
		return
	}

	slackUserID, err := h.cipher.Decrypt(encState)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	tokenResp, err := h.github.ExchangeCodeFull(r.Context(), code)
	if err != nil {
		h.logger.Error("failed to exchange GitHub code for Slack user", zap.Error(err))
		http.Error(w, "GitHub authentication failed", http.StatusUnauthorized)
		return
	}

	// Save GitHub access + refresh token to the user's link
	if err := (*h.linkRepo).SetGitHubToken(r.Context(), slackUserID, tokenResp.AccessToken, tokenResp.RefreshToken); err != nil {
		h.logger.Error("failed to save GitHub token", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Get GitHub username for display
	username := ""
	if h.github != nil {
		username, _ = h.github.GetUser(r.Context(), tokenResp.AccessToken)
	}

	h.logger.Info("slack user linked GitHub",
		zap.String("slack_user_id", slackUserID),
		zap.String("github_user", username))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>GitHub linked</title>
<style>body{font-family:system-ui;display:flex;justify-content:center;align-items:center;min-height:100vh;background:#111;color:#fff}
.card{text-align:center;padding:2rem;border-radius:12px;background:#1a1a1a;max-width:400px}</style></head>
<body><div class="card">
<h2>GitHub linked successfully</h2>
<p>Your GitHub account <strong>%s</strong> has been linked.</p>
<p>You can now go back to Slack and use the agent.</p>
</div></body></html>`, html.EscapeString(username))
}

// AdminRevokeGitHub revokes a Slack user's GitHub token (admin only).
func (h *SlackLinkHandler) AdminRevokeGitHub(w http.ResponseWriter, r *http.Request) {
	slackUserID := r.URL.Query().Get("slack_user_id")
	if slackUserID == "" {
		http.Error(w, "missing slack_user_id", http.StatusBadRequest)
		return
	}
	if err := (*h.linkRepo).RevokeGitHub(r.Context(), slackUserID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listAllLinks fetches all links from DB (no tokens exposed).
func (h *SlackLinkHandler) listAllLinks(ctx context.Context) ([]slackLinkResponse, error) {
	links, err := (*h.linkRepo).ListAll(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]slackLinkResponse, 0, len(links))
	for _, l := range links {
		result = append(result, slackLinkResponse{
			SlackUserID: l.SlackUserID,
			Email:       l.Email,
			HasGitHub:   l.HasGitHub,
			CreatedAt:   l.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   l.UpdatedAt.Format(time.RFC3339),
		})
	}
	return result, nil
}

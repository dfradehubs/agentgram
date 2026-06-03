package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	slackapi "github.com/slack-go/slack"

	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	slackpkg "github.com/dfradehubs/agentgram-api/internal/slack"
	"go.uber.org/zap"
)

// AdminSlackHandler handles admin CRUD for Slack integrations.
type AdminSlackHandler struct {
	slackRepo  repository.SlackIntegrationRepository
	auditRepo  repository.AuditRepository
	botManager *slackpkg.BotManager
	logger     *zap.Logger
}

// NewAdminSlackHandler creates a new AdminSlackHandler.
func NewAdminSlackHandler(slackRepo repository.SlackIntegrationRepository, auditRepo repository.AuditRepository, botManager *slackpkg.BotManager, logger *zap.Logger) *AdminSlackHandler {
	return &AdminSlackHandler{
		slackRepo:  slackRepo,
		auditRepo:  auditRepo,
		botManager: botManager,
		logger:     logger,
	}
}

type upsertSlackRequest struct {
	BotToken string `json:"bot_token"`
	AppToken string `json:"app_token"`
	Enabled  bool   `json:"enabled"`
}

type testSlackRequest struct {
	BotToken string `json:"bot_token"`
	AppToken string `json:"app_token"`
}

type testSlackResponse struct {
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
}

// Get returns the Slack integration for an agent (no tokens).
func (h *AdminSlackHandler) Get(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	integ, err := h.slackRepo.Get(r.Context(), agentID)
	if err != nil {
		h.logger.Error("failed to get slack integration", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if integ == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"agent_id": agentID, "enabled": false})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(integ.ToResponse())
}

// Upsert creates or updates a Slack integration.
func (h *AdminSlackHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	var req upsertSlackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Get existing to detect changes
	existing, _ := h.slackRepo.Get(r.Context(), agentID)

	integ := &models.SlackIntegration{
		AgentID:  agentID,
		BotToken: req.BotToken,
		AppToken: req.AppToken,
		Enabled:  req.Enabled,
	}

	// Preserve existing tokens if not provided (partial update)
	if existing != nil {
		if integ.BotToken == "" {
			integ.BotToken = existing.BotToken
		}
		if integ.AppToken == "" {
			integ.AppToken = existing.AppToken
		}
		integ.WorkspaceID = existing.WorkspaceID
		integ.WorkspaceName = existing.WorkspaceName
	}

	if err := h.slackRepo.Upsert(r.Context(), integ); err != nil {
		h.logger.Error("failed to upsert slack integration", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Manage bot lifecycle
	if req.Enabled {
		tokensChanged := existing != nil && (req.BotToken != "" || req.AppToken != "")
		wasEnabled := existing != nil && existing.Enabled
		if wasEnabled && tokensChanged {
			h.botManager.RestartBot(agentID)
		} else {
			h.botManager.StartBot(agentID)
		}
	} else {
		h.botManager.StopBot(agentID)
	}

	// Audit log
	claims := middleware.GetUserFromContext(r.Context())
	if claims != nil {
		h.auditRepo.Log(r.Context(), &models.AuditEntry{
			UserEmail:    claims.GetEmail(),
			Action:       "upsert_slack",
			ResourceType: "agent",
			ResourceID:   agentID,
		})
	}

	// Return updated integration
	updated, _ := h.slackRepo.Get(r.Context(), agentID)
	if updated == nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated.ToResponse())
}

// Delete removes a Slack integration.
func (h *AdminSlackHandler) Delete(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	h.botManager.StopBot(agentID)

	if err := h.slackRepo.Delete(r.Context(), agentID); err != nil {
		h.logger.Error("failed to delete slack integration", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	claims := middleware.GetUserFromContext(r.Context())
	if claims != nil {
		h.auditRepo.Log(r.Context(), &models.AuditEntry{
			UserEmail:    claims.GetEmail(),
			Action:       "delete_slack",
			ResourceType: "agent",
			ResourceID:   agentID,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// TestConnection validates Slack tokens and returns workspace info.
func (h *AdminSlackHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	var req testSlackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.BotToken == "" {
		http.Error(w, "bot_token is required", http.StatusBadRequest)
		return
	}

	client := slackapi.New(req.BotToken)
	resp, err := client.AuthTestContext(r.Context())
	if err != nil {
		h.logger.Warn("slack connection test failed", zap.Error(err))
		http.Error(w, "failed to connect to Slack: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(testSlackResponse{
		WorkspaceID:   resp.TeamID,
		WorkspaceName: resp.Team,
	})
}

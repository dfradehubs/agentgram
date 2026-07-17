package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/settings"
	"go.uber.org/zap"
)

// AdminSettingsHandler serves the admin "General Configuration" panel:
// runtime-editable operational settings backed by app_settings.
type AdminSettingsHandler struct {
	service   *settings.Service
	repo      settings.Repository
	auditRepo repository.AuditRepository
	logger    *zap.Logger
}

// NewAdminSettingsHandler creates the handler.
func NewAdminSettingsHandler(service *settings.Service, repo settings.Repository, auditRepo repository.AuditRepository, logger *zap.Logger) *AdminSettingsHandler {
	return &AdminSettingsHandler{service: service, repo: repo, auditRepo: auditRepo, logger: logger}
}

// settingItem is one setting's definition plus its current effective value.
type settingItem struct {
	settings.Def
	Value string `json:"value"`
}

// ListSettings handles GET /api/admin/settings — every known setting with its
// definition (label, type, default, bounds) and current effective value.
func (h *AdminSettingsHandler) ListSettings(w http.ResponseWriter, r *http.Request) {
	effective := h.service.Effective()
	items := make([]settingItem, 0, len(settings.Defs))
	for _, d := range settings.Defs {
		items = append(items, settingItem{Def: d, Value: effective[d.Key]})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"settings": items})
}

// UpdateSettings handles PUT /api/admin/settings — validates and persists a
// map of key→value overrides, then reloads the in-memory cache.
func (h *AdminSettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Validate everything before writing anything.
	for key, value := range req {
		if msg := settings.Validate(key, value); msg != "" {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, msg), http.StatusBadRequest)
			return
		}
	}

	for key, value := range req {
		if err := h.repo.Set(r.Context(), key, value); err != nil {
			h.logger.Error("failed to persist setting", zap.String("key", key), zap.Error(err))
			http.Error(w, `{"error":"failed to save settings"}`, http.StatusInternalServerError)
			return
		}
	}

	if err := h.service.Reload(r.Context()); err != nil {
		h.logger.Error("failed to reload settings", zap.Error(err))
	}

	if claims := middleware.GetUserFromContext(r.Context()); claims != nil && h.auditRepo != nil {
		h.auditRepo.Log(context.Background(), &models.AuditEntry{
			UserEmail:    claims.GetEmail(),
			Action:       "update",
			ResourceType: "settings",
			ResourceID:   "app_settings",
		})
	}

	h.ListSettings(w, r)
}

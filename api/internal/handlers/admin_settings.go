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
// runtime-editable operational settings backed by runtime_config.
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
// @Summary List runtime settings
// @Description Returns every known runtime setting with its definition and current effective value.
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/admin/settings [get]
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
// @Summary Update runtime settings
// @Description Validates and atomically persists key→value overrides, then reloads the cache.
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body map[string]string true "Key/value overrides"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} models.ErrorResponse
// @Router /api/admin/settings [put]
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

	if err := h.repo.SetMany(r.Context(), req); err != nil {
		h.logger.Error("failed to persist settings", zap.Error(err))
		http.Error(w, `{"error":"failed to save settings"}`, http.StatusInternalServerError)
		return
	}

	// The write committed, but if the in-memory cache can't be refreshed this
	// instance would keep serving stale values — report failure rather than a
	// misleading success.
	if err := h.service.Reload(r.Context()); err != nil {
		h.logger.Error("failed to reload settings after save", zap.Error(err))
		http.Error(w, `{"error":"settings saved but reload failed; retry"}`, http.StatusInternalServerError)
		return
	}

	if claims := middleware.GetUserFromContext(r.Context()); claims != nil && h.auditRepo != nil {
		h.auditRepo.Log(context.Background(), &models.AuditEntry{
			UserEmail:    claims.GetEmail(),
			Action:       "update",
			ResourceType: "settings",
			ResourceID:   "runtime_config",
		})
	}

	h.ListSettings(w, r)
}

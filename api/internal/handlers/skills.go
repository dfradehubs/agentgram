package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

// SkillsHandler serves the user-facing, read-only view of skills.
// Skill creation/editing/deletion is admin-only (see AdminSkillsHandler).
type SkillsHandler struct {
	skillRepo repository.SkillRepository
	logger    *zap.Logger
}

// NewSkillsHandler creates a new user-facing skills handler.
func NewSkillsHandler(skillRepo repository.SkillRepository, logger *zap.Logger) *SkillsHandler {
	return &SkillsHandler{skillRepo: skillRepo, logger: logger}
}

// UserSkillResponse is the list view — no content (loaded on demand via GetSkill).
type UserSkillResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ListSkills handles GET /api/skills — the skills this user may read.
func (h *SkillsHandler) ListSkills(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	all, err := h.skillRepo.List(r.Context())
	if err != nil {
		h.logger.Error("list skills failed", zap.Error(err))
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	email, groups := claims.GetEmail(), claims.GetGroups()
	responses := make([]UserSkillResponse, 0, len(all))
	for _, s := range all {
		if s.HasAccess(email, groups) {
			responses = append(responses, UserSkillResponse{ID: s.ID, Name: s.Name, Description: s.Description})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"skills": responses})
}

// GetSkill handles GET /api/skills/{id} — the full content, if the user has access.
func (h *SkillsHandler) GetSkill(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	skill, err := h.skillRepo.Get(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"skill not found"}`, http.StatusNotFound)
		return
	}

	if !skill.HasAccess(claims.GetEmail(), claims.GetGroups()) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":          skill.ID,
		"name":        skill.Name,
		"description": skill.Description,
		"content":     skill.Content,
	})
}

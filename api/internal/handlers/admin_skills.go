package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

// AdminSkillsHandler handles admin CRUD for skills.
type AdminSkillsHandler struct {
	skillRepo repository.SkillRepository
	auditRepo repository.AuditRepository
	logger    *zap.Logger
}

// NewAdminSkillsHandler creates a new admin skills handler.
func NewAdminSkillsHandler(skillRepo repository.SkillRepository, auditRepo repository.AuditRepository, logger *zap.Logger) *AdminSkillsHandler {
	return &AdminSkillsHandler{skillRepo: skillRepo, auditRepo: auditRepo, logger: logger}
}

// AdminSkillRequest is the request body for creating/updating skills.
type AdminSkillRequest struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Content       string   `json:"content"`
	AllowedUsers  []string `json:"allowed_users"`
	AllowedGroups []string `json:"allowed_groups"`
}

// ListSkills handles GET /api/admin/skills
func (h *AdminSkillsHandler) ListSkills(w http.ResponseWriter, r *http.Request) {
	skills, err := h.skillRepo.List(r.Context())
	if err != nil {
		h.logger.Error("list skills failed", zap.Error(err))
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"skills": skills})
}

// GetSkill handles GET /api/admin/skills/{id}
func (h *AdminSkillsHandler) GetSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	skill, err := h.skillRepo.Get(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"skill not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(skill)
}

// CreateSkill handles POST /api/admin/skills
func (h *AdminSkillsHandler) CreateSkill(w http.ResponseWriter, r *http.Request) {
	var req AdminSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.ID == "" || req.Name == "" || req.Content == "" {
		http.Error(w, `{"error":"id, name, and content are required"}`, http.StatusBadRequest)
		return
	}
	if err := models.ValidateSkillID(req.ID); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}

	skill := &models.Skill{
		ID:            req.ID,
		Name:          req.Name,
		Description:   req.Description,
		Content:       req.Content,
		AllowedUsers:  req.AllowedUsers,
		AllowedGroups: req.AllowedGroups,
	}
	if err := h.skillRepo.Create(r.Context(), skill); err != nil {
		h.logger.Error("create skill failed", zap.Error(err))
		http.Error(w, `{"error":"failed to create skill"}`, http.StatusInternalServerError)
		return
	}

	h.audit(r, "create", req.ID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(skill)
}

// UpdateSkill handles PUT /api/admin/skills/{id}
func (h *AdminSkillsHandler) UpdateSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req AdminSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Content == "" {
		http.Error(w, `{"error":"name and content are required"}`, http.StatusBadRequest)
		return
	}

	skill := &models.Skill{
		ID:            id,
		Name:          req.Name,
		Description:   req.Description,
		Content:       req.Content,
		AllowedUsers:  req.AllowedUsers,
		AllowedGroups: req.AllowedGroups,
	}
	if err := h.skillRepo.Update(r.Context(), skill); err != nil {
		h.logger.Error("update skill failed", zap.Error(err))
		http.Error(w, `{"error":"failed to update skill"}`, http.StatusInternalServerError)
		return
	}

	h.audit(r, "update", id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(skill)
}

// DeleteSkill handles DELETE /api/admin/skills/{id}
func (h *AdminSkillsHandler) DeleteSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.skillRepo.Delete(r.Context(), id); err != nil {
		http.Error(w, `{"error":"skill not found"}`, http.StatusNotFound)
		return
	}
	h.audit(r, "delete", id)
	w.WriteHeader(http.StatusNoContent)
}

// UpdateSkillPermissions handles PUT /api/admin/skills/{id}/permissions
func (h *AdminSkillsHandler) UpdateSkillPermissions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req PermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if err := h.skillRepo.UpdatePermissions(r.Context(), id, req.AllowedUsers, req.AllowedGroups); err != nil {
		h.logger.Error("update skill permissions failed", zap.Error(err))
		http.Error(w, `{"error":"failed to update permissions"}`, http.StatusInternalServerError)
		return
	}

	h.audit(r, "update_permissions", id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"allowed_users":  req.AllowedUsers,
		"allowed_groups": req.AllowedGroups,
	})
}

func (h *AdminSkillsHandler) audit(r *http.Request, action, id string) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil || h.auditRepo == nil {
		return
	}
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       action,
		ResourceType: "skill",
		ResourceID:   id,
	})
}

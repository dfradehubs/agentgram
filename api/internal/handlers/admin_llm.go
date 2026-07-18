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

var validLLMRoles = map[string]struct{}{
	"chat":            {},
	"summarizer":      {},
	"file_processor":  {},
	"chart_extractor": {},
	"session_namer":   {},
	"moderator":       {},
}

func validateLLMRole(role string) string {
	if _, ok := validLLMRoles[role]; !ok {
		return "role must be one of: chat, summarizer, file_processor, chart_extractor, session_namer, moderator"
	}
	return ""
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

// AdminLLMHandler handles admin CRUD for LLM models
type AdminLLMHandler struct {
	llmRepo   repository.LLMModelRepository
	auditRepo repository.AuditRepository
	logger    *zap.Logger
}

// NewAdminLLMHandler creates a new admin LLM handler
func NewAdminLLMHandler(llmRepo repository.LLMModelRepository, auditRepo repository.AuditRepository, logger *zap.Logger) *AdminLLMHandler {
	return &AdminLLMHandler{
		llmRepo:   llmRepo,
		auditRepo: auditRepo,
		logger:    logger,
	}
}

// AdminLLMRequest is the request body for creating/updating LLM models
type AdminLLMRequest struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
	Role       string `json:"role"`
	Enabled    *bool  `json:"enabled"`
	IsDefault  bool   `json:"is_default"`
}

// ListLLMModels handles GET /api/admin/llm
func (h *AdminLLMHandler) ListLLMModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.llmRepo.List(r.Context())
	if err != nil {
		h.logger.Error("list llm models failed", zap.Error(err))
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Mask API keys in response
	type safeModel struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		ProviderID      string `json:"provider_id"`
		ProviderName    string `json:"provider_name"`
		ProviderType    string `json:"provider_type"`
		ProviderEnabled bool   `json:"provider_enabled"`
		Model           string `json:"model"`
		Role            string `json:"role"`
		Enabled         bool   `json:"enabled"`
		IsDefault       bool   `json:"is_default"`
	}

	safe := make([]safeModel, len(models))
	for i, m := range models {
		safe[i] = safeModel{
			ID: m.ID, Name: m.Name, ProviderID: m.ProviderID,
			ProviderName: m.ProviderName, ProviderType: m.ProviderType,
			ProviderEnabled: m.ProviderEnabled,
			Model:           m.Model, Role: m.Role, Enabled: m.Enabled, IsDefault: m.IsDefault,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"models": safe})
}

// GetLLMModel handles GET /api/admin/llm/{id}
func (h *AdminLLMHandler) GetLLMModel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	model, err := h.llmRepo.Get(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"llm model not found"}`, http.StatusNotFound)
		return
	}

	model.APIKey = ""
	model.Endpoint = ""

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(model)
}

// CreateLLMModel handles POST /api/admin/llm
func (h *AdminLLMHandler) CreateLLMModel(w http.ResponseWriter, r *http.Request) {
	var req AdminLLMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.ID == "" || req.Name == "" || req.ProviderID == "" || req.Model == "" {
		http.Error(w, `{"error":"id, name, provider_id, and model are required"}`, http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = "chat"
	}
	if errMsg := validateLLMRole(req.Role); errMsg != "" {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, errMsg), http.StatusBadRequest)
		return
	}
	model := &models.LLMModel{
		ID:         req.ID,
		Name:       req.Name,
		ProviderID: req.ProviderID,
		Model:      req.Model,
		Role:       req.Role,
		Enabled:    boolValue(req.Enabled, true),
		IsDefault:  req.IsDefault,
	}

	if err := h.llmRepo.Create(r.Context(), model); err != nil {
		h.logger.Error("create llm model failed", zap.Error(err))
		http.Error(w, `{"error":"failed to create llm model"}`, http.StatusInternalServerError)
		return
	}

	claims := middleware.GetUserFromContext(r.Context())
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       "create",
		ResourceType: "llm_model",
		ResourceID:   req.ID,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(model)
}

// UpdateLLMModel handles PUT /api/admin/llm/{id}
func (h *AdminLLMHandler) UpdateLLMModel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req AdminLLMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	existing, err := h.llmRepo.Get(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"llm model not found"}`, http.StatusNotFound)
		return
	}

	if req.Role == "" {
		req.Role = "chat"
	}
	if errMsg := validateLLMRole(req.Role); errMsg != "" {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, errMsg), http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.ProviderID == "" || req.Model == "" {
		http.Error(w, `{"error":"name, provider_id, and model are required"}`, http.StatusBadRequest)
		return
	}

	model := &models.LLMModel{
		ID:         id,
		Name:       req.Name,
		ProviderID: req.ProviderID,
		Model:      req.Model,
		Role:       req.Role,
		Enabled:    boolValue(req.Enabled, existing.Enabled),
		IsDefault:  req.IsDefault,
	}

	if err := h.llmRepo.Update(r.Context(), model); err != nil {
		h.logger.Error("update llm model failed", zap.Error(err))
		http.Error(w, `{"error":"failed to update llm model"}`, http.StatusInternalServerError)
		return
	}

	claims := middleware.GetUserFromContext(r.Context())
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       "update",
		ResourceType: "llm_model",
		ResourceID:   id,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(model)
}

// DeleteLLMModel handles DELETE /api/admin/llm/{id}
func (h *AdminLLMHandler) DeleteLLMModel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.llmRepo.Delete(r.Context(), id); err != nil {
		http.Error(w, `{"error":"llm model not found"}`, http.StatusNotFound)
		return
	}

	claims := middleware.GetUserFromContext(r.Context())
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       "delete",
		ResourceType: "llm_model",
		ResourceID:   id,
	})

	w.WriteHeader(http.StatusNoContent)
}

package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/security"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type AdminProviderHandler struct {
	providerRepo repository.LLMProviderRepository
	auditRepo    repository.AuditRepository
	logger       *zap.Logger
}

type AdminProviderRequest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	APIKey       string `json:"api_key"`
	Endpoint     string `json:"endpoint"`
	Enabled      *bool  `json:"enabled"`
	ClearAPIKey  bool   `json:"clear_api_key"`
}

func NewAdminProviderHandler(providerRepo repository.LLMProviderRepository, auditRepo repository.AuditRepository, logger *zap.Logger) *AdminProviderHandler {
	return &AdminProviderHandler{providerRepo: providerRepo, auditRepo: auditRepo, logger: logger}
}

func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) > 8 {
		return key[:4] + "****" + key[len(key)-4:]
	}
	return "****"
}

func isMaskedAPIKey(key string) bool {
	return key == "****" || (len(key) > 8 && key[4:8] == "****")
}

func validateProvider(provider *models.LLMProvider) string {
	switch provider.ProviderType {
	case "anthropic", "google", "openai":
		if provider.APIKey == "" {
			return "api_key is required for native providers"
		}
		if provider.Endpoint != "" {
			return "endpoint is only supported by Custom Endpoint providers"
		}
	case "custom":
		if provider.Endpoint == "" {
			return "endpoint is required for Custom Endpoint providers"
		}
		parsed, err := url.Parse(provider.Endpoint)
		if err != nil || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
			return "endpoint must not contain credentials or fragments"
		}
		if err := security.ValidateEndpointURL(provider.Endpoint); err != nil {
			return fmt.Sprintf("unsafe endpoint: %v", err)
		}
	default:
		return "provider_type must be one of: anthropic, google, openai, custom"
	}
	return ""
}

func safeProvider(provider *models.LLMProvider) *models.LLMProvider {
	copy := *provider
	copy.APIKey = maskAPIKey(copy.APIKey)
	return &copy
}

func (h *AdminProviderHandler) List(w http.ResponseWriter, r *http.Request) {
	providers, err := h.providerRepo.List(r.Context())
	if err != nil {
		h.logger.Error("list llm providers failed", zap.Error(err))
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	safe := make([]*models.LLMProvider, len(providers))
	for i, provider := range providers {
		safe[i] = safeProvider(provider)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"providers": safe})
}

func (h *AdminProviderHandler) Get(w http.ResponseWriter, r *http.Request) {
	provider, err := h.providerRepo.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"provider not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(safeProvider(provider))
}

func (h *AdminProviderHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req AdminProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	provider := &models.LLMProvider{
		ID: strings.TrimSpace(req.ID), Name: strings.TrimSpace(req.Name),
		ProviderType: strings.ToLower(strings.TrimSpace(req.ProviderType)),
		APIKey:       strings.TrimSpace(req.APIKey), Endpoint: strings.TrimSpace(req.Endpoint),
		Enabled: boolValue(req.Enabled, true),
	}
	if provider.ID == "" || provider.Name == "" {
		http.Error(w, `{"error":"id and name are required"}`, http.StatusBadRequest)
		return
	}
	if message := validateProvider(provider); message != "" {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, message), http.StatusBadRequest)
		return
	}
	if err := h.providerRepo.Create(r.Context(), provider); err != nil {
		h.logger.Error("create llm provider failed", zap.Error(err))
		http.Error(w, `{"error":"failed to create provider"}`, http.StatusInternalServerError)
		return
	}
	h.audit(r, "create", provider.ID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(safeProvider(provider))
}

func (h *AdminProviderHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.providerRepo.Get(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"provider not found"}`, http.StatusNotFound)
		return
	}
	var req AdminProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(req.APIKey)
	if req.ClearAPIKey {
		key = ""
	} else if key == "" || isMaskedAPIKey(key) {
		key = existing.APIKey
	}
	provider := &models.LLMProvider{
		ID: id, Name: strings.TrimSpace(req.Name),
		ProviderType: strings.ToLower(strings.TrimSpace(req.ProviderType)),
		APIKey:       key, Endpoint: strings.TrimSpace(req.Endpoint),
		Enabled: boolValue(req.Enabled, existing.Enabled),
	}
	if provider.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}
	if message := validateProvider(provider); message != "" {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, message), http.StatusBadRequest)
		return
	}
	if err := h.providerRepo.Update(r.Context(), provider); err != nil {
		h.logger.Error("update llm provider failed", zap.Error(err))
		http.Error(w, `{"error":"failed to update provider"}`, http.StatusInternalServerError)
		return
	}
	h.audit(r, "update", id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(safeProvider(provider))
}

func (h *AdminProviderHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.providerRepo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, repository.ErrProviderInUse) {
			http.Error(w, `{"error":"provider is still used by one or more LLM models"}`, http.StatusConflict)
			return
		}
		http.Error(w, `{"error":"provider not found"}`, http.StatusNotFound)
		return
	}
	h.audit(r, "delete", id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminProviderHandler) audit(r *http.Request, action, id string) {
	claims := middleware.GetUserFromContext(r.Context())
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail: claims.GetEmail(), Action: action, ResourceType: "llm_provider", ResourceID: id,
	})
}

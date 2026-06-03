package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/dfradehubs/agentgram-api/internal/repository"
)

// ConfigHandler handles public configuration endpoints
type ConfigHandler struct {
	llmRepo repository.LLMModelRepository
}

// NewConfigHandler creates a new config handler
func NewConfigHandler(llmRepo repository.LLMModelRepository) *ConfigHandler {
	return &ConfigHandler{llmRepo: llmRepo}
}

// PublicConfigResponse is the response for GET /api/config
type PublicConfigResponse struct {
	Features        FeatureFlags     `json:"features"`
	AvailableModels []PublicLLMModel `json:"available_models,omitempty"`
}

// FeatureFlags exposes boolean feature flags to the frontend (never secrets)
type FeatureFlags struct {
	SummarizerEnabled bool `json:"summarizer_enabled"`
}

// PublicLLMModel is a safe-to-expose LLM model (no api_key)
type PublicLLMModel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Default  bool   `json:"default,omitempty"`
}

// GetConfig handles GET /api/config
// @Summary Get public configuration
// @Description Returns feature flags and available LLM models (without API keys)
// @Tags config
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Success 200 {object} PublicConfigResponse
// @Router /api/config [get]
func (h *ConfigHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if summarizer is enabled
	summarizers, _ := h.llmRepo.ListByRole(ctx, "summarizer")
	summarizerEnabled := len(summarizers) > 0

	resp := PublicConfigResponse{
		Features: FeatureFlags{
			SummarizerEnabled: summarizerEnabled,
		},
	}

	// Expose available chat LLM models (without api_key)
	chatModels, _ := h.llmRepo.ListByRole(ctx, "chat")
	for _, m := range chatModels {
		resp.AvailableModels = append(resp.AvailableModels, PublicLLMModel{
			ID:       m.ID,
			Name:     m.Name,
			Provider: m.Provider,
			Default:  m.IsDefault,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

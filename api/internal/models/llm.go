package models

import "time"

// LLMProvider stores shared credentials and connection settings for LLM models.
type LLMProvider struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ProviderType string    `json:"provider_type"`
	APIKey       string    `json:"api_key,omitempty"`
	Endpoint     string    `json:"endpoint,omitempty"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// LLMModel represents an LLM model stored in the database
type LLMModel struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	ProviderID      string    `json:"provider_id"`
	ProviderName    string    `json:"provider_name,omitempty"`
	ProviderType    string    `json:"provider_type,omitempty"`
	ProviderEnabled bool      `json:"provider_enabled"`
	Provider        string    `json:"provider"` // Effective runtime driver (custom uses openai).
	Model           string    `json:"model"`
	APIKey          string    `json:"api_key,omitempty"`
	Endpoint        string    `json:"endpoint,omitempty"`
	Role            string    `json:"role"`
	Enabled         bool      `json:"enabled"`
	IsDefault       bool      `json:"is_default"`
	// MaxTokens overrides the output token cap for this model's calls. 0 = auto:
	// the caller uses its built-in per-role default.
	MaxTokens int       `json:"max_tokens"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

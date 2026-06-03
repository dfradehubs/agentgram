package models

import "time"

// LLMModel represents an LLM model stored in the database
type LLMModel struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	APIKey    string    `json:"api_key,omitempty"`
	Role      string    `json:"role"`
	Enabled   bool      `json:"enabled"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

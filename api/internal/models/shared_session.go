package models

import "time"

// SharedSession represents a shared session link
type SharedSession struct {
	Token     string    `json:"token"`
	SessionID string    `json:"session_id"`
	AgentID   string    `json:"agent_id"`
	SharedBy  string    `json:"shared_by"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
}

// ShareRequest is the request body for creating a shared session link
type ShareRequest struct {
	ExpiresInHours int `json:"expires_in_hours,omitempty"` // default 168 (7 days), max 168
}

// ShareResponse is returned after creating a share link
type ShareResponse struct {
	Token     string    `json:"token"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SharedSessionInfo is returned when fetching share info (no messages)
type SharedSessionInfo struct {
	Token        string `json:"token"`
	AgentID      string `json:"agent_id"`
	AgentName    string `json:"agent_name"`
	SessionName  string `json:"session_name"`
	SharedBy     string `json:"shared_by"`
	MessageCount int    `json:"message_count"`
	ExpiresAt    string `json:"expires_at"`
}

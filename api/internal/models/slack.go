package models

import "time"

// SlackIntegration represents a Slack bot integration for an agent.
type SlackIntegration struct {
	ID            string    `json:"id"`
	AgentID       string    `json:"agent_id"`
	BotToken      string    `json:"-"`
	AppToken      string    `json:"-"`
	Enabled       bool      `json:"enabled"`
	WorkspaceID   string    `json:"workspace_id"`
	WorkspaceName string    `json:"workspace_name"`
	Status        string    `json:"status"`
	StatusMessage string    `json:"status_message"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// SlackUserLink represents a Slack user linked to a Keycloak identity via offline token.
type SlackUserLink struct {
	SlackUserID  string    `json:"slack_user_id"`
	Email        string    `json:"email"`
	RefreshToken string    `json:"-"`
	GitHubToken        string    `json:"-"`
	GitHubRefreshToken string    `json:"-"`
	HasGitHub          bool      `json:"has_github"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SlackIntegrationResponse is the safe response for admin API (no tokens).
type SlackIntegrationResponse struct {
	AgentID       string `json:"agent_id"`
	Enabled       bool   `json:"enabled"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	Status        string `json:"status"`
	StatusMessage string `json:"status_message"`
	HasBotToken   bool   `json:"has_bot_token"`
	HasAppToken   bool   `json:"has_app_token"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// ToResponse converts to a safe response (no tokens).
func (s *SlackIntegration) ToResponse() SlackIntegrationResponse {
	return SlackIntegrationResponse{
		AgentID:       s.AgentID,
		Enabled:       s.Enabled,
		WorkspaceID:   s.WorkspaceID,
		WorkspaceName: s.WorkspaceName,
		Status:        s.Status,
		StatusMessage: s.StatusMessage,
		HasBotToken:   s.BotToken != "",
		HasAppToken:   s.AppToken != "",
		CreatedAt:     s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     s.UpdatedAt.Format(time.RFC3339),
	}
}

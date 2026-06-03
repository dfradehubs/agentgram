package models

// AgentListResponse response for GET /api/agents
type AgentListResponse struct {
	Agents []AgentResponse `json:"agents"`
}

// ErrorResponse generic error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// HealthResponse response for health endpoints
type HealthResponse struct {
	Status  string                 `json:"status"`
	Details map[string]interface{} `json:"details,omitempty"`
}

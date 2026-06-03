package models

import "time"

// ChatEvent represents a single chat interaction for analytics
type ChatEvent struct {
	ID             string         `json:"id"`
	ResourceType   string         `json:"resource_type"`   // agent, custom_agent, mcp
	ResourceID     string         `json:"resource_id"`
	ResourceName   string         `json:"resource_name"`
	Protocol       string         `json:"protocol,omitempty"` // custom/a2a/adk (system agents only)
	UserEmail      string         `json:"user_email"`
	SessionID      string         `json:"session_id,omitempty"`
	Status         string         `json:"status"`     // ok, error
	ErrorType      string         `json:"error_type,omitempty"`
	ErrorMsg       string         `json:"error_msg,omitempty"`
	DurationMs     int            `json:"duration_ms"`
	TTFBMs         *int           `json:"ttfb_ms,omitempty"`
	MessageCount   int            `json:"message_count"`
	ToolCalls      []ToolCallInfo `json:"tool_calls,omitempty"`
	TokenUsage     *TokenUsage    `json:"token_usage,omitempty"`
	LLMModel       string         `json:"llm_model,omitempty"`
	SessionRotated bool           `json:"session_rotated"`
	Source         string         `json:"source"`          // web, slack
	CreatedAt      time.Time      `json:"created_at"`
}

// ToolCallInfo holds info about a single tool call for analytics
type ToolCallInfo struct {
	Name       string `json:"name"`
	DurationMs int    `json:"duration_ms,omitempty"`
}

// TokenUsage holds LLM token usage
type TokenUsage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
	Total  int `json:"total"`
}

// ResourceStats holds aggregate stats for a resource
type ResourceStats struct {
	TotalRequests    int64        `json:"total_requests"`
	SuccessCount     int64        `json:"success_count"`
	ErrorCount       int64        `json:"error_count"`
	ErrorRate        float64      `json:"error_rate"`
	AvgDurationMs    float64      `json:"avg_duration_ms"`
	P95DurationMs    float64      `json:"p95_duration_ms"`
	AvgTTFBMs        *float64     `json:"avg_ttfb_ms,omitempty"`
	TokenUsage       *TokenUsage  `json:"token_usage,omitempty"`
	UniqueUsers      int64        `json:"unique_users"`
	ContextRotations int64        `json:"context_rotations"`
	TotalToolCalls   int64        `json:"total_tool_calls"`
	LLMModel         *string      `json:"llm_model,omitempty"`
}

// TimelineBucket holds a single bucket in a timeline chart
type TimelineBucket struct {
	Timestamp    time.Time `json:"timestamp"`
	Requests     int64     `json:"requests"`
	Errors       int64     `json:"errors"`
	AvgDuration  float64   `json:"avg_duration_ms"`
	AvgTTFB      *float64  `json:"avg_ttfb_ms,omitempty"`
}

// UserStat holds per-user statistics
type UserStat struct {
	UserEmail   string    `json:"user_email"`
	Requests    int64     `json:"requests"`
	Errors      int64     `json:"errors"`
	LastAccess  time.Time `json:"last_access"`
}

// ErrorStat holds error statistics
type ErrorStat struct {
	ErrorType string `json:"error_type"`
	Count     int64  `json:"count"`
	LastSeen  time.Time `json:"last_seen"`
	LastMsg   string    `json:"last_msg,omitempty"`
}

// GlobalStats holds global metrics overview
type GlobalStats struct {
	TotalRequests int64        `json:"total_requests"`
	SuccessCount  int64        `json:"success_count"`
	ErrorCount    int64        `json:"error_count"`
	ErrorRate     float64      `json:"error_rate"`
	AvgDurationMs float64      `json:"avg_duration_ms"`
	P95DurationMs float64      `json:"p95_duration_ms"`
	UniqueUsers   int64        `json:"unique_users"`
	ActiveAgents  int64        `json:"active_agents"`
	TokenUsage    *TokenUsage  `json:"token_usage,omitempty"`
}

// UserDetailStats holds detailed stats for a specific user
type UserDetailStats struct {
	TotalRequests int64        `json:"total_requests"`
	ErrorRate     float64      `json:"error_rate"`
	AvgDurationMs float64      `json:"avg_duration_ms"`
	P95DurationMs float64      `json:"p95_duration_ms"`
	TokenUsage    *TokenUsage  `json:"token_usage,omitempty"`
	ActiveAgents  int64        `json:"active_agents"`
}

// ErrorEvent holds a single error event with full details
type ErrorEvent struct {
	ID           string    `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	ResourceName string    `json:"resource_name"`
	ErrorType    string    `json:"error_type"`
	ErrorMsg     string    `json:"error_msg"`
	DurationMs   int       `json:"duration_ms"`
	UserEmail    string    `json:"user_email"`
}

// ResourceRanking holds a resource's ranking info for the top resources view
type ResourceRanking struct {
	ResourceType  string  `json:"resource_type"`
	ResourceID    string  `json:"resource_id"`
	ResourceName  string  `json:"resource_name"`
	Requests      int64   `json:"requests"`
	ErrorRate     float64 `json:"error_rate"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
}

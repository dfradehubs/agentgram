package mcpserver

import (
	"encoding/json"
	"fmt"

	"strings"

	"github.com/dfradehubs/agentgram-api/internal/agents"
	"github.com/dfradehubs/agentgram-api/internal/mcp"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/service"
	"go.uber.org/zap"
)

const (
	// ProtocolVersion is the MCP protocol version we implement
	ProtocolVersion = "2025-03-26"
	// ServerName identifies this MCP server
	ServerName = "agentgram"
	// ServerVersion is the version of this MCP server
	ServerVersion = "1.0.0"
)

// jsonRPCRequest represents a JSON-RPC 2.0 request
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCResponse represents a JSON-RPC 2.0 response
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError represents a JSON-RPC 2.0 error
type jsonRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC error codes
const (
	errCodeParse          = -32700
	errCodeInvalidRequest = -32600
	errCodeMethodNotFound = -32601
	errCodeInvalidParams  = -32602
	errCodeInternal       = -32603
)

// mcpToolPrefix is used to namespace MCP server tools: mcp_{serverID}__{toolName}
const mcpToolPrefix = "mcp_"

// Server is the MCP protocol server that exposes agents and MCP server tools
type Server struct {
	registry    *agents.Registry
	mcpRegistry *mcp.Registry
	userService *service.UserService
	sessions    *SessionStore
	logger      *zap.Logger
}

// NewServer creates a new MCP server
func NewServer(registry *agents.Registry, mcpRegistry *mcp.Registry, userService *service.UserService, logger *zap.Logger) *Server {
	return &Server{
		registry:    registry,
		mcpRegistry: mcpRegistry,
		userService: userService,
		sessions:    NewSessionStore(),
		logger:      logger,
	}
}

// HandleMessage processes a JSON-RPC 2.0 message and returns the response.
// userEmail and userGroups come from the authenticated JWT.
func (s *Server) HandleMessage(raw []byte, userEmail string, userGroups []string, mcpSessionID string) ([]byte, string, error) {
	var req jsonRPCRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return s.marshalError(nil, errCodeParse, "parse error"), mcpSessionID, nil
	}

	if req.JSONRPC != "2.0" {
		return s.marshalError(req.ID, errCodeInvalidRequest, "invalid jsonrpc version"), mcpSessionID, nil
	}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		// Client acknowledgement — no response needed
		return nil, mcpSessionID, nil
	case "tools/list":
		return s.handleToolsList(req, userEmail, userGroups)
	case "tools/call":
		// Handled by the HTTP handler since it needs access to the proxy
		return s.marshalError(req.ID, errCodeInternal, "tools/call handled at HTTP layer"), mcpSessionID, nil
	case "ping":
		return s.marshalResult(req.ID, map[string]interface{}{}), mcpSessionID, nil
	default:
		return s.marshalError(req.ID, errCodeMethodNotFound, fmt.Sprintf("method not found: %s", req.Method)), mcpSessionID, nil
	}
}

// handleInitialize handles the MCP initialize handshake
func (s *Server) handleInitialize(req jsonRPCRequest) ([]byte, string, error) {
	sessionID := s.sessions.Create()

	result := map[string]interface{}{
		"protocolVersion": ProtocolVersion,
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{
				"listChanged": true,
			},
		},
		"serverInfo": map[string]interface{}{
			"name":    ServerName,
			"version": ServerVersion,
		},
	}

	s.logger.Info("MCP session initialized", zap.String("mcp_session_id", sessionID))
	return s.marshalResult(req.ID, result), sessionID, nil
}

// handleToolsList returns the list of available tools (agents + MCP servers) for this user
func (s *Server) handleToolsList(req jsonRPCRequest, userEmail string, userGroups []string) ([]byte, string, error) {
	agentList := s.registry.List()

	var tools []interface{}
	for _, agent := range agentList {
		if !agents.HasAccess(agent, userEmail, userGroups) {
			continue
		}

		tool := buildAgentTool(agent)
		tools = append(tools, tool)
	}

	// Add MCP server tools
	if s.mcpRegistry != nil {
		for _, server := range s.mcpRegistry.List() {
			if !mcp.HasAccess(server, userEmail, userGroups) {
				continue
			}
			status, _ := server.GetStatus()
			if status != "connected" {
				continue
			}
			for _, t := range server.GetTools() {
				tool := buildMCPServerTool(server, t)
				tools = append(tools, tool)
			}
		}
	}

	// Add utility tools
	tools = append(tools, buildListAgentsTool())

	result := map[string]interface{}{
		"tools": tools,
	}

	return s.marshalResult(req.ID, result), "", nil
}

// buildAgentTool creates an MCP tool definition from an agent
func buildAgentTool(agent *models.Agent) map[string]interface{} {
	toolName := "ask_" + agent.ID

	return map[string]interface{}{
		"name":        toolName,
		"description": fmt.Sprintf("[%s] %s", agent.Name, agent.Description),
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"question": map[string]interface{}{
					"type":        "string",
					"description": "The question or task to send to the agent",
				},
				"session_id": map[string]interface{}{
					"type":        "string",
					"description": "Optional session ID to continue a previous conversation. Omit to start a new conversation.",
				},
			},
			"required": []string{"question"},
		},
	}
}

// buildListAgentsTool creates the list_agents utility tool
func buildListAgentsTool() map[string]interface{} {
	return map[string]interface{}{
		"name":        "list_agents",
		"description": "List all available agents and their descriptions",
		"inputSchema": map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
}

// buildMCPServerTool creates an MCP tool definition from an MCP server tool.
// Tool name format: mcp_{serverID}__{toolName}
func buildMCPServerTool(server *mcp.ServerInfo, tool mcp.Tool) map[string]interface{} {
	toolName := fmt.Sprintf("%s%s__%s", mcpToolPrefix, server.Config.ID, tool.Name)

	description := tool.Description
	if description == "" {
		description = tool.Name
	}
	description = fmt.Sprintf("[MCP: %s] %s", server.Config.Name, description)

	result := map[string]interface{}{
		"name":        toolName,
		"description": description,
	}
	if tool.InputSchema != nil {
		result["inputSchema"] = tool.InputSchema
	} else {
		result["inputSchema"] = map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}
	return result
}

// GetAgentIDFromToolName extracts the agent ID from a tool name (ask_<agent-id>)
func GetAgentIDFromToolName(toolName string) (string, bool) {
	if len(toolName) > 4 && toolName[:4] == "ask_" {
		return toolName[4:], true
	}
	return "", false
}

// GetMCPToolFromName extracts the server ID and tool name from an MCP tool name (mcp_{serverID}__{toolName})
func GetMCPToolFromName(toolName string) (serverID, mcpToolName string, ok bool) {
	if !strings.HasPrefix(toolName, mcpToolPrefix) {
		return "", "", false
	}
	rest := toolName[len(mcpToolPrefix):]
	parts := strings.SplitN(rest, "__", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}


// marshalResult creates a JSON-RPC success response
func (s *Server) marshalResult(id json.RawMessage, result interface{}) []byte {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error("failed to marshal JSON-RPC response", zap.Error(err))
		return []byte(`{"jsonrpc":"2.0","error":{"code":-32603,"message":"internal marshal error"}}`)
	}
	return data
}

// marshalError creates a JSON-RPC error response
func (s *Server) marshalError(id json.RawMessage, code int, message string) []byte {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &jsonRPCError{
			Code:    code,
			Message: message,
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error("failed to marshal JSON-RPC error", zap.Error(err))
		return []byte(`{"jsonrpc":"2.0","error":{"code":-32603,"message":"internal marshal error"}}`)
	}
	return data
}

// MarshalToolResult creates a JSON-RPC response for a tools/call result
func (s *Server) MarshalToolResult(id json.RawMessage, text string, isError bool) []byte {
	content := []map[string]interface{}{
		{
			"type": "text",
			"text": text,
		},
	}
	result := map[string]interface{}{
		"content": content,
		"isError": isError,
	}
	return s.marshalResult(id, result)
}

// MarshalError exposes marshalError for the handler
func (s *Server) MarshalError(id json.RawMessage, code int, message string) []byte {
	return s.marshalError(id, code, message)
}

// ListAccessibleAgents returns agents the user can access (for list_agents tool)
func (s *Server) ListAccessibleAgents(userEmail string, userGroups []string) []models.AgentResponse {
	agentList := s.registry.List()
	var result []models.AgentResponse
	for _, agent := range agentList {
		if !agents.HasAccess(agent, userEmail, userGroups) {
			continue
		}
		result = append(result, agent.ToResponse())
	}
	return result
}


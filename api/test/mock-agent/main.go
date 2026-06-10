package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var rdb *redis.Client

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}

	rdb = redis.NewClient(&redis.Options{
		Addr: redisURL,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis at %s: %v", redisURL, err)
	}
	log.Printf("Connected to Redis at %s", redisURL)

	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc("/health", handleHealth)

	// REST streaming endpoint (SSE)
	mux.HandleFunc("/chat", handleRESTChat)
	mux.HandleFunc("/query/stream", handleRESTChat)

	// Sessions endpoints
	mux.HandleFunc("/api/sessions", handleSessions)
	mux.HandleFunc("/api/sessions/", handleSessionByID)

	// A2A endpoints
	mux.HandleFunc("/.well-known/agent-card.json", handleAgentCard)
	mux.HandleFunc("/", handleA2AJSONRPC)

	log.Printf("Mock agent starting on port %s", port)
	log.Printf("Endpoints:")
	log.Printf("  GET  /health - Health check")
	log.Printf("  POST /chat - REST streaming (SSE)")
	log.Printf("  POST /query/stream - REST streaming (SSE)")
	log.Printf("  GET  /api/sessions - List sessions")
	log.Printf("  GET  /api/sessions/{id} - Get session")
	log.Printf("  PATCH  /api/sessions/{id} - Rename session")
	log.Printf("  DELETE /api/sessions/{id} - Delete session")
	log.Printf("  GET  /.well-known/agent-card.json - A2A agent card")
	log.Printf("  POST / - A2A JSON-RPC")

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleRESTChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Log auth headers so e2e tests can assert what credential the gateway sent
	log.Printf("chat auth headers: Authorization=%q X-API-Key=%q",
		r.Header.Get("Authorization"), r.Header.Get("X-API-Key"))

	// Parse request - supports both formats:
	// Standard REST: {"query": "...", "conversation_id": "..."}
	// Messages:      {"messages": [...], "session_id": "..."}
	var req struct {
		Query    string `json:"query,omitempty"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		SessionID      string `json:"session_id,omitempty"`
		ConversationID string `json:"conversation_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Normalize session ID (conversation_id takes precedence from backend format)
	sessionID := req.SessionID
	if req.ConversationID != "" {
		sessionID = req.ConversationID
	}

	// Get user message from query or messages
	var userMessage string
	if req.Query != "" {
		userMessage = req.Query
	} else {
		for i := len(req.Messages) - 1; i >= 0; i-- {
			if req.Messages[i].Role == "user" {
				userMessage = req.Messages[i].Content
				break
			}
		}
	}

	log.Printf("REST Chat received: %s (session: %s)", userMessage, sessionID)

	// Get or create session
	ctx := r.Context()
	userID := getUserFromAuth(r)
	agentID := getAgentID(r)
	session := getOrCreateSession(ctx, sessionID, userID, agentID, userMessage)

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Generate response chunks
	response := fmt.Sprintf("Hello! I received your message: \"%s\". This is a mock response from the REST agent.", userMessage)
	words := strings.Fields(response)

	// Send start event
	sendSSE(w, flusher, map[string]interface{}{
		"type":       "start",
		"messageId":  fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		"session_id": session.SessionID,
	})

	// Simulate tool calls before the text response
	toolID := fmt.Sprintf("tool-%d", time.Now().UnixNano())
	sendSSEWithEvent(w, flusher, "tool_start", map[string]interface{}{
		"tool_id":   toolID,
		"tool_name": "SearchTool",
	})
	time.Sleep(100 * time.Millisecond)
	sendSSEWithEvent(w, flusher, "tool_input", map[string]interface{}{
		"tool_id":   toolID,
		"tool_name": "SearchTool",
		"args":      map[string]interface{}{"query": userMessage},
	})
	time.Sleep(300 * time.Millisecond)
	sendSSEWithEvent(w, flusher, "tool_result", map[string]interface{}{
		"tool_id":   toolID,
		"tool_name": "SearchTool",
		"result":    fmt.Sprintf("Found 3 results for: %s", userMessage),
		"is_error":  false,
	})

	// Send text response as content_delta events (after tool calls)
	for i, word := range words {
		time.Sleep(50 * time.Millisecond)
		chunk := word
		if i < len(words)-1 {
			chunk += " "
		}
		sendSSEWithEvent(w, flusher, "content_delta", map[string]interface{}{
			"text": chunk,
		})
	}

	// Send end event
	sendSSE(w, flusher, map[string]interface{}{
		"type":   "end",
		"status": "completed",
	})

	// Store assistant message in session
	addMessageToSession(ctx, session.SessionID, "user", userMessage)
	addMessageToSession(ctx, session.SessionID, "assistant", response)
}

func sendSSE(w http.ResponseWriter, flusher http.Flusher, data interface{}) {
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()
}

func sendSSEWithEvent(w http.ResponseWriter, flusher http.Flusher, eventName string, data interface{}) {
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, jsonData)
	flusher.Flush()
}

// ============== Sessions (Redis) ==============

type Session struct {
	SessionID    string    `json:"session_id"`
	SessionName  string    `json:"session_name"`
	UserID       string    `json:"user_id"`
	AppName      string    `json:"app_name"`
	CreatedAt    int64     `json:"created_at"`
	LastActivity int64     `json:"last_activity"`
	MessageCount int       `json:"message_count"`
	Messages     []Message `json:"messages,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func sessionKey(sessionID string) string {
	return "session:" + sessionID
}

func userSessionsKey(userID, agentID string) string {
	return "user_sessions:" + userID + ":" + agentID
}

func handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listSessions(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleSessionByID(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from path
	sessionID := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		getSession(w, r, sessionID)
	case http.MethodPatch:
		renameSession(w, r, sessionID)
	case http.MethodDelete:
		deleteSession(w, r, sessionID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func listSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserFromAuth(r)
	agentID := getAgentID(r)

	// Get all session IDs for this user+agent
	key := userSessionsKey(userID, agentID)
	sessionIDs, err := rdb.SMembers(ctx, key).Result()
	if err != nil {
		log.Printf("Error listing sessions from Redis: %v", err)
		sessionIDs = []string{}
	}

	userSessions := []Session{}
	for _, sid := range sessionIDs {
		data, err := rdb.Get(ctx, sessionKey(sid)).Result()
		if err != nil {
			continue
		}
		var s Session
		if err := json.Unmarshal([]byte(data), &s); err != nil {
			continue
		}
		// Return without messages
		s.Messages = nil
		userSessions = append(userSessions, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sessions": userSessions,
	})
}

func getSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	ctx := r.Context()
	userID := getUserFromAuth(r)

	data, err := rdb.Get(ctx, sessionKey(sessionID)).Result()
	if err == redis.Nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	var session Session
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	if session.UserID != userID && userID != "anonymous" {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(session)
}

func renameSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	ctx := r.Context()
	userID := getUserFromAuth(r)

	var req struct {
		SessionName string `json:"session_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	data, err := rdb.Get(ctx, sessionKey(sessionID)).Result()
	if err == redis.Nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	var session Session
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	if session.UserID != userID && userID != "anonymous" {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}

	session.SessionName = req.SessionName
	session.LastActivity = time.Now().Unix()

	saveSession(ctx, &session)

	// Return without messages
	sessionCopy := session
	sessionCopy.Messages = nil

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessionCopy)
}

func deleteSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	ctx := r.Context()
	userID := getUserFromAuth(r)

	data, err := rdb.Get(ctx, sessionKey(sessionID)).Result()
	if err == redis.Nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	var session Session
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	if session.UserID != userID && userID != "anonymous" {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}

	// Remove from Redis
	rdb.Del(ctx, sessionKey(sessionID))
	rdb.SRem(ctx, userSessionsKey(session.UserID, session.AppName), sessionID)

	w.WriteHeader(http.StatusNoContent)
}

func getUserFromAuth(r *http.Request) string {
	if email := r.Header.Get("X-User-Email"); email != "" {
		return email
	}
	return "anonymous"
}

func getAgentID(r *http.Request) string {
	if id := r.Header.Get("X-Agent-ID"); id != "" {
		return id
	}
	return "mock-agent"
}

func saveSession(ctx context.Context, session *Session) {
	data, err := json.Marshal(session)
	if err != nil {
		log.Printf("Error marshaling session: %v", err)
		return
	}
	if err := rdb.Set(ctx, sessionKey(session.SessionID), data, 0).Err(); err != nil {
		log.Printf("Error saving session to Redis: %v", err)
	}
}

func getOrCreateSession(ctx context.Context, sessionID, userID, agentID, firstMessage string) *Session {
	// If session ID provided and exists, return it
	if sessionID != "" {
		data, err := rdb.Get(ctx, sessionKey(sessionID)).Result()
		if err == nil {
			var session Session
			if err := json.Unmarshal([]byte(data), &session); err == nil {
				return &session
			}
		}
	}

	// Create new session
	now := time.Now()
	newSessionID := sessionID
	if newSessionID == "" {
		newSessionID = fmt.Sprintf("session-%d", now.UnixNano())
	}

	// Generate session name from first message
	sessionName := firstMessage
	if len(sessionName) > 50 {
		sessionName = sessionName[:50] + "..."
	}

	session := &Session{
		SessionID:    newSessionID,
		SessionName:  sessionName,
		UserID:       userID,
		AppName:      agentID,
		CreatedAt:    now.Unix(),
		LastActivity: now.Unix(),
		MessageCount: 0,
		Messages:     []Message{},
	}

	saveSession(ctx, session)
	rdb.SAdd(ctx, userSessionsKey(userID, agentID), newSessionID)

	log.Printf("Created new session: %s for user: %s agent: %s", newSessionID, userID, agentID)
	return session
}

func addMessageToSession(ctx context.Context, sessionID, role, content string) {
	data, err := rdb.Get(ctx, sessionKey(sessionID)).Result()
	if err != nil {
		return
	}

	var session Session
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return
	}

	session.Messages = append(session.Messages, Message{
		Role:    role,
		Content: content,
	})
	session.MessageCount = len(session.Messages)
	session.LastActivity = time.Now().Unix()

	saveSession(ctx, &session)
}

// ============== A2A ==============

func handleAgentCard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":        "Mock A2A Agent",
		"description": "A mock agent for testing A2A protocol",
		"url":         "http://mock-agent:9000",
		"version":     "1.0.0",
		"capabilities": map[string]interface{}{
			"streaming": false,
			"pushNotifications": map[string]bool{
				"stateChanges": false,
			},
		},
		"skills": []map[string]string{
			{
				"id":          "chat",
				"name":        "Chat",
				"description": "General chat capability",
			},
		},
	})
}

// A2A task storage (in-memory for testing)
var tasks = make(map[string]*a2aTask)

type a2aTask struct {
	ID        string
	Status    string
	Message   string
	Artifacts []map[string]interface{}
	CreatedAt time.Time
}

func handleA2AJSONRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var rpcReq struct {
		JSONRPC string                 `json:"jsonrpc"`
		Method  string                 `json:"method"`
		ID      interface{}            `json:"id"`
		Params  map[string]interface{} `json:"params"`
	}

	if err := json.NewDecoder(r.Body).Decode(&rpcReq); err != nil {
		sendJSONRPCError(w, nil, -32700, "Parse error")
		return
	}

	log.Printf("A2A JSON-RPC method: %s", rpcReq.Method)

	w.Header().Set("Content-Type", "application/json")

	switch rpcReq.Method {
	case "message/stream":
		handleMessageStream(w, r, rpcReq.ID, rpcReq.Params)
	case "message/send":
		handleMessageSend(w, r, rpcReq.ID, rpcReq.Params)
	case "tasks/send":
		handleTasksSend(w, r, rpcReq.ID, rpcReq.Params)
	case "tasks/get":
		handleTasksGet(w, rpcReq.ID, rpcReq.Params)
	default:
		sendJSONRPCError(w, rpcReq.ID, -32601, "Method not found")
	}
}

func handleTasksSend(w http.ResponseWriter, r *http.Request, id interface{}, params map[string]interface{}) {
	// Extract message from params
	var message string
	if msgData, ok := params["message"].(map[string]interface{}); ok {
		if parts, ok := msgData["parts"].([]interface{}); ok && len(parts) > 0 {
			if part, ok := parts[0].(map[string]interface{}); ok {
				if text, ok := part["text"].(string); ok {
					message = text
				}
			}
		}
	}

	// Extract session_id from params (if provided)
	sessionID, _ := params["session_id"].(string)

	// Get or create session (same as REST handler)
	ctx := r.Context()
	userID := getUserFromAuth(r)
	agentID := getAgentID(r)
	session := getOrCreateSession(ctx, sessionID, userID, agentID, message)

	// Create task
	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
	responseText := fmt.Sprintf("Hello from A2A agent! I received: \"%s\"", message)
	task := &a2aTask{
		ID:        taskID,
		Status:    "working",
		Message:   message,
		CreatedAt: time.Now(),
	}
	tasks[taskID] = task

	log.Printf("A2A task created: %s with message: %s (session: %s)", taskID, message, session.SessionID)

	// Simulate async processing - complete after a short delay
	capturedSessionID := session.SessionID
	go func() {
		time.Sleep(500 * time.Millisecond)
		task.Status = "completed"
		task.Artifacts = []map[string]interface{}{
			{
				"index": 0,
				"parts": []map[string]interface{}{
					{
						"type": "text",
						"text": responseText,
					},
				},
			},
		}

		// Store messages in session
		bgCtx := context.Background()
		addMessageToSession(bgCtx, capturedSessionID, "user", message)
		addMessageToSession(bgCtx, capturedSessionID, "assistant", responseText)
	}()

	// Return task info with session_id
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]interface{}{
			"id":         taskID,
			"status":     task.Status,
			"session_id": session.SessionID,
		},
	})
}

func handleTasksGet(w http.ResponseWriter, id interface{}, params map[string]interface{}) {
	taskID, ok := params["id"].(string)
	if !ok {
		sendJSONRPCError(w, id, -32602, "Invalid params: missing task id")
		return
	}

	task, exists := tasks[taskID]
	if !exists {
		sendJSONRPCError(w, id, -32602, "Task not found")
		return
	}

	result := map[string]interface{}{
		"id":     task.ID,
		"status": task.Status,
	}

	if task.Status == "completed" && len(task.Artifacts) > 0 {
		result["artifacts"] = task.Artifacts
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

// ============== Standard A2A Protocol (message/send, message/stream) ==============

// extractMessageText extracts text from standard A2A message params
func extractMessageText(params map[string]interface{}) (string, string) {
	var message string
	var contextID string

	if msgData, ok := params["message"].(map[string]interface{}); ok {
		// Extract contextId
		if cid, ok := msgData["contextId"].(string); ok {
			contextID = cid
		}
		// Extract text from parts
		if parts, ok := msgData["parts"].([]interface{}); ok && len(parts) > 0 {
			if part, ok := parts[0].(map[string]interface{}); ok {
				if text, ok := part["text"].(string); ok {
					message = text
				}
			}
		}
	}

	return message, contextID
}

func handleMessageStream(w http.ResponseWriter, r *http.Request, id interface{}, params map[string]interface{}) {
	message, contextID := extractMessageText(params)

	// Get or create session
	ctx := r.Context()
	userID := getUserFromAuth(r)
	agentID := getAgentID(r)
	session := getOrCreateSession(ctx, contextID, userID, agentID, message)

	log.Printf("A2A message/stream received: %s (contextId: %s, session: %s)", message, contextID, session.SessionID)

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
	responseText := fmt.Sprintf("Hello from A2A agent! I received: \"%s\"", message)

	// Send status-update: working
	sendSSEJSONRPC(w, flusher, id, map[string]interface{}{
		"kind":      "status-update",
		"taskId":    taskID,
		"contextId": session.SessionID,
		"status": map[string]interface{}{
			"state": "working",
		},
		"final": false,
	})

	// Send artifact-update with the response text (word by word for streaming effect)
	words := strings.Fields(responseText)
	for i, word := range words {
		time.Sleep(30 * time.Millisecond)
		chunk := word
		if i < len(words)-1 {
			chunk += " "
		}
		sendSSEJSONRPC(w, flusher, id, map[string]interface{}{
			"kind":      "artifact-update",
			"taskId":    taskID,
			"contextId": session.SessionID,
			"artifact": map[string]interface{}{
				"artifactId": fmt.Sprintf("artifact-%s", taskID),
				"parts": []map[string]interface{}{
					{
						"kind": "text",
						"text": chunk,
					},
				},
			},
		})
	}

	// Send status-update: completed
	sendSSEJSONRPC(w, flusher, id, map[string]interface{}{
		"kind":      "status-update",
		"taskId":    taskID,
		"contextId": session.SessionID,
		"status": map[string]interface{}{
			"state": "completed",
		},
		"final": true,
	})

	// Store messages in session
	addMessageToSession(ctx, session.SessionID, "user", message)
	addMessageToSession(ctx, session.SessionID, "assistant", responseText)
}

func handleMessageSend(w http.ResponseWriter, r *http.Request, id interface{}, params map[string]interface{}) {
	message, contextID := extractMessageText(params)

	ctx := r.Context()
	userID := getUserFromAuth(r)
	agentID := getAgentID(r)
	session := getOrCreateSession(ctx, contextID, userID, agentID, message)

	log.Printf("A2A message/send received: %s (contextId: %s, session: %s)", message, contextID, session.SessionID)

	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
	responseText := fmt.Sprintf("Hello from A2A agent! I received: \"%s\"", message)

	// Store messages in session
	addMessageToSession(ctx, session.SessionID, "user", message)
	addMessageToSession(ctx, session.SessionID, "assistant", responseText)

	// Return completed task with artifact
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]interface{}{
			"kind":      "status-update",
			"taskId":    taskID,
			"contextId": session.SessionID,
			"status": map[string]interface{}{
				"state": "completed",
				"message": map[string]interface{}{
					"messageId": fmt.Sprintf("msg-%d", time.Now().UnixNano()),
					"role":      "agent",
					"parts": []map[string]interface{}{
						{
							"kind": "text",
							"text": responseText,
						},
					},
				},
			},
			"final": true,
		},
	})
}

func sendSSEJSONRPC(w http.ResponseWriter, flusher http.Flusher, id interface{}, result interface{}) {
	data, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

func sendJSONRPCError(w http.ResponseWriter, id interface{}, code int, message string) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	})
}

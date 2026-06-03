package mcpserver

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// MCPSession tracks an MCP server-side session
type MCPSession struct {
	ID        string
	CreatedAt time.Time
	// AgentSessions maps agent IDs to their Agentgram session IDs,
	// allowing continuity when the MCP client calls the same agent multiple times.
	AgentSessions map[string]string
}

// SessionStore manages MCP sessions in memory.
// These are lightweight and short-lived — they map MCP session IDs to
// per-agent Agentgram session IDs for conversation continuity.
type SessionStore struct {
	sessions map[string]*MCPSession
	mu       sync.RWMutex
}

// NewSessionStore creates a new in-memory session store
func NewSessionStore() *SessionStore {
	s := &SessionStore{
		sessions: make(map[string]*MCPSession),
	}
	// Start background cleanup
	go s.cleanup()
	return s
}

// Create creates a new MCP session and returns its ID
func (s *SessionStore) Create() string {
	id := uuid.New().String()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = &MCPSession{
		ID:            id,
		CreatedAt:     time.Now(),
		AgentSessions: make(map[string]string),
	}
	return id
}

// Get retrieves an MCP session by ID
func (s *SessionStore) Get(id string) (*MCPSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[id]
	return session, ok
}

// SetAgentSession stores the Agentgram session ID for a given agent within an MCP session
func (s *SessionStore) SetAgentSession(mcpSessionID, agentID, agentgramSessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session, ok := s.sessions[mcpSessionID]; ok {
		session.AgentSessions[agentID] = agentgramSessionID
	}
}

// GetAgentSession retrieves the Agentgram session ID for a given agent within an MCP session
func (s *SessionStore) GetAgentSession(mcpSessionID, agentID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[mcpSessionID]
	if !ok {
		return "", false
	}
	sid, ok := session.AgentSessions[agentID]
	return sid, ok
}

// cleanup removes expired sessions (older than 7 days) every hour
func (s *SessionStore) cleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for id, session := range s.sessions {
			if now.Sub(session.CreatedAt) > 7*24*time.Hour {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

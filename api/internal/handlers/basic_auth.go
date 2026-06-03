package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/dfradehubs/agentgram-api/internal/config"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/store"
	"go.uber.org/zap"
)

// BasicAuthHandler handles basic auth login and admin CRUD
type BasicAuthHandler struct {
	repo         repository.BasicAuthRepository
	sessionStore store.AuthSessionStore
	cfg          *config.Config
	logger       *zap.Logger
}

// NewBasicAuthHandler creates a new basic auth handler
func NewBasicAuthHandler(repo repository.BasicAuthRepository, sessionStore store.AuthSessionStore, cfg *config.Config, logger *zap.Logger) *BasicAuthHandler {
	return &BasicAuthHandler{
		repo:         repo,
		sessionStore: sessionStore,
		cfg:          cfg,
		logger:       logger,
	}
}

// Login handles POST /auth/basic/login
func (h *BasicAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		writeJSONError(w, "username and password required", http.StatusBadRequest)
		return
	}

	user, err := h.repo.GetByUsername(r.Context(), req.Username)
	if err != nil {
		writeJSONError(w, "credenciales invalidas", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		writeJSONError(w, "credenciales invalidas", http.StatusUnauthorized)
		return
	}

	// Create session
	sessionID, err := store.GenerateSessionID()
	if err != nil {
		h.logger.Error("failed to generate session ID", zap.Error(err))
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	authSession := &store.AuthSession{
		SessionID: sessionID,
		Email:     user.Email,
		Sub:       user.ID,
		Groups:    []string{},
		ExpiresAt: now.Add(time.Duration(h.cfg.Auth.SessionMaxAge) * time.Second).Unix(),
		CreatedAt: now.Unix(),
	}

	if err := h.sessionStore.Create(r.Context(), authSession); err != nil {
		h.logger.Error("failed to create auth session", zap.Error(err))
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}

	maxAge := h.cfg.Auth.SessionMaxAge
	if maxAge == 0 {
		maxAge = 86400
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_session",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.cfg.Auth.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	h.logger.Info("basic auth login", zap.String("username", user.Username), zap.String("email", user.Email))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "email": user.Email})
}

// CreateUser handles POST /api/admin/basic-auth/users
func (h *BasicAuthHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Email == "" || req.Password == "" {
		writeJSONError(w, "username, email, and password required", http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("failed to hash password", zap.Error(err))
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}

	user := &models.BasicAuthUser{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hash),
	}

	if err := h.repo.Create(r.Context(), user); err != nil {
		h.logger.Error("failed to create basic auth user", zap.Error(err))
		writeJSONError(w, "failed to create user", http.StatusInternalServerError)
		return
	}

	h.logger.Info("basic auth user created", zap.String("username", req.Username))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "username": req.Username, "email": req.Email})
}

// ListUsers handles GET /api/admin/basic-auth/users
func (h *BasicAuthHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.repo.List(r.Context())
	if err != nil {
		h.logger.Error("failed to list basic auth users", zap.Error(err))
		writeJSONError(w, "failed to list users", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"users": users})
}

// DeleteUser handles DELETE /api/admin/basic-auth/users/{id}
func (h *BasicAuthHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.repo.Delete(r.Context(), id); err != nil {
		h.logger.Error("failed to delete basic auth user", zap.Error(err))
		writeJSONError(w, "failed to delete user", http.StatusInternalServerError)
		return
	}

	h.logger.Info("basic auth user deleted", zap.String("id", id))
	w.WriteHeader(http.StatusNoContent)
}

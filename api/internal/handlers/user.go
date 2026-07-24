package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/service"
	"go.uber.org/zap"
)

// UserResponse represents the user info response
type UserResponse struct {
	Email   string   `json:"email"`
	Groups  []string `json:"groups"`
	IsAdmin bool     `json:"is_admin"`
	Role    string   `json:"role"` // admin | editor | viewer | user
}

// UserHandler handles user endpoints
type UserHandler struct {
	userService *service.UserService // nil when running without DB
	logger      *zap.Logger
}

// NewUserHandler creates a new user handler (without admin check)
func NewUserHandler(logger *zap.Logger) *UserHandler {
	return &UserHandler{logger: logger}
}

// NewUserHandlerWithService creates a new user handler with admin check support
func NewUserHandlerWithService(userService *service.UserService, logger *zap.Logger) *UserHandler {
	return &UserHandler{userService: userService, logger: logger}
}

// Me handles GET /api/me
// @Summary Get current user info
// @Description Returns the authenticated user's email, groups, and admin status
// @Tags user
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Success 200 {object} UserResponse
// @Failure 401 {string} string "unauthorized"
// @Router /api/me [get]
func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	role := models.RoleUser
	if h.userService != nil {
		// Ensure user exists in DB (creates on first visit) and update last access
		if _, err := h.userService.EnsureUser(r.Context(), claims.GetEmail(), claims.GetGroups()); err != nil {
			h.logger.Error("EnsureUser failed", zap.String("email", claims.GetEmail()), zap.Error(err))
		}
		role = h.userService.ResolveRole(r.Context(), claims.GetEmail(), claims.GetGroups())
	}

	response := UserResponse{
		Email:   claims.GetEmail(),
		Groups:  claims.GetGroups(),
		IsAdmin: models.HasRole(role, models.RoleAdmin),
		Role:    role,
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/dfradehubs/agentgram-api/internal/middleware"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

// UserGroupsProvider can retrieve a user's groups from the identity provider

// AdminUsersHandler handles admin CRUD for users
type AdminUsersHandler struct {
	userRepo        repository.UserRepository
	auditRepo       repository.AuditRepository
	bootstrapAdmins []string
	logger          *zap.Logger
}

// NewAdminUsersHandler creates a new admin users handler
func NewAdminUsersHandler(userRepo repository.UserRepository, auditRepo repository.AuditRepository, bootstrapAdmins []string, logger *zap.Logger) *AdminUsersHandler {
	return &AdminUsersHandler{
		userRepo:        userRepo,
		auditRepo:       auditRepo,
		bootstrapAdmins: bootstrapAdmins,
		logger:          logger,
	}
}

// isProtectedAdmin checks if a user is admin due to bootstrap config
func (h *AdminUsersHandler) isProtectedAdmin(email string) bool {
	for _, admin := range h.bootstrapAdmins {
		if strings.EqualFold(admin, email) {
			return true
		}
	}
	return false
}

// isAdminByGroup checks if a user belongs to an admin group via Keycloak.
// Returns (true, nil) if in admin group, (false, nil) if not, (false, err) if check failed.

// userResponse is a user with an additional protected field
type userResponse struct {
	*models.User
	Protected bool `json:"protected"`
}

// ListUsers handles GET /api/admin/users
func (h *AdminUsersHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.userRepo.List(r.Context())
	if err != nil {
		h.logger.Error("list users failed", zap.Error(err))
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	resp := make([]userResponse, len(users))
	for i, u := range users {
		resp[i] = userResponse{User: u, Protected: h.isProtectedAdmin(u.Email)}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"users": resp})
}

// UpdateRoleRequest is the request body for updating user role
type UpdateRoleRequest struct {
	Role string `json:"role"`
}

// UpdateRole handles PUT /api/admin/users/{email}/role
func (h *AdminUsersHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	email := chi.URLParam(r, "email")

	var req UpdateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if !models.IsValidRole(req.Role) {
		http.Error(w, `{"error":"role must be 'admin', 'editor', 'viewer' or 'user'"}`, http.StatusBadRequest)
		return
	}

	// Block changing the role of a bootstrap-config admin. Admins granted by an
	// admin group are already protected by ResolveRole (config wins over the
	// stored DB role), so demoting their DB role has no real effect and no
	// Keycloak group lookup is needed here.
	if req.Role != models.RoleAdmin && h.isProtectedAdmin(email) {
		http.Error(w, `{"error":"this user is an admin via system configuration and cannot be modified"}`, http.StatusForbidden)
		return
	}

	if err := h.userRepo.UpdateRole(r.Context(), email, req.Role); err != nil {
		h.logger.Error("update role failed", zap.Error(err))
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}

	claims := middleware.GetUserFromContext(r.Context())
	h.auditRepo.Log(r.Context(), &models.AuditEntry{
		UserEmail:    claims.GetEmail(),
		Action:       "update_role",
		ResourceType: "user",
		ResourceID:   email,
		Details:      map[string]interface{}{"new_role": req.Role},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"email": email, "role": req.Role})
}

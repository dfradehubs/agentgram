package handlers

import (
	"context"
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
type UserGroupsProvider interface {
	GetUserGroups(ctx context.Context, email string) ([]string, error)
}

// AdminUsersHandler handles admin CRUD for users
type AdminUsersHandler struct {
	userRepo        repository.UserRepository
	auditRepo       repository.AuditRepository
	bootstrapAdmins []string
	adminGroups     []string
	groupsProvider  UserGroupsProvider
	logger          *zap.Logger
}

// NewAdminUsersHandler creates a new admin users handler
func NewAdminUsersHandler(userRepo repository.UserRepository, auditRepo repository.AuditRepository, bootstrapAdmins []string, adminGroups []string, groupsProvider UserGroupsProvider, logger *zap.Logger) *AdminUsersHandler {
	return &AdminUsersHandler{
		userRepo:        userRepo,
		auditRepo:       auditRepo,
		bootstrapAdmins: bootstrapAdmins,
		adminGroups:     adminGroups,
		groupsProvider:  groupsProvider,
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
func (h *AdminUsersHandler) isAdminByGroup(ctx context.Context, email string) (bool, error) {
	if h.groupsProvider == nil || len(h.adminGroups) == 0 {
		return false, nil
	}

	userGroups, err := h.groupsProvider.GetUserGroups(ctx, email)
	if err != nil {
		h.logger.Error("failed to fetch user groups from Keycloak", zap.String("email", email), zap.Error(err))
		return false, err
	}

	for _, adminGroup := range h.adminGroups {
		for _, userGroup := range userGroups {
			if strings.EqualFold(adminGroup, userGroup) {
				return true, nil
			}
		}
	}
	return false, nil
}

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

	if req.Role != "admin" && req.Role != "user" {
		http.Error(w, `{"error":"role must be 'admin' or 'user'"}`, http.StatusBadRequest)
		return
	}

	// Block demotion of protected admins (bootstrap config or admin group membership)
	if req.Role == "user" {
		if h.isProtectedAdmin(email) {
			http.Error(w, `{"error":"this user is an admin via system configuration and cannot be modified"}`, http.StatusForbidden)
			return
		}
		inGroup, err := h.isAdminByGroup(r.Context(), email)
		if err != nil {
			http.Error(w, `{"error":"could not verify the user's groups in Keycloak, operation blocked for security"}`, http.StatusInternalServerError)
			return
		}
		if inGroup {
			http.Error(w, `{"error":"this user belongs to an administrators group and cannot be removed from admin"}`, http.StatusForbidden)
			return
		}
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

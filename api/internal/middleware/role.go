package middleware

import (
	"net/http"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/service"
	"go.uber.org/zap"
)

// RoleGate restricts access to users whose effective role meets a minimum
// (hierarchical: admin > editor > viewer > user).
type RoleGate struct {
	userService *service.UserService
	minRole     string
	logger      *zap.Logger
}

// RequireRole builds a middleware that allows only users with at least minRole.
func RequireRole(userService *service.UserService, minRole string, logger *zap.Logger) *RoleGate {
	return &RoleGate{userService: userService, minRole: minRole, logger: logger}
}

// Handler returns the HTTP middleware.
func (g *RoleGate) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetUserFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		role := g.userService.ResolveRole(r.Context(), claims.GetEmail(), claims.GetGroups())
		if !models.HasRole(role, g.minRole) {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

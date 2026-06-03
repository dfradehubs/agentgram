package middleware

import (
	"net/http"

	"github.com/dfradehubs/agentgram-api/internal/service"
	"go.uber.org/zap"
)

// Admin middleware restricts access to admin users only
type Admin struct {
	userService *service.UserService
	logger      *zap.Logger
}

// NewAdmin creates a new admin middleware
func NewAdmin(userService *service.UserService, logger *zap.Logger) *Admin {
	return &Admin{
		userService: userService,
		logger:      logger,
	}
}

// Handler returns the HTTP middleware
func (a *Admin) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetUserFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		isAdmin, err := a.userService.IsAdmin(r.Context(), claims.GetEmail(), claims.GetGroups())
		if err != nil {
			a.logger.Error("admin check failed", zap.Error(err))
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			return
		}

		if !isAdmin {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/service"
	"go.uber.org/zap"
)

// roleFakeUserRepo returns a user with a fixed DB role (or "not found").
type roleFakeUserRepo struct {
	repository.UserRepository
	role string
}

func (f *roleFakeUserRepo) GetByEmail(_ context.Context, _ string) (*models.User, error) {
	if f.role == "" {
		return nil, fmt.Errorf("not found")
	}
	return &models.User{Email: "u@example.com", Role: f.role}, nil
}

func TestRoleGateHandler(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	reqWithUser := func() *http.Request {
		ctx := context.WithValue(context.Background(), UserContextKey, &auth.Claims{Email: "u@example.com"})
		return httptest.NewRequest(http.MethodGet, "/admin/x", nil).WithContext(ctx)
	}

	tests := []struct {
		name       string
		dbRole     string
		minRole    string
		withClaims bool
		wantStatus int
	}{
		{"no claims → 401", models.RoleAdmin, models.RoleAdmin, false, http.StatusUnauthorized},
		{"editor vs admin → 403", models.RoleEditor, models.RoleAdmin, true, http.StatusForbidden},
		{"editor vs editor → ok", models.RoleEditor, models.RoleEditor, true, http.StatusOK},
		{"editor vs viewer → ok", models.RoleEditor, models.RoleViewer, true, http.StatusOK},
		{"admin vs admin → ok", models.RoleAdmin, models.RoleAdmin, true, http.StatusOK},
		{"user vs editor → 403", models.RoleUser, models.RoleEditor, true, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			us := service.NewUserService(&roleFakeUserRepo{role: tt.dbRole}, nil, nil, nil, nil)
			gate := RequireRole(us, tt.minRole, zap.NewNop())

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/admin/x", nil)
			if tt.withClaims {
				req = reqWithUser()
			}
			gate.Handler(next).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

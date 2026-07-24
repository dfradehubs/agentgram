package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
)

// fakeUserRepo returns a fixed DB user (or "not found" when nil).
type fakeUserRepo struct {
	repository.UserRepository
	user *models.User
}

func (f *fakeUserRepo) GetByEmail(_ context.Context, _ string) (*models.User, error) {
	if f.user == nil {
		return nil, fmt.Errorf("not found")
	}
	return f.user, nil
}

func TestResolveRole(t *testing.T) {
	const email = "u@example.com"
	tests := []struct {
		name         string
		bootstrap    []string
		adminGroups  []string
		editorGroups []string
		viewerGroups []string
		groups       []string
		dbRole       string // "" => user not in DB
		want         string
	}{
		{name: "bootstrap admin", bootstrap: []string{email}, want: models.RoleAdmin},
		{name: "admin group", adminGroups: []string{"g-admin"}, groups: []string{"g-admin"}, want: models.RoleAdmin},
		{name: "editor group", editorGroups: []string{"g-ed"}, groups: []string{"g-ed"}, want: models.RoleEditor},
		{name: "viewer group", viewerGroups: []string{"g-vw"}, groups: []string{"g-vw"}, want: models.RoleViewer},
		{name: "db role editor only", dbRole: models.RoleEditor, want: models.RoleEditor},
		{name: "db user + config editor group", editorGroups: []string{"g-ed"}, groups: []string{"g-ed"}, dbRole: models.RoleUser, want: models.RoleEditor},
		{name: "db admin beats config viewer", viewerGroups: []string{"g-vw"}, groups: []string{"g-vw"}, dbRole: models.RoleAdmin, want: models.RoleAdmin},
		{name: "config editor beats db viewer", editorGroups: []string{"g-ed"}, groups: []string{"g-ed"}, dbRole: models.RoleViewer, want: models.RoleEditor},
		{name: "nothing matches", groups: []string{"other"}, want: models.RoleUser},
		{name: "admin group case-insensitive", adminGroups: []string{"G-Admin"}, groups: []string{"g-admin"}, want: models.RoleAdmin},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var repo *fakeUserRepo
			if tt.dbRole != "" {
				repo = &fakeUserRepo{user: &models.User{Email: email, Role: tt.dbRole}}
			} else {
				repo = &fakeUserRepo{}
			}
			s := NewUserService(repo, tt.bootstrap, tt.adminGroups, tt.editorGroups, tt.viewerGroups)
			if got := s.ResolveRole(context.Background(), email, tt.groups); got != tt.want {
				t.Errorf("ResolveRole = %q, want %q", got, tt.want)
			}
		})
	}
}

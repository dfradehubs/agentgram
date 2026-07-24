package models

import "testing"

func TestHasRole(t *testing.T) {
	tests := []struct {
		role, min string
		want      bool
	}{
		{RoleAdmin, RoleAdmin, true},
		{RoleAdmin, RoleEditor, true},
		{RoleAdmin, RoleViewer, true},
		{RoleEditor, RoleAdmin, false},
		{RoleEditor, RoleEditor, true},
		{RoleEditor, RoleViewer, true},
		{RoleViewer, RoleEditor, false},
		{RoleViewer, RoleViewer, true},
		{RoleUser, RoleViewer, false},
		{RoleUser, RoleUser, true},
		{"unknown", RoleViewer, false},
	}
	for _, tt := range tests {
		if got := HasRole(tt.role, tt.min); got != tt.want {
			t.Errorf("HasRole(%q,%q) = %v, want %v", tt.role, tt.min, got, tt.want)
		}
	}
}

func TestNormalizeRole(t *testing.T) {
	for in, want := range map[string]string{
		RoleAdmin: RoleAdmin, RoleEditor: RoleEditor, RoleViewer: RoleViewer,
		RoleUser: RoleUser, "": RoleUser, "bogus": RoleUser,
	} {
		if got := NormalizeRole(in); got != want {
			t.Errorf("NormalizeRole(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsValidRole(t *testing.T) {
	for _, r := range []string{RoleAdmin, RoleEditor, RoleViewer, RoleUser} {
		if !IsValidRole(r) {
			t.Errorf("IsValidRole(%q) = false, want true", r)
		}
	}
	for _, r := range []string{"", "root", "superuser"} {
		if IsValidRole(r) {
			t.Errorf("IsValidRole(%q) = true, want false", r)
		}
	}
}

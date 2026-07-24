package service

import (
	"context"
	"strings"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
)

// UserService manages user operations and admin checks
type UserService struct {
	userRepo        repository.UserRepository
	bootstrapAdmins []string
	adminGroups     []string
	editorGroups    []string
	viewerGroups    []string
}

// NewUserService creates a new user service
func NewUserService(userRepo repository.UserRepository, bootstrapAdmins, adminGroups, editorGroups, viewerGroups []string) *UserService {
	return &UserService{
		userRepo:        userRepo,
		bootstrapAdmins: bootstrapAdmins,
		adminGroups:     adminGroups,
		editorGroups:    editorGroups,
		viewerGroups:    viewerGroups,
	}
}

// ResolveRole returns the effective role for a user: the highest of the role
// granted by config (admin users/groups, editor/viewer groups) and the role
// stored in the DB. Unknown/absent → user.
func (s *UserService) ResolveRole(ctx context.Context, email string, groups []string) string {
	// Config-admin always wins.
	for _, admin := range s.bootstrapAdmins {
		if strings.EqualFold(admin, email) {
			return models.RoleAdmin
		}
	}
	groupSet := make(map[string]bool, len(groups))
	for _, g := range groups {
		groupSet[strings.ToLower(g)] = true
	}
	for _, g := range s.adminGroups {
		if groupSet[strings.ToLower(g)] {
			return models.RoleAdmin
		}
	}

	// Config editor/viewer groups.
	configRole := models.RoleUser
	for _, g := range s.editorGroups {
		if groupSet[strings.ToLower(g)] {
			configRole = models.RoleEditor
			break
		}
	}
	if configRole == models.RoleUser {
		for _, g := range s.viewerGroups {
			if groupSet[strings.ToLower(g)] {
				configRole = models.RoleViewer
				break
			}
		}
	}

	// DB role.
	dbRole := models.RoleUser
	if user, err := s.userRepo.GetByEmail(ctx, email); err == nil {
		dbRole = models.NormalizeRole(user.Role)
	}

	// Highest of config vs DB.
	if models.HasRole(dbRole, configRole) {
		return dbRole
	}
	return configRole
}

// IsAdmin checks if a user is an admin (from DB, bootstrap config, or group membership)
func (s *UserService) IsAdmin(ctx context.Context, email string, groups []string) (bool, error) {
	// Check bootstrap admins first (always admin even if DB hasn't been updated)
	for _, admin := range s.bootstrapAdmins {
		if strings.EqualFold(admin, email) {
			return true, nil
		}
	}

	// Check admin groups
	for _, adminGroup := range s.adminGroups {
		for _, userGroup := range groups {
			if strings.EqualFold(adminGroup, userGroup) {
				return true, nil
			}
		}
	}

	// Check DB
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		// User not in DB yet — not admin
		return false, nil
	}

	return user.IsAdmin(), nil
}

// EnsureUser creates a user if they don't exist, updates last access, returns the user
func (s *UserService) EnsureUser(ctx context.Context, email string, groups []string) (*models.User, error) {
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err == nil {
		// Update last access timestamp
		_ = s.userRepo.UpdateLastAccess(ctx, email)
		return user, nil
	}

	// Determine role: admin if in bootstrap list or admin group
	role := "user"
	for _, admin := range s.bootstrapAdmins {
		if strings.EqualFold(admin, email) {
			role = "admin"
			break
		}
	}
	if role == "user" {
		for _, adminGroup := range s.adminGroups {
			for _, userGroup := range groups {
				if strings.EqualFold(adminGroup, userGroup) {
					role = "admin"
					break
				}
			}
			if role == "admin" {
				break
			}
		}
	}

	newUser := &models.User{
		Email: email,
		Role:  role,
	}
	if err := s.userRepo.Create(ctx, newUser); err != nil {
		return nil, err
	}

	return s.userRepo.GetByEmail(ctx, email)
}

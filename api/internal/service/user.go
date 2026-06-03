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
}

// NewUserService creates a new user service
func NewUserService(userRepo repository.UserRepository, bootstrapAdmins []string, adminGroups []string) *UserService {
	return &UserService{
		userRepo:        userRepo,
		bootstrapAdmins: bootstrapAdmins,
		adminGroups:     adminGroups,
	}
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

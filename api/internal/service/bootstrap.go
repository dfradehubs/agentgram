package service

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/dfradehubs/agentgram-api/internal/config"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

// BootstrapService seeds admin users from config.yaml
type BootstrapService struct {
	userRepo      repository.UserRepository
	basicAuthRepo repository.BasicAuthRepository
	logger        *zap.Logger
}

// NewBootstrapService creates a new bootstrap service
func NewBootstrapService(
	userRepo repository.UserRepository,
	basicAuthRepo repository.BasicAuthRepository,
	logger *zap.Logger,
) *BootstrapService {
	return &BootstrapService{
		userRepo:      userRepo,
		basicAuthRepo: basicAuthRepo,
		logger:        logger,
	}
}

// SeedAdminUsers seeds admin users from config if they don't exist yet
func (s *BootstrapService) SeedAdminUsers(ctx context.Context, cfg *config.Config) error {
	for _, email := range cfg.Auth.AdminUsers {
		existing, _ := s.userRepo.GetByEmail(ctx, email)
		if existing != nil {
			continue
		}
		user := &models.User{
			Email: email,
			Role:  "admin",
		}
		if err := s.userRepo.Create(ctx, user); err != nil {
			return fmt.Errorf("seed admin user %s: %w", email, err)
		}
		s.logger.Info("seeded admin user", zap.String("email", email))
	}
	return nil
}

// SeedBasicAuthUsers seeds basic auth users from config if they don't exist yet
func (s *BootstrapService) SeedBasicAuthUsers(ctx context.Context, cfg *config.Config) error {
	if !cfg.Auth.Basic.Enabled || s.basicAuthRepo == nil {
		return nil
	}

	for _, seed := range cfg.Auth.Basic.SeedUsers {
		existing, _ := s.basicAuthRepo.GetByUsername(ctx, seed.Username)
		if existing != nil {
			continue
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(seed.Password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash password for %s: %w", seed.Username, err)
		}

		user := &models.BasicAuthUser{
			Username:     seed.Username,
			Email:        seed.Email,
			PasswordHash: string(hash),
		}
		if err := s.basicAuthRepo.Create(ctx, user); err != nil {
			return fmt.Errorf("seed basic auth user %s: %w", seed.Username, err)
		}
		s.logger.Info("seeded basic auth user", zap.String("username", seed.Username), zap.String("email", seed.Email))
	}
	return nil
}

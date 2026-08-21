package service

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/dfradehubs/agentgram-api/internal/config"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

// BootstrapService seeds admin users, basic-auth users and first-run agents from config.yaml
type BootstrapService struct {
	userRepo      repository.UserRepository
	basicAuthRepo repository.BasicAuthRepository
	agentRepo     repository.AgentRepository
	logger        *zap.Logger
}

// NewBootstrapService creates a new bootstrap service. agentRepo may be nil if agent seeding is unused.
func NewBootstrapService(
	userRepo repository.UserRepository,
	basicAuthRepo repository.BasicAuthRepository,
	agentRepo repository.AgentRepository,
	logger *zap.Logger,
) *BootstrapService {
	return &BootstrapService{
		userRepo:      userRepo,
		basicAuthRepo: basicAuthRepo,
		agentRepo:     agentRepo,
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

// SeedAgents inserts first-run agents (demo + YAML list) if they do not already exist.
func (s *BootstrapService) SeedAgents(ctx context.Context, cfg *config.Config) error {
	if s.agentRepo == nil {
		return nil
	}

	var seeds []config.SeedAgent
	if cfg.Seed.DemoAgent {
		port := cfg.Server.Port
		if port == "" {
			port = "8080"
		}
		seeds = append(seeds, config.SeedAgent{
			ID:            "demo",
			Name:          "Demo",
			Description:   "Built-in echo agent so you can chat immediately. Replace it with your own from Admin → Agents.",
			Category:      "getting-started",
			Protocol:      "custom",
			Endpoint:      "http://127.0.0.1:" + port + "/demo/chat",
			AllowedGroups: []string{"*"},
		})
	}
	seeds = append(seeds, cfg.Seed.Agents...)

	for _, seed := range seeds {
		if err := s.seedOneAgent(ctx, seed); err != nil {
			return err
		}
	}
	return nil
}

func (s *BootstrapService) seedOneAgent(ctx context.Context, seed config.SeedAgent) error {
	if seed.ID == "" || strings.TrimSpace(seed.Endpoint) == "" {
		s.logger.Debug("skipping seed agent with empty id or endpoint", zap.String("id", seed.ID))
		return nil
	}
	existing, _, _, err := s.agentRepo.Get(ctx, seed.ID)
	if err == nil && existing != nil {
		return nil
	}

	protocol := seed.Protocol
	if protocol == "" {
		protocol = "custom"
	}
	name := seed.Name
	if name == "" {
		name = seed.ID
	}
	groups := seed.AllowedGroups
	if len(groups) == 0 {
		groups = []string{"*"}
	}

	agent := &models.Agent{
		ID:            seed.ID,
		Name:          name,
		Description:   seed.Description,
		Category:      seed.Category,
		Protocol:      protocol,
		Endpoint:      seed.Endpoint,
		AllowedGroups: groups,
		AllowedUsers:  seed.AllowedUsers,
	}
	if err := s.agentRepo.Create(ctx, agent, seed.AllowedUsers, groups); err != nil {
		return fmt.Errorf("seed agent %s: %w", seed.ID, err)
	}
	s.logger.Info("seeded agent", zap.String("id", seed.ID), zap.String("endpoint", seed.Endpoint))
	return nil
}

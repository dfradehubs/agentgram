package orchestrator

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"sync"

	lf "github.com/dfradehubs/agentgram-api/internal/langfuse"
	"github.com/dfradehubs/agentgram-api/internal/llm"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

var (
	ErrModeratorNotConfigured = errors.New("moderator LLM is not configured")
	ErrModeratorUnavailable   = errors.New("moderator LLM is unavailable")
)

// ModeratorResolver resolves the current enabled moderator model for each
// debate. It deliberately does not cache configuration: admin changes and
// revocations must apply to every API instance without a restart.
type ModeratorResolver struct {
	repo        repository.LLMModelRepository
	tracer      *lf.Tracer
	logger      *zap.Logger
	newProvider func(*models.LLMModel) (llm.Provider, error)
	mu          sync.Mutex
	cacheKey    [sha256.Size]byte
	cached      *Moderator
}

func NewModeratorResolver(repo repository.LLMModelRepository, tracer *lf.Tracer, logger *zap.Logger) *ModeratorResolver {
	return &ModeratorResolver{repo: repo, tracer: tracer, logger: logger, newProvider: llm.NewProvider}
}

func (r *ModeratorResolver) Resolve(ctx context.Context) (*Moderator, error) {
	if r == nil || r.repo == nil {
		return nil, ErrModeratorNotConfigured
	}
	modelsByRole, err := r.repo.ListByRole(ctx, "moderator")
	if err != nil {
		return nil, fmt.Errorf("%w: load model: %w", ErrModeratorUnavailable, err)
	}
	if len(modelsByRole) == 0 {
		return nil, ErrModeratorNotConfigured
	}

	model := modelsByRole[0]
	for _, candidate := range modelsByRole {
		if candidate.IsDefault {
			model = candidate
			break
		}
	}
	cacheKey := moderatorModelCacheKey(model)

	// A provider owns an HTTP client/transport. Reuse it while configuration is
	// unchanged so dynamic admin reloads do not sacrifice connection pooling.
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cached != nil && r.cacheKey == cacheKey {
		return r.cached, nil
	}
	provider, err := r.newProvider(model)
	if err != nil {
		return nil, fmt.Errorf("%w: create provider: %w", ErrModeratorUnavailable, err)
	}
	if r.tracer != nil && r.tracer.Enabled() {
		provider = lf.WrapProvider(provider, "moderator", model.Model)
	}
	mod := NewWithProvider(provider, r.logger)
	mod.maxTokens = model.MaxTokens
	r.cached = mod
	r.cacheKey = cacheKey
	return r.cached, nil
}

func moderatorModelCacheKey(model *models.LLMModel) [sha256.Size]byte {
	return sha256.Sum256([]byte(model.ID + "\x00" + model.Provider + "\x00" + model.Model + "\x00" + model.APIKey + "\x00" + model.Endpoint + "\x00" + strconv.Itoa(model.MaxTokens)))
}

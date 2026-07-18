package llmresolver

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"

	lf "github.com/dfradehubs/agentgram-api/internal/langfuse"
	"github.com/dfradehubs/agentgram-api/internal/llm"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

var ErrNotConfigured = errors.New("LLM role is not configured")

type cachedProvider struct {
	key      [sha256.Size]byte
	model    *models.LLMModel
	provider llm.Provider
}

// Resolver loads the current default model on every operation while reusing
// HTTP clients when the complete effective configuration is unchanged.
type Resolver struct {
	repo   repository.LLMModelRepository
	tracer *lf.Tracer
	logger *zap.Logger
	mu     sync.Mutex
	cache  map[string]cachedProvider
}

func New(repo repository.LLMModelRepository, tracer *lf.Tracer, logger *zap.Logger) *Resolver {
	return &Resolver{repo: repo, tracer: tracer, logger: logger, cache: make(map[string]cachedProvider)}
}

func (r *Resolver) ResolveRole(ctx context.Context, role, traceName string) (*models.LLMModel, llm.Provider, error) {
	if r == nil || r.repo == nil {
		return nil, nil, ErrNotConfigured
	}
	modelsByRole, err := r.repo.ListByRole(ctx, role)
	if err != nil {
		return nil, nil, err
	}
	if len(modelsByRole) == 0 {
		return nil, nil, ErrNotConfigured
	}
	model := modelsByRole[0]
	key := sha256.Sum256([]byte(model.ID + "\x00" + model.ProviderID + "\x00" + model.Provider + "\x00" + model.Model + "\x00" + model.APIKey + "\x00" + model.Endpoint))

	r.mu.Lock()
	defer r.mu.Unlock()
	if cached, ok := r.cache[role]; ok && cached.key == key {
		return cached.model, cached.provider, nil
	}
	provider, err := llm.NewProvider(model)
	if err != nil {
		return nil, nil, err
	}
	if r.tracer != nil && r.tracer.Enabled() {
		provider = lf.WrapProvider(provider, traceName, model.Model)
	}
	modelCopy := *model
	r.cache[role] = cachedProvider{key: key, model: &modelCopy, provider: provider}
	r.logger.Debug("resolved LLM role", zap.String("role", role), zap.String("model", model.ID))
	return &modelCopy, provider, nil
}

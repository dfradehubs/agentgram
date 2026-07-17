package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/llm"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

type mutableModeratorRepo struct {
	repository.LLMModelRepository
	mu     sync.Mutex
	models []*models.LLMModel
	err    error
}

func (r *mutableModeratorRepo) ListByRole(context.Context, string) ([]*models.LLMModel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*models.LLMModel(nil), r.models...), r.err
}

type fixedProvider struct{ response string }

func (p fixedProvider) GenerateContent(context.Context, *llm.Request) (*llm.Response, error) {
	return &llm.Response{Text: p.response}, nil
}

func TestModeratorResolverSeesAdminChangesWithoutRestart(t *testing.T) {
	repo := &mutableModeratorRepo{}
	resolver := NewModeratorResolver(repo, nil, zap.NewNop())
	resolver.newProvider = func(*models.LLMModel) (llm.Provider, error) {
		return fixedProvider{response: "FINISH"}, nil
	}

	if _, err := resolver.Resolve(context.Background()); !errors.Is(err, ErrModeratorNotConfigured) {
		t.Fatalf("Resolve before create = %v, want ErrModeratorNotConfigured", err)
	}
	repo.models = []*models.LLMModel{{ID: "moderator", Enabled: true}}
	if moderator, err := resolver.Resolve(context.Background()); err != nil || moderator == nil {
		t.Fatalf("Resolve after create = (%v, %v), want moderator", moderator, err)
	}
	repo.models = nil
	if _, err := resolver.Resolve(context.Background()); !errors.Is(err, ErrModeratorNotConfigured) {
		t.Fatalf("Resolve after delete = %v, want ErrModeratorNotConfigured", err)
	}
}

func TestModeratorResolverPrefersDefaultModel(t *testing.T) {
	repo := &mutableModeratorRepo{models: []*models.LLMModel{
		{ID: "alphabetical", Name: "A"},
		{ID: "preferred", Name: "Z", IsDefault: true},
	}}
	resolver := NewModeratorResolver(repo, nil, zap.NewNop())
	selected := ""
	resolver.newProvider = func(model *models.LLMModel) (llm.Provider, error) {
		selected = model.ID
		return fixedProvider{response: "FINISH"}, nil
	}

	if _, err := resolver.Resolve(context.Background()); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if selected != "preferred" {
		t.Fatalf("selected %q, want preferred", selected)
	}
}

func TestModeratorResolverReusesProviderUntilConfigurationChanges(t *testing.T) {
	repo := &mutableModeratorRepo{models: []*models.LLMModel{{ID: "moderator", Provider: "openai", Model: "first", APIKey: "secret"}}}
	resolver := NewModeratorResolver(repo, nil, zap.NewNop())
	providerCreations := 0
	resolver.newProvider = func(*models.LLMModel) (llm.Provider, error) {
		providerCreations++
		return fixedProvider{response: "FINISH"}, nil
	}

	first, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	second, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if first != second || providerCreations != 1 {
		t.Fatalf("unchanged config created %d providers; moderator pointers equal=%v", providerCreations, first == second)
	}

	repo.models = []*models.LLMModel{{ID: "moderator", Provider: "openai", Model: "second", APIKey: "secret"}}
	third, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve after update: %v", err)
	}
	if third == second || providerCreations != 2 {
		t.Fatalf("changed config created %d providers; moderator pointer was reused=%v", providerCreations, third == second)
	}
}

func TestModeratorResolverClassifiesLoadAndProviderFailures(t *testing.T) {
	repo := &mutableModeratorRepo{err: errors.New("database down")}
	resolver := NewModeratorResolver(repo, nil, zap.NewNop())
	if _, err := resolver.Resolve(context.Background()); !errors.Is(err, ErrModeratorUnavailable) {
		t.Fatalf("load error = %v, want ErrModeratorUnavailable", err)
	}

	repo.err = nil
	repo.models = []*models.LLMModel{{ID: "broken"}}
	resolver.newProvider = func(*models.LLMModel) (llm.Provider, error) {
		return nil, fmt.Errorf("unsupported provider")
	}
	if _, err := resolver.Resolve(context.Background()); !errors.Is(err, ErrModeratorUnavailable) {
		t.Fatalf("provider error = %v, want ErrModeratorUnavailable", err)
	}
}

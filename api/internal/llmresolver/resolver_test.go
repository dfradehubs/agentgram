package llmresolver

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

type mutableRepo struct {
	repository.LLMModelRepository
	mu     sync.Mutex
	models []*models.LLMModel
}

func (r *mutableRepo) ListByRole(context.Context, string) ([]*models.LLMModel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*models.LLMModel(nil), r.models...), nil
}

func TestResolverAppliesConfigurationChangesWithoutRestart(t *testing.T) {
	repo := &mutableRepo{}
	resolver := New(repo, nil, zap.NewNop())
	if _, _, err := resolver.ResolveRole(t.Context(), "summarizer", "test"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured error = %v", err)
	}

	repo.models = []*models.LLMModel{{ID: "sum", ProviderID: "p", Provider: "openai", Model: "gpt", APIKey: "first"}}
	_, first, err := resolver.ResolveRole(t.Context(), "summarizer", "test")
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := resolver.ResolveRole(t.Context(), "summarizer", "test")
	if err != nil || first != second {
		t.Fatalf("unchanged configuration did not reuse provider: %v", err)
	}

	repo.models = []*models.LLMModel{{ID: "sum", ProviderID: "p", Provider: "openai", Model: "gpt", APIKey: "second"}}
	_, third, err := resolver.ResolveRole(t.Context(), "summarizer", "test")
	if err != nil || third == second {
		t.Fatalf("changed API key reused stale provider: %v", err)
	}
}

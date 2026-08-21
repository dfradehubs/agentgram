package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/dfradehubs/agentgram-api/internal/config"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"go.uber.org/zap"
)

type fakeAgentRepo struct {
	repository.AgentRepository
	byID    map[string]*models.Agent
	creates int
}

func (f *fakeAgentRepo) Get(_ context.Context, id string) (*models.Agent, []string, []string, error) {
	if a, ok := f.byID[id]; ok {
		return a, a.AllowedUsers, a.AllowedGroups, nil
	}
	return nil, nil, nil, fmt.Errorf("not found")
}

func (f *fakeAgentRepo) Create(_ context.Context, agent *models.Agent, users, groups []string) error {
	if f.byID == nil {
		f.byID = map[string]*models.Agent{}
	}
	cp := *agent
	cp.AllowedUsers = users
	cp.AllowedGroups = groups
	f.byID[agent.ID] = &cp
	f.creates++
	return nil
}

func TestSeedAgents_DemoAndYAML(t *testing.T) {
	repo := &fakeAgentRepo{byID: map[string]*models.Agent{}}
	svc := NewBootstrapService(nil, nil, repo, zap.NewNop())
	cfg := &config.Config{
		Server: config.ServerConfig{Port: "8080"},
		Seed: config.SeedConfig{
			DemoAgent: true,
			Agents: []config.SeedAgent{
				{ID: "mock-agent", Name: "Mock", Protocol: "custom", Endpoint: "http://mock-agent:9000/chat"},
				{ID: "skipped", Endpoint: ""},
			},
		},
	}
	if err := svc.SeedAgents(context.Background(), cfg); err != nil {
		t.Fatalf("SeedAgents: %v", err)
	}
	if repo.creates != 2 {
		t.Fatalf("creates = %d, want 2", repo.creates)
	}
	demo, _, _, err := repo.Get(context.Background(), "demo")
	if err != nil {
		t.Fatalf("demo missing: %v", err)
	}
	if demo.Endpoint != "http://127.0.0.1:8080/demo/chat" {
		t.Fatalf("demo endpoint = %q", demo.Endpoint)
	}
	if len(demo.AllowedGroups) != 1 || demo.AllowedGroups[0] != "*" {
		t.Fatalf("demo groups = %#v", demo.AllowedGroups)
	}
}

func TestSeedAgents_Idempotent(t *testing.T) {
	repo := &fakeAgentRepo{byID: map[string]*models.Agent{
		"demo": {ID: "demo", Endpoint: "http://127.0.0.1:8080/demo/chat"},
	}}
	svc := NewBootstrapService(nil, nil, repo, zap.NewNop())
	cfg := &config.Config{
		Server: config.ServerConfig{Port: "8080"},
		Seed:   config.SeedConfig{DemoAgent: true},
	}
	if err := svc.SeedAgents(context.Background(), cfg); err != nil {
		t.Fatalf("SeedAgents: %v", err)
	}
	if repo.creates != 0 {
		t.Fatalf("creates = %d, want 0 (already present)", repo.creates)
	}
}

func TestSeedAgents_NilRepo(t *testing.T) {
	svc := NewBootstrapService(nil, nil, nil, zap.NewNop())
	if err := svc.SeedAgents(context.Background(), &config.Config{Seed: config.SeedConfig{DemoAgent: true}}); err != nil {
		t.Fatalf("nil repo should no-op: %v", err)
	}
}

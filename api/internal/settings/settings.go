// Package settings holds runtime-editable operational settings, backed by the
// runtime_config table and edited from the admin "General Configuration" panel.
// Only values consulted per-request live here; startup/infra config stays in
// YAML. Unknown keys are ignored; missing keys fall back to the code default.
package settings

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Type of a setting, used for validation and admin-form rendering.
type Type string

const (
	TypeInt      Type = "int"
	TypeDuration Type = "duration" // Go duration string, e.g. "10m", "90s"
)

// Def describes a known setting: its default, type and admin-facing metadata.
type Def struct {
	Key         string `json:"key"`
	Section     string `json:"section"` // Admin grouping, e.g. "MCP", "MCP Chat", "Groups"
	Label       string `json:"label"`
	Type        Type   `json:"type"`
	Default     string `json:"default"`
	Description string `json:"description"`
	Min         int    `json:"min,omitempty"` // int type only (0 = use the global minimum of 1)
	Max         int    `json:"max,omitempty"` // int type only (0 = no upper bound)
}

// Keys of known settings.
const (
	KeyMCPToolCallTimeout = "mcp_tool_call_timeout"
	KeyMCPMaxToolRounds   = "mcp_max_tool_call_rounds"
	KeyGroupDebateTimeout = "group_debate_timeout_api"
	KeyGroupMaxTurnsAPI   = "group_max_turns_api"
	KeyGroupMaxTurnsMCP   = "group_max_turns_mcp"
)

// Defs is the registry of known settings, in admin-display order. Adding a
// value here (plus a consumer that reads it) is all it takes to expose it.
var Defs = []Def{
	{Key: KeyMCPToolCallTimeout, Section: "MCP", Label: "Tool-call timeout", Type: TypeDuration, Default: "10m",
		Description: "How long an MCP agent or group tool waits before aborting. Progress pings keep the client alive meanwhile."},
	{Key: KeyMCPMaxToolRounds, Section: "MCP Chat", Label: "Max tool-call rounds", Type: TypeInt, Default: "10",
		Description: "Max LLM ↔ tool iterations per MCP chat request before stopping.", Min: 1, Max: 50},
	{Key: KeyGroupDebateTimeout, Section: "Group debates", Label: "Debate timeout (API / web)", Type: TypeDuration, Default: "10m",
		Description: "Absolute execution budget for an entire moderated debate over the streaming API, shared by moderator and agent turns. Final persistence has its own short bounded grace period."},
	{Key: KeyGroupMaxTurnsAPI, Section: "Group debates", Label: "Max turns (API / web)", Type: TypeInt, Default: "6",
		Description: "Default cap on moderated-debate turns over the streaming API. A group's own max_turns overrides this.", Min: 1, Max: 50},
	{Key: KeyGroupMaxTurnsMCP, Section: "Group debates", Label: "Max turns (MCP)", Type: TypeInt, Default: "3",
		Description: "Default cap for synchronous MCP group__ tools (kept lower to fit tool-call timeouts). A group's own max_turns overrides this.", Min: 1, Max: 50},
}

var defByKey = func() map[string]Def {
	m := make(map[string]Def, len(Defs))
	for _, d := range Defs {
		m[d.Key] = d
	}
	return m
}()

// Repository persists setting overrides.
type Repository interface {
	GetAll(ctx context.Context) (map[string]string, error)
	// SetMany upserts several overrides atomically (all-or-nothing).
	SetMany(ctx context.Context, values map[string]string) error
}

// Service resolves settings from an in-memory cache (DB overrides on top of
// code defaults), refreshed on startup and after each write.
type Service struct {
	repo   Repository
	logger *zap.Logger
	// loadMu serializes Reload end-to-end (DB read + apply) so a stale periodic
	// snapshot can't overwrite a newer PUT-triggered reload. It's distinct from
	// mu so the DB read doesn't block hot-path readers (Int/Duration).
	loadMu sync.Mutex
	mu     sync.RWMutex
	values map[string]string // effective values (default merged with DB)
}

// New builds a Service and loads current overrides. A nil repo (or load error)
// leaves it running on defaults only.
func New(repo Repository, logger *zap.Logger) *Service {
	s := &Service{repo: repo, logger: logger, values: map[string]string{}}
	for _, d := range Defs {
		s.values[d.Key] = d.Default
	}
	if repo != nil {
		if err := s.Reload(context.Background()); err != nil {
			logger.Warn("failed to load app settings; using defaults", zap.Error(err))
		}
	}
	return s
}

// StartPeriodicReload refreshes the cache from the DB every interval, so a
// change made on one pod propagates to the others within one interval (the
// writing pod reloads immediately; peers converge here). Mirrors the MCP
// registry's periodic refresh. Runs until ctx is cancelled.
func (s *Service) StartPeriodicReload(ctx context.Context, interval time.Duration) {
	if s.repo == nil || interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rc, cancel := context.WithTimeout(ctx, 10*time.Second)
				if err := s.Reload(rc); err != nil {
					s.logger.Warn("periodic settings reload failed", zap.Error(err))
				}
				cancel()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Reload refreshes the cache from the repository.
func (s *Service) Reload(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}
	// Hold loadMu across the DB read AND the apply: this serializes concurrent
	// reloads so the DB snapshot a reload applies is always at least as fresh as
	// any reload that already completed (a periodic tick can't clobber a PUT).
	s.loadMu.Lock()
	defer s.loadMu.Unlock()
	stored, err := s.repo.GetAll(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range Defs {
		if v, ok := stored[d.Key]; ok && v != "" {
			s.values[d.Key] = v
		} else {
			s.values[d.Key] = d.Default
		}
	}
	return nil
}

func (s *Service) raw(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.values[key]
}

// Int returns the setting as an int, falling back to the default on parse error.
func (s *Service) Int(key string) int {
	if n, err := strconv.Atoi(s.raw(key)); err == nil {
		return n
	}
	if d, ok := defByKey[key]; ok {
		n, _ := strconv.Atoi(d.Default)
		return n
	}
	return 0
}

// Duration returns the setting as a time.Duration, falling back to the default.
func (s *Service) Duration(key string) time.Duration {
	if d, err := time.ParseDuration(s.raw(key)); err == nil {
		return d
	}
	if def, ok := defByKey[key]; ok {
		d, _ := time.ParseDuration(def.Default)
		return d
	}
	return 0
}

// Effective returns the current value of every known setting (for the admin GET).
func (s *Service) Effective() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.values))
	for k, v := range s.values {
		out[k] = v
	}
	return out
}

// Validate checks a proposed key/value against the registry. Returns an error
// message, or "" when valid. Unknown keys are rejected.
func Validate(key, value string) string {
	d, ok := defByKey[key]
	if !ok {
		return fmt.Sprintf("unknown setting: %s", key)
	}
	switch d.Type {
	case TypeInt:
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Sprintf("%s must be an integer", key)
		}
		if n < 1 || (d.Min > 0 && n < d.Min) {
			return fmt.Sprintf("%s must be >= %d", key, max(1, d.Min))
		}
		if d.Max > 0 && n > d.Max {
			return fmt.Sprintf("%s must be <= %d", key, d.Max)
		}
	case TypeDuration:
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Sprintf("%s must be a duration like \"10m\" or \"90s\"", key)
		}
		if d <= 0 {
			return fmt.Sprintf("%s must be a positive duration", key)
		}
	}
	return ""
}

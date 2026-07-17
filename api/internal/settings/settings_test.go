package settings

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

type fakeRepo struct {
	values map[string]string
}

func (f *fakeRepo) GetAll(_ context.Context) (map[string]string, error) { return f.values, nil }
func (f *fakeRepo) SetMany(_ context.Context, v map[string]string) error {
	for k, val := range v {
		f.values[k] = val
	}
	return nil
}

// Reload must pick up DB changes made by another pod (cross-pod convergence).
func TestReloadPicksUpNewValues(t *testing.T) {
	repo := &fakeRepo{values: map[string]string{}}
	s := New(repo, zap.NewNop())

	if got := s.Int(KeyGroupMaxTurnsAPI); got != 6 {
		t.Fatalf("default = %d, want 6", got)
	}

	// Another pod writes a new value straight to the store.
	repo.values[KeyGroupMaxTurnsAPI] = "9"
	if err := s.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := s.Int(KeyGroupMaxTurnsAPI); got != 9 {
		t.Errorf("after reload = %d, want 9", got)
	}

	// Clearing the override falls back to the code default.
	delete(repo.values, KeyGroupMaxTurnsAPI)
	if err := s.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := s.Int(KeyGroupMaxTurnsAPI); got != 6 {
		t.Errorf("after clear = %d, want default 6", got)
	}
}

func TestDurationFallsBackToDefault(t *testing.T) {
	s := New(nil, zap.NewNop())
	if got := s.Duration(KeyMCPToolCallTimeout); got != 10*time.Minute {
		t.Errorf("default timeout = %v, want 10m", got)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		key, value string
		wantErr    bool
	}{
		{KeyGroupMaxTurnsAPI, "6", false},
		{KeyGroupMaxTurnsAPI, "0", true},   // below min
		{KeyGroupMaxTurnsAPI, "999", true}, // above max
		{KeyGroupMaxTurnsAPI, "abc", true}, // not an int
		{KeyMCPToolCallTimeout, "10m", false},
		{KeyMCPToolCallTimeout, "0s", true}, // non-positive duration
		{KeyMCPToolCallTimeout, "nope", true},
		{"unknown_key", "1", true},
	}
	for _, c := range cases {
		got := Validate(c.key, c.value)
		if (got != "") != c.wantErr {
			t.Errorf("Validate(%q,%q) = %q, wantErr=%v", c.key, c.value, got, c.wantErr)
		}
	}
}

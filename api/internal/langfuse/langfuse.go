package langfuse

import (
	"context"
	"sync"
	"time"

	lfgo "github.com/git-hulk/langfuse-go"
	"github.com/git-hulk/langfuse-go/pkg/traces"

	"github.com/dfradehubs/agentgram-api/internal/config"
	"go.uber.org/zap"
)

// Tracer wraps the Langfuse client. No-op when disabled.
type Tracer struct {
	client      *lfgo.Langfuse
	enabled     bool
	environment string
	inputCost   float64
	outputCost  float64
	logger      *zap.Logger
}

// New creates a Tracer. Returns a no-op tracer if disabled or misconfigured.
func New(cfg *config.LangfuseConfig, logger *zap.Logger) *Tracer {
	if cfg == nil || !cfg.Enabled || cfg.PublicKey == "" || cfg.SecretKey == "" {
		logger.Info("langfuse disabled")
		return &Tracer{enabled: false, logger: logger}
	}
	client := lfgo.NewClient(cfg.Host, cfg.PublicKey, cfg.SecretKey)
	logger.Info("langfuse enabled",
		zap.String("host", cfg.Host),
		zap.String("environment", cfg.Environment))
	return &Tracer{
		client:      client,
		enabled:     true,
		environment: cfg.Environment,
		inputCost:   cfg.InputCostPer1M,
		outputCost:  cfg.OutputCostPer1M,
		logger:      logger,
	}
}

// Close flushes pending events and shuts down the client.
func (t *Tracer) Close() {
	if t.client != nil {
		t.client.Flush()
		if err := t.client.Close(); err != nil {
			t.logger.Warn("langfuse close error", zap.Error(err))
		}
	}
}

// StartTrace begins a new trace for a chat request.
func (t *Tracer) StartTrace(ctx context.Context, name, userEmail, sessionID string, metadata map[string]interface{}) *Trace {
	if !t.enabled {
		return &Trace{}
	}
	tr := t.client.StartTrace(ctx, name)
	tr.UserID = userEmail
	tr.SessionID = sessionID
	tr.Metadata = metadata
	tr.Tags = []string{"agentgram-api"}
	tr.Environment = t.environment
	return &Trace{
		raw:         tr,
		tracer:      t,
		environment: t.environment,
		startTime:   time.Now(),
	}
}

// Trace is the per-request scope. Thread-safe for concurrent tool call spans.
type Trace struct {
	raw         *traces.Trace
	tracer      *Tracer
	environment string
	startTime   time.Time
	mu          sync.Mutex
}

// IsEnabled returns whether this trace is active.
func (t *Trace) IsEnabled() bool {
	return t.raw != nil
}

// SetInput sets the trace-level input (e.g. user message).
func (t *Trace) SetInput(input interface{}) {
	if !t.IsEnabled() {
		return
	}
	t.mu.Lock()
	t.raw.Input = input
	t.mu.Unlock()
}

// StartGeneration begins a generation observation (LLM call).
func (t *Trace) StartGeneration(name, model string, input interface{}) *Generation {
	if !t.IsEnabled() {
		return &Generation{}
	}
	t.mu.Lock()
	obs := t.raw.StartGeneration(name)
	t.mu.Unlock()

	obs.Model = model
	obs.Input = input
	obs.Environment = t.environment
	return &Generation{obs: obs, tracer: t.tracer}
}

// StartToolCall begins a tool call span.
func (t *Trace) StartToolCall(name string, args interface{}) *Span {
	if !t.IsEnabled() {
		return &Span{}
	}
	t.mu.Lock()
	obs := t.raw.StartSpan(name)
	t.mu.Unlock()

	obs.Input = args
	obs.Environment = t.environment
	return &Span{obs: obs}
}

// Enabled returns whether the underlying Langfuse client is active.
func (t *Tracer) Enabled() bool {
	return t.enabled
}

// End finalizes the trace.
func (t *Trace) End(success bool, output interface{}) {
	if !t.IsEnabled() {
		return
	}
	t.mu.Lock()
	t.raw.Output = output
	if !success {
		t.raw.Tags = append(t.raw.Tags, "error")
	}
	t.mu.Unlock()
	t.raw.End()
}

// Generation wraps a Langfuse generation observation.
type Generation struct {
	obs    *traces.Observation
	tracer *Tracer
}

// End finalizes the generation with output and token usage.
func (g *Generation) End(output interface{}, inputTokens, outputTokens int) {
	if g.obs == nil {
		return
	}
	g.obs.Output = output
	g.obs.Usage = traces.Usage{
		Input:  inputTokens,
		Output: outputTokens,
		Total:  inputTokens + outputTokens,
		Unit:   traces.UnitTokens,
	}
	g.obs.End()
}

// EndWithError finalizes the generation as an error.
func (g *Generation) EndWithError(err error) {
	if g.obs == nil {
		return
	}
	g.obs.Level = traces.ObservationLevelError
	g.obs.StatusMessage = err.Error()
	g.obs.End()
}

// Span wraps a Langfuse span observation (for tool calls).
type Span struct {
	obs *traces.Observation
}

// End finalizes the span with output.
func (s *Span) End(output interface{}) {
	if s.obs == nil {
		return
	}
	s.obs.Output = output
	s.obs.End()
}

// EndWithError finalizes the span as an error.
func (s *Span) EndWithError(err error) {
	if s.obs == nil {
		return
	}
	s.obs.Level = traces.ObservationLevelError
	s.obs.StatusMessage = err.Error()
	s.obs.End()
}

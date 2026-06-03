package langfuse

import (
	"context"

	"github.com/dfradehubs/agentgram-api/internal/llm"
)

// TracedProvider wraps an llm.Provider and creates Langfuse generations
// automatically when a Trace is present in the context.
type TracedProvider struct {
	inner llm.Provider
	name  string // generation name (e.g. "summarizer", "file-processor")
	model string // model name for Langfuse display
}

// WrapProvider creates a TracedProvider that instruments LLM calls.
func WrapProvider(inner llm.Provider, name, model string) llm.Provider {
	if inner == nil {
		return nil
	}
	return &TracedProvider{inner: inner, name: name, model: model}
}

// GenerateContent delegates to the inner provider, recording a Langfuse generation
// if a trace is present in the context.
func (p *TracedProvider) GenerateContent(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	trace := TraceFromContext(ctx)
	if trace == nil || !trace.IsEnabled() {
		return p.inner.GenerateContent(ctx, req)
	}

	gen := trace.StartGeneration(p.name, p.model, req.Messages)

	resp, err := p.inner.GenerateContent(ctx, req)
	if err != nil {
		gen.EndWithError(err)
		return nil, err
	}

	gen.End(resp.Text, resp.InputTokens, resp.OutputTokens)
	return resp, nil
}

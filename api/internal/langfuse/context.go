package langfuse

import "context"

type contextKey struct{}

// ContextWithTrace stores a Trace in the context.
func ContextWithTrace(ctx context.Context, trace *Trace) context.Context {
	return context.WithValue(ctx, contextKey{}, trace)
}

// TraceFromContext retrieves the Trace from context, or nil.
func TraceFromContext(ctx context.Context) *Trace {
	t, _ := ctx.Value(contextKey{}).(*Trace)
	return t
}

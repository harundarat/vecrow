package logger

import (
	"context"
	"log/slog"
	"slices"
)

type ctxKey struct{}

// WithAttrs returns a copy of ctx carrying attrs. Every record logged with
// that context (or a context derived from it) includes them.
func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	if len(attrs) == 0 {
		return ctx
	}
	existing, _ := ctx.Value(ctxKey{}).([]slog.Attr)
	merged := make([]slog.Attr, 0, len(existing)+len(attrs))
	merged = append(merged, existing...)
	merged = append(merged, attrs...)
	return context.WithValue(ctx, ctxKey{}, merged)
}

// ContextHandler appends the attributes stored by WithAttrs to each record,
// always at the top level, even on a logger built with WithGroup.
type ContextHandler struct {
	slog.Handler
	// base is the handler before any WithGroup, and ops replays the
	// WithAttrs/WithGroup calls made since. They are set only once a group
	// is open, so context attributes can be added outside that group.
	base slog.Handler
	ops  []func(slog.Handler) slog.Handler
}

// NewContextHandler wraps h.
func NewContextHandler(h slog.Handler) *ContextHandler {
	return &ContextHandler{Handler: h}
}

// Handle implements slog.Handler.
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs, _ := ctx.Value(ctxKey{}).([]slog.Attr)
	if len(attrs) == 0 {
		return h.Handler.Handle(ctx, r)
	}
	if h.base == nil {
		r = r.Clone()
		r.AddAttrs(attrs...)
		return h.Handler.Handle(ctx, r)
	}
	inner := h.base.WithAttrs(attrs)
	for _, op := range h.ops {
		inner = op(inner)
	}
	return inner.Handle(ctx, r)
}

// WithAttrs implements slog.Handler.
func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := &ContextHandler{Handler: h.Handler.WithAttrs(attrs), base: h.base}
	if h.base != nil {
		next.ops = append(slices.Clip(h.ops), func(in slog.Handler) slog.Handler { return in.WithAttrs(attrs) })
	}
	return next
}

// WithGroup implements slog.Handler.
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	base := h.base
	if base == nil {
		base = h.Handler
	}
	return &ContextHandler{
		Handler: h.Handler.WithGroup(name),
		base:    base,
		ops:     append(slices.Clip(h.ops), func(in slog.Handler) slog.Handler { return in.WithGroup(name) }),
	}
}

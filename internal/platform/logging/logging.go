// Package logging configures log/slog and carries a request-scoped logger in
// the context, so every log line emitted while serving a request is tagged
// with its request ID without passing a logger around explicitly.
package logging

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

// Options configures New.
type Options struct {
	Level   string // debug|info|warn|error
	Format  string // json|text
	Service string
	Version string
	Env     string
}

// New returns a structured logger writing to w.
func New(w io.Writer, o Options) *slog.Logger {
	opts := &slog.HandlerOptions{Level: ParseLevel(o.Level)}
	var h slog.Handler
	if o.Format == "text" {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	var attrs []any
	for _, a := range []slog.Attr{
		slog.String("service", o.Service),
		slog.String("version", o.Version),
		slog.String("env", o.Env),
	} {
		if a.Value.String() != "" {
			attrs = append(attrs, a)
		}
	}
	return slog.New(h).With(attrs...)
}

// ParseLevel converts a level name to a slog.Level, defaulting to Info.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Discard returns a logger that drops everything (tests).
func Discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

type ctxKey struct{}

// WithContext returns a copy of ctx carrying l.
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext returns the logger stored in ctx, or slog.Default().
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

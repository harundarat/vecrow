// Package logger builds the structured slog logger used across the service.
//
// Conventions:
//   - The *slog.Logger is injected through constructors; packages never reach
//     for a global logger.
//   - Always log with the *Context variants (InfoContext, ErrorContext, ...)
//     so request- and message-scoped attributes added with WithAttrs are
//     included automatically.
//   - Errors are logged once, at the boundary that handles them (HTTP access
//     log, queue consumer, scheduler). Inner layers wrap and return errors.
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Config controls the logger output.
type Config struct {
	Level   string // debug, info, warn, error
	Format  string // json or text
	Service string
	Env     string
	Version string
	// Output defaults to os.Stdout.
	Output io.Writer
}

// New returns a logger that writes Format records at Level or above, carries
// service/env/version on every record, appends context attributes, and
// redacts sensitive keys.
func New(cfg Config) (*slog.Logger, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		return nil, fmt.Errorf("logger: invalid level %q", cfg.Level)
	}
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}
	opts := &slog.HandlerOptions{
		Level:       level,
		AddSource:   level <= slog.LevelDebug,
		ReplaceAttr: replaceAttr,
	}

	var h slog.Handler
	switch strings.ToLower(cfg.Format) {
	case "json":
		h = slog.NewJSONHandler(out, opts)
	case "text":
		h = slog.NewTextHandler(out, opts)
	default:
		return nil, fmt.Errorf("logger: invalid format %q", cfg.Format)
	}

	var base []any
	for _, a := range []slog.Attr{
		slog.String("service", cfg.Service),
		slog.String("env", cfg.Env),
		slog.String("version", cfg.Version),
	} {
		if a.Value.String() != "" {
			base = append(base, a)
		}
	}
	return slog.New(NewContextHandler(h)).With(base...), nil
}

// Err returns the canonical attribute for an error.
func Err(err error) slog.Attr {
	if err == nil {
		return slog.Attr{}
	}
	return slog.String("error", err.Error())
}

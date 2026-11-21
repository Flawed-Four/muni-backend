// Package logging builds the application's slog logger and carries
// request-scoped fields (request_id, user_id) through context.Context.
//
// Code anywhere in the app logs through the slog default with the request
// context, e.g. slog.ErrorContext(ctx, ...), and gets those fields for free.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"

	"github.com/Flawed-Four/muni-backend/internal/config"
)

// New builds a logger writing to w: JSON in production, text elsewhere,
// unless LOG_FORMAT says otherwise.
func New(cfg *config.Config, w io.Writer) (*slog.Logger, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.Log.Level)); err != nil {
		return nil, fmt.Errorf("invalid LOG_LEVEL %q: want debug, info, warn or error", cfg.Log.Level)
	}

	format := strings.ToLower(cfg.Log.Format)
	if format == "" {
		format = "text"
		if cfg.App.Env == "production" {
			format = "json"
		}
	}

	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	switch format {
	case "json":
		h = slog.NewJSONHandler(w, opts)
	case "text":
		h = slog.NewTextHandler(w, opts)
	default:
		return nil, fmt.Errorf("invalid LOG_FORMAT %q: want json or text", cfg.Log.Format)
	}
	return slog.New(contextHandler{h}), nil
}

type ctxKey struct{}

// With returns a copy of ctx whose log records also carry attrs.
func With(ctx context.Context, attrs ...slog.Attr) context.Context {
	prev, _ := ctx.Value(ctxKey{}).([]slog.Attr)
	return context.WithValue(ctx, ctxKey{}, append(slices.Clip(prev), attrs...))
}

// contextHandler adds the attrs stored by With to every record.
type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if attrs, ok := ctx.Value(ctxKey{}).([]slog.Attr); ok {
		r.AddAttrs(attrs...)
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs and WithGroup re-wrap the result, otherwise logger.With(...)
// would return a plain handler that ignores the context.
func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}

// Package logging builds the structured logger shared by all services and
// carries request-scoped log attributes through context.Context.
package logging

import (
	"context"
	"io"
	"log/slog"
	"slices"
)

// Output formats accepted by New.
const (
	FormatJSON = "json"
	FormatText = "text"
)

// New returns a logger writing to w in the given format (FormatJSON or
// FormatText). Unknown formats fall back to JSON.
//
// JSON output uses the field names Google Cloud Logging recognises (severity,
// message), so log levels and messages are parsed without agent configuration.
// Attributes stored in a context with WithAttrs are added to every record
// logged through the *Context methods (InfoContext, ErrorContext, ...).
func New(w io.Writer, level slog.Level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}

	var h slog.Handler
	if format == FormatText {
		h = slog.NewTextHandler(w, opts)
	} else {
		opts.ReplaceAttr = cloudLoggingAttrs
		h = slog.NewJSONHandler(w, opts)
	}
	return slog.New(contextHandler{h})
}

func cloudLoggingAttrs(groups []string, a slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return a
	}
	switch a.Key {
	case slog.MessageKey:
		a.Key = "message"
	case slog.LevelKey:
		a.Key = "severity"
		if lvl, ok := a.Value.Any().(slog.Level); ok && lvl == slog.LevelWarn {
			a.Value = slog.StringValue("WARNING")
		}
	}
	return a
}

type attrsKey struct{}

// WithAttrs returns a copy of ctx carrying attrs in addition to any attributes
// already stored in it. Every record logged with that context includes them.
func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	existing, _ := ctx.Value(attrsKey{}).([]slog.Attr)
	return context.WithValue(ctx, attrsKey{}, append(slices.Clip(existing), attrs...))
}

type contextHandler struct {
	slog.Handler
}

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if attrs, ok := ctx.Value(attrsKey{}).([]slog.Attr); ok {
		r.AddAttrs(attrs...)
	}
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}

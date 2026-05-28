package observability

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// logger wraps slog.Logger while keeping the type private
type logger struct {
	*slog.Logger
}

// loggerKey is the context key for request-scoped loggers
type loggerKey struct{}

// newLogger creates the base structured JSON logger
func newLogger(level string) *logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	default:
		lvl = slog.LevelInfo
	}

	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     lvl,
		AddSource: lvl == slog.LevelDebug,
	})

	base := slog.New(h).With(slog.String("service", "verge"))
	return &logger{base}
}

// WithLogger injects a logger into the context
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// L returns the request-scoped logger enriched with trace metadata
func L(ctx context.Context) *slog.Logger {
	var base *slog.Logger

	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && l != nil {
		base = l
	} else {
		base = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})).With(slog.String("service", "verge"))
	}

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		sc := span.SpanContext()
		return base.With(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}

	return base
}

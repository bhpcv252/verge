//go:build integration

package observability

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestL_InjectsTraceAndSpanIDs_WhenSpanIsActive(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(t.Context(), "test-op")
	defer span.End()

	sc := span.SpanContext()
	require.True(t, sc.IsValid(), "real SDK span context must be valid")

	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	ctx = WithLogger(ctx, slog.New(h))

	L(ctx).Info("traced log")

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))

	assert.Equal(t, sc.TraceID().String(), record["trace_id"],
		"trace_id must match the active span")
	assert.Equal(t, sc.SpanID().String(), record["span_id"],
		"span_id must match the active span")
}

package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newLogger

func TestNewLogger_InfoLevel(t *testing.T) {
	l := newLogger("info")
	require.NotNil(t, l)

	ctx := context.Background()
	assert.True(t, l.Enabled(ctx, slog.LevelInfo))
	assert.True(t, l.Enabled(ctx, slog.LevelWarn))
	assert.True(t, l.Enabled(ctx, slog.LevelError))
	assert.False(t, l.Enabled(ctx, slog.LevelDebug))
}

func TestNewLogger_DebugLevel(t *testing.T) {
	l := newLogger("debug")
	require.NotNil(t, l)
	assert.True(t, l.Enabled(context.Background(), slog.LevelDebug))
}

func TestNewLogger_UnknownLevel_DefaultsToInfo(t *testing.T) {
	for _, level := range []string{"", "trace", "VERBOSE", "0"} {
		l := newLogger(level)
		require.NotNil(t, l, "level=%q", level)
		assert.True(t, l.Enabled(context.Background(), slog.LevelInfo), "level=%q", level)
		assert.False(t, l.Enabled(context.Background(), slog.LevelDebug), "level=%q", level)
	}
}

func TestNewLogger_LevelParsing_IsCaseInsensitive(t *testing.T) {
	for _, level := range []string{"DEBUG", "Debug", "dEbUg"} {
		l := newLogger(level)
		require.NotNil(t, l, "level=%q", level)
		assert.True(t, l.Enabled(context.Background(), slog.LevelDebug), "level=%q", level)
	}
}

// WithLogger / L

func TestWithLogger_StoresAndRetrievesLogger(t *testing.T) {
	var buf bytes.Buffer
	injected := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx := WithLogger(context.Background(), injected)
	L(ctx).Info("sentinel")

	assert.Contains(t, buf.String(), "sentinel", "L() must use the injected logger")
}

func TestL_FallsBackWhenNoLoggerInContext(t *testing.T) {
	got := L(context.Background())
	require.NotNil(t, got)
	assert.NotPanics(t, func() { got.Info("fallback") })
}

func TestL_NilLoggerValueInContext_FallsBack(t *testing.T) {
	ctx := context.WithValue(context.Background(), loggerKey{}, (*slog.Logger)(nil))
	got := L(ctx)
	require.NotNil(t, got)
	assert.NotPanics(t, func() { got.Info("nil fallback") })
}

func TestL_PreservesExtraFieldsFromInjectedLogger(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})).
		With(slog.String("request_id", "req-xyz"))

	ctx := WithLogger(context.Background(), base)
	L(ctx).Info("with fields")

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
	assert.Equal(t, "req-xyz", record["request_id"])
}

func TestL_NoTraceIDsInjected_WhenNoSpan(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	ctx := WithLogger(context.Background(), slog.New(h))

	L(ctx).Info("no span")

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))

	_, hasTraceID := record["trace_id"]
	_, hasSpanID := record["span_id"]
	assert.False(t, hasTraceID, "trace_id must not appear when there is no active span")
	assert.False(t, hasSpanID, "span_id must not appear when there is no active span")
}

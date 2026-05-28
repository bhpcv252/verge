//go:build integration

package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/config"
)

func enabledStdoutCfg() config.OTelConfig {
	return config.OTelConfig{
		Enabled:         true,
		Exporter:        "stdout",
		ServiceName:     "verge-test",
		SampleRate:      1.0,
		MetricsInterval: 15 * time.Second,
		LogLevel:        "info",
	}
}

func enabledPrometheusCfg() config.OTelConfig {
	cfg := enabledStdoutCfg()
	cfg.Exporter = "prometheus"
	return cfg
}

// New

func TestNew_StdoutExporter_ReturnsValidProvider(t *testing.T) {
	p, err := New(enabledStdoutCfg())
	require.NoError(t, err)
	require.NotNil(t, p)
	defer func() { _ = p.Shutdown(context.Background()) }()

	assert.NotNil(t, p.Tracer)
	assert.NotNil(t, p.Meter)
	assert.NotNil(t, p.Logger)
	assert.NotNil(t, p.Metrics)
}

func TestNew_StdoutExporter_ShutdownFlushes(t *testing.T) {
	p, err := New(enabledStdoutCfg())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	assert.NoError(t, p.Shutdown(ctx))
}

func TestNew_StdoutExporter_ShutdownCalledTwice_DoesNotPanic(t *testing.T) {
	p, err := New(enabledStdoutCfg())
	require.NoError(t, err)

	ctx := context.Background()
	_ = p.Shutdown(ctx)
	assert.NotPanics(t, func() { _ = p.Shutdown(ctx) })
}

func TestNew_PrometheusExporter_ExposesHandler(t *testing.T) {
	p, err := New(enabledPrometheusCfg())
	require.NoError(t, err)
	defer func() { _ = p.Shutdown(context.Background()) }()

	require.NotNil(t, p.PrometheusHandler,
		"PrometheusHandler must be non-nil when exporter=prometheus")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	p.PrometheusHandler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/plain")
}

func TestNew_PrometheusExporter_FullInstrumentSet(t *testing.T) {
	p, err := New(enabledPrometheusCfg())
	require.NoError(t, err)
	defer func() { _ = p.Shutdown(context.Background()) }()

	m := p.Metrics
	require.NotNil(t, m)
	assert.NotNil(t, m.HTTPRequestsTotal)
	assert.NotNil(t, m.StorageOperationDuration)
	assert.NotNil(t, m.OutboxLagEvents)
}

func TestNew_ZeroSampleRate_DoesNotError(t *testing.T) {
	cfg := enabledStdoutCfg()
	cfg.SampleRate = 0.0

	p, err := New(cfg)
	require.NoError(t, err)
	defer func() { _ = p.Shutdown(context.Background()) }()

	assert.NotNil(t, p.Tracer)
}

func TestNew_DebugLogLevel_DoesNotError(t *testing.T) {
	cfg := enabledStdoutCfg()
	cfg.LogLevel = "debug"

	p, err := New(cfg)
	require.NoError(t, err)
	defer func() { _ = p.Shutdown(context.Background()) }()

	assert.NotNil(t, p.Logger)
}

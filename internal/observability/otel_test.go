package observability

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/config"
)

func disabledCfg() config.OTelConfig {
	return config.OTelConfig{
		Enabled:         false,
		ServiceName:     "verge-test",
		SampleRate:      1.0,
		MetricsInterval: 15 * time.Second,
		LogLevel:        "info",
	}
}

// New

func TestNew_Disabled_ReturnsValidProvider(t *testing.T) {
	p, err := New(disabledCfg())
	require.NoError(t, err)
	require.NotNil(t, p)

	assert.NotNil(t, p.Tracer)
	assert.NotNil(t, p.Meter)
	assert.NotNil(t, p.Logger)
	assert.NotNil(t, p.Metrics)
}

func TestNew_Disabled_PrometheusHandlerIsNil(t *testing.T) {
	p, err := New(disabledCfg())
	require.NoError(t, err)
	assert.Nil(t, p.PrometheusHandler)
}

func TestNew_Disabled_ShutdownIsNoop(t *testing.T) {
	p, err := New(disabledCfg())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	assert.NoError(t, p.Shutdown(ctx))
}

func TestNew_Disabled_TracerAndMeterDoNotPanic(t *testing.T) {
	p, err := New(disabledCfg())
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		ctx, span := p.Tracer.Start(t.Context(), "test")
		span.End()
		p.Metrics.HTTPRequestsTotal.Add(ctx, 1)
	})
}

// Noop()

func TestNoop_ReturnsFullyInitializedProvider(t *testing.T) {
	p := Noop()
	require.NotNil(t, p)

	assert.NotNil(t, p.Tracer)
	assert.NotNil(t, p.Meter)
	assert.NotNil(t, p.Logger)
	assert.NotNil(t, p.Metrics)
}

func TestNoop_AllMetricInstrumentsNonNil(t *testing.T) {
	m := Noop().Metrics

	assert.NotNil(t, m.HTTPRequestsTotal)
	assert.NotNil(t, m.HTTPRequestDuration)
	assert.NotNil(t, m.HTTPRequestsInFlight)
	assert.NotNil(t, m.GRPCRequestsTotal)
	assert.NotNil(t, m.GRPCRequestDuration)
	assert.NotNil(t, m.StorageOperationDuration)
	assert.NotNil(t, m.StorageErrorsTotal)
	assert.NotNil(t, m.StorageCacheHitsTotal)
	assert.NotNil(t, m.StorageCacheMissesTotal)
	assert.NotNil(t, m.OutboxEventsProcessedTotal)
	assert.NotNil(t, m.OutboxPollDuration)
	assert.NotNil(t, m.OutboxLagEvents)
	assert.NotNil(t, m.OutboxBatchSize)
}

func TestNoop_ShutdownReturnsNil(t *testing.T) {
	p := Noop()
	assert.NoError(t, p.Shutdown(context.Background()))
}

func TestNoop_TracerAndMeter_DoNotPanic(t *testing.T) {
	p := Noop()
	assert.NotPanics(t, func() {
		ctx, span := p.Tracer.Start(t.Context(), "test")
		span.End()
		p.Metrics.HTTPRequestsTotal.Add(ctx, 1)
	})
}

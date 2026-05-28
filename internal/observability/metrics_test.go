package observability

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

// newMetrics

func TestNewMetrics_WithNoopMeter_DoesNotError(t *testing.T) {
	m, err := newMetrics(otel.GetMeterProvider().Meter("verge"))
	require.NoError(t, err)
	require.NotNil(t, m)
}

func TestNewMetrics_AllHTTPInstrumentsCreated(t *testing.T) {
	m, err := newMetrics(otel.GetMeterProvider().Meter("verge"))
	require.NoError(t, err)

	assert.NotNil(t, m.HTTPRequestsTotal, "HTTPRequestsTotal")
	assert.NotNil(t, m.HTTPRequestDuration, "HTTPRequestDuration")
	assert.NotNil(t, m.HTTPRequestsInFlight, "HTTPRequestsInFlight")
}

func TestNewMetrics_AllGRPCInstrumentsCreated(t *testing.T) {
	m, err := newMetrics(otel.GetMeterProvider().Meter("verge"))
	require.NoError(t, err)

	assert.NotNil(t, m.GRPCRequestsTotal, "GRPCRequestsTotal")
	assert.NotNil(t, m.GRPCRequestDuration, "GRPCRequestDuration")
}

func TestNewMetrics_AllStorageInstrumentsCreated(t *testing.T) {
	m, err := newMetrics(otel.GetMeterProvider().Meter("verge"))
	require.NoError(t, err)

	assert.NotNil(t, m.StorageOperationDuration, "StorageOperationDuration")
	assert.NotNil(t, m.StorageErrorsTotal, "StorageErrorsTotal")
	assert.NotNil(t, m.StorageCacheHitsTotal, "StorageCacheHitsTotal")
	assert.NotNil(t, m.StorageCacheMissesTotal, "StorageCacheMissesTotal")
}

func TestNewMetrics_AllOutboxInstrumentsCreated(t *testing.T) {
	m, err := newMetrics(otel.GetMeterProvider().Meter("verge"))
	require.NoError(t, err)

	assert.NotNil(t, m.OutboxEventsProcessedTotal, "OutboxEventsProcessedTotal")
	assert.NotNil(t, m.OutboxPollDuration, "OutboxPollDuration")
	assert.NotNil(t, m.OutboxLagEvents, "OutboxLagEvents")
	assert.NotNil(t, m.OutboxBatchSize, "OutboxBatchSize")
}

func TestNewMetrics_AllAuthInstrumentsCreated(t *testing.T) {
	m, err := newMetrics(otel.GetMeterProvider().Meter("verge"))
	require.NoError(t, err)

	assert.NotNil(t, m.AuthFailuresTotal, "AuthFailuresTotal")
}

func TestNewMetrics_CalledTwice_DoesNotError(t *testing.T) {
	meter := otel.GetMeterProvider().Meter("verge-metrics-test")
	m1, err := newMetrics(meter)
	require.NoError(t, err)
	require.NotNil(t, m1)

	m2, err := newMetrics(meter)
	require.NoError(t, err)
	require.NotNil(t, m2)
}

// instrument usability

func TestMetrics_AllInstrumentsCallable(t *testing.T) {
	obs := Noop()
	require.NotNil(t, obs)
	ctx := t.Context()

	assert.NotPanics(t, func() {
		obs.Metrics.HTTPRequestsTotal.Add(ctx, 1)
		obs.Metrics.HTTPRequestDuration.Record(ctx, 0.01)
		obs.Metrics.HTTPRequestsInFlight.Add(ctx, 1)

		obs.Metrics.GRPCRequestsTotal.Add(ctx, 1)
		obs.Metrics.GRPCRequestDuration.Record(ctx, 0.01)

		obs.Metrics.StorageOperationDuration.Record(ctx, 0.001)
		obs.Metrics.StorageErrorsTotal.Add(ctx, 1)
		obs.Metrics.StorageCacheHitsTotal.Add(ctx, 1)
		obs.Metrics.StorageCacheMissesTotal.Add(ctx, 1)

		obs.Metrics.OutboxEventsProcessedTotal.Add(ctx, 1)
		obs.Metrics.OutboxPollDuration.Record(ctx, 0.5)
		obs.Metrics.OutboxLagEvents.Record(ctx, 42)
		obs.Metrics.OutboxBatchSize.Record(ctx, 10)

		obs.Metrics.AuthFailuresTotal.Add(ctx, 1)
	})
}

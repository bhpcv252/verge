package observability

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

type Metrics struct {
	// HTTP
	HTTPRequestsTotal    metric.Int64Counter
	HTTPRequestDuration  metric.Float64Histogram
	HTTPRequestsInFlight metric.Int64UpDownCounter

	// gRPC
	GRPCRequestsTotal   metric.Int64Counter
	GRPCRequestDuration metric.Float64Histogram

	// storage
	StorageOperationDuration metric.Float64Histogram
	StorageErrorsTotal       metric.Int64Counter
	StorageCacheHitsTotal    metric.Int64Counter
	StorageCacheMissesTotal  metric.Int64Counter

	// outbox worker
	OutboxEventsProcessedTotal metric.Int64Counter
	OutboxPollDuration         metric.Float64Histogram
	OutboxLagEvents            metric.Int64Gauge
	OutboxBatchSize            metric.Float64Histogram
}

func newMetrics(meter metric.Meter) (*Metrics, error) {
	httpBuckets := metric.WithExplicitBucketBoundaries(
		.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5,
	)

	storageBuckets := metric.WithExplicitBucketBoundaries(
		.001, .005, .01, .025, .05, .1, .25, .5, 1,
	)

	outboxBuckets := metric.WithExplicitBucketBoundaries(
		.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 30,
	)

	var (
		m   Metrics
		err error
	)

	// HTTP

	m.HTTPRequestsTotal, err = meter.Int64Counter(
		"verge_http_requests_total",
		metric.WithDescription("Total number of completed HTTP requests."),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: verge_http_requests_total: %w", err)
	}

	m.HTTPRequestDuration, err = meter.Float64Histogram(
		"verge_http_request_duration_seconds",
		metric.WithDescription("Duration of HTTP requests in seconds."),
		metric.WithUnit("s"),
		httpBuckets,
	)
	if err != nil {
		return nil, fmt.Errorf("observability: verge_http_request_duration_seconds: %w", err)
	}

	m.HTTPRequestsInFlight, err = meter.Int64UpDownCounter(
		"verge_http_requests_in_flight",
		metric.WithDescription("Number of HTTP requests currently being processed."),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: verge_http_requests_in_flight: %w", err)
	}

	// gRPC

	m.GRPCRequestsTotal, err = meter.Int64Counter(
		"verge_grpc_requests_total",
		metric.WithDescription("Total number of completed gRPC RPCs."),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: verge_grpc_requests_total: %w", err)
	}

	m.GRPCRequestDuration, err = meter.Float64Histogram(
		"verge_grpc_request_duration_seconds",
		metric.WithDescription("Duration of gRPC RPCs in seconds."),
		metric.WithUnit("s"),
		httpBuckets,
	)
	if err != nil {
		return nil, fmt.Errorf("observability: verge_grpc_request_duration_seconds: %w", err)
	}

	// storage

	m.StorageOperationDuration, err = meter.Float64Histogram(
		"verge_storage_operation_duration_seconds",
		metric.WithDescription("Duration of storage backend operations in seconds."),
		metric.WithUnit("s"),
		storageBuckets,
	)
	if err != nil {
		return nil, fmt.Errorf("observability: verge_storage_operation_duration_seconds: %w", err)
	}

	m.StorageErrorsTotal, err = meter.Int64Counter(
		"verge_storage_errors_total",
		metric.WithDescription("Total number of storage backend errors."),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: verge_storage_errors_total: %w", err)
	}

	m.StorageCacheHitsTotal, err = meter.Int64Counter(
		"verge_storage_cache_hits_total",
		metric.WithDescription("Total number of cache hits (Redis)."),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: verge_storage_cache_hits_total: %w", err)
	}

	m.StorageCacheMissesTotal, err = meter.Int64Counter(
		"verge_storage_cache_misses_total",
		metric.WithDescription("Total number of cache misses that fell through to PostgreSQL."),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: verge_storage_cache_misses_total: %w", err)
	}

	// outbox worker

	m.OutboxEventsProcessedTotal, err = meter.Int64Counter(
		"verge_outbox_events_processed_total",
		metric.WithDescription("Total number of outbox events processed by the worker."),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: verge_outbox_events_processed_total: %w", err)
	}

	m.OutboxPollDuration, err = meter.Float64Histogram(
		"verge_outbox_poll_duration_seconds",
		metric.WithDescription("Duration of a single outbox worker poll cycle in seconds."),
		metric.WithUnit("s"),
		outboxBuckets,
	)
	if err != nil {
		return nil, fmt.Errorf("observability: verge_outbox_poll_duration_seconds: %w", err)
	}

	m.OutboxLagEvents, err = meter.Int64Gauge(
		"verge_outbox_lag_events",
		metric.WithDescription("Number of pending events in the outbox table."),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: verge_outbox_lag_events: %w", err)
	}

	m.OutboxBatchSize, err = meter.Float64Histogram(
		"verge_outbox_batch_size",
		metric.WithDescription("Number of events processed per poll cycle."),
		metric.WithExplicitBucketBoundaries(1, 5, 10, 25, 50, 100, 250, 500, 1000),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: verge_outbox_batch_size: %w", err)
	}

	return &m, nil
}

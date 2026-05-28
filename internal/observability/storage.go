package observability

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

func (p *Provider) StorageSpan(
	ctx context.Context,
	backend,
	operation string,
) (context.Context, func(error)) {
	ctx, span := p.Tracer.Start(
		ctx,
		"verge.storage "+backend+"."+operation,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("db.system", dbSystem(backend)),
			attribute.String("db.operation.name", operation),
			attribute.String("verge.storage.backend", backend),
		),
	)

	start := time.Now()

	done := func(err error) {
		duration := time.Since(start).Seconds()

		p.Metrics.StorageOperationDuration.Record(
			ctx,
			duration,
			metric.WithAttributes(
				attribute.String("backend", backend),
				attribute.String("operation", operation),
			),
		)

		if err != nil {
			p.Metrics.StorageErrorsTotal.Add(
				ctx,
				1,
				metric.WithAttributes(
					attribute.String("backend", backend),
					attribute.String("operation", operation),
				),
			)

			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
		} else {
			span.SetStatus(otelcodes.Ok, "")
		}

		span.End()
	}

	return ctx, done
}

func (p *Provider) RecordCacheHit(
	ctx context.Context,
	backend,
	cache string,
) {
	p.Metrics.StorageCacheHitsTotal.Add(
		ctx,
		1,
		metric.WithAttributes(
			attribute.String("backend", backend),
			attribute.String("cache", cache),
		),
	)
}

func (p *Provider) RecordCacheMiss(
	ctx context.Context,
	backend,
	cache string,
) {
	p.Metrics.StorageCacheMissesTotal.Add(
		ctx,
		1,
		metric.WithAttributes(
			attribute.String("backend", backend),
			attribute.String("cache", cache),
		),
	)
}

func dbSystem(backend string) string {
	switch backend {
	case "postgres":
		return "postgresql"

	case "redis":
		return "redis"

	case "neo4j":
		return "neo4j"

	default:
		return backend
	}
}

//go:build integration

package observability

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func storageObsWithRealTracer(exp *tracetest.InMemoryExporter) *Provider {
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	obs := Noop()
	obs.Tracer = tp.Tracer("verge")
	return obs
}

// StorageSpan

func TestStorageSpan_SpanName_FollowsConvention(t *testing.T) {
	cases := []struct{ backend, operation, wantName string }{
		{"postgres", "commit.insert", "verge.storage postgres.commit.insert"},
		{"redis", "branch.get", "verge.storage redis.branch.get"},
		{"neo4j", "graph.ancestors", "verge.storage neo4j.graph.ancestors"},
	}

	for _, tc := range cases {
		t.Run(tc.wantName, func(t *testing.T) {
			exp := tracetest.NewInMemoryExporter()
			obs := storageObsWithRealTracer(exp)

			_, done := obs.StorageSpan(context.Background(), tc.backend, tc.operation)
			done(nil)

			spans := exp.GetSpans()
			require.Len(t, spans, 1)
			assert.Equal(t, tc.wantName, spans[0].Name)
		})
	}
}

func TestStorageSpan_SetsDBSystemAttribute(t *testing.T) {
	cases := []struct {
		backend   string
		wantDBSys string
	}{
		{"postgres", "postgresql"},
		{"redis", "redis"},
		{"neo4j", "neo4j"},
		{"custom", "custom"},
	}

	for _, tc := range cases {
		t.Run(tc.backend, func(t *testing.T) {
			exp := tracetest.NewInMemoryExporter()
			obs := storageObsWithRealTracer(exp)

			_, done := obs.StorageSpan(context.Background(), tc.backend, "test.op")
			done(nil)

			spans := exp.GetSpans()
			require.Len(t, spans, 1)

			attrs := make(map[string]string)
			for _, a := range spans[0].Attributes {
				attrs[string(a.Key)] = a.Value.AsString()
			}

			assert.Equal(t, tc.wantDBSys, attrs["db.system"])
			assert.Equal(t, tc.backend, attrs["verge.storage.backend"])
			assert.Equal(t, "test.op", attrs["db.operation.name"])
		})
	}
}

func TestStorageSpan_SpanKindIsInternal(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	obs := storageObsWithRealTracer(exp)

	_, done := obs.StorageSpan(context.Background(), "neo4j", "graph.ancestors")
	done(nil)

	spans := exp.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, trace.SpanKindInternal, spans[0].SpanKind)
}

func TestStorageSpan_Error_RecordsExceptionEvent(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	obs := storageObsWithRealTracer(exp)

	_, done := obs.StorageSpan(context.Background(), "postgres", "commit.insert")
	done(errors.New("connection refused"))

	spans := exp.GetSpans()
	require.Len(t, spans, 1)

	require.NotEmpty(t, spans[0].Events, "error event must be recorded on the span")
	assert.Equal(t, "exception", spans[0].Events[0].Name)
}

func TestStorageSpan_Success_RecordsNoErrorEvents(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	obs := storageObsWithRealTracer(exp)

	_, done := obs.StorageSpan(context.Background(), "postgres", "branch.update")
	done(nil)

	spans := exp.GetSpans()
	require.Len(t, spans, 1)
	assert.Empty(t, spans[0].Events)
}

func TestStorageSpan_IsChildOfActiveParentSpan(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	tracer := tp.Tracer("verge")

	obs := Noop()
	obs.Tracer = tracer

	parentCtx, parentSpan := tracer.Start(context.Background(), "parent")
	_, done := obs.StorageSpan(parentCtx, "postgres", "commit.insert")
	done(nil)
	parentSpan.End()

	spans := exp.GetSpans()
	require.Len(t, spans, 2)

	var child *tracetest.SpanStub
	for i := range spans {
		if spans[i].Name == "verge.storage postgres.commit.insert" {
			child = &spans[i]
		}
	}
	require.NotNil(t, child, "storage span must be present in exported spans")

	assert.Equal(t, parentSpan.SpanContext().TraceID(), child.SpanContext.TraceID())
	assert.Equal(t, parentSpan.SpanContext().SpanID(), child.Parent.SpanID())
}

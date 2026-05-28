//go:build integration

package observability

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc/metadata"
)

func providerWithRealTracer(exp *tracetest.InMemoryExporter) *Provider {
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	obs := Noop()
	obs.Tracer = tp.Tracer("verge")
	return obs
}

func buildRouterWith(obs *Provider, method, pattern string, fn http.HandlerFunc) http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(HTTPMiddleware(obs))
	r.MethodFunc(method, pattern, fn)
	return r
}

// HTTPMiddleware

func TestHTTPMiddleware_SpanName_UsesRoutePattern(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	obs := providerWithRealTracer(exp)

	router := buildRouterWith(obs, http.MethodPost, "/v1/repos/{repo_id}/commits",
		func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusCreated) })

	req := httptest.NewRequest(http.MethodPost, "/v1/repos/repo_abc/commits", nil)
	router.ServeHTTP(httptest.NewRecorder(), req)

	spans := exp.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "POST /v1/repos/{repo_id}/commits", spans[0].Name)
}

func TestHTTPMiddleware_PropagatesIncomingTraceParent(t *testing.T) {
	const traceID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const spanID = "bbbbbbbbbbbbbbbb"

	exp := tracetest.NewInMemoryExporter()
	obs := providerWithRealTracer(exp)

	router := buildRouterWith(obs, http.MethodGet, "/v1/repos",
		func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/v1/repos", nil)
	req.Header.Set("traceparent", fmt.Sprintf("00-%s-%s-01", traceID, spanID))
	router.ServeHTTP(httptest.NewRecorder(), req)

	spans := exp.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, traceID, spans[0].SpanContext.TraceID().String())
}

// GRPCUnaryInterceptor

func TestGRPCUnaryInterceptor_PropagatesIncomingTraceParent(t *testing.T) {
	const traceID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const spanID = "bbbbbbbbbbbbbbbb"

	exp := tracetest.NewInMemoryExporter()
	obs := providerWithRealTracer(exp)

	interceptor := GRPCUnaryInterceptor(obs)

	md := metadata.New(map[string]string{
		"traceparent": fmt.Sprintf("00-%s-%s-01", traceID, spanID),
	})
	ctx := metadata.NewIncomingContext(t.Context(), md)

	_, err := interceptor(ctx, nil, noopUnaryInfo("/verge.v1.CommitService/GetCommit"), okHandler)
	require.NoError(t, err)

	spans := exp.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, traceID, spans[0].SpanContext.TraceID().String())
}

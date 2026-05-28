package observability

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func HTTPMiddleware(obs *Provider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// extract trace context from request headers
			ctx = otel.GetTextMapPropagator().Extract(
				ctx,
				propagation.HeaderCarrier(r.Header),
			)

			// start request span
			ctx, span := obs.Tracer.Start(ctx, r.Method,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					semconv.HTTPRequestMethodKey.String(r.Method),
					attribute.String("url.path", r.URL.Path),
				),
			)
			defer span.End()

			// create request-scoped logger
			requestID := chimiddleware.GetReqID(ctx)

			reqLogger := obs.Logger.With(
				slog.String("request_id", requestID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			)

			ctx = WithLogger(ctx, reqLogger)

			// track in-flight requests
			obs.Metrics.HTTPRequestsInFlight.Add(ctx, 1)
			defer obs.Metrics.HTTPRequestsInFlight.Add(ctx, -1)

			L(ctx).Info("request started")

			// execute handler
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()

			next.ServeHTTP(ww, r.WithContext(ctx))

			// finalize telemetry
			statusCode := ww.Status()
			if statusCode == 0 {
				statusCode = http.StatusOK
			}

			duration := time.Since(start)

			routePattern := chi.RouteContext(ctx).RoutePattern()
			if routePattern == "" {
				routePattern = r.URL.Path
			}

			span.SetName(r.Method + " " + routePattern)

			span.SetAttributes(
				semconv.HTTPRouteKey.String(routePattern),
				semconv.HTTPResponseStatusCodeKey.Int(statusCode),
			)

			if statusCode >= 500 {
				span.SetStatus(otelcodes.Error, fmt.Sprintf("HTTP %d", statusCode))
			} else {
				span.SetStatus(otelcodes.Ok, "")
			}

			totalAttrs := metric.WithAttributes(
				attribute.String("method", r.Method),
				attribute.String("route", routePattern),
				attribute.Int("status_code", statusCode),
			)

			durationAttrs := metric.WithAttributes(
				attribute.String("method", r.Method),
				attribute.String("route", routePattern),
			)

			obs.Metrics.HTTPRequestsTotal.Add(ctx, 1, totalAttrs)

			obs.Metrics.HTTPRequestDuration.Record(
				ctx,
				duration.Seconds(),
				durationAttrs,
			)

			L(ctx).Info("request completed",
				slog.String("route", routePattern),
				slog.Int("status_code", statusCode),
				slog.Int64("duration_ms", duration.Milliseconds()),
			)
		})
	}
}

func GRPCUnaryInterceptor(obs *Provider) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		// extract trace context from gRPC metadata
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			ctx = otel.GetTextMapPropagator().Extract(
				ctx,
				grpcMetadataCarrier{md: md},
			)
		}

		// start RPC span
		svcName, methodName := parseFullMethod(info.FullMethod)
		spanName := "gRPC " + info.FullMethod

		ctx, span := obs.Tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.service", svcName),
				attribute.String("rpc.method", methodName),
			),
		)
		defer span.End()

		// create request-scoped logger
		reqLogger := obs.Logger.With(
			slog.String("rpc.service", svcName),
			slog.String("rpc.method", methodName),
		)

		ctx = WithLogger(ctx, reqLogger)

		L(ctx).Info("rpc started")

		// execute handler
		start := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(start)

		// finalize telemetry
		grpcCode := grpccodes.OK

		if err != nil {
			if st, ok := status.FromError(err); ok {
				grpcCode = st.Code()
			} else {
				grpcCode = grpccodes.Internal
			}
		}

		codeStr := grpcCode.String()

		span.SetAttributes(
			attribute.String("rpc.grpc.status_code", codeStr),
		)

		if grpcCode != grpccodes.OK {
			span.SetStatus(otelcodes.Error, codeStr)
		} else {
			span.SetStatus(otelcodes.Ok, "")
		}

		obs.Metrics.GRPCRequestsTotal.Add(
			ctx,
			1,
			metric.WithAttributes(
				attribute.String("service", svcName),
				attribute.String("method", methodName),
				attribute.String("code", codeStr),
			),
		)

		obs.Metrics.GRPCRequestDuration.Record(
			ctx,
			duration.Seconds(),
			metric.WithAttributes(
				attribute.String("service", svcName),
				attribute.String("method", methodName),
			),
		)

		logFn := L(ctx).Info
		if grpcCode != grpccodes.OK {
			logFn = L(ctx).Warn
		}

		logFn("rpc completed",
			slog.String("code", codeStr),
			slog.Int64("duration_ms", duration.Milliseconds()),
		)

		return resp, err
	}
}

// grpcMetadataCarrier adapts gRPC metadata to TextMapCarrier
type grpcMetadataCarrier struct {
	md metadata.MD
}

func (c grpcMetadataCarrier) Get(key string) string {
	vals := c.md.Get(strings.ToLower(key))

	if len(vals) == 0 {
		return ""
	}

	return vals[0]
}

func (c grpcMetadataCarrier) Set(key, val string) {
	c.md.Set(strings.ToLower(key), val)
}

func (c grpcMetadataCarrier) Keys() []string {
	keys := make([]string, 0, len(c.md))

	for k := range c.md {
		keys = append(keys, k)
	}

	return keys
}

func parseFullMethod(fullMethod string) (service, method string) {
	if len(fullMethod) == 0 || fullMethod[0] != '/' {
		return "unknown", "unknown"
	}

	parts := strings.SplitN(fullMethod[1:], "/", 2)

	if len(parts) != 2 {
		return fullMethod, "unknown"
	}

	return parts[0], parts[1]
}

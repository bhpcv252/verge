package auth

import (
	"context"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/bhpcv252/verge/internal/observability"
)

const (
	grpcAuthHeader = "authorization"
)

func UnaryInterceptor(
	validator *Validator,
	obs *observability.Provider,
) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if validator == nil {
			return handler(ctx, req)
		}
		if err := validateMD(ctx, validator, obs, info.FullMethod); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func StreamInterceptor(
	validator *Validator,
	obs *observability.Provider,
) grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if validator == nil {
			return handler(srv, ss)
		}
		if err := validateMD(ss.Context(), validator, obs, info.FullMethod); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func validateMD(
	ctx context.Context,
	v *Validator,
	obs *observability.Provider,
	fullMethod string,
) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return recordAndReturn(ctx, obs, fullMethod)
	}

	values := md.Get(grpcAuthHeader)
	if len(values) == 0 {
		return recordAndReturn(ctx, obs, fullMethod)
	}

	// use the first value only
	key := extractBearer(values[0])
	if !v.Validate(key) {
		return recordAndReturn(ctx, obs, fullMethod)
	}

	return nil
}

func recordAndReturn(ctx context.Context, obs *observability.Provider, fullMethod string) error {
	obs.Logger.Warn(
		"auth: rejected gRPC call - missing or invalid API key",
		slog.String("full_method", fullMethod),
	)

	obs.Metrics.AuthFailuresTotal.Add(ctx, 1,
		metric.WithAttributes(attribute.String("transport", "grpc")),
	)

	return unauthenticated()
}

func unauthenticated() error {
	return status.Error(
		codes.Unauthenticated,
		"a valid API key is required; set metadata: authorization = Bearer <key>",
	)
}

func ExtractBearerFromMD(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(values[0], "Bearer "))
}

package auth

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	grpcAuthHeader = "authorization"
)

func UnaryInterceptor(validator *Validator) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if validator == nil {
			return handler(ctx, req)
		}
		if err := validateMD(ctx, validator); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func StreamInterceptor(validator *Validator) grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		_ *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if validator == nil {
			return handler(srv, ss)
		}
		if err := validateMD(ss.Context(), validator); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func validateMD(ctx context.Context, v *Validator) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return unauthenticated()
	}

	values := md.Get(grpcAuthHeader)
	if len(values) == 0 {
		return unauthenticated()
	}

	// Use the first value only
	key := extractBearer(values[0])
	if !v.Validate(key) {
		return unauthenticated()
	}

	return nil
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

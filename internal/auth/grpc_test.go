package auth

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func noopHandler(ctx context.Context, req any) (any, error) {
	return "ok", nil
}

func unaryInfo() *grpc.UnaryServerInfo {
	return &grpc.UnaryServerInfo{FullMethod: "/verge.v1.RepoService/GetRepo"}
}

func ctxWithMD(key, value string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(key, value))
}

func TestUnaryInterceptor_Disabled(t *testing.T) {
	// nil validator
	interceptor := UnaryInterceptor(nil)

	for _, md := range []metadata.MD{
		nil,
		metadata.Pairs("authorization", ""),
		metadata.Pairs("authorization", "Bearer wrong"),
	} {
		ctx := context.Background()
		if md != nil {
			ctx = metadata.NewIncomingContext(ctx, md)
		}
		_, err := interceptor(ctx, nil, unaryInfo(), noopHandler)
		if err != nil {
			t.Errorf("nil validator: expected no error, got %v", err)
		}
	}
}

func TestUnaryInterceptor_Enabled(t *testing.T) {
	v, err := NewValidator([]string{"key-good"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	interceptor := UnaryInterceptor(v)

	cases := []struct {
		name     string
		ctx      context.Context
		wantCode codes.Code
	}{
		{
			name:     "valid key",
			ctx:      ctxWithMD("authorization", "Bearer key-good"),
			wantCode: codes.OK,
		},
		{
			name:     "wrong key",
			ctx:      ctxWithMD("authorization", "Bearer key-bad"),
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "no metadata",
			ctx:      context.Background(),
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "empty authorization value",
			ctx:      ctxWithMD("authorization", ""),
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "missing Bearer prefix",
			ctx:      ctxWithMD("authorization", "key-good"),
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "Basic scheme rejected",
			ctx:      ctxWithMD("authorization", "Basic key-good"),
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "Bearer with spaces trimmed",
			ctx:      ctxWithMD("authorization", "Bearer   key-good  "),
			wantCode: codes.OK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := interceptor(tc.ctx, nil, unaryInfo(), noopHandler)

			if tc.wantCode == codes.OK {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected gRPC status error, got %T: %v", err, err)
			}
			if st.Code() != tc.wantCode {
				t.Errorf("code: want %v, got %v", tc.wantCode, st.Code())
			}
		})
	}
}

func TestUnaryInterceptor_HandlerNotCalledOnFailure(t *testing.T) {
	v, err := NewValidator([]string{"key-good"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	interceptor := UnaryInterceptor(v)

	called := false
	tracingHandler := func(ctx context.Context, req any) (any, error) {
		called = true
		return "ok", nil
	}

	ctx := ctxWithMD("authorization", "Bearer bad-key")
	_, err = interceptor(ctx, nil, unaryInfo(), tracingHandler)
	if called {
		t.Error("handler must not be called when auth fails")
	}
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("want Unauthenticated, got %v", st.Code())
	}
}

func TestStreamInterceptor_Disabled(t *testing.T) {
	interceptor := StreamInterceptor(nil)
	ss := &fakeStream{ctx: context.Background()}
	err := interceptor(nil, ss, nil, func(srv any, stream grpc.ServerStream) error {
		return nil
	})
	if err != nil {
		t.Errorf("nil validator: expected no error, got %v", err)
	}
}

func TestStreamInterceptor_Enabled(t *testing.T) {
	v, err := NewValidator([]string{"key-good"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	interceptor := StreamInterceptor(v)

	t.Run("valid key", func(t *testing.T) {
		ss := &fakeStream{ctx: ctxWithMD("authorization", "Bearer key-good")}
		err := interceptor(nil, ss, nil, func(_ any, _ grpc.ServerStream) error { return nil })
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("wrong key", func(t *testing.T) {
		ss := &fakeStream{ctx: ctxWithMD("authorization", "Bearer key-bad")}
		err := interceptor(nil, ss, nil, func(_ any, _ grpc.ServerStream) error { return nil })
		if err == nil {
			t.Fatal("expected error")
		}
		st, _ := status.FromError(err)
		if st.Code() != codes.Unauthenticated {
			t.Errorf("want Unauthenticated, got %v", st.Code())
		}
	})
}

type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeStream) Context() context.Context { return f.ctx }

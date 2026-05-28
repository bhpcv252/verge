package observability

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// helpers

func buildRouter(obs *Provider, pattern string, fn http.HandlerFunc) http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(HTTPMiddleware(obs))
	r.Get(pattern, fn)
	return r
}

func noopUnaryInfo(fullMethod string) *grpc.UnaryServerInfo {
	return &grpc.UnaryServerInfo{FullMethod: fullMethod}
}

func okHandler(_ context.Context, _ any) (any, error) { return "ok", nil }

func grpcErrHandler(code codes.Code) grpc.UnaryHandler {
	return func(_ context.Context, _ any) (any, error) {
		return nil, status.Error(code, code.String())
	}
}

// HTTPMiddleware

func TestHTTPMiddleware_CallsNextHandler(t *testing.T) {
	called := false
	router := buildRouter(Noop(), "/ping", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	router.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/ping", nil),
	)

	assert.True(t, called)
}

func TestHTTPMiddleware_PreservesResponseBody(t *testing.T) {
	router := buildRouter(Noop(), "/data", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello")
	})

	w := httptest.NewRecorder()

	router.ServeHTTP(
		w,
		httptest.NewRequest(http.MethodGet, "/data", nil),
	)

	assert.Equal(t, "hello", w.Body.String())
}

func TestHTTPMiddleware_VariousStatusCodes(t *testing.T) {
	codesToTest := []int{
		http.StatusOK,
		http.StatusCreated,
		http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusConflict,
		http.StatusInternalServerError,
	}

	for _, code := range codesToTest {
		t.Run(http.StatusText(code), func(t *testing.T) {
			router := buildRouter(Noop(), "/probe", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			})

			w := httptest.NewRecorder()

			router.ServeHTTP(
				w,
				httptest.NewRequest(http.MethodGet, "/probe", nil),
			)

			assert.Equal(t, code, w.Code)
		})
	}
}

func TestHTTPMiddleware_UnmatchedRoute_Returns404(t *testing.T) {
	router := buildRouter(Noop(), "/repos", func(w http.ResponseWriter, r *http.Request) {})

	w := httptest.NewRecorder()

	router.ServeHTTP(
		w,
		httptest.NewRequest(http.MethodGet, "/does-not-exist", nil),
	)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHTTPMiddleware_InjectsScopedLoggerIntoContext(t *testing.T) {
	var loggerPresent bool

	router := buildRouter(Noop(), "/ctx", func(w http.ResponseWriter, r *http.Request) {
		loggerPresent = L(r.Context()) != nil
	})

	router.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/ctx", nil),
	)

	assert.True(t, loggerPresent)
}

func TestHTTPMiddleware_DoesNotPanicOnInFlightCounter(t *testing.T) {
	router := buildRouter(Noop(), "/ok", func(w http.ResponseWriter, r *http.Request) {})

	assert.NotPanics(t, func() {
		router.ServeHTTP(
			httptest.NewRecorder(),
			httptest.NewRequest(http.MethodGet, "/ok", nil),
		)
	})
}

// parseFullMethod

func TestParseFullMethod_ValidInput(t *testing.T) {
	cases := []struct {
		input   string
		wantSvc string
		wantMth string
	}{
		{
			"/verge.v1.CommitService/CreateCommit",
			"verge.v1.CommitService",
			"CreateCommit",
		},
		{
			"/verge.v1.BranchService/GetBranch",
			"verge.v1.BranchService",
			"GetBranch",
		},
		{
			"/verge.v1.MergeService/CreateMerge",
			"verge.v1.MergeService",
			"CreateMerge",
		},
	}

	for _, tc := range cases {
		svc, mth := parseFullMethod(tc.input)

		assert.Equal(t, tc.wantSvc, svc, "service for %q", tc.input)
		assert.Equal(t, tc.wantMth, mth, "method for %q", tc.input)
	}
}

func TestParseFullMethod_InvalidInput(t *testing.T) {
	cases := []struct {
		input   string
		wantSvc string
		wantMth string
	}{
		{"", "unknown", "unknown"},
		{"no-leading-slash", "unknown", "unknown"},
		{"/onlyone", "/onlyone", "unknown"},
	}

	for _, tc := range cases {
		svc, mth := parseFullMethod(tc.input)

		assert.Equal(t, tc.wantSvc, svc, "service for %q", tc.input)
		assert.Equal(t, tc.wantMth, mth, "method for %q", tc.input)
	}
}

// grpcMetadataCarrier

func TestGRPCMetadataCarrier_Get(t *testing.T) {
	md := metadata.New(map[string]string{
		"x-foo": "bar",
	})

	c := grpcMetadataCarrier{md: md}

	assert.Equal(t, "bar", c.Get("x-foo"))
	assert.Equal(t, "bar", c.Get("X-Foo"), "Get must be case-insensitive")
	assert.Equal(t, "", c.Get("x-missing"))
}

func TestGRPCMetadataCarrier_Set(t *testing.T) {
	md := metadata.MD{}

	c := grpcMetadataCarrier{md: md}

	c.Set("x-new", "value")

	assert.Equal(t, "value", c.Get("x-new"))
}

func TestGRPCMetadataCarrier_Keys(t *testing.T) {
	md := metadata.New(map[string]string{
		"key-a": "1",
		"key-b": "2",
	})

	c := grpcMetadataCarrier{md: md}

	assert.ElementsMatch(t, []string{"key-a", "key-b"}, c.Keys())
}

// GRPCUnaryInterceptor

func TestGRPCUnaryInterceptor_Success_ReturnsResponse(t *testing.T) {
	interceptor := GRPCUnaryInterceptor(Noop())

	resp, err := interceptor(
		context.Background(),
		nil,
		noopUnaryInfo("/verge.v1.CommitService/CreateCommit"),
		okHandler,
	)

	assert.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestGRPCUnaryInterceptor_PropagatesGRPCStatusError(t *testing.T) {
	interceptor := GRPCUnaryInterceptor(Noop())

	_, err := interceptor(
		context.Background(),
		nil,
		noopUnaryInfo("/verge.v1.BranchService/AdvanceBranch"),
		grpcErrHandler(codes.Aborted),
	)

	require.Error(t, err)

	st, ok := status.FromError(err)

	require.True(t, ok)
	assert.Equal(t, codes.Aborted, st.Code())
}

func TestGRPCUnaryInterceptor_PropagatesRawError(t *testing.T) {
	interceptor := GRPCUnaryInterceptor(Noop())

	_, err := interceptor(
		context.Background(),
		nil,
		noopUnaryInfo("/verge.v1.RepoService/CreateRepo"),
		func(_ context.Context, _ any) (any, error) {
			return nil, errors.New("raw error")
		},
	)

	require.Error(t, err)
}

func TestGRPCUnaryInterceptor_NoMetadata_DoesNotPanic(t *testing.T) {
	interceptor := GRPCUnaryInterceptor(Noop())

	assert.NotPanics(t, func() {
		_, _ = interceptor(
			context.Background(),
			nil,
			noopUnaryInfo("/verge.v1.MergeService/CreateMerge"),
			okHandler,
		)
	})
}

func TestGRPCUnaryInterceptor_InjectsScopedLoggerIntoHandlerContext(t *testing.T) {
	interceptor := GRPCUnaryInterceptor(Noop())

	var loggerPresent bool

	_, _ = interceptor(
		context.Background(),
		nil,
		noopUnaryInfo("/verge.v1.CommitService/ListCommits"),
		func(ctx context.Context, _ any) (any, error) {
			loggerPresent = L(ctx) != nil
			return nil, nil
		},
	)

	assert.True(t, loggerPresent)
}

package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestHTTPMiddleware_Disabled(t *testing.T) {
	// nil validator
	mw := HTTPMiddleware(nil)(okHandler)

	for _, authHeader := range []string{"", "Bearer wrong", "Bearer anything"} {
		t.Run("header="+authHeader, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/repos", nil)
			if authHeader != "" {
				req.Header.Set("Authorization", authHeader)
			}
			rec := httptest.NewRecorder()
			mw.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("want 200, got %d", rec.Code)
			}
		})
	}
}

func TestHTTPMiddleware_Enabled(t *testing.T) {
	v, err := NewValidator([]string{"key-good"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	mw := HTTPMiddleware(v)(okHandler)

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{
			name:       "valid key",
			authHeader: "Bearer key-good",
			wantStatus: http.StatusOK,
		},
		{
			name:       "wrong key",
			authHeader: "Bearer key-bad",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing header",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "no Bearer prefix",
			authHeader: "key-good",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Basic scheme rejected",
			authHeader: "Basic key-good",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Bearer with extra spaces trimmed",
			authHeader: "Bearer   key-good  ",
			wantStatus: http.StatusOK,
		},
		{
			name:       "empty Bearer value",
			authHeader: "Bearer ",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/repos", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()
			mw.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status: want %d, got %d", tc.wantStatus, rec.Code)
			}

			if tc.wantStatus == http.StatusUnauthorized {
				if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
					t.Errorf("Content-Type: want application/json, got %q", ct)
				}
				var body unauthorizedBody
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if body.Error != "unauthorized" {
					t.Errorf("body.error: want %q, got %q", "unauthorized", body.Error)
				}
				if body.Message == "" {
					t.Error("body.message must not be empty")
				}
			}
		})
	}
}

func TestHTTPMiddleware_HandlerNotCalledOnFailure(t *testing.T) {
	v, err := NewValidator([]string{"key-good"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw := HTTPMiddleware(v)(inner)
	req := httptest.NewRequest(http.MethodGet, "/v1/repos", nil)
	req.Header.Set("Authorization", "Bearer bad-key")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if called {
		t.Error("inner handler must not be called when auth fails")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestExtractBearer(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Bearer mykey", "mykey"},
		{"Bearer   mykey  ", "mykey"},
		{"Bearer ", ""},
		{"bearer mykey", ""}, // case-sensitive
		{"Basic mykey", ""},
		{"mykey", ""},
		{"", ""},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := extractBearer(tc.input)
			if got != tc.want {
				t.Errorf("extractBearer(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

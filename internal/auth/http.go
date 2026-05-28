package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/bhpcv252/verge/internal/observability"
)

type unauthorizedBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

var errBody = unauthorizedBody{
	Error:   "unauthorized",
	Message: "A valid API key is required. Set the Authorization header to: Bearer <key>",
}

func HTTPMiddleware(
	validator *Validator,
	obs *observability.Provider,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if validator == nil {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := extractBearer(r.Header.Get("Authorization"))
			if !validator.Validate(key) {
				observability.L(r.Context()).Warn(
					"auth: rejected request - missing or invalid API key",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
				)

				obs.Metrics.AuthFailuresTotal.Add(r.Context(), 1,
					metric.WithAttributes(attribute.String("transport", "http")),
				)

				writeUnauthorized(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractBearer(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(errBody)
}

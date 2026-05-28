package auth

import (
	"encoding/json"
	"net/http"
	"strings"
)

type unauthorizedBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

var errBody = unauthorizedBody{
	Error:   "unauthorized",
	Message: "A valid API key is required. Set the Authorization header to: Bearer <key>",
}

func HTTPMiddleware(validator *Validator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if validator == nil {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := extractBearer(r.Header.Get("Authorization"))
			if !validator.Validate(key) {
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

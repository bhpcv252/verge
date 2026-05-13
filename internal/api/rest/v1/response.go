package v1

import (
	"encoding/json"
	"net/http"
)

// serialises v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// structured error body returned on every non-2xx response.
type errResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errResponse{Error: code, Message: message})
}

// standard error helpers.
func badRequest(w http.ResponseWriter, message string) {
	writeError(w, http.StatusBadRequest, "invalid_request", message)
}

func notFound(w http.ResponseWriter, code, message string) {
	writeError(w, http.StatusNotFound, code, message)
}

// func conflict(w http.ResponseWriter, code, message string) {
// 	writeError(w, http.StatusConflict, code, message)
// }

func internalError(w http.ResponseWriter) {
	writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
}

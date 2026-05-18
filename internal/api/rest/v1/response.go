package v1

import (
	"encoding/json"
	"net/http"

	"github.com/bhpcv252/verge/internal/api/core"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errResponse struct {
	Error       string  `json:"error"`
	Message     string  `json:"message"`
	CurrentHead *string `json:"current_head,omitempty"`
}

// codeToHTTPStatus is the single mapping
// from API error codes to HTTP status codes
var codeToHTTPStatus = map[string]int{
	"invalid_request":              http.StatusBadRequest,
	"repo_not_found":               http.StatusNotFound,
	"branch_not_found":             http.StatusNotFound,
	"commit_not_found":             http.StatusNotFound,
	"branch_already_exists":        http.StatusConflict,
	"branch_conflict":              http.StatusConflict,
	"stale_merge_target":           http.StatusConflict,
	"cannot_delete_default_branch": http.StatusConflict,
	"invalid_parent":               http.StatusUnprocessableEntity,
	"internal_error":               http.StatusInternalServerError,
}

func writeAppError(w http.ResponseWriter, e *core.AppError) {
	httpStatus, ok := codeToHTTPStatus[e.Code]
	if !ok {
		httpStatus = http.StatusInternalServerError
	}
	writeJSON(w, httpStatus, errResponse{
		Error:       e.Code,
		Message:     e.Message,
		CurrentHead: e.CurrentHead,
	})
}

func badRequest(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusBadRequest, errResponse{
		Error:   "invalid_request",
		Message: message,
	})
}

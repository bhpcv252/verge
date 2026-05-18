package v1

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/bhpcv252/verge/internal/api/core"
)

// codeToGRPCCode maps API error codes to gRPC status codes
var codeToGRPCCode = map[string]codes.Code{
	"invalid_request":              codes.InvalidArgument,
	"repo_not_found":               codes.NotFound,
	"branch_not_found":             codes.NotFound,
	"commit_not_found":             codes.NotFound,
	"branch_already_exists":        codes.AlreadyExists,
	"branch_conflict":              codes.Aborted,
	"stale_merge_target":           codes.Aborted,
	"cannot_delete_default_branch": codes.FailedPrecondition,
	"invalid_parent":               codes.FailedPrecondition,
	"internal_error":               codes.Internal,
}

// toGRPCError converts a core.AppError from
// core.MapDomainError into a gRPC status error
func toGRPCError(e *core.AppError) error {
	code, ok := codeToGRPCCode[e.Code]
	if !ok {
		code = codes.Internal
	}
	return status.Error(code, e.Message)
}

func invalidArg(msg string) error {
	return status.Error(codes.InvalidArgument, msg)
}

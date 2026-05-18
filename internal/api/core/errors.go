package core

import (
	"errors"
	"fmt"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/service"
)

type AppError struct {
	Code        string
	Message     string
	CurrentHead *string // non-nil only for branch_conflict and stale_merge_target
}

func MapDomainError(err error) *AppError {
	if errors.Is(err, domain.ErrRepoNotFound) {
		return &AppError{
			Code:    "repo_not_found",
			Message: "The requested repository does not exist.",
		}
	}
	if errors.Is(err, domain.ErrBranchNotFound) {
		return &AppError{
			Code:    "branch_not_found",
			Message: "The requested branch does not exist in this repository.",
		}
	}
	if errors.Is(err, domain.ErrBranchAlreadyExists) {
		return &AppError{
			Code:    "branch_already_exists",
			Message: "A branch with this name already exists in this repository.",
		}
	}
	if errors.Is(err, domain.ErrCommitNotFound) {
		return &AppError{
			Code:    "commit_not_found",
			Message: "The requested commit does not exist in this repository.",
		}
	}
	if errors.Is(err, domain.ErrInvalidParent) {
		return &AppError{
			Code:    "invalid_parent",
			Message: "One or more parent_ids do not exist in this repository.",
		}
	}
	if errors.Is(err, domain.ErrCannotDeleteDefaultBranch) {
		return &AppError{
			Code:    "cannot_delete_default_branch",
			Message: "Cannot delete the default branch. Set a different default branch first.",
		}
	}

	var branchConflict *service.BranchConflictError
	if errors.As(err, &branchConflict) {
		msg := fmt.Sprintf(
			"Branch %q has advanced. Current head is %q but expected %q. "+
				"Fetch the latest head and retry.",
			branchConflict.BranchName, branchConflict.CurrentHead, branchConflict.ExpectedHead,
		)
		return &AppError{
			Code:        "branch_conflict",
			Message:     msg,
			CurrentHead: &branchConflict.CurrentHead,
		}
	}

	var mergeConflict *service.MergeBranchConflictError
	if errors.As(err, &mergeConflict) {
		msg := fmt.Sprintf(
			"Branch %q has moved during the merge. Current head is %q but expected %q. "+
				"Fetch the latest heads and retry.",
			mergeConflict.BranchName, mergeConflict.CurrentHead, mergeConflict.ExpectedHead,
		)
		return &AppError{
			Code:        "stale_merge_target",
			Message:     msg,
			CurrentHead: &mergeConflict.CurrentHead,
		}
	}

	var valErr *service.ValidationError
	if errors.As(err, &valErr) {
		return &AppError{Code: "invalid_request", Message: valErr.Msg}
	}

	return &AppError{Code: "internal_error", Message: "An unexpected error occurred."}
}

package service

import (
	"fmt"

	"github.com/bhpcv252/verge/internal/domain"
)

type ValidationError struct {
	Msg string
}

func (e *ValidationError) Error() string { return e.Msg }

type BranchConflictError struct {
	BranchName   string
	CurrentHead  string
	ExpectedHead string
}

func (e *BranchConflictError) Error() string {
	return fmt.Sprintf(
		"branch conflict: %q is at %q but expected %q",
		e.BranchName, e.CurrentHead, e.ExpectedHead,
	)
}

type MergeBranchConflictError struct {
	BranchName   string
	CurrentHead  string
	ExpectedHead string
}

func (e *MergeBranchConflictError) Error() string {
	return fmt.Sprintf(
		"merge branch conflict: %q is at %q but expected %q",
		e.BranchName, e.CurrentHead, e.ExpectedHead,
	)
}

func (e *MergeBranchConflictError) Is(target error) bool {
	return target == domain.ErrStaleMergeTarget
}

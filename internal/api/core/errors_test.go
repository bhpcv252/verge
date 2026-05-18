package core_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/api/core"
	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/service"
)

// domain errors

func TestMapDomainError_RepoNotFound(t *testing.T) {
	app := core.MapDomainError(domain.ErrRepoNotFound)

	require.NotNil(t, app)
	assert.Equal(t, "repo_not_found", app.Code)
	assert.Equal(t, "The requested repository does not exist.", app.Message)
	assert.Nil(t, app.CurrentHead)
}

func TestMapDomainError_RepoNotFound_Wrapped(t *testing.T) {
	wrapped := fmt.Errorf("storage: %w", domain.ErrRepoNotFound)
	app := core.MapDomainError(wrapped)

	require.NotNil(t, app)
	assert.Equal(t, "repo_not_found", app.Code)
	assert.Nil(t, app.CurrentHead)
}

func TestMapDomainError_BranchNotFound(t *testing.T) {
	app := core.MapDomainError(domain.ErrBranchNotFound)

	require.NotNil(t, app)
	assert.Equal(t, "branch_not_found", app.Code)
	assert.Equal(t, "The requested branch does not exist in this repository.", app.Message)
	assert.Nil(t, app.CurrentHead)
}

func TestMapDomainError_BranchNotFound_Wrapped(t *testing.T) {
	wrapped := fmt.Errorf("store: %w", domain.ErrBranchNotFound)
	app := core.MapDomainError(wrapped)

	require.NotNil(t, app)
	assert.Equal(t, "branch_not_found", app.Code)
}

func TestMapDomainError_BranchAlreadyExists(t *testing.T) {
	app := core.MapDomainError(domain.ErrBranchAlreadyExists)

	require.NotNil(t, app)
	assert.Equal(t, "branch_already_exists", app.Code)
	assert.Equal(t, "A branch with this name already exists in this repository.", app.Message)
	assert.Nil(t, app.CurrentHead)
}

func TestMapDomainError_CommitNotFound(t *testing.T) {
	app := core.MapDomainError(domain.ErrCommitNotFound)

	require.NotNil(t, app)
	assert.Equal(t, "commit_not_found", app.Code)
	assert.Equal(t, "The requested commit does not exist in this repository.", app.Message)
	assert.Nil(t, app.CurrentHead)
}

func TestMapDomainError_InvalidParent(t *testing.T) {
	app := core.MapDomainError(domain.ErrInvalidParent)

	require.NotNil(t, app)
	assert.Equal(t, "invalid_parent", app.Code)
	assert.Equal(t, "One or more parent_ids do not exist in this repository.", app.Message)
	assert.Nil(t, app.CurrentHead)
}

func TestMapDomainError_CannotDeleteDefaultBranch(t *testing.T) {
	app := core.MapDomainError(domain.ErrCannotDeleteDefaultBranch)

	require.NotNil(t, app)
	assert.Equal(t, "cannot_delete_default_branch", app.Code)
	assert.Equal(
		t,
		"Cannot delete the default branch. Set a different default branch first.",
		app.Message,
	)
	assert.Nil(t, app.CurrentHead)
}

// conflict error

func TestMapDomainError_BranchConflict(t *testing.T) {
	bce := &service.BranchConflictError{
		BranchName:   "main",
		CurrentHead:  "abc123",
		ExpectedHead: "def456",
	}

	app := core.MapDomainError(bce)

	require.NotNil(t, app)
	assert.Equal(t, "branch_conflict", app.Code)
	require.NotNil(t, app.CurrentHead)
	assert.Equal(t, "abc123", *app.CurrentHead)

	// message must mention all three relevant values
	assert.Contains(t, app.Message, "main")
	assert.Contains(t, app.Message, "abc123")
	assert.Contains(t, app.Message, "def456")
}

func TestMapDomainError_BranchConflict_Wrapped(t *testing.T) {
	bce := &service.BranchConflictError{
		BranchName:   "feature",
		CurrentHead:  "111",
		ExpectedHead: "222",
	}
	wrapped := fmt.Errorf("service: %w", bce)

	app := core.MapDomainError(wrapped)

	require.NotNil(t, app)
	assert.Equal(t, "branch_conflict", app.Code)
	require.NotNil(t, app.CurrentHead)
	assert.Equal(t, "111", *app.CurrentHead)
}

func TestMapDomainError_BranchConflict_CurrentHeadIsIndependent(t *testing.T) {
	bce := &service.BranchConflictError{
		BranchName:   "main",
		CurrentHead:  "abc123",
		ExpectedHead: "def456",
	}

	app := core.MapDomainError(bce)

	require.NotNil(t, app.CurrentHead)
	assert.Equal(t, bce.CurrentHead, *app.CurrentHead)
}

func TestMapDomainError_StaleMergeTarget(t *testing.T) {
	mce := &service.MergeBranchConflictError{
		BranchName:   "release",
		CurrentHead:  "aaabbb",
		ExpectedHead: "cccddd",
	}

	app := core.MapDomainError(mce)

	require.NotNil(t, app)
	assert.Equal(t, "stale_merge_target", app.Code)
	require.NotNil(t, app.CurrentHead)
	assert.Equal(t, "aaabbb", *app.CurrentHead)

	assert.Contains(t, app.Message, "release")
	assert.Contains(t, app.Message, "aaabbb")
	assert.Contains(t, app.Message, "cccddd")
}

func TestMapDomainError_StaleMergeTarget_Wrapped(t *testing.T) {
	mce := &service.MergeBranchConflictError{
		BranchName:   "hotfix",
		CurrentHead:  "x1",
		ExpectedHead: "y2",
	}
	wrapped := fmt.Errorf("merge svc: %w", mce)

	app := core.MapDomainError(wrapped)

	require.NotNil(t, app)
	assert.Equal(t, "stale_merge_target", app.Code)
	require.NotNil(t, app.CurrentHead)
	assert.Equal(t, "x1", *app.CurrentHead)
}

// validation error

func TestMapDomainError_ValidationError(t *testing.T) {
	valErr := &service.ValidationError{Msg: "name must not be empty"}

	app := core.MapDomainError(valErr)

	require.NotNil(t, app)
	assert.Equal(t, "invalid_request", app.Code)
	assert.Equal(t, "name must not be empty", app.Message)
	assert.Nil(t, app.CurrentHead)
}

func TestMapDomainError_ValidationError_Wrapped(t *testing.T) {
	valErr := &service.ValidationError{Msg: "invalid uuid"}
	wrapped := fmt.Errorf("handler: %w", valErr)

	app := core.MapDomainError(wrapped)

	require.NotNil(t, app)
	assert.Equal(t, "invalid_request", app.Code)
	assert.Equal(t, "invalid uuid", app.Message)
}

// unknown / internal errors

func TestMapDomainError_UnknownError(t *testing.T) {
	app := core.MapDomainError(errors.New("some random db error"))

	require.NotNil(t, app)
	assert.Equal(t, "internal_error", app.Code)
	assert.Equal(t, "An unexpected error occurred.", app.Message)
	assert.Nil(t, app.CurrentHead)
}

func TestMapDomainError_NilIsInternalError(t *testing.T) {
	app := core.MapDomainError(nil)

	require.NotNil(t, app)
	assert.Equal(t, "internal_error", app.Code)
}

func TestMapDomainError_SentinelBeatsUnknown(t *testing.T) {
	// multi-level wrap: the sentinel is still reachable via errors.Is
	deep := fmt.Errorf(
		"layer3: %w",
		fmt.Errorf("layer2: %w", fmt.Errorf("layer1: %w", domain.ErrCommitNotFound)),
	)

	app := core.MapDomainError(deep)

	require.NotNil(t, app)
	assert.Equal(t, "commit_not_found", app.Code)
}

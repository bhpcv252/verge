package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBranch_Fields(t *testing.T) {
	now := time.Now().UTC()
	b := Branch{
		Name:      "feature-x",
		RepoID:    "repo_abc",
		CommitID:  "commit_xyz",
		CreatedAt: now,
	}

	assert.Equal(t, "feature-x", b.Name)
	assert.Equal(t, "repo_abc", b.RepoID)
	assert.Equal(t, "commit_xyz", b.CommitID)
	assert.Equal(t, now, b.CreatedAt)
}

func TestBranchValidate_EmptyName_Rejected(t *testing.T) {
	b := Branch{
		Name:      "",
		RepoID:    "repo_abc",
		CommitID:  "commit_xyz",
		CreatedAt: time.Now().UTC(),
	}

	err := b.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch name is required")
}

func TestBranchValidate_WhitespaceOnlyName_Rejected(t *testing.T) {
	b := Branch{
		Name:      "   ",
		RepoID:    "repo_abc",
		CommitID:  "commit_xyz",
		CreatedAt: time.Now().UTC(),
	}

	err := b.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch name is required")
}

func TestBranchValidate_NonEmptyName_Accepted(t *testing.T) {
	b := Branch{
		Name:      "feature-x",
		RepoID:    "repo_abc",
		CommitID:  "commit_xyz",
		CreatedAt: time.Now().UTC(),
	}

	err := b.Validate()
	assert.NoError(t, err, "branch with non-empty name should be valid")
}

func TestErrBranchNotFound_IsNotNil(t *testing.T) {
	assert.NotNil(t, ErrBranchNotFound)
}

func TestErrBranchNotFound_HasMessage(t *testing.T) {
	assert.NotEmpty(t, ErrBranchNotFound.Error())
}

func TestErrBranchAlreadyExists_IsNotNil(t *testing.T) {
	assert.NotNil(t, ErrBranchAlreadyExists)
}

func TestErrBranchAlreadyExists_HasMessage(t *testing.T) {
	assert.NotEmpty(t, ErrBranchAlreadyExists.Error())
}

func TestErrBranchConflict_IsNotNil(t *testing.T) {
	assert.NotNil(t, ErrBranchConflict)
}

func TestErrBranchConflict_HasMessage(t *testing.T) {
	assert.NotEmpty(t, ErrBranchConflict.Error())
}

func TestErrCannotDeleteDefaultBranch_IsNotNil(t *testing.T) {
	assert.NotNil(t, ErrCannotDeleteDefaultBranch)
}

func TestErrCannotDeleteDefaultBranch_HasMessage(t *testing.T) {
	assert.NotEmpty(t, ErrCannotDeleteDefaultBranch.Error())
}

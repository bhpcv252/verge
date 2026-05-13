package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepo_Fields(t *testing.T) {
	now := time.Now().UTC()
	r := Repo{
		ID:            "repo_abc",
		Name:          "my-doc",
		DefaultBranch: "main",
		CreatedAt:     now,
	}

	assert.Equal(t, "repo_abc", r.ID)
	assert.Equal(t, "my-doc", r.Name)
	assert.Equal(t, "main", r.DefaultBranch)
	assert.Equal(t, now, r.CreatedAt)
}

func TestRepoValidate_EmptyName_Rejected(t *testing.T) {
	r := Repo{
		ID:            "repo_abc",
		Name:          "",
		DefaultBranch: "main",
		CreatedAt:     time.Now().UTC(),
	}

	err := r.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repo name is required")
}

func TestRepoValidate_WhitespaceOnlyName_Rejected(t *testing.T) {
	r := Repo{
		ID:            "repo_abc",
		Name:          "   ",
		DefaultBranch: "main",
		CreatedAt:     time.Now().UTC(),
	}

	err := r.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repo name is required")
}

func TestRepoValidate_EmptyDefaultBranch_Rejected(t *testing.T) {
	r := Repo{
		ID:            "repo_abc",
		Name:          "my-doc",
		DefaultBranch: "",
		CreatedAt:     time.Now().UTC(),
	}

	err := r.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default_branch is required")
}

func TestRepoValidate_WhitespaceOnlyDefaultBranch_Rejected(t *testing.T) {
	r := Repo{
		ID:            "repo_abc",
		Name:          "my-doc",
		DefaultBranch: "   ",
		CreatedAt:     time.Now().UTC(),
	}

	err := r.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default_branch is required")
}

func TestRepoValidate_ValidRepo_Accepted(t *testing.T) {
	r := Repo{
		ID:            "repo_abc",
		Name:          "my-doc",
		DefaultBranch: "main",
		CreatedAt:     time.Now().UTC(),
	}

	err := r.Validate()
	assert.NoError(t, err, "repo with valid name and default_branch should be valid")
}

func TestErrRepoNotFound_IsNotNil(t *testing.T) {
	assert.NotNil(t, ErrRepoNotFound)
}

func TestErrRepoNotFound_HasMessage(t *testing.T) {
	assert.NotEmpty(t, ErrRepoNotFound.Error())
}

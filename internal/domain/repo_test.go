package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func TestErrRepoNotFound_IsNotNil(t *testing.T) {
	assert.NotNil(t, ErrRepoNotFound)
}

func TestErrRepoNotFound_HasMessage(t *testing.T) {
	assert.NotEmpty(t, ErrRepoNotFound.Error())
}

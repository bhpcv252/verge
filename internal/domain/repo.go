package domain

import (
	"errors"
	"time"
)

// Repo (Repository) is the top-level container.
// One product resource maps to one repository.
type Repo struct {
	ID            string
	Name          string
	DefaultBranch string
	CreatedAt     time.Time
}

var ErrRepoNotFound = errors.New("repo_not_found")

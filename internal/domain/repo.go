package domain

import (
	"errors"
	"fmt"
	"strings"
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

func (r *Repo) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("repo name is required and must not be empty")
	}
	if strings.TrimSpace(r.DefaultBranch) == "" {
		return fmt.Errorf("repo default_branch is required and must not be empty")
	}
	return nil
}

var ErrRepoNotFound = errors.New("repo_not_found")

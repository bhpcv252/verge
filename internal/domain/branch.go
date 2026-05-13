package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Branch is a mutable named pointer to a commit.
type Branch struct {
	Name      string
	RepoID    string
	CommitID  string
	CreatedAt time.Time
}

func (b *Branch) Validate() error {
	if strings.TrimSpace(b.Name) == "" {
		return fmt.Errorf("branch name is required and must not be empty")
	}
	return nil
}

var (
	ErrBranchNotFound            = errors.New("branch_not_found")
	ErrBranchAlreadyExists       = errors.New("branch_already_exists")
	ErrBranchConflict            = errors.New("branch_conflict")
	ErrCannotDeleteDefaultBranch = errors.New("cannot_delete_default_branch")
)

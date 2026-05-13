package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Commit struct {
	ID             string
	RepoID         string
	ParentIDs      []string // zero = root, one = regular, two = merge
	DataPointer    DataPointer
	Message        string
	Author         string
	Timestamp      time.Time
	IdempotencyKey string // optional, for client-side deduplication
}

// DataPointer is an embedded value type that references the product's actual data
type DataPointer struct {
	Type     string          `json:"type"`               // "s3" | "url" | "db" | "custom"
	Location string          `json:"location"`           // required
	Hash     string          `json:"hash,omitempty"`     // optional, "sha256:..."
	Metadata json.RawMessage `json:"metadata,omitempty"` // optional
}

func (dp *DataPointer) Validate() error {
	validTypes := map[string]bool{
		"s3":     true,
		"url":    true,
		"db":     true,
		"custom": true,
	}
	if !validTypes[dp.Type] {
		return fmt.Errorf("data_pointer.type must be one of: s3, url, db, custom")
	}

	if strings.TrimSpace(dp.Location) == "" {
		return fmt.Errorf("data_pointer.location is required and must not be empty")
	}

	if dp.Hash != "" {
		if !strings.HasPrefix(dp.Hash, "sha256:") {
			return fmt.Errorf("data_pointer.hash must start with 'sha256:' if provided")
		}
		// must have content after "sha256:"
		if len(dp.Hash) <= 7 { // "sha256:" is 7 characters
			return fmt.Errorf("data_pointer.hash must have content after 'sha256:' prefix")
		}
	}

	return nil
}

// CommitParent is a DAG edge from commit to parent
type CommitParent struct {
	CommitID string
	ParentID string
	RepoID   string
}

func (c *Commit) Validate() error {
	if len(c.ParentIDs) >= 2 {
		return fmt.Errorf(
			"commit cannot have 2 or more parents - use /merges endpoint for merge commits",
		)
	}
	return nil
}

var (
	ErrCommitNotFound   = errors.New("commit_not_found")
	ErrInvalidParent    = errors.New("invalid_parent")
	ErrStaleMergeTarget = errors.New("stale_merge_target")
)

package testhelper

import (
	"time"

	"github.com/google/uuid"

	"github.com/bhpcv252/verge/internal/domain"
)

func FixedRepo() *domain.Repo {
	return &domain.Repo{
		ID:            "repo_abc123",
		Name:          "my-doc",
		DefaultBranch: "main",
		CreatedAt:     time.Date(2024, 4, 5, 10, 0, 0, 0, time.UTC),
	}
}

func FixedBranch() *domain.Branch {
	return &domain.Branch{
		Name:      "main",
		RepoID:    "repo_abc123",
		CommitID:  "commit_xyz789",
		CreatedAt: time.Date(2024, 4, 5, 10, 0, 0, 0, time.UTC),
	}
}

func FixedCommit() *domain.Commit {
	return &domain.Commit{
		ID:        "commit_abc123",
		RepoID:    "repo_xyz789",
		ParentIDs: []string{"commit_parent"},
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
			Metadata: []byte(`{"key":"value"}`),
		},
		Message:   "Add feature",
		Author:    "alice@example.com",
		Timestamp: time.Date(2024, 4, 5, 10, 0, 0, 0, time.UTC),
	}
}

func FixedMergeCommit() *domain.Commit {
	return &domain.Commit{
		ID:        "commit_merge_abc",
		RepoID:    "repo_xyz",
		ParentIDs: []string{"commit_source", "commit_target"},
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
			Metadata: []byte(`{"key":"value"}`),
		},
		Message:   "Merge feature into main",
		Author:    "alice@example.com",
		Timestamp: time.Date(2024, 4, 5, 10, 0, 0, 0, time.UTC),
	}
}

func UniqueID(prefix string) string {
	return prefix + "_" + uuid.New().String()
}

func UniqueName(prefix string) string {
	return prefix + "-" + uuid.New().String()
}

func NewTestRepo(name string) *domain.Repo {
	return &domain.Repo{
		ID:            UniqueID("repo"),
		Name:          name,
		DefaultBranch: "main",
		CreatedAt:     time.Now().UTC().Truncate(time.Microsecond),
	}
}

func NewTestBranch(repoID, commitID string) *domain.Branch {
	return &domain.Branch{
		Name:      UniqueName("branch"),
		RepoID:    repoID,
		CommitID:  commitID,
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
}

func NewTestCommit(repoID string, parentIDs []string) *domain.Commit {
	return &domain.Commit{
		ID:     UniqueID("commit"),
		RepoID: repoID,
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message:   "test commit",
		Author:    "test@example.com",
		Timestamp: time.Now().UTC().Truncate(time.Microsecond),
		ParentIDs: parentIDs,
	}
}

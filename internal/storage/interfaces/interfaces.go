package interfaces

import (
	"context"
	"errors"
	"time"

	"github.com/bhpcv252/verge/internal/domain"
)

var ErrCacheMiss = errors.New("cache_miss")

type BranchHeadStore interface {
	GetHead(ctx context.Context, repoID, name string) (string, error)

	SetHead(ctx context.Context, repoID, name, commitID string, version int64) error
}

type CommitCache interface {
	GetCommit(ctx context.Context, repoID, commitID string) (*domain.Commit, error)
	SetCommit(ctx context.Context, commit *domain.Commit) error
}

// GraphStore is the interface for DAG traversal queries,
// implemented by both postgres (recursive CTE) and neo4j (Cypher)
type GraphStore interface {
	TraverseDAG(ctx context.Context, params TraversalParams) ([]*domain.Commit, string, error)

	GetAncestors(ctx context.Context, repoID, commitID string, limit int) ([]*domain.Commit, error)

	FindMergeBase(ctx context.Context, repoID, commitA, commitB string) (*domain.Commit, error)
}

type TraversalParams struct {
	RepoID string
	Head   string
	Author string
	Since  *time.Time
	Until  *time.Time
	Limit  int
	Cursor string
}

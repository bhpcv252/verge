package composite

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/storage/interfaces"
	"github.com/bhpcv252/verge/internal/storage/postgres"
)

type pgBranchDelegate interface {
	Create(ctx context.Context, branch *domain.Branch) error
	GetByName(ctx context.Context, repoID, name string) (*domain.Branch, error)
	List(
		ctx context.Context,
		repoID string,
		limit int,
		cursor string,
	) (*postgres.ListBranchesPage, error)
	Advance(
		ctx context.Context,
		repoID, name, commitID, expectedCommitID string,
	) (*domain.Branch, error)
	Delete(ctx context.Context, repoID, name string) error
}

type BranchRouter struct {
	pg    pgBranchDelegate
	redis interfaces.BranchHeadStore
}

func NewBranchRouter(pg pgBranchDelegate, redis interfaces.BranchHeadStore) *BranchRouter {
	return &BranchRouter{pg: pg, redis: redis}
}

func (r *BranchRouter) GetHead(ctx context.Context, repoID, name string) (string, error) {
	commitID, err := r.redis.GetHead(ctx, repoID, name)
	if err == nil {
		return commitID, nil
	}
	if !errors.Is(err, interfaces.ErrCacheMiss) {
		log.Printf("branch router: redis GetHead error (falling back to postgres): %v", err)
	}

	// postgres fallback
	branch, err := r.pg.GetByName(ctx, repoID, name)
	if err != nil {
		return "", fmt.Errorf("branch router: get head fallback: %w", err)
	}

	if setErr := r.redis.SetHead(ctx, repoID, name, branch.CommitID, time.Now().UnixMilli()); setErr != nil {
		log.Printf("branch router: redis SetHead on miss failed (non-fatal): %v", setErr)
	}

	return branch.CommitID, nil
}

func (r *BranchRouter) SetHead(
	ctx context.Context,
	repoID, name, commitID string,
	version int64,
) error {
	return r.redis.SetHead(ctx, repoID, name, commitID, version)
}

func (r *BranchRouter) Create(ctx context.Context, branch *domain.Branch) error {
	return r.pg.Create(ctx, branch)
}

func (r *BranchRouter) GetByName(ctx context.Context, repoID, name string) (*domain.Branch, error) {
	return r.pg.GetByName(ctx, repoID, name)
}

func (r *BranchRouter) List(
	ctx context.Context,
	repoID string,
	limit int,
	cursor string,
) (*postgres.ListBranchesPage, error) {
	return r.pg.List(ctx, repoID, limit, cursor)
}

func (r *BranchRouter) Advance(
	ctx context.Context,
	repoID, name, commitID, expectedCommitID string,
) (*domain.Branch, error) {
	branch, err := r.pg.Advance(ctx, repoID, name, commitID, expectedCommitID)
	if err != nil {
		return nil, err
	}

	version := time.Now().UnixMilli()
	if setErr := r.redis.SetHead(ctx, repoID, name, commitID, version); setErr != nil {
		log.Printf("branch router: sync redis SetHead after Advance failed (non-fatal): %v", setErr)
	}

	return branch, nil
}

func (r *BranchRouter) Delete(ctx context.Context, repoID, name string) error {
	return r.pg.Delete(ctx, repoID, name)
}

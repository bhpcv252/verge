package composite

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/storage/interfaces"
	"github.com/bhpcv252/verge/internal/storage/postgres"
)

type pgCommitDelegate interface {
	Create(ctx context.Context, commit *domain.Commit, parentIDs []string) (*domain.Commit, error)
	GetByID(ctx context.Context, repoID, commitID string) (*domain.Commit, error)
	GetByIdempotencyKey(ctx context.Context, repoID, idempotencyKey string) (*domain.Commit, error)
	List(ctx context.Context, in postgres.ListCommitsFilter) (*postgres.ListCommitsPage, error)
	GetParents(ctx context.Context, repoID, commitID string) ([]*domain.Commit, error)
	ValidateParentsExist(ctx context.Context, repoID string, parentIDs []string) error
}

type CommitRouter struct {
	pg    pgCommitDelegate
	cache interfaces.CommitCache
}

func NewCommitRouter(pg pgCommitDelegate, cache interfaces.CommitCache) *CommitRouter {
	return &CommitRouter{pg: pg, cache: cache}
}

func (r *CommitRouter) Create(
	ctx context.Context,
	commit *domain.Commit,
	parentIDs []string,
) (*domain.Commit, error) {
	return r.pg.Create(ctx, commit, parentIDs)
}

func (r *CommitRouter) GetByID(
	ctx context.Context,
	repoID, commitID string,
) (*domain.Commit, error) {
	commit, err := r.cache.GetCommit(ctx, repoID, commitID)
	if err == nil {
		return commit, nil
	}
	if !errors.Is(err, interfaces.ErrCacheMiss) {
		log.Printf("commit router: redis GetCommit error (falling back to postgres): %v", err)
	}

	commit, err = r.pg.GetByID(ctx, repoID, commitID)
	if err != nil {
		return nil, fmt.Errorf("commit router: get by id fallback: %w", err)
	}

	if setErr := r.cache.SetCommit(ctx, commit); setErr != nil {
		log.Printf("commit router: redis SetCommit on miss failed (non-fatal): %v", setErr)
	}

	return commit, nil
}

func (r *CommitRouter) GetByIdempotencyKey(
	ctx context.Context,
	repoID, idempotencyKey string,
) (*domain.Commit, error) {
	return r.pg.GetByIdempotencyKey(ctx, repoID, idempotencyKey)
}

func (r *CommitRouter) List(
	ctx context.Context,
	in postgres.ListCommitsFilter,
) (*postgres.ListCommitsPage, error) {
	return r.pg.List(ctx, in)
}

func (r *CommitRouter) GetParents(
	ctx context.Context,
	repoID, commitID string,
) ([]*domain.Commit, error) {
	return r.pg.GetParents(ctx, repoID, commitID)
}

func (r *CommitRouter) ValidateParentsExist(
	ctx context.Context,
	repoID string,
	parentIDs []string,
) error {
	return r.pg.ValidateParentsExist(ctx, repoID, parentIDs)
}

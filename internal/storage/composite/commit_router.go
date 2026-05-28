package composite

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/observability"
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
	obs   *observability.Provider
}

func NewCommitRouter(
	pg pgCommitDelegate,
	cache interfaces.CommitCache,
	obs *observability.Provider,
) *CommitRouter {
	return &CommitRouter{pg: pg, cache: cache, obs: obs}
}

func (r *CommitRouter) Create(
	ctx context.Context,
	commit *domain.Commit,
	parentIDs []string,
) (*domain.Commit, error) {
	ctx, done := r.obs.StorageSpan(ctx, "postgres", "commit.create")
	c, err := r.pg.Create(ctx, commit, parentIDs)
	done(err)
	return c, err
}

func (r *CommitRouter) GetByID(
	ctx context.Context,
	repoID, commitID string,
) (_ *domain.Commit, err error) {
	ctx, done := r.obs.StorageSpan(ctx, "redis", "commit.get_by_id")
	defer func() { done(err) }()

	commit, cacheErr := r.cache.GetCommit(ctx, repoID, commitID)
	if cacheErr == nil {
		r.obs.RecordCacheHit(ctx, "redis", "commit")
		return commit, nil
	}

	if !errors.Is(cacheErr, interfaces.ErrCacheMiss) {
		observability.L(ctx).Warn("commit router: redis GetCommit error, falling back to postgres",
			slog.String("error", cacheErr.Error()),
		)
	} else {
		r.obs.RecordCacheMiss(ctx, "redis", "commit")
	}

	commit, err = r.pg.GetByID(ctx, repoID, commitID)
	if err != nil {
		return nil, fmt.Errorf("commit router: get by id fallback: %w", err)
	}

	if setErr := r.cache.SetCommit(ctx, commit); setErr != nil {
		observability.L(ctx).Debug("commit router: redis SetCommit on miss failed",
			slog.String("error", setErr.Error()),
		)
	}

	return commit, nil
}

func (r *CommitRouter) GetByIdempotencyKey(
	ctx context.Context,
	repoID, idempotencyKey string,
) (*domain.Commit, error) {
	ctx, done := r.obs.StorageSpan(ctx, "postgres", "commit.get_by_idempotency_key")
	c, err := r.pg.GetByIdempotencyKey(ctx, repoID, idempotencyKey)
	done(err)
	return c, err
}

func (r *CommitRouter) List(
	ctx context.Context,
	in postgres.ListCommitsFilter,
) (*postgres.ListCommitsPage, error) {
	ctx, done := r.obs.StorageSpan(ctx, "postgres", "commit.list")
	page, err := r.pg.List(ctx, in)
	done(err)
	return page, err
}

func (r *CommitRouter) GetParents(
	ctx context.Context,
	repoID, commitID string,
) ([]*domain.Commit, error) {
	ctx, done := r.obs.StorageSpan(ctx, "postgres", "commit.get_parents")
	parents, err := r.pg.GetParents(ctx, repoID, commitID)
	done(err)
	return parents, err
}

func (r *CommitRouter) ValidateParentsExist(
	ctx context.Context,
	repoID string,
	parentIDs []string,
) error {
	ctx, done := r.obs.StorageSpan(ctx, "postgres", "commit.validate_parents")
	err := r.pg.ValidateParentsExist(ctx, repoID, parentIDs)
	done(err)
	return err
}

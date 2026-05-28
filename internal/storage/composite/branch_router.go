package composite

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/observability"
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
	obs   *observability.Provider
}

func NewBranchRouter(
	pg pgBranchDelegate,
	redis interfaces.BranchHeadStore,
	obs *observability.Provider,
) *BranchRouter {
	return &BranchRouter{pg: pg, redis: redis, obs: obs}
}

func (r *BranchRouter) GetHead(ctx context.Context, repoID, name string) (_ string, err error) {
	ctx, done := r.obs.StorageSpan(ctx, "redis", "branch.get_head")
	defer func() { done(err) }()

	commitID, redisErr := r.redis.GetHead(ctx, repoID, name)
	if redisErr == nil {
		r.obs.RecordCacheHit(ctx, "redis", "branch_head")
		return commitID, nil
	}

	if !errors.Is(redisErr, interfaces.ErrCacheMiss) {
		// unexpected redis error, log it, but continue to postgres
		observability.L(ctx).Warn("branch router: redis GetHead error, falling back to postgres",
			slog.String("error", redisErr.Error()),
		)
	} else {
		r.obs.RecordCacheMiss(ctx, "redis", "branch_head")
	}

	branch, err := r.pg.GetByName(ctx, repoID, name)
	if err != nil {
		return "", fmt.Errorf("branch router: get head fallback: %w", err)
	}

	// repopulate the cache on miss
	if setErr := r.redis.SetHead(ctx, repoID, name, branch.CommitID, time.Now().UnixMilli()); setErr != nil {
		observability.L(ctx).Debug("branch router: redis SetHead on miss failed",
			slog.String("error", setErr.Error()),
		)
	}

	return branch.CommitID, nil
}

func (r *BranchRouter) SetHead(
	ctx context.Context,
	repoID, name, commitID string,
	version int64,
) error {
	ctx, done := r.obs.StorageSpan(ctx, "redis", "branch.set_head")
	err := r.redis.SetHead(ctx, repoID, name, commitID, version)
	done(err)
	return err
}

func (r *BranchRouter) Create(ctx context.Context, branch *domain.Branch) error {
	ctx, done := r.obs.StorageSpan(ctx, "postgres", "branch.create")
	err := r.pg.Create(ctx, branch)
	done(err)
	return err
}

func (r *BranchRouter) GetByName(ctx context.Context, repoID, name string) (*domain.Branch, error) {
	ctx, done := r.obs.StorageSpan(ctx, "postgres", "branch.get_by_name")
	branch, err := r.pg.GetByName(ctx, repoID, name)
	done(err)
	return branch, err
}

func (r *BranchRouter) List(
	ctx context.Context,
	repoID string,
	limit int,
	cursor string,
) (*postgres.ListBranchesPage, error) {
	ctx, done := r.obs.StorageSpan(ctx, "postgres", "branch.list")
	page, err := r.pg.List(ctx, repoID, limit, cursor)
	done(err)
	return page, err
}

func (r *BranchRouter) Advance(
	ctx context.Context,
	repoID, name, commitID, expectedCommitID string,
) (_ *domain.Branch, err error) {
	ctx, done := r.obs.StorageSpan(ctx, "postgres", "branch.advance")
	defer func() { done(err) }()

	branch, err := r.pg.Advance(ctx, repoID, name, commitID, expectedCommitID)
	if err != nil {
		return nil, err
	}

	if setErr := r.redis.SetHead(ctx, repoID, name, commitID, time.Now().UnixMilli()); setErr != nil {
		observability.L(ctx).Warn("branch router: redis sync after Advance failed",
			slog.String("error", setErr.Error()),
		)
	}

	return branch, nil
}

func (r *BranchRouter) Delete(ctx context.Context, repoID, name string) error {
	ctx, done := r.obs.StorageSpan(ctx, "postgres", "branch.delete")
	err := r.pg.Delete(ctx, repoID, name)
	done(err)
	return err
}

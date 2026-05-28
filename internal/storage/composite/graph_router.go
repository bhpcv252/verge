package composite

import (
	"context"
	"log/slog"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/observability"
	"github.com/bhpcv252/verge/internal/storage/interfaces"
)

type GraphRouter struct {
	neo4j interfaces.GraphStore // primary
	pg    interfaces.GraphStore // fallback
	obs   *observability.Provider
}

func NewGraphRouter(
	neo4j interfaces.GraphStore,
	pg interfaces.GraphStore,
	obs *observability.Provider,
) *GraphRouter {
	return &GraphRouter{neo4j: neo4j, pg: pg, obs: obs}
}

func (r *GraphRouter) TraverseDAG(
	ctx context.Context,
	params interfaces.TraversalParams,
) (_ []*domain.Commit, _ string, err error) {
	ctx, done := r.obs.StorageSpan(ctx, "neo4j", "graph.traverse_dag")
	defer func() { done(err) }()

	commits, cursor, err := r.neo4j.TraverseDAG(ctx, params)
	if err == nil {
		return commits, cursor, nil
	}

	observability.L(ctx).Warn("graph router: neo4j TraverseDAG error, falling back to postgres",
		slog.String("error", err.Error()),
	)

	commits, cursor, err = r.pg.TraverseDAG(ctx, params)
	return commits, cursor, err
}

func (r *GraphRouter) GetAncestors(
	ctx context.Context,
	repoID, commitID string,
	limit int,
) (_ []*domain.Commit, err error) {
	ctx, done := r.obs.StorageSpan(ctx, "neo4j", "graph.get_ancestors")
	defer func() { done(err) }()

	commits, err := r.neo4j.GetAncestors(ctx, repoID, commitID, limit)
	if err == nil {
		return commits, nil
	}

	observability.L(ctx).Warn("graph router: neo4j GetAncestors error, falling back to postgres",
		slog.String("error", err.Error()),
	)

	commits, err = r.pg.GetAncestors(ctx, repoID, commitID, limit)
	return commits, err
}

func (r *GraphRouter) FindMergeBase(
	ctx context.Context,
	repoID, commitA, commitB string,
) (_ *domain.Commit, err error) {
	ctx, done := r.obs.StorageSpan(ctx, "neo4j", "graph.find_merge_base")
	defer func() { done(err) }()

	commit, err := r.neo4j.FindMergeBase(ctx, repoID, commitA, commitB)
	if err == nil {
		return commit, nil
	}

	observability.L(ctx).Warn("graph router: neo4j FindMergeBase error, falling back to postgres",
		slog.String("error", err.Error()),
	)

	commit, err = r.pg.FindMergeBase(ctx, repoID, commitA, commitB)
	return commit, err
}

package composite

import (
	"context"
	"log"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/storage/interfaces"
)

type GraphRouter struct {
	neo4j interfaces.GraphStore // primary
	pg    interfaces.GraphStore // fallback
}

func NewGraphRouter(neo4j, pg interfaces.GraphStore) *GraphRouter {
	return &GraphRouter{neo4j: neo4j, pg: pg}
}

func (r *GraphRouter) TraverseDAG(
	ctx context.Context,
	params interfaces.TraversalParams,
) ([]*domain.Commit, string, error) {
	commits, cursor, err := r.neo4j.TraverseDAG(ctx, params)
	if err == nil {
		return commits, cursor, nil
	}
	log.Printf("graph router: neo4j TraverseDAG error (falling back to postgres): %v", err)
	return r.pg.TraverseDAG(ctx, params)
}

func (r *GraphRouter) GetAncestors(
	ctx context.Context,
	repoID, commitID string,
	limit int,
) ([]*domain.Commit, error) {
	commits, err := r.neo4j.GetAncestors(ctx, repoID, commitID, limit)
	if err == nil {
		return commits, nil
	}
	log.Printf("graph router: neo4j GetAncestors error (falling back to postgres): %v", err)
	return r.pg.GetAncestors(ctx, repoID, commitID, limit)
}

func (r *GraphRouter) FindMergeBase(
	ctx context.Context,
	repoID, commitA, commitB string,
) (*domain.Commit, error) {
	commit, err := r.neo4j.FindMergeBase(ctx, repoID, commitA, commitB)
	if err == nil {
		return commit, nil
	}
	log.Printf("graph router: neo4j FindMergeBase error (falling back to postgres): %v", err)
	return r.pg.FindMergeBase(ctx, repoID, commitA, commitB)
}

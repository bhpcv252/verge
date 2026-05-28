package composite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/observability"
	"github.com/bhpcv252/verge/internal/storage/interfaces"
)

func newTestGraphRouter(neo4j interfaces.GraphStore, pg interfaces.GraphStore) *GraphRouter {
	return NewGraphRouter(neo4j, pg, observability.Noop())
}

// mock

type stubGraphStore struct {
	traverseResult []*domain.Commit
	traverseCursor string
	traverseErr    error
	traverseCalls  int

	ancestorsResult []*domain.Commit
	ancestorsErr    error
	ancestorsCalls  int

	mergeBaseResult *domain.Commit
	mergeBaseErr    error
	mergeBaseCalls  int
}

func (s *stubGraphStore) TraverseDAG(
	_ context.Context,
	_ interfaces.TraversalParams,
) ([]*domain.Commit, string, error) {
	s.traverseCalls++
	return s.traverseResult, s.traverseCursor, s.traverseErr
}

func (s *stubGraphStore) GetAncestors(
	_ context.Context,
	_, _ string,
	_ int,
) ([]*domain.Commit, error) {
	s.ancestorsCalls++
	return s.ancestorsResult, s.ancestorsErr
}

func (s *stubGraphStore) FindMergeBase(_ context.Context, _, _, _ string) (*domain.Commit, error) {
	s.mergeBaseCalls++
	return s.mergeBaseResult, s.mergeBaseErr
}

func makeCommits(ids ...string) []*domain.Commit {
	commits := make([]*domain.Commit, len(ids))
	for i, id := range ids {
		commits[i] = &domain.Commit{
			ID:        id,
			RepoID:    "r1",
			Timestamp: time.Now(),
			ParentIDs: []string{},
		}
	}
	return commits
}

func makeGraphParams() interfaces.TraversalParams {
	return interfaces.TraversalParams{
		RepoID: "r1",
		Head:   "c-head",
		Limit:  10,
	}
}

// TraverseDAG

func TestGraphRouter_TraverseDAG_Neo4jSucceeds_ReturnsNeo4jResult(t *testing.T) {
	want := makeCommits("c1", "c2", "c3")
	neo4j := &stubGraphStore{traverseResult: want, traverseCursor: "next-page"}
	pg := &stubGraphStore{}
	r := newTestGraphRouter(neo4j, pg)

	got, cursor, err := r.TraverseDAG(context.Background(), makeGraphParams())

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, "next-page", cursor)
	assert.Equal(t, 0, pg.traverseCalls, "postgres must not be called when neo4j succeeds")
}

func TestGraphRouter_TraverseDAG_Neo4jFails_FallsBackToPostgres(t *testing.T) {
	want := makeCommits("c1", "c2")
	neo4j := &stubGraphStore{traverseErr: errors.New("neo4j unavailable")}
	pg := &stubGraphStore{traverseResult: want}
	r := newTestGraphRouter(neo4j, pg)

	got, _, err := r.TraverseDAG(context.Background(), makeGraphParams())

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, 1, pg.traverseCalls)
}

func TestGraphRouter_TraverseDAG_Neo4jFails_PostgresResultReturned(t *testing.T) {
	want := makeCommits("pg-c1")
	neo4j := &stubGraphStore{traverseErr: errors.New("driver error")}
	pg := &stubGraphStore{traverseResult: want, traverseCursor: "pg-cursor"}
	r := newTestGraphRouter(neo4j, pg)

	got, cursor, err := r.TraverseDAG(context.Background(), makeGraphParams())

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, "pg-cursor", cursor)
}

func TestGraphRouter_TraverseDAG_BothFail_ReturnsPGError(t *testing.T) {
	pgErr := errors.New("postgres also down")
	neo4j := &stubGraphStore{traverseErr: errors.New("neo4j down")}
	pg := &stubGraphStore{traverseErr: pgErr}
	r := newTestGraphRouter(neo4j, pg)

	_, _, err := r.TraverseDAG(context.Background(), makeGraphParams())

	require.Error(t, err)
	assert.ErrorIs(t, err, pgErr)
}

func TestGraphRouter_TraverseDAG_Neo4jSucceeds_CalledExactlyOnce(t *testing.T) {
	neo4j := &stubGraphStore{traverseResult: makeCommits("c1")}
	r := newTestGraphRouter(neo4j, &stubGraphStore{})

	_, _, _ = r.TraverseDAG(context.Background(), makeGraphParams())

	assert.Equal(t, 1, neo4j.traverseCalls)
}

// GetAncestors

func TestGraphRouter_GetAncestors_Neo4jSucceeds_ReturnsNeo4jResult(t *testing.T) {
	want := makeCommits("parent1", "parent2")
	neo4j := &stubGraphStore{ancestorsResult: want}
	pg := &stubGraphStore{}
	r := newTestGraphRouter(neo4j, pg)

	got, err := r.GetAncestors(context.Background(), "r1", "c1", 10)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, 0, pg.ancestorsCalls, "postgres must not be called when neo4j succeeds")
}

func TestGraphRouter_GetAncestors_Neo4jFails_FallsBackToPostgres(t *testing.T) {
	want := makeCommits("pg-parent")
	neo4j := &stubGraphStore{ancestorsErr: errors.New("session error")}
	pg := &stubGraphStore{ancestorsResult: want}
	r := newTestGraphRouter(neo4j, pg)

	got, err := r.GetAncestors(context.Background(), "r1", "c1", 10)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, 1, pg.ancestorsCalls)
}

func TestGraphRouter_GetAncestors_BothFail_ReturnsPGError(t *testing.T) {
	pgErr := errors.New("postgres recursive CTE failed")
	neo4j := &stubGraphStore{ancestorsErr: errors.New("neo4j error")}
	pg := &stubGraphStore{ancestorsErr: pgErr}
	r := newTestGraphRouter(neo4j, pg)

	_, err := r.GetAncestors(context.Background(), "r1", "c1", 10)

	require.Error(t, err)
	assert.ErrorIs(t, err, pgErr)
}

func TestGraphRouter_GetAncestors_Neo4jSucceeds_CalledExactlyOnce(t *testing.T) {
	neo4j := &stubGraphStore{ancestorsResult: makeCommits("p1")}
	r := newTestGraphRouter(neo4j, &stubGraphStore{})

	_, _ = r.GetAncestors(context.Background(), "r1", "c1", 10)

	assert.Equal(t, 1, neo4j.ancestorsCalls)
}

// FindMergeBase

func TestGraphRouter_FindMergeBase_Neo4jSucceeds_ReturnsNeo4jResult(t *testing.T) {
	lca := &domain.Commit{ID: "lca", RepoID: "r1"}
	neo4j := &stubGraphStore{mergeBaseResult: lca}
	pg := &stubGraphStore{}
	r := newTestGraphRouter(neo4j, pg)

	got, err := r.FindMergeBase(context.Background(), "r1", "c-a", "c-b")

	require.NoError(t, err)
	assert.Equal(t, lca, got)
	assert.Equal(t, 0, pg.mergeBaseCalls, "postgres must not be called when neo4j succeeds")
}

func TestGraphRouter_FindMergeBase_Neo4jFails_FallsBackToPostgres(t *testing.T) {
	lca := &domain.Commit{ID: "pg-lca", RepoID: "r1"}
	neo4j := &stubGraphStore{mergeBaseErr: errors.New("query timeout")}
	pg := &stubGraphStore{mergeBaseResult: lca}
	r := newTestGraphRouter(neo4j, pg)

	got, err := r.FindMergeBase(context.Background(), "r1", "c-a", "c-b")

	require.NoError(t, err)
	assert.Equal(t, lca, got)
	assert.Equal(t, 1, pg.mergeBaseCalls)
}

func TestGraphRouter_FindMergeBase_BothFail_ReturnsPGError(t *testing.T) {
	pgErr := errors.New("no common ancestor found")
	neo4j := &stubGraphStore{mergeBaseErr: errors.New("neo4j down")}
	pg := &stubGraphStore{mergeBaseErr: pgErr}
	r := newTestGraphRouter(neo4j, pg)

	_, err := r.FindMergeBase(context.Background(), "r1", "c-a", "c-b")

	require.Error(t, err)
	assert.ErrorIs(t, err, pgErr)
}

func TestGraphRouter_FindMergeBase_Neo4jSucceeds_CalledExactlyOnce(t *testing.T) {
	neo4j := &stubGraphStore{mergeBaseResult: &domain.Commit{ID: "lca"}}
	r := newTestGraphRouter(neo4j, &stubGraphStore{})

	_, _ = r.FindMergeBase(context.Background(), "r1", "c-a", "c-b")

	assert.Equal(t, 1, neo4j.mergeBaseCalls)
}

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
	"github.com/bhpcv252/verge/internal/storage/postgres"
)

func newTestCommitRouter(pg pgCommitDelegate, cache interfaces.CommitCache) *CommitRouter {
	return NewCommitRouter(pg, cache, observability.Noop())
}

// mocks

// stubPGCommit implements pgCommitDelegate
type stubPGCommit struct {
	getByIDResult *domain.Commit
	getByIDErr    error
	getByIDCalls  int

	createResult *domain.Commit
	createErr    error
	createCalls  int

	getByIKResult *domain.Commit
	getByIKErr    error
	getByIKCalls  int

	listResult *postgres.ListCommitsPage
	listErr    error
	listCalls  int

	getParentsResult []*domain.Commit
	getParentsErr    error
	getParentsCalls  int

	validateErr   error
	validateCalls int
}

func (s *stubPGCommit) Create(
	_ context.Context,
	c *domain.Commit,
	_ []string,
) (*domain.Commit, error) {
	s.createCalls++
	if s.createResult != nil {
		return s.createResult, s.createErr
	}
	return c, s.createErr
}

func (s *stubPGCommit) GetByID(_ context.Context, _, _ string) (*domain.Commit, error) {
	s.getByIDCalls++
	return s.getByIDResult, s.getByIDErr
}

func (s *stubPGCommit) GetByIdempotencyKey(_ context.Context, _, _ string) (*domain.Commit, error) {
	s.getByIKCalls++
	return s.getByIKResult, s.getByIKErr
}

func (s *stubPGCommit) List(
	_ context.Context,
	_ postgres.ListCommitsFilter,
) (*postgres.ListCommitsPage, error) {
	s.listCalls++
	return s.listResult, s.listErr
}

func (s *stubPGCommit) GetParents(_ context.Context, _, _ string) ([]*domain.Commit, error) {
	s.getParentsCalls++
	return s.getParentsResult, s.getParentsErr
}

func (s *stubPGCommit) ValidateParentsExist(_ context.Context, _ string, _ []string) error {
	s.validateCalls++
	return s.validateErr
}

// stubCommitCache implements interfaces.CommitCache
type stubCommitCache struct {
	getResult *domain.Commit
	getErr    error
	getCalls  int

	setErr   error
	setCalls int
}

func (s *stubCommitCache) GetCommit(_ context.Context, _, _ string) (*domain.Commit, error) {
	s.getCalls++
	return s.getResult, s.getErr
}

func (s *stubCommitCache) SetCommit(_ context.Context, _ *domain.Commit) error {
	s.setCalls++
	return s.setErr
}

func makeCommit(id, repoID string) *domain.Commit {
	return &domain.Commit{
		ID:        id,
		RepoID:    repoID,
		Message:   "test commit",
		Author:    "test@example.com",
		Timestamp: time.Now(),
		ParentIDs: []string{},
	}
}

// GetByID - cache hit

func TestCommitRouter_GetByID_CacheHit_ReturnsCachedCommit(t *testing.T) {
	want := makeCommit("c1", "r1")
	cache := &stubCommitCache{getResult: want}
	pg := &stubPGCommit{}
	r := newTestCommitRouter(pg, cache)

	got, err := r.GetByID(context.Background(), "r1", "c1")

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, 0, pg.getByIDCalls, "postgres must not be called on cache hit")
}

// GetByID - cache miss

func TestCommitRouter_GetByID_CacheMiss_FallsBackToPostgres(t *testing.T) {
	cache := &stubCommitCache{getErr: interfaces.ErrCacheMiss}
	pg := &stubPGCommit{getByIDResult: makeCommit("c1", "r1")}
	r := newTestCommitRouter(pg, cache)

	got, err := r.GetByID(context.Background(), "r1", "c1")

	require.NoError(t, err)
	assert.Equal(t, "c1", got.ID)
	assert.Equal(t, 1, pg.getByIDCalls)
}

func TestCommitRouter_GetByID_CacheMiss_PopulatesCache(t *testing.T) {
	cache := &stubCommitCache{getErr: interfaces.ErrCacheMiss}
	pg := &stubPGCommit{getByIDResult: makeCommit("c1", "r1")}
	r := newTestCommitRouter(pg, cache)

	_, err := r.GetByID(context.Background(), "r1", "c1")

	require.NoError(t, err)
	assert.Equal(t, 1, cache.setCalls, "SetCommit must be called to populate cache after miss")
}

func TestCommitRouter_GetByID_CacheMiss_SetCommitFails_StillReturnsPGValue(t *testing.T) {
	cache := &stubCommitCache{
		getErr: interfaces.ErrCacheMiss,
		setErr: errors.New("redis full"),
	}
	pg := &stubPGCommit{getByIDResult: makeCommit("c1", "r1")}
	r := newTestCommitRouter(pg, cache)

	got, err := r.GetByID(context.Background(), "r1", "c1")

	require.NoError(t, err, "SetCommit failure must be non-fatal")
	assert.Equal(t, "c1", got.ID)
}

func TestCommitRouter_GetByID_CacheMiss_PostgresNotFound_ReturnsError(t *testing.T) {
	cache := &stubCommitCache{getErr: interfaces.ErrCacheMiss}
	pg := &stubPGCommit{getByIDErr: domain.ErrCommitNotFound}
	r := newTestCommitRouter(pg, cache)

	_, err := r.GetByID(context.Background(), "r1", "c-nonexistent")

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrCommitNotFound)
	assert.Equal(t, 0, cache.setCalls,
		"SetCommit must not be called when postgres returns not-found")
}

// GetByID - cache error (not a miss)

func TestCommitRouter_GetByID_CacheError_FallsBackToPostgres(t *testing.T) {
	cache := &stubCommitCache{getErr: errors.New("connection refused")} // not ErrCacheMiss
	pg := &stubPGCommit{getByIDResult: makeCommit("c1", "r1")}
	r := newTestCommitRouter(pg, cache)

	got, err := r.GetByID(context.Background(), "r1", "c1")

	require.NoError(t, err)
	assert.Equal(t, "c1", got.ID)
	assert.Equal(t, 1, pg.getByIDCalls)
}

func TestCommitRouter_GetByID_CacheError_StillTriesToSetCommit(t *testing.T) {
	cache := &stubCommitCache{getErr: errors.New("timeout")}
	pg := &stubPGCommit{getByIDResult: makeCommit("c1", "r1")}
	r := newTestCommitRouter(pg, cache)

	_, _ = r.GetByID(context.Background(), "r1", "c1")

	assert.Equal(t, 1, cache.setCalls)
}

// Delegate methods - straight pass-through to postgres

func TestCommitRouter_Create_DelegatesToPostgres(t *testing.T) {
	commit := makeCommit("c1", "r1")
	pg := &stubPGCommit{createResult: commit}
	r := newTestCommitRouter(pg, &stubCommitCache{getErr: interfaces.ErrCacheMiss})

	got, err := r.Create(context.Background(), commit, []string{})

	require.NoError(t, err)
	assert.Equal(t, commit, got)
	assert.Equal(t, 1, pg.createCalls)
}

func TestCommitRouter_GetByIdempotencyKey_DelegatesToPostgres(t *testing.T) {
	want := makeCommit("c1", "r1")
	pg := &stubPGCommit{getByIKResult: want}
	r := newTestCommitRouter(pg, &stubCommitCache{getErr: interfaces.ErrCacheMiss})

	got, err := r.GetByIdempotencyKey(context.Background(), "r1", "idem-key")

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, 1, pg.getByIKCalls)
}

func TestCommitRouter_List_DelegatesToPostgres(t *testing.T) {
	want := &postgres.ListCommitsPage{Commits: []*domain.Commit{makeCommit("c1", "r1")}}
	pg := &stubPGCommit{listResult: want}
	r := newTestCommitRouter(pg, &stubCommitCache{getErr: interfaces.ErrCacheMiss})

	got, err := r.List(context.Background(), postgres.ListCommitsFilter{RepoID: "r1", Limit: 10})

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, 1, pg.listCalls)
}

func TestCommitRouter_GetParents_DelegatesToPostgres(t *testing.T) {
	want := []*domain.Commit{makeCommit("p1", "r1")}
	pg := &stubPGCommit{getParentsResult: want}
	r := newTestCommitRouter(pg, &stubCommitCache{getErr: interfaces.ErrCacheMiss})

	got, err := r.GetParents(context.Background(), "r1", "c1")

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, 1, pg.getParentsCalls)
}

func TestCommitRouter_ValidateParentsExist_DelegatesToPostgres(t *testing.T) {
	pg := &stubPGCommit{}
	r := newTestCommitRouter(pg, &stubCommitCache{getErr: interfaces.ErrCacheMiss})

	err := r.ValidateParentsExist(context.Background(), "r1", []string{"p1", "p2"})

	require.NoError(t, err)
	assert.Equal(t, 1, pg.validateCalls)
}

func TestCommitRouter_DelegateMethods_NeverTouchCache(t *testing.T) {
	cache := &stubCommitCache{}
	pg := &stubPGCommit{
		listResult:       &postgres.ListCommitsPage{},
		getParentsResult: []*domain.Commit{},
	}
	r := newTestCommitRouter(pg, cache)

	_, _ = r.Create(context.Background(), makeCommit("c1", "r1"), []string{})
	_, _ = r.GetByIdempotencyKey(context.Background(), "r1", "key")
	_, _ = r.List(context.Background(), postgres.ListCommitsFilter{RepoID: "r1", Limit: 5})
	_, _ = r.GetParents(context.Background(), "r1", "c1")
	_ = r.ValidateParentsExist(context.Background(), "r1", []string{"p1"})

	assert.Equal(t, 0, cache.getCalls)
	assert.Equal(t, 0, cache.setCalls)
}

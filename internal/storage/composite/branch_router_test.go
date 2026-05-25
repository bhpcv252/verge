package composite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/storage/interfaces"
	"github.com/bhpcv252/verge/internal/storage/postgres"
)

// mocks

// stubPGBranch implements pgBranchDelegate
type stubPGBranch struct {
	getByNameResult *domain.Branch
	getByNameErr    error
	getByNameCalls  int

	advanceResult *domain.Branch
	advanceErr    error
	advanceCalls  int

	createErr   error
	createCalls int

	listResult *postgres.ListBranchesPage
	listErr    error
	listCalls  int

	deleteErr   error
	deleteCalls int
}

func (s *stubPGBranch) Create(_ context.Context, _ *domain.Branch) error {
	s.createCalls++
	return s.createErr
}

func (s *stubPGBranch) GetByName(_ context.Context, _, _ string) (*domain.Branch, error) {
	s.getByNameCalls++
	return s.getByNameResult, s.getByNameErr
}

func (s *stubPGBranch) List(
	_ context.Context,
	_ string,
	_ int,
	_ string,
) (*postgres.ListBranchesPage, error) {
	s.listCalls++
	return s.listResult, s.listErr
}

func (s *stubPGBranch) Advance(_ context.Context, _, _, _, _ string) (*domain.Branch, error) {
	s.advanceCalls++
	return s.advanceResult, s.advanceErr
}

func (s *stubPGBranch) Delete(_ context.Context, _, _ string) error {
	s.deleteCalls++
	return s.deleteErr
}

// stubRedisHead implements interfaces.BranchHeadStore
type stubRedisHead struct {
	getHeadResult string
	getHeadErr    error
	getHeadCalls  int

	setHeadErr   error
	setHeadCalls int
	setHeadArgs  []setHeadCall
}

type setHeadCall struct {
	repoID, name, commitID string
	version                int64
}

func (s *stubRedisHead) GetHead(_ context.Context, _, _ string) (string, error) {
	s.getHeadCalls++
	return s.getHeadResult, s.getHeadErr
}

func (s *stubRedisHead) SetHead(
	_ context.Context,
	repoID, name, commitID string,
	version int64,
) error {
	s.setHeadCalls++
	s.setHeadArgs = append(s.setHeadArgs, setHeadCall{repoID, name, commitID, version})
	return s.setHeadErr
}

func makeBranch(repoID, name, commitID string) *domain.Branch {
	return &domain.Branch{
		Name:      name,
		RepoID:    repoID,
		CommitID:  commitID,
		CreatedAt: time.Now(),
	}
}

// GetHead - cache hit

func TestBranchRouter_GetHead_CacheHit_ReturnsCachedValue(t *testing.T) {
	redis := &stubRedisHead{getHeadResult: "commit-from-redis"}
	pg := &stubPGBranch{}
	r := NewBranchRouter(pg, redis)

	commitID, err := r.GetHead(context.Background(), "repo-1", "main")

	require.NoError(t, err)
	assert.Equal(t, "commit-from-redis", commitID)
	assert.Equal(t, 0, pg.getByNameCalls, "postgres must not be called on cache hit")
	assert.Equal(t, 0, redis.setHeadCalls, "SetHead must not be called on cache hit")
}

// GetHead - cache miss

func TestBranchRouter_GetHead_CacheMiss_FallsBackToPostgres(t *testing.T) {
	redis := &stubRedisHead{getHeadErr: interfaces.ErrCacheMiss}
	pg := &stubPGBranch{getByNameResult: makeBranch("repo-1", "main", "commit-from-pg")}
	r := NewBranchRouter(pg, redis)

	commitID, err := r.GetHead(context.Background(), "repo-1", "main")

	require.NoError(t, err)
	assert.Equal(t, "commit-from-pg", commitID)
	assert.Equal(t, 1, pg.getByNameCalls)
}

func TestBranchRouter_GetHead_CacheMiss_PopulatesRedis(t *testing.T) {
	redis := &stubRedisHead{getHeadErr: interfaces.ErrCacheMiss}
	pg := &stubPGBranch{getByNameResult: makeBranch("repo-1", "main", "commit-abc")}
	r := NewBranchRouter(pg, redis)

	_, err := r.GetHead(context.Background(), "repo-1", "main")

	require.NoError(t, err)
	assert.Equal(t, 1, redis.setHeadCalls, "SetHead must be called to warm cache after miss")
	assert.Equal(t, "commit-abc", redis.setHeadArgs[0].commitID)
}

func TestBranchRouter_GetHead_CacheMiss_SetHeadFails_StillReturnsPGValue(t *testing.T) {
	redis := &stubRedisHead{
		getHeadErr: interfaces.ErrCacheMiss,
		setHeadErr: errors.New("redis down"),
	}
	pg := &stubPGBranch{getByNameResult: makeBranch("repo-1", "main", "commit-abc")}
	r := NewBranchRouter(pg, redis)

	commitID, err := r.GetHead(context.Background(), "repo-1", "main")

	require.NoError(t, err, "SetHead failure must be non-fatal")
	assert.Equal(t, "commit-abc", commitID)
}

func TestBranchRouter_GetHead_CacheMiss_PostgresFails_ReturnsError(t *testing.T) {
	pgErr := errors.New("postgres connection lost")
	redis := &stubRedisHead{getHeadErr: interfaces.ErrCacheMiss}
	pg := &stubPGBranch{getByNameErr: pgErr}
	r := NewBranchRouter(pg, redis)

	_, err := r.GetHead(context.Background(), "repo-1", "main")

	require.Error(t, err)
	assert.ErrorIs(t, err, pgErr)
	assert.Equal(t, 0, redis.setHeadCalls, "SetHead must not be called when postgres fails")
}

// GetHead - redis error (not a miss)

func TestBranchRouter_GetHead_RedisError_FallsBackToPostgres(t *testing.T) {
	redis := &stubRedisHead{getHeadErr: errors.New("connection refused")} // not ErrCacheMiss
	pg := &stubPGBranch{getByNameResult: makeBranch("repo-1", "main", "commit-pg")}
	r := NewBranchRouter(pg, redis)

	commitID, err := r.GetHead(context.Background(), "repo-1", "main")

	require.NoError(t, err)
	assert.Equal(t, "commit-pg", commitID)
	assert.Equal(t, 1, pg.getByNameCalls)
}

func TestBranchRouter_GetHead_RedisError_StillTriesToSetHead(t *testing.T) {
	redis := &stubRedisHead{getHeadErr: errors.New("timeout")}
	pg := &stubPGBranch{getByNameResult: makeBranch("repo-1", "main", "commit-pg")}
	r := NewBranchRouter(pg, redis)

	_, _ = r.GetHead(context.Background(), "repo-1", "main")

	assert.Equal(t, 1, redis.setHeadCalls)
}

// SetHead - delegates directly to redis

func TestBranchRouter_SetHead_DelegatesToRedis(t *testing.T) {
	redis := &stubRedisHead{}
	r := NewBranchRouter(&stubPGBranch{}, redis)

	err := r.SetHead(context.Background(), "repo-1", "main", "commit-new", 1000)

	require.NoError(t, err)
	require.Len(t, redis.setHeadArgs, 1)
	arg := redis.setHeadArgs[0]
	assert.Equal(t, "repo-1", arg.repoID)
	assert.Equal(t, "main", arg.name)
	assert.Equal(t, "commit-new", arg.commitID)
	assert.Equal(t, int64(1000), arg.version)
}

func TestBranchRouter_SetHead_RedisError_IsReturned(t *testing.T) {
	redis := &stubRedisHead{setHeadErr: errors.New("redis write failed")}
	r := NewBranchRouter(&stubPGBranch{}, redis)

	err := r.SetHead(context.Background(), "repo-1", "main", "commit-new", 1000)

	require.Error(t, err)
}

// Advance - postgres + redis sync

func TestBranchRouter_Advance_PostgresSucceeds_ReturnsBranch(t *testing.T) {
	want := makeBranch("repo-1", "main", "commit-new")
	pg := &stubPGBranch{advanceResult: want}
	r := NewBranchRouter(pg, &stubRedisHead{})

	got, err := r.Advance(context.Background(), "repo-1", "main", "commit-new", "commit-old")

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestBranchRouter_Advance_PostgresSucceeds_SyncsRedis(t *testing.T) {
	pg := &stubPGBranch{advanceResult: makeBranch("repo-1", "main", "commit-new")}
	redis := &stubRedisHead{}
	r := NewBranchRouter(pg, redis)

	_, err := r.Advance(context.Background(), "repo-1", "main", "commit-new", "commit-old")

	require.NoError(t, err)
	assert.Equal(t, 1, redis.setHeadCalls, "Redis must be synced after successful Advance")
	assert.Equal(t, "commit-new", redis.setHeadArgs[0].commitID)
}

func TestBranchRouter_Advance_RedisSetHeadFails_BranchStillReturned(t *testing.T) {
	pg := &stubPGBranch{advanceResult: makeBranch("repo-1", "main", "commit-new")}
	redis := &stubRedisHead{setHeadErr: errors.New("redis unavailable")}
	r := NewBranchRouter(pg, redis)

	got, err := r.Advance(context.Background(), "repo-1", "main", "commit-new", "commit-old")

	require.NoError(t, err, "Redis sync failure must be non-fatal")
	assert.NotNil(t, got)
}

func TestBranchRouter_Advance_PostgresFails_ReturnsError(t *testing.T) {
	pgErr := errors.New("branch conflict")
	pg := &stubPGBranch{advanceErr: pgErr}
	redis := &stubRedisHead{}
	r := NewBranchRouter(pg, redis)

	_, err := r.Advance(context.Background(), "repo-1", "main", "commit-new", "commit-old")

	require.Error(t, err)
	assert.ErrorIs(t, err, pgErr)
	assert.Equal(t, 0, redis.setHeadCalls, "Redis must not be touched when postgres fails")
}

// Delegate methods - straight pass-through to postgres

func TestBranchRouter_Create_DelegatesToPostgres(t *testing.T) {
	pg := &stubPGBranch{}
	r := NewBranchRouter(pg, &stubRedisHead{})

	err := r.Create(context.Background(), makeBranch("repo-1", "feat", "c1"))

	require.NoError(t, err)
	assert.Equal(t, 1, pg.createCalls)
}

func TestBranchRouter_GetByName_DelegatesToPostgres(t *testing.T) {
	want := makeBranch("repo-1", "main", "c1")
	pg := &stubPGBranch{getByNameResult: want}
	r := NewBranchRouter(pg, &stubRedisHead{})

	got, err := r.GetByName(context.Background(), "repo-1", "main")

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, 1, pg.getByNameCalls)
}

func TestBranchRouter_List_DelegatesToPostgres(t *testing.T) {
	want := &postgres.ListBranchesPage{Branches: []*domain.Branch{makeBranch("r1", "main", "c1")}}
	pg := &stubPGBranch{listResult: want}
	r := NewBranchRouter(pg, &stubRedisHead{})

	got, err := r.List(context.Background(), "r1", 10, "")

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, 1, pg.listCalls)
}

func TestBranchRouter_Delete_DelegatesToPostgres(t *testing.T) {
	pg := &stubPGBranch{}
	r := NewBranchRouter(pg, &stubRedisHead{})

	err := r.Delete(context.Background(), "repo-1", "feat")

	require.NoError(t, err)
	assert.Equal(t, 1, pg.deleteCalls)
}

func TestBranchRouter_DelegateMethods_NeverTouchRedis(t *testing.T) {
	redis := &stubRedisHead{}
	pg := &stubPGBranch{
		listResult:      &postgres.ListBranchesPage{},
		getByNameResult: makeBranch("r", "m", "c"),
	}
	r := NewBranchRouter(pg, redis)

	_ = r.Create(context.Background(), makeBranch("r", "feat", "c"))
	_, _ = r.GetByName(context.Background(), "r", "m")
	_, _ = r.List(context.Background(), "r", 10, "")
	_ = r.Delete(context.Background(), "r", "feat")

	assert.Equal(t, 0, redis.getHeadCalls)
	assert.Equal(t, 0, redis.setHeadCalls)
}

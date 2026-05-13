package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/storage/postgres"
)

// mock

type mockRepoStore struct {
	createFn  func(ctx context.Context, repo *domain.Repo) error
	getByIDFn func(ctx context.Context, id string) (*domain.Repo, error)
	listFn    func(ctx context.Context, limit int, cursor string) (*postgres.ListReposPage, error)
}

func (m *mockRepoStore) Create(ctx context.Context, repo *domain.Repo) error {
	return m.createFn(ctx, repo)
}

func (m *mockRepoStore) GetByID(ctx context.Context, id string) (*domain.Repo, error) {
	return m.getByIDFn(ctx, id)
}

func (m *mockRepoStore) List(
	ctx context.Context,
	limit int,
	cursor string,
) (*postgres.ListReposPage, error) {
	return m.listFn(ctx, limit, cursor)
}

// CreateRepo

func TestCreateRepo_ValidInput_CallsStoreAndReturnsRepo(t *testing.T) {
	var stored *domain.Repo

	svc := NewRepoService(&mockRepoStore{
		createFn: func(_ context.Context, r *domain.Repo) error {
			stored = r
			return nil
		},
	})

	repo, err := svc.CreateRepo(context.Background(), CreateRepoInput{
		Name:          "my-doc",
		DefaultBranch: "main",
	})

	require.NoError(t, err)
	require.NotNil(t, repo)
	assert.Equal(t, "my-doc", repo.Name)
	assert.Equal(t, "main", repo.DefaultBranch)
	assert.NotEmpty(t, repo.ID)
	require.NotNil(t, stored, "store.Create was never called")
	assert.Equal(t, repo.ID, stored.ID)
}

func TestCreateRepo_StoreError_PropagatesWrappedError(t *testing.T) {
	storeErr := errors.New("db exploded")

	svc := NewRepoService(&mockRepoStore{
		createFn: func(_ context.Context, _ *domain.Repo) error {
			return storeErr
		},
	})

	_, err := svc.CreateRepo(context.Background(), CreateRepoInput{
		Name:          "my-doc",
		DefaultBranch: "main",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, storeErr)
}

// GetRepo

func TestGetRepo_RepoExists_ReturnsRepoFromStore(t *testing.T) {
	want := &domain.Repo{ID: "repo_abc", Name: "my-doc", DefaultBranch: "main"}

	svc := NewRepoService(&mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			assert.Equal(t, want.ID, id)
			return want, nil
		},
	})

	got, err := svc.GetRepo(context.Background(), want.ID)

	require.NoError(t, err)
	assert.Equal(t, want.ID, got.ID)
}

func TestGetRepo_RepoNotFound_ReturnsDomainError(t *testing.T) {
	svc := NewRepoService(&mockRepoStore{
		getByIDFn: func(_ context.Context, _ string) (*domain.Repo, error) {
			return nil, domain.ErrRepoNotFound
		},
	})

	_, err := svc.GetRepo(context.Background(), "repo_missing")

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrRepoNotFound)
}

// ListRepos

func TestListRepos_NoCursor_PassesEmptyCursorToStore(t *testing.T) {
	svc := NewRepoService(&mockRepoStore{
		listFn: func(_ context.Context, _ int, cursor string) (*postgres.ListReposPage, error) {
			assert.Equal(t, "", cursor)
			return &postgres.ListReposPage{Repos: []*domain.Repo{{ID: "repo_1"}}}, nil
		},
	})

	result, err := svc.ListRepos(context.Background(), ListReposInput{Limit: 20})

	require.NoError(t, err)
	assert.Len(t, result.Repos, 1)
}

func TestListRepos_WithCursor_PassesCursorToStore(t *testing.T) {
	svc := NewRepoService(&mockRepoStore{
		listFn: func(_ context.Context, _ int, cursor string) (*postgres.ListReposPage, error) {
			assert.Equal(t, "cursor-abc", cursor)
			return &postgres.ListReposPage{}, nil
		},
	})

	_, err := svc.ListRepos(context.Background(), ListReposInput{Limit: 20, Cursor: "cursor-abc"})
	require.NoError(t, err)
}

func TestListRepos_ZeroLimit_DefaultsTo20(t *testing.T) {
	svc := NewRepoService(&mockRepoStore{
		listFn: func(_ context.Context, limit int, _ string) (*postgres.ListReposPage, error) {
			assert.Equal(t, 20, limit)
			return &postgres.ListReposPage{}, nil
		},
	})

	_, err := svc.ListRepos(context.Background(), ListReposInput{Limit: 0})
	require.NoError(t, err)
}

func TestListRepos_LimitOver100_ClampsTo100(t *testing.T) {
	svc := NewRepoService(&mockRepoStore{
		listFn: func(_ context.Context, limit int, _ string) (*postgres.ListReposPage, error) {
			assert.Equal(t, 100, limit)
			return &postgres.ListReposPage{}, nil
		},
	})

	_, err := svc.ListRepos(context.Background(), ListReposInput{Limit: 500})
	require.NoError(t, err)
}

func TestListRepos_StoreReturnsNextCursor_PropagatedToResult(t *testing.T) {
	svc := NewRepoService(&mockRepoStore{
		listFn: func(_ context.Context, _ int, _ string) (*postgres.ListReposPage, error) {
			return &postgres.ListReposPage{
				Repos:      []*domain.Repo{{ID: "repo_1"}},
				NextCursor: "next-cursor",
			}, nil
		},
	})

	result, err := svc.ListRepos(context.Background(), ListReposInput{Limit: 1})

	require.NoError(t, err)
	assert.Equal(t, "next-cursor", result.NextCursor)
}

func TestListRepos_StoreError_PropagatesWrappedError(t *testing.T) {
	storeErr := errors.New("db timeout")

	svc := NewRepoService(&mockRepoStore{
		listFn: func(_ context.Context, _ int, _ string) (*postgres.ListReposPage, error) {
			return nil, storeErr
		},
	})

	_, err := svc.ListRepos(context.Background(), ListReposInput{Limit: 20})

	require.Error(t, err)
	assert.ErrorIs(t, err, storeErr)
}

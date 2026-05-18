//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/testhelper"
)

func spinUpPostgres(t *testing.T) *RepoStore {
	t.Helper()
	pool, cleanup := testhelper.SetupPostgres(t)
	t.Cleanup(cleanup)
	return NewRepoStore(pool)
}

func uniqueRepoName() string { return testhelper.UniqueName("test") }

// Insert

func TestRepoStore_Insert_NewRepo_RowExistsInDB(t *testing.T) {
	store := spinUpPostgres(t)
	ctx := context.Background()

	r := testhelper.NewTestRepo(uniqueRepoName())
	require.NoError(t, store.Create(ctx, r))

	got, err := store.GetByID(ctx, r.ID)
	require.NoError(t, err)
	assert.Equal(t, r.ID, got.ID)
	assert.Equal(t, r.Name, got.Name)
	assert.Equal(t, r.DefaultBranch, got.DefaultBranch)
}

func TestRepoStore_Insert_DuplicateID_Fails(t *testing.T) {
	store := spinUpPostgres(t)
	ctx := context.Background()

	r := testhelper.NewTestRepo(uniqueRepoName())
	require.NoError(t, store.Create(ctx, r))

	// try to insert another repo with the same ID
	r2 := &domain.Repo{
		ID:            r.ID, // same ID
		Name:          uniqueRepoName(),
		DefaultBranch: "main",
		CreatedAt:     time.Now().UTC().Truncate(time.Microsecond),
	}

	err := store.Create(ctx, r2)
	require.Error(t, err, "should fail on duplicate ID")
	// the error should be a constraint violation,
	// not a typed domain error
	assert.NotErrorIs(t, err, domain.ErrRepoNotFound)
}

// GetByID

func TestRepoStore_GetByID_Exists_ReturnsCorrectStruct(t *testing.T) {
	store := spinUpPostgres(t)
	ctx := context.Background()

	r := testhelper.NewTestRepo(uniqueRepoName())
	require.NoError(t, store.Create(ctx, r))

	got, err := store.GetByID(ctx, r.ID)
	require.NoError(t, err)
	assert.Equal(t, r.ID, got.ID)
	assert.Equal(t, r.Name, got.Name)
	assert.Equal(t, r.DefaultBranch, got.DefaultBranch)
}

func TestRepoStore_GetByID_NotFound_ReturnsTypedError(t *testing.T) {
	store := spinUpPostgres(t)
	ctx := context.Background()

	_, err := store.GetByID(ctx, "repo_does-not-exist")
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrRepoNotFound)
}

// List

func TestRepoStore_List_EmptyTable_ReturnsEmptySlice(t *testing.T) {
	store := spinUpPostgres(t)
	ctx := context.Background()

	page, err := store.List(ctx, 20, "")
	require.NoError(t, err)
	assert.Empty(t, page.Repos)
	assert.Empty(t, page.NextCursor)
}

func TestRepoStore_List_MultipleRepos_ReturnsDescendingOrder(t *testing.T) {
	store := spinUpPostgres(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		r := testhelper.NewTestRepo(uniqueRepoName())
		r.CreatedAt = time.Now().UTC().Add(time.Duration(i) * time.Millisecond)
		require.NoError(t, store.Create(ctx, r))
	}

	page, err := store.List(ctx, 20, "")
	require.NoError(t, err)
	require.Len(t, page.Repos, 3)

	for i := 1; i < len(page.Repos); i++ {
		assert.False(t,
			page.Repos[i].CreatedAt.After(page.Repos[i-1].CreatedAt),
			"repos not in descending order at index %d", i,
		)
	}
}

func TestRepoStore_List_CursorPagination_NoDuplicatesNoGaps(t *testing.T) {
	store := spinUpPostgres(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		r := testhelper.NewTestRepo(uniqueRepoName())
		r.CreatedAt = time.Now().UTC().Add(time.Duration(i) * time.Millisecond)
		require.NoError(t, store.Create(ctx, r))
	}

	page1, err := store.List(ctx, 2, "")
	require.NoError(t, err)
	require.Len(t, page1.Repos, 2)
	require.NotEmpty(t, page1.NextCursor, "expected NextCursor after page 1")

	page2, err := store.List(ctx, 2, page1.NextCursor)
	require.NoError(t, err)
	require.Len(t, page2.Repos, 2)
	require.NotEmpty(t, page2.NextCursor, "expected NextCursor after page 2")

	page3, err := store.List(ctx, 2, page2.NextCursor)
	require.NoError(t, err)
	require.Len(t, page3.Repos, 1)
	assert.Empty(t, page3.NextCursor, "expected empty NextCursor on last page")

	// no duplicates across all pages
	seen := make(map[string]int)
	for _, p := range [][]*domain.Repo{page1.Repos, page2.Repos, page3.Repos} {
		for _, r := range p {
			seen[r.ID]++
		}
	}
	for id, count := range seen {
		assert.Equal(t, 1, count, "repo %s appeared %d times across pages", id, count)
	}
	assert.Len(t, seen, 5, "expected 5 distinct repos across all pages")
}

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

func setupBranchStoreTest(t *testing.T) (*BranchStore, *RepoStore, *CommitStore, func()) {
	t.Helper()
	pool, cleanup := testhelper.SetupPostgres(t)

	branchStore := NewBranchStore(pool)
	repoStore := NewRepoStore(pool)
	commitStore := NewCommitStore(pool)

	return branchStore, repoStore, commitStore, cleanup
}

func createTestRepo(t *testing.T, repoStore *RepoStore) *domain.Repo {
	t.Helper()
	repo := testhelper.NewTestRepo(testhelper.UniqueName("test-repo"))
	err := repoStore.Create(context.Background(), repo)
	require.NoError(t, err)
	return repo
}

func createTestCommit(
	t *testing.T,
	commitStore *CommitStore,
	repoID string,
	parentIDs []string,
) *domain.Commit {
	t.Helper()
	commit := testhelper.NewTestCommit(repoID, parentIDs)
	_, err := commitStore.Create(context.Background(), commit, parentIDs)
	require.NoError(t, err)
	return commit
}

// Insert

func TestBranchStore_Create_ValidBranch_Succeeds(t *testing.T) {
	store, repoStore, commitStore, cleanup := setupBranchStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	commit := createTestCommit(t, commitStore, repo.ID, []string{})

	err := store.Create(context.Background(), testhelper.NewTestBranch(repo.ID, commit.ID))
	assert.NoError(t, err)
}

func TestBranchStore_Create_DuplicateName_ReturnsBranchAlreadyExists(t *testing.T) {
	store, repoStore, commitStore, cleanup := setupBranchStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	commit := createTestCommit(t, commitStore, repo.ID, []string{})

	branch := testhelper.NewTestBranch(repo.ID, commit.ID)
	err := store.Create(context.Background(), branch)
	require.NoError(t, err)

	duplicate := &domain.Branch{
		Name:      branch.Name, // same name on purpose
		RepoID:    repo.ID,
		CommitID:  commit.ID,
		CreatedAt: time.Now().UTC(),
	}
	err = store.Create(context.Background(), duplicate)
	assert.ErrorIs(t, err, domain.ErrBranchAlreadyExists)
}

func TestBranchStore_Create_InvalidRepoID_ReturnsRepoNotFound(t *testing.T) {
	store, repoStore, commitStore, cleanup := setupBranchStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	commit := createTestCommit(t, commitStore, repo.ID, []string{})

	err := store.Create(
		context.Background(),
		testhelper.NewTestBranch("repo_nonexistent", commit.ID),
	)
	assert.ErrorIs(t, err, domain.ErrRepoNotFound)
}

func TestBranchStore_Create_InvalidCommitID_ReturnsCommitNotFound(t *testing.T) {
	store, repoStore, _, cleanup := setupBranchStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)

	err := store.Create(
		context.Background(),
		testhelper.NewTestBranch(repo.ID, "commit_nonexistent"),
	)
	assert.ErrorIs(t, err, domain.ErrCommitNotFound)
}

// Get

func TestBranchStore_GetByName_Exists_ReturnsCorrectBranch(t *testing.T) {
	store, repoStore, commitStore, cleanup := setupBranchStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	commit := createTestCommit(t, commitStore, repo.ID, []string{})

	branch := testhelper.NewTestBranch(repo.ID, commit.ID)
	err := store.Create(context.Background(), branch)
	require.NoError(t, err)

	got, err := store.GetByName(context.Background(), repo.ID, branch.Name)
	require.NoError(t, err)
	assert.Equal(t, branch.Name, got.Name)
	assert.Equal(t, repo.ID, got.RepoID)
	assert.Equal(t, commit.ID, got.CommitID)
	assert.False(t, got.CreatedAt.IsZero())
}

func TestBranchStore_GetByName_NotFound_ReturnsBranchNotFound(t *testing.T) {
	store, repoStore, _, cleanup := setupBranchStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)

	_, err := store.GetByName(context.Background(), repo.ID, "nonexistent")
	assert.ErrorIs(t, err, domain.ErrBranchNotFound)
}

// List

func TestBranchStore_List_ReturnsAllBranchesInRepo(t *testing.T) {
	store, repoStore, commitStore, cleanup := setupBranchStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	commit := createTestCommit(t, commitStore, repo.ID, []string{})

	for i := 0; i < 3; i++ {
		err := store.Create(context.Background(), testhelper.NewTestBranch(repo.ID, commit.ID))
		require.NoError(t, err)
	}

	page, err := store.List(context.Background(), repo.ID, 10, "")
	require.NoError(t, err)
	assert.Len(t, page.Branches, 3)
}

func TestBranchStore_List_Pagination_WorksCorrectly(t *testing.T) {
	store, repoStore, commitStore, cleanup := setupBranchStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	commit := createTestCommit(t, commitStore, repo.ID, []string{})

	for i := 0; i < 5; i++ {
		err := store.Create(context.Background(), testhelper.NewTestBranch(repo.ID, commit.ID))
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}

	page1, err := store.List(context.Background(), repo.ID, 2, "")
	require.NoError(t, err)
	assert.Len(t, page1.Branches, 2)
	assert.NotEmpty(t, page1.NextCursor)

	page2, err := store.List(context.Background(), repo.ID, 2, page1.NextCursor)
	require.NoError(t, err)
	assert.Len(t, page2.Branches, 2)

	seen := make(map[string]bool)
	for _, b := range append(page1.Branches, page2.Branches...) {
		assert.False(t, seen[b.Name], "duplicate branch %s across pages", b.Name)
		seen[b.Name] = true
	}
}

// Advance

func TestBranchStore_Advance_ValidInput_UpdatesCommitID(t *testing.T) {
	store, repoStore, commitStore, cleanup := setupBranchStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	commit1 := createTestCommit(t, commitStore, repo.ID, []string{})
	commit2 := createTestCommit(t, commitStore, repo.ID, []string{commit1.ID})

	branch := testhelper.NewTestBranch(repo.ID, commit1.ID)
	err := store.Create(context.Background(), branch)
	require.NoError(t, err)

	updated, err := store.Advance(
		context.Background(),
		repo.ID,
		branch.Name,
		commit2.ID,
		commit1.ID,
	)
	require.NoError(t, err)
	assert.Equal(t, commit2.ID, updated.CommitID)

	fetched, err := store.GetByName(context.Background(), repo.ID, branch.Name)
	require.NoError(t, err)
	assert.Equal(t, commit2.ID, fetched.CommitID)
}

func TestBranchStore_Advance_StaleExpectedCommitID_ReturnsBranchConflict(t *testing.T) {
	store, repoStore, commitStore, cleanup := setupBranchStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	commit1 := createTestCommit(t, commitStore, repo.ID, []string{})
	commit2 := createTestCommit(t, commitStore, repo.ID, []string{commit1.ID})
	commit3 := createTestCommit(t, commitStore, repo.ID, []string{commit2.ID})

	branch := testhelper.NewTestBranch(repo.ID, commit2.ID)
	err := store.Create(context.Background(), branch)
	require.NoError(t, err)

	// advance with stale expected (commit1 instead of commit2)
	_, err = store.Advance(context.Background(), repo.ID, branch.Name, commit3.ID, commit1.ID)
	assert.ErrorIs(t, err, domain.ErrBranchConflict)

	var conflictErr *BranchConflictError
	assert.ErrorAs(t, err, &conflictErr)
	assert.Equal(t, commit2.ID, conflictErr.CurrentHead)
}

func TestBranchStore_Advance_BranchNotFound_ReturnsBranchNotFound(t *testing.T) {
	store, repoStore, commitStore, cleanup := setupBranchStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	commit := createTestCommit(t, commitStore, repo.ID, []string{})

	_, err := store.Advance(context.Background(), repo.ID, "nonexistent", commit.ID, "commit_old")
	assert.ErrorIs(t, err, domain.ErrBranchNotFound)
}

// Delete

func TestBranchStore_Delete_Exists_RemovesBranch(t *testing.T) {
	store, repoStore, commitStore, cleanup := setupBranchStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	commit := createTestCommit(t, commitStore, repo.ID, []string{})

	branch := testhelper.NewTestBranch(repo.ID, commit.ID)
	err := store.Create(context.Background(), branch)
	require.NoError(t, err)

	err = store.Delete(context.Background(), repo.ID, branch.Name)
	assert.NoError(t, err)

	_, err = store.GetByName(context.Background(), repo.ID, branch.Name)
	assert.ErrorIs(t, err, domain.ErrBranchNotFound)
}

func TestBranchStore_Delete_NotFound_ReturnsBranchNotFound(t *testing.T) {
	store, repoStore, _, cleanup := setupBranchStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)

	err := store.Delete(context.Background(), repo.ID, "nonexistent")
	assert.ErrorIs(t, err, domain.ErrBranchNotFound)
}

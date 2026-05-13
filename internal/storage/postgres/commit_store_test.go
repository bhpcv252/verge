//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/testhelper"
)

func setupCommitStoreTest(t *testing.T) (*CommitStore, *RepoStore, func()) {
	t.Helper()
	pool, cleanup := testhelper.SetupPostgres(t)

	commitStore := NewCommitStore(pool)
	repoStore := NewRepoStore(pool)

	return commitStore, repoStore, cleanup
}

// Insert

func TestCommitStore_Create_RootCommit_Succeeds(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)

	commit := &domain.Commit{
		ID:     "commit_" + uuid.New().String(),
		RepoID: repo.ID,
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message:   "Initial commit",
		Author:    "test@example.com",
		Timestamp: time.Now().UTC(),
	}

	created, err := store.Create(context.Background(), commit, []string{})
	require.NoError(t, err)
	assert.Equal(t, commit.ID, created.ID)
	assert.Equal(t, commit.Message, created.Message)
}

func TestCommitStore_Create_RegularCommit_Succeeds(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	parent := createTestCommit(t, store, repo.ID, []string{})

	commit := &domain.Commit{
		ID:     "commit_" + uuid.New().String(),
		RepoID: repo.ID,
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message:   "Add feature",
		Author:    "test@example.com",
		Timestamp: time.Now().UTC(),
	}

	created, err := store.Create(context.Background(), commit, []string{parent.ID})
	require.NoError(t, err)
	assert.Equal(t, commit.ID, created.ID)

	// verify parent relationship
	parents, err := store.GetParents(context.Background(), repo.ID, created.ID)
	require.NoError(t, err)
	require.Len(t, parents, 1)
	assert.Equal(t, parent.ID, parents[0].ID)
}

func TestCommitStore_Create_WithIdempotencyKey_Succeeds(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)

	commit := &domain.Commit{
		ID:     "commit_" + uuid.New().String(),
		RepoID: repo.ID,
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message:        "Test commit",
		Author:         "test@example.com",
		Timestamp:      time.Now().UTC(),
		IdempotencyKey: "key_" + uuid.New().String(),
	}

	created, err := store.Create(context.Background(), commit, []string{})
	require.NoError(t, err)
	assert.Equal(t, commit.IdempotencyKey, created.IdempotencyKey)
}

func TestCommitStore_Create_InvalidRepoID_FKViolation(t *testing.T) {
	store, _, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	commit := &domain.Commit{
		ID:     "commit_" + uuid.New().String(),
		RepoID: "repo_nonexistent", // FK violation
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message:   "Test commit",
		Author:    "test@example.com",
		Timestamp: time.Now().UTC(),
	}

	_, err := store.Create(context.Background(), commit, []string{})
	require.Error(t, err, "should fail on invalid repo_id FK")
	assert.Contains(t, err.Error(), "violates foreign key constraint")
}

func TestCommitStore_Create_InvalidParentID_FKViolation(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)

	commit := &domain.Commit{
		ID:     "commit_" + uuid.New().String(),
		RepoID: repo.ID,
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message:   "Test commit",
		Author:    "test@example.com",
		Timestamp: time.Now().UTC(),
	}

	// try to create with non-existent parent
	_, err := store.Create(context.Background(), commit, []string{"commit_nonexistent"})
	require.Error(t, err, "should fail on invalid parent_id FK")
	assert.Contains(t, err.Error(), "violates foreign key constraint")
}

// Get

func TestCommitStore_GetByIdempotencyKey_Exists_ReturnsCommit(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	idempotencyKey := "key_" + uuid.New().String()

	commit := &domain.Commit{
		ID:     "commit_" + uuid.New().String(),
		RepoID: repo.ID,
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message:        "Test commit",
		Author:         "test@example.com",
		Timestamp:      time.Now().UTC(),
		IdempotencyKey: idempotencyKey,
	}

	_, err := store.Create(context.Background(), commit, []string{})
	require.NoError(t, err)

	// retrieve by idempotency key
	found, err := store.GetByIdempotencyKey(context.Background(), repo.ID, idempotencyKey)
	require.NoError(t, err)
	assert.Equal(t, commit.ID, found.ID)
	assert.Equal(t, idempotencyKey, found.IdempotencyKey)
}

func TestCommitStore_GetByIdempotencyKey_NotFound_ReturnsError(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)

	_, err := store.GetByIdempotencyKey(context.Background(), repo.ID, "key_nonexistent")
	assert.ErrorIs(t, err, domain.ErrCommitNotFound)
}

func TestCommitStore_GetByID_Exists_ReturnsCommit(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	commit := createTestCommit(t, store, repo.ID, []string{})

	found, err := store.GetByID(context.Background(), repo.ID, commit.ID)
	require.NoError(t, err)
	assert.Equal(t, commit.ID, found.ID)
	assert.Equal(t, commit.Message, found.Message)
	assert.Equal(t, commit.Author, found.Author)
}

func TestCommitStore_GetByID_NotFound_ReturnsError(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)

	_, err := store.GetByID(context.Background(), repo.ID, "commit_nonexistent")
	assert.ErrorIs(t, err, domain.ErrCommitNotFound)
}

// Validate

func TestCommitStore_ValidateParentsExist_AllExist_Succeeds(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	parent1 := createTestCommit(t, store, repo.ID, []string{})
	parent2 := createTestCommit(t, store, repo.ID, []string{})

	err := store.ValidateParentsExist(
		context.Background(),
		repo.ID,
		[]string{parent1.ID, parent2.ID},
	)
	assert.NoError(t, err)
}

func TestCommitStore_ValidateParentsExist_OneInvalid_ReturnsError(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	parent1 := createTestCommit(t, store, repo.ID, []string{})

	err := store.ValidateParentsExist(
		context.Background(),
		repo.ID,
		[]string{parent1.ID, "commit_nonexistent"},
	)
	assert.ErrorIs(t, err, domain.ErrInvalidParent)
}

// List

func TestCommitStore_List_ReturnsAllCommitsInRepo(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)

	// create 3 commits
	for i := 0; i < 3; i++ {
		createTestCommit(t, store, repo.ID, []string{})
		time.Sleep(10 * time.Millisecond)
	}

	page, err := store.List(context.Background(), ListCommitsFilter{
		RepoID: repo.ID,
		Limit:  10,
	})
	require.NoError(t, err)
	assert.Len(t, page.Commits, 3)
}

func TestCommitStore_List_FlatTraversal_ReverseChronological(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)

	// create commits with distinct timestamps
	now := time.Now().UTC()
	commits := make([]*domain.Commit, 3)
	for i := 0; i < 3; i++ {
		commit := &domain.Commit{
			ID:     "commit_" + uuid.New().String(),
			RepoID: repo.ID,
			DataPointer: domain.DataPointer{
				Type:     "db",
				Location: "test/fixture",
			},
			Message:   "Commit " + string(rune('A'+i)),
			Author:    "test@example.com",
			Timestamp: now.Add(time.Duration(i) * time.Second),
		}
		created, err := store.Create(context.Background(), commit, []string{})
		require.NoError(t, err)
		commits[i] = created
	}

	// list without branch filter (flat traversal)
	page, err := store.List(context.Background(), ListCommitsFilter{
		RepoID: repo.ID,
		Limit:  10,
	})
	require.NoError(t, err)
	assert.Len(t, page.Commits, 3)

	// verify reverse chronological order (newest first)
	for i := 0; i < len(page.Commits)-1; i++ {
		assert.False(t,
			page.Commits[i].Timestamp.Before(page.Commits[i+1].Timestamp),
			"commits not in reverse chronological order at index %d: %v should be >= %v",
			i, page.Commits[i].Timestamp, page.Commits[i+1].Timestamp,
		)
	}
}

func TestCommitStore_List_Pagination_WorksCorrectly(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)

	// create 5 commits
	for i := 0; i < 5; i++ {
		createTestCommit(t, store, repo.ID, []string{})
		time.Sleep(10 * time.Millisecond)
	}

	// page 1: limit=2
	page1, err := store.List(context.Background(), ListCommitsFilter{
		RepoID: repo.ID,
		Limit:  2,
	})
	require.NoError(t, err)
	assert.Len(t, page1.Commits, 2)
	assert.NotEmpty(t, page1.NextCursor)

	// page 2: use cursor
	page2, err := store.List(context.Background(), ListCommitsFilter{
		RepoID: repo.ID,
		Limit:  2,
		Cursor: page1.NextCursor,
	})
	require.NoError(t, err)
	assert.Len(t, page2.Commits, 2)

	// verify no duplicates
	seen := make(map[string]bool)
	for _, c := range append(page1.Commits, page2.Commits...) {
		assert.False(t, seen[c.ID], "duplicate commit %s across pages", c.ID)
		seen[c.ID] = true
	}
}

func TestCommitStore_List_FilterByAuthor_ReturnsOnlyMatchingCommits(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)

	// create commits by different authors
	commit1 := &domain.Commit{
		ID:     "commit_" + uuid.New().String(),
		RepoID: repo.ID,
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message:   "Alice's commit",
		Author:    "alice@example.com",
		Timestamp: time.Now().UTC(),
	}
	_, err := store.Create(context.Background(), commit1, []string{})
	require.NoError(t, err)

	commit2 := &domain.Commit{
		ID:     "commit_" + uuid.New().String(),
		RepoID: repo.ID,
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message:   "Bob's commit",
		Author:    "bob@example.com",
		Timestamp: time.Now().UTC(),
	}
	_, err = store.Create(context.Background(), commit2, []string{})
	require.NoError(t, err)

	// filter by Alice
	page, err := store.List(context.Background(), ListCommitsFilter{
		RepoID: repo.ID,
		Author: "alice@example.com",
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, page.Commits, 1)
	assert.Equal(t, "alice@example.com", page.Commits[0].Author)
}

func TestCommitStore_List_FilterBySince_ReturnsOnlyMatchingCommits(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)

	now := time.Now().UTC()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	// create commit in the past
	commit1 := &domain.Commit{
		ID:     "commit_" + uuid.New().String(),
		RepoID: repo.ID,
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message:   "Past commit",
		Author:    "test@example.com",
		Timestamp: past,
	}
	_, err := store.Create(context.Background(), commit1, []string{})
	require.NoError(t, err)

	// create commit in the future
	commit2 := &domain.Commit{
		ID:     "commit_" + uuid.New().String(),
		RepoID: repo.ID,
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message:   "Future commit",
		Author:    "test@example.com",
		Timestamp: future,
	}
	_, err = store.Create(context.Background(), commit2, []string{})
	require.NoError(t, err)

	// filter by since=now (should only return future commit)
	page, err := store.List(context.Background(), ListCommitsFilter{
		RepoID: repo.ID,
		Since:  &now,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, page.Commits, 1)
	assert.Equal(t, commit2.ID, page.Commits[0].ID)
}

func TestCommitStore_List_FilterByUntil_ReturnsOnlyMatchingCommits(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)

	now := time.Now().UTC()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	// create commit in the past
	commit1 := &domain.Commit{
		ID:     "commit_" + uuid.New().String(),
		RepoID: repo.ID,
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message:   "Past commit",
		Author:    "test@example.com",
		Timestamp: past,
	}
	_, err := store.Create(context.Background(), commit1, []string{})
	require.NoError(t, err)

	// create commit in the future
	commit2 := &domain.Commit{
		ID:     "commit_" + uuid.New().String(),
		RepoID: repo.ID,
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message:   "Future commit",
		Author:    "test@example.com",
		Timestamp: future,
	}
	_, err = store.Create(context.Background(), commit2, []string{})
	require.NoError(t, err)

	// filter by until=now (should only return past commit)
	page, err := store.List(context.Background(), ListCommitsFilter{
		RepoID: repo.ID,
		Until:  &now,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, page.Commits, 1)
	assert.Equal(t, commit1.ID, page.Commits[0].ID)
}

func TestCommitStore_List_FilterByBranch_ReturnsOnlyAncestors(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	branchStore := NewBranchStore(store.db)

	// create linear history: root -> c1 -> c2
	root := createTestCommit(t, store, repo.ID, []string{})
	time.Sleep(10 * time.Millisecond)
	c1 := createTestCommit(t, store, repo.ID, []string{root.ID})
	time.Sleep(10 * time.Millisecond)
	c2 := createTestCommit(t, store, repo.ID, []string{c1.ID})

	// create branch pointing to c2
	err := branchStore.Create(context.Background(), &domain.Branch{
		Name:      "main",
		RepoID:    repo.ID,
		CommitID:  c2.ID,
		CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	// create another commit not on main branch
	orphan := createTestCommit(t, store, repo.ID, []string{})

	// list commits on main branch
	page, err := store.List(context.Background(), ListCommitsFilter{
		RepoID: repo.ID,
		Branch: "main",
		Limit:  10,
	})
	require.NoError(t, err)

	// should return c2, c1, root (ancestors of main)
	assert.Len(t, page.Commits, 3)
	commitIDs := make(map[string]bool)
	for _, c := range page.Commits {
		commitIDs[c.ID] = true
	}
	assert.True(t, commitIDs[c2.ID])
	assert.True(t, commitIDs[c1.ID])
	assert.True(t, commitIDs[root.ID])
	assert.False(t, commitIDs[orphan.ID], "orphan commit should not be on main branch")
}

// GetParents

func TestCommitStore_GetParents_RootCommit_ReturnsEmpty(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	commit := createTestCommit(t, store, repo.ID, []string{})

	parents, err := store.GetParents(context.Background(), repo.ID, commit.ID)
	require.NoError(t, err)
	assert.Len(t, parents, 0)
}

func TestCommitStore_GetParents_RegularCommit_ReturnsOneParent(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)
	parent := createTestCommit(t, store, repo.ID, []string{})
	commit := createTestCommit(t, store, repo.ID, []string{parent.ID})

	parents, err := store.GetParents(context.Background(), repo.ID, commit.ID)
	require.NoError(t, err)
	require.Len(t, parents, 1)
	assert.Equal(t, parent.ID, parents[0].ID)
}

func TestCommitStore_GetParents_MergeCommit_ReturnsTwoParents(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)

	// create two parent commits
	parent1 := createTestCommit(t, store, repo.ID, []string{})
	parent2 := createTestCommit(t, store, repo.ID, []string{})

	// create merge commit with two parents
	mergeCommit := &domain.Commit{
		ID:     "commit_" + uuid.New().String(),
		RepoID: repo.ID,
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message:   "Merge commit",
		Author:    "test@example.com",
		Timestamp: time.Now().UTC(),
	}
	_, err := store.Create(context.Background(), mergeCommit, []string{parent1.ID, parent2.ID})
	require.NoError(t, err)

	// get parents of merge commit
	parents, err := store.GetParents(context.Background(), repo.ID, mergeCommit.ID)
	require.NoError(t, err)
	require.Len(t, parents, 2, "merge commit should have exactly two parents")

	// verify both parents are returned (order may vary)
	parentIDs := make(map[string]bool)
	for _, p := range parents {
		parentIDs[p.ID] = true
	}
	assert.True(t, parentIDs[parent1.ID], "parent1 should be in the list")
	assert.True(t, parentIDs[parent2.ID], "parent2 should be in the list")
}

func TestCommitStore_GetParents_CommitNotFound_ReturnsError(t *testing.T) {
	store, repoStore, cleanup := setupCommitStoreTest(t)
	defer cleanup()

	repo := createTestRepo(t, repoStore)

	_, err := store.GetParents(context.Background(), repo.ID, "commit_nonexistent")
	assert.ErrorIs(t, err, domain.ErrCommitNotFound)
}

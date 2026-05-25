//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/storage/interfaces"
	"github.com/bhpcv252/verge/testhelper"
)

// helpers

func setupGraphStoreTest(t *testing.T) (*GraphStore, *CommitStore, *RepoStore) {
	t.Helper()
	pool, cleanup := testhelper.SetupPostgres(t)
	t.Cleanup(cleanup)
	return NewGraphStore(pool), NewCommitStore(pool), NewRepoStore(pool)
}

// buildChain creates a linear commit chain root -> c1 -> c2 -> ... -> cN
// with timestamps spaced 1s apart starting from base, and returns them in order
func buildChain(
	t *testing.T,
	cs *CommitStore,
	repoID string,
	length int,
	base time.Time,
	author string,
) []*domain.Commit {
	t.Helper()
	commits := make([]*domain.Commit, length)
	for i := 0; i < length; i++ {
		var parentIDs []string
		if i > 0 {
			parentIDs = []string{commits[i-1].ID}
		}
		c := testhelper.NewTestCommit(repoID, parentIDs)
		c.Timestamp = base.Add(time.Duration(i) * time.Second)
		c.Author = author
		created, err := cs.Create(context.Background(), c, parentIDs)
		require.NoError(t, err)
		commits[i] = created
	}
	return commits
}

func commitIDs(commits []*domain.Commit) map[string]struct{} {
	m := make(map[string]struct{}, len(commits))
	for _, c := range commits {
		m[c.ID] = struct{}{}
	}
	return m
}

// TraverseDAG

func TestGraphStore_TraverseDAG_EmptyHead_ReturnsError(t *testing.T) {
	gs, _, _ := setupGraphStoreTest(t)

	_, _, err := gs.TraverseDAG(context.Background(), interfaces.TraversalParams{
		RepoID: "r1",
		Head:   "", // empty
		Limit:  10,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Head")
}

func TestGraphStore_TraverseDAG_LinearChain_ReturnsAllCommitsNewestFirst(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	base := time.Now().UTC().Truncate(time.Second)
	chain := buildChain(t, cs, repo.ID, 3, base, "author@test.com")
	// chain[0]=root, chain[1]=child, chain[2]=head

	commits, cursor, err := gs.TraverseDAG(context.Background(), interfaces.TraversalParams{
		RepoID: repo.ID,
		Head:   chain[2].ID,
		Limit:  10,
	})

	require.NoError(t, err)
	assert.Empty(t, cursor, "no next page for 3 commits with limit=10")
	require.Len(t, commits, 3)

	// must be returned newest-first
	assert.Equal(t, chain[2].ID, commits[0].ID)
	assert.Equal(t, chain[1].ID, commits[1].ID)
	assert.Equal(t, chain[0].ID, commits[2].ID)
}

func TestGraphStore_TraverseDAG_HeadOnlyCommit_ReturnsSingleCommit(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	c := createTestCommit(t, cs, repo.ID, []string{})

	commits, _, err := gs.TraverseDAG(context.Background(), interfaces.TraversalParams{
		RepoID: repo.ID,
		Head:   c.ID,
		Limit:  10,
	})

	require.NoError(t, err)
	require.Len(t, commits, 1)
	assert.Equal(t, c.ID, commits[0].ID)
}

func TestGraphStore_TraverseDAG_RepoIsolation_OnlyReturnsCommitsInRepo(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo1 := createTestRepo(t, rs)
	repo2 := createTestRepo(t, rs)

	base := time.Now().UTC().Truncate(time.Second)
	chain1 := buildChain(t, cs, repo1.ID, 2, base, "a@test.com")
	buildChain(t, cs, repo2.ID, 3, base, "a@test.com") // different repo

	commits, _, err := gs.TraverseDAG(context.Background(), interfaces.TraversalParams{
		RepoID: repo1.ID,
		Head:   chain1[1].ID,
		Limit:  10,
	})

	require.NoError(t, err)
	assert.Len(t, commits, 2, "must not include commits from repo2")
	for _, c := range commits {
		assert.Equal(t, repo1.ID, c.RepoID)
	}
}

func TestGraphStore_TraverseDAG_AuthorFilter_ReturnsOnlyMatchingAuthor(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	base := time.Now().UTC().Truncate(time.Second)

	// root by alice, child by bob, grandchild by alice
	root := testhelper.NewTestCommit(repo.ID, []string{})
	root.Timestamp = base
	root.Author = "alice@test.com"
	root, _ = cs.Create(context.Background(), root, []string{})

	child := testhelper.NewTestCommit(repo.ID, []string{root.ID})
	child.Timestamp = base.Add(time.Second)
	child.Author = "bob@test.com"
	child, _ = cs.Create(context.Background(), child, []string{root.ID})

	grandchild := testhelper.NewTestCommit(repo.ID, []string{child.ID})
	grandchild.Timestamp = base.Add(2 * time.Second)
	grandchild.Author = "alice@test.com"
	grandchild, _ = cs.Create(context.Background(), grandchild, []string{child.ID})

	commits, _, err := gs.TraverseDAG(context.Background(), interfaces.TraversalParams{
		RepoID: repo.ID,
		Head:   grandchild.ID,
		Author: "alice@test.com",
		Limit:  10,
	})

	require.NoError(t, err)
	require.Len(t, commits, 2)
	for _, c := range commits {
		assert.Equal(t, "alice@test.com", c.Author)
	}
}

func TestGraphStore_TraverseDAG_SinceFilter_ExcludesOlderCommits(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	base := time.Now().UTC().Truncate(time.Second)
	chain := buildChain(t, cs, repo.ID, 3, base, "a@test.com")

	// since = timestamp of chain[1], should return chain[2] and chain[1], not chain[0]
	since := chain[1].Timestamp
	commits, _, err := gs.TraverseDAG(context.Background(), interfaces.TraversalParams{
		RepoID: repo.ID,
		Head:   chain[2].ID,
		Since:  &since,
		Limit:  10,
	})

	require.NoError(t, err)
	ids := commitIDs(commits)
	assert.Contains(t, ids, chain[2].ID)
	assert.Contains(t, ids, chain[1].ID)
	assert.NotContains(t, ids, chain[0].ID)
}

func TestGraphStore_TraverseDAG_UntilFilter_ExcludesNewerCommits(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	base := time.Now().UTC().Truncate(time.Second)
	chain := buildChain(t, cs, repo.ID, 3, base, "a@test.com")

	// until = timestamp of chain[1], should return chain[0] and chain[1], not chain[2]
	until := chain[1].Timestamp
	commits, _, err := gs.TraverseDAG(context.Background(), interfaces.TraversalParams{
		RepoID: repo.ID,
		Head:   chain[2].ID,
		Until:  &until,
		Limit:  10,
	})

	require.NoError(t, err)
	ids := commitIDs(commits)
	assert.Contains(t, ids, chain[1].ID)
	assert.Contains(t, ids, chain[0].ID)
	assert.NotContains(t, ids, chain[2].ID)
}

func TestGraphStore_TraverseDAG_Pagination_CursorReturnsNextPage(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	base := time.Now().UTC().Truncate(time.Second)
	chain := buildChain(t, cs, repo.ID, 4, base, "a@test.com")
	head := chain[3]

	page1, cursor1, err := gs.TraverseDAG(context.Background(), interfaces.TraversalParams{
		RepoID: repo.ID,
		Head:   head.ID,
		Limit:  2,
	})
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.NotEmpty(t, cursor1, "cursor must be set when more pages exist")

	page2, cursor2, err := gs.TraverseDAG(context.Background(), interfaces.TraversalParams{
		RepoID: repo.ID,
		Head:   head.ID,
		Limit:  2,
		Cursor: cursor1,
	})
	require.NoError(t, err)
	require.Len(t, page2, 2)
	assert.Empty(t, cursor2, "no more pages after page 2")

	// no duplicates across pages
	seen := make(map[string]int)
	for _, c := range append(page1, page2...) {
		seen[c.ID]++
	}
	for id, count := range seen {
		assert.Equal(t, 1, count, "commit %s appeared on multiple pages", id)
	}
	assert.Len(t, seen, 4)
}

func TestGraphStore_TraverseDAG_DefaultLimit_UsedWhenZero(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	base := time.Now().UTC().Truncate(time.Second)
	chain := buildChain(t, cs, repo.ID, 3, base, "a@test.com")

	// limit=0 means use default (20), so all 3 commits should be returned
	commits, _, err := gs.TraverseDAG(context.Background(), interfaces.TraversalParams{
		RepoID: repo.ID,
		Head:   chain[2].ID,
		Limit:  0,
	})

	require.NoError(t, err)
	assert.Len(t, commits, 3)
}

// GetAncestors

func TestGraphStore_GetAncestors_RootCommit_ReturnsEmpty(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	root := createTestCommit(t, cs, repo.ID, []string{})

	ancestors, err := gs.GetAncestors(context.Background(), repo.ID, root.ID, 10)

	require.NoError(t, err)
	assert.Empty(t, ancestors)
}

func TestGraphStore_GetAncestors_LinearChain_ExcludesStartCommit(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	base := time.Now().UTC().Truncate(time.Second)
	chain := buildChain(t, cs, repo.ID, 3, base, "a@test.com")
	// chain: [0]=root, [1]=child, [2]=grandchild

	ancestors, err := gs.GetAncestors(context.Background(), repo.ID, chain[2].ID, 10)

	require.NoError(t, err)
	require.Len(t, ancestors, 2)

	ids := commitIDs(ancestors)
	assert.Contains(t, ids, chain[1].ID)
	assert.Contains(t, ids, chain[0].ID)
	assert.NotContains(t, ids, chain[2].ID, "start commit must be excluded from its own ancestors")
}

func TestGraphStore_GetAncestors_MergeCommit_ReturnsBothBranches(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	base := time.Now().UTC().Truncate(time.Second)

	commonBase := createTestCommit(t, cs, repo.ID, []string{})

	branchA := testhelper.NewTestCommit(repo.ID, []string{commonBase.ID})
	branchA.Timestamp = base.Add(time.Second)
	branchA, _ = cs.Create(context.Background(), branchA, []string{commonBase.ID})

	branchB := testhelper.NewTestCommit(repo.ID, []string{commonBase.ID})
	branchB.Timestamp = base.Add(2 * time.Second)
	branchB, _ = cs.Create(context.Background(), branchB, []string{commonBase.ID})

	merge := testhelper.NewTestCommit(repo.ID, []string{branchA.ID, branchB.ID})
	merge.Timestamp = base.Add(3 * time.Second)
	merge, _ = cs.Create(context.Background(), merge, []string{branchA.ID, branchB.ID})

	ancestors, err := gs.GetAncestors(context.Background(), repo.ID, merge.ID, 10)

	require.NoError(t, err)
	ids := commitIDs(ancestors)
	assert.Contains(t, ids, branchA.ID)
	assert.Contains(t, ids, branchB.ID)
	assert.Contains(t, ids, commonBase.ID)
	assert.NotContains(t, ids, merge.ID)
}

func TestGraphStore_GetAncestors_RespectsLimit(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	base := time.Now().UTC().Truncate(time.Second)
	chain := buildChain(t, cs, repo.ID, 5, base, "a@test.com")

	ancestors, err := gs.GetAncestors(context.Background(), repo.ID, chain[4].ID, 2)

	require.NoError(t, err)
	assert.Len(t, ancestors, 2, "limit must be respected")
}

func TestGraphStore_GetAncestors_DefaultLimit_WhenZero(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	base := time.Now().UTC().Truncate(time.Second)
	chain := buildChain(t, cs, repo.ID, 3, base, "a@test.com")

	// limit=0 → default 20, should return all 2 ancestors
	ancestors, err := gs.GetAncestors(context.Background(), repo.ID, chain[2].ID, 0)

	require.NoError(t, err)
	assert.Len(t, ancestors, 2)
}

// FindMergeBase

func TestGraphStore_FindMergeBase_LinearHistory_ReturnsCommonAncestor(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	base := time.Now().UTC().Truncate(time.Second)

	commonBase := createTestCommit(t, cs, repo.ID, []string{})

	commitA := testhelper.NewTestCommit(repo.ID, []string{commonBase.ID})
	commitA.Timestamp = base.Add(time.Second)
	commitA, _ = cs.Create(context.Background(), commitA, []string{commonBase.ID})

	commitB := testhelper.NewTestCommit(repo.ID, []string{commitA.ID})
	commitB.Timestamp = base.Add(2 * time.Second)
	commitB, _ = cs.Create(context.Background(), commitB, []string{commitA.ID})

	commitC := testhelper.NewTestCommit(repo.ID, []string{commonBase.ID})
	commitC.Timestamp = base.Add(time.Second)
	commitC, _ = cs.Create(context.Background(), commitC, []string{commonBase.ID})

	lca, err := gs.FindMergeBase(context.Background(), repo.ID, commitB.ID, commitC.ID)

	require.NoError(t, err)
	assert.Equal(t, commonBase.ID, lca.ID)
}

func TestGraphStore_FindMergeBase_SameCommit_ReturnsThatCommit(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	c := createTestCommit(t, cs, repo.ID, []string{})

	lca, err := gs.FindMergeBase(context.Background(), repo.ID, c.ID, c.ID)

	require.NoError(t, err)
	assert.Equal(t, c.ID, lca.ID)
}

func TestGraphStore_FindMergeBase_NoCommonAncestor_ReturnsError(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	// two completely unrelated root commits
	c1 := createTestCommit(t, cs, repo.ID, []string{})
	c2 := createTestCommit(t, cs, repo.ID, []string{})

	_, err := gs.FindMergeBase(context.Background(), repo.ID, c1.ID, c2.ID)

	require.Error(t, err)
}

func TestGraphStore_FindMergeBase_DeepDivergence_ReturnsCorrectLCA(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	base := time.Now().UTC().Truncate(time.Second)

	root := createTestCommit(t, cs, repo.ID, []string{})

	shared1 := testhelper.NewTestCommit(repo.ID, []string{root.ID})
	shared1.Timestamp = base.Add(time.Second)
	shared1, _ = cs.Create(context.Background(), shared1, []string{root.ID})

	shared2 := testhelper.NewTestCommit(repo.ID, []string{shared1.ID})
	shared2.Timestamp = base.Add(2 * time.Second)
	shared2, _ = cs.Create(context.Background(), shared2, []string{shared1.ID})

	branchA := testhelper.NewTestCommit(repo.ID, []string{shared2.ID})
	branchA.Timestamp = base.Add(3 * time.Second)
	branchA, _ = cs.Create(context.Background(), branchA, []string{shared2.ID})

	branchB := testhelper.NewTestCommit(repo.ID, []string{shared2.ID})
	branchB.Timestamp = base.Add(3 * time.Second)
	branchB, _ = cs.Create(context.Background(), branchB, []string{shared2.ID})

	lca, err := gs.FindMergeBase(context.Background(), repo.ID, branchA.ID, branchB.ID)

	require.NoError(t, err)
	// LCA is shared2 (most recent common ancestor)
	assert.Equal(t, shared2.ID, lca.ID)
}

func TestGraphStore_FindMergeBase_OneIsAncestorOfOther_ReturnsAncestor(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo := createTestRepo(t, rs)
	base := time.Now().UTC().Truncate(time.Second)
	chain := buildChain(t, cs, repo.ID, 3, base, "a@test.com")

	// chain[0] is ancestor of chain[2], LCA should be chain[0]
	lca, err := gs.FindMergeBase(context.Background(), repo.ID, chain[2].ID, chain[0].ID)

	require.NoError(t, err)
	assert.Equal(t, chain[0].ID, lca.ID)
}

func TestGraphStore_FindMergeBase_RepoIsolation_DoesNotFindCrossRepoLCA(t *testing.T) {
	gs, cs, rs := setupGraphStoreTest(t)

	repo1 := createTestRepo(t, rs)
	repo2 := createTestRepo(t, rs)

	base := time.Now().UTC().Truncate(time.Second)
	chain1 := buildChain(t, cs, repo1.ID, 2, base, "a@test.com")
	chain2 := buildChain(t, cs, repo2.ID, 2, base, "a@test.com")

	_, err := gs.FindMergeBase(context.Background(), repo1.ID, chain1[1].ID, chain2[1].ID)

	require.Error(t, err, "cross-repo merge base must not be found")
}

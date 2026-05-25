//go:build integration

package neo4j

import (
	"context"
	"fmt"
	"testing"
	"time"

	neo4jdriver "github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/storage/interfaces"
	"github.com/bhpcv252/verge/testhelper"
)

// helpers

func nodeProps(id, repoID, author, message string, ts time.Time) map[string]interface{} {
	return map[string]interface{}{
		"id":        id,
		"repo_id":   repoID,
		"author":    author,
		"message":   message,
		"timestamp": ts.UTC().Format(time.RFC3339),
	}
}

func createNode(t *testing.T, driver neo4jdriver.DriverWithContext, props map[string]interface{}) {
	t.Helper()
	testhelper.Neo4jRunWrite(t, driver,
		`MERGE (c:Commit {id: $id, repo_id: $repo_id})
		 SET c.author    = $author,
		     c.message   = $message,
		     c.timestamp = $timestamp`,
		props)
}

func createEdge(t *testing.T, driver neo4jdriver.DriverWithContext, childID, parentID string) {
	t.Helper()
	testhelper.Neo4jRunWrite(t, driver,
		`MATCH (child:Commit {id: $child_id})
		 MATCH (parent:Commit {id: $parent_id})
		 MERGE (child)-[:PARENT_OF]->(parent)`,
		map[string]interface{}{"child_id": childID, "parent_id": parentID})
}

var uniqueIDCounter int64

func uniqueID(prefix string) string {
	uniqueIDCounter++
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), uniqueIDCounter)
}

func setupNeo4jGraphStore(t *testing.T) (*GraphStore, neo4jdriver.DriverWithContext) {
	t.Helper()
	driver := testhelper.SetupNeo4j(t)
	return NewGraphStore(driver), driver
}

// TraverseDAG

func TestNeo4jGraphStore_TraverseDAG_EmptyHead_ReturnsError(t *testing.T) {
	gs, _ := setupNeo4jGraphStore(t)

	_, _, err := gs.TraverseDAG(context.Background(), interfaces.TraversalParams{
		RepoID: uniqueID("repo"),
		Head:   "",
		Limit:  10,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Head")
}

func TestNeo4jGraphStore_TraverseDAG_LinearChain_ReturnsAllCommitsNewestFirst(t *testing.T) {
	gs, driver := setupNeo4jGraphStore(t)
	repoID := uniqueID("repo")
	base := time.Now().UTC().Truncate(time.Second)

	// root → child → head (oldest → newest)
	rootID := uniqueID("commit")
	childID := uniqueID("commit")
	headID := uniqueID("commit")

	createNode(t, driver, nodeProps(rootID, repoID, "a@t.com", "root", base))
	createNode(t, driver, nodeProps(childID, repoID, "a@t.com", "child", base.Add(time.Second)))
	createNode(t, driver, nodeProps(headID, repoID, "a@t.com", "head", base.Add(2*time.Second)))
	createEdge(t, driver, childID, rootID)
	createEdge(t, driver, headID, childID)

	commits, cursor, err := gs.TraverseDAG(context.Background(), interfaces.TraversalParams{
		RepoID: repoID,
		Head:   headID,
		Limit:  10,
	})

	require.NoError(t, err)
	assert.Empty(t, cursor)
	require.Len(t, commits, 3)
	assert.Equal(t, headID, commits[0].ID)
	assert.Equal(t, childID, commits[1].ID)
	assert.Equal(t, rootID, commits[2].ID)
}

func TestNeo4jGraphStore_TraverseDAG_AuthorFilter_ReturnsOnlyMatchingAuthor(t *testing.T) {
	gs, driver := setupNeo4jGraphStore(t)
	repoID := uniqueID("repo")
	base := time.Now().UTC().Truncate(time.Second)

	rootID := uniqueID("commit")
	aliceID := uniqueID("commit")
	bobID := uniqueID("commit")

	createNode(t, driver, nodeProps(rootID, repoID, "alice@t.com", "root", base))
	createNode(
		t,
		driver,
		nodeProps(aliceID, repoID, "alice@t.com", "alice commit", base.Add(time.Second)),
	)
	createNode(
		t,
		driver,
		nodeProps(bobID, repoID, "bob@t.com", "bob commit", base.Add(2*time.Second)),
	)
	createEdge(t, driver, aliceID, rootID)
	createEdge(t, driver, bobID, aliceID)

	commits, _, err := gs.TraverseDAG(context.Background(), interfaces.TraversalParams{
		RepoID: repoID,
		Head:   bobID,
		Author: "alice@t.com",
		Limit:  10,
	})

	require.NoError(t, err)
	require.Len(t, commits, 2)
	for _, c := range commits {
		assert.Equal(t, "alice@t.com", c.Author)
	}
}

func TestNeo4jGraphStore_TraverseDAG_Pagination_CursorWorks(t *testing.T) {
	gs, driver := setupNeo4jGraphStore(t)
	repoID := uniqueID("repo")
	base := time.Now().UTC().Truncate(time.Second)

	// 4-commit chain
	ids := make([]string, 4)
	for i := range ids {
		ids[i] = uniqueID("commit")
		createNode(
			t,
			driver,
			nodeProps(
				ids[i],
				repoID,
				"a@t.com",
				fmt.Sprintf("c%d", i),
				base.Add(time.Duration(i)*time.Second),
			),
		)
		if i > 0 {
			createEdge(t, driver, ids[i], ids[i-1])
		}
	}
	head := ids[3]

	page1, cursor1, err := gs.TraverseDAG(context.Background(), interfaces.TraversalParams{
		RepoID: repoID,
		Head:   head,
		Limit:  2,
	})
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.NotEmpty(t, cursor1)

	page2, cursor2, err := gs.TraverseDAG(context.Background(), interfaces.TraversalParams{
		RepoID: repoID,
		Head:   head,
		Limit:  2,
		Cursor: cursor1,
	})
	require.NoError(t, err)
	require.Len(t, page2, 2)
	assert.Empty(t, cursor2)

	// no duplicates
	seen := make(map[string]int)
	for _, c := range append(page1, page2...) {
		seen[c.ID]++
	}
	assert.Len(t, seen, 4)
	for id, count := range seen {
		assert.Equal(t, 1, count, "commit %s appeared on multiple pages", id)
	}
}

func TestNeo4jGraphStore_TraverseDAG_RepoIsolation_OnlyReturnsMatchingRepo(t *testing.T) {
	gs, driver := setupNeo4jGraphStore(t)
	repo1 := uniqueID("repo")
	repo2 := uniqueID("repo")
	base := time.Now().UTC().Truncate(time.Second)

	r1head := uniqueID("commit")
	r2head := uniqueID("commit")
	createNode(t, driver, nodeProps(r1head, repo1, "a@t.com", "repo1 head", base))
	createNode(t, driver, nodeProps(r2head, repo2, "a@t.com", "repo2 head", base))

	commits, _, err := gs.TraverseDAG(context.Background(), interfaces.TraversalParams{
		RepoID: repo1,
		Head:   r1head,
		Limit:  10,
	})

	require.NoError(t, err)
	for _, c := range commits {
		assert.Equal(t, repo1, c.RepoID)
	}
}

// GetAncestors

func TestNeo4jGraphStore_GetAncestors_LinearChain_ExcludesStartCommit(t *testing.T) {
	gs, driver := setupNeo4jGraphStore(t)
	repoID := uniqueID("repo")
	base := time.Now().UTC().Truncate(time.Second)

	rootID := uniqueID("commit")
	childID := uniqueID("commit")
	headID := uniqueID("commit")
	createNode(t, driver, nodeProps(rootID, repoID, "a@t.com", "root", base))
	createNode(t, driver, nodeProps(childID, repoID, "a@t.com", "child", base.Add(time.Second)))
	createNode(t, driver, nodeProps(headID, repoID, "a@t.com", "head", base.Add(2*time.Second)))
	createEdge(t, driver, childID, rootID)
	createEdge(t, driver, headID, childID)

	ancestors, err := gs.GetAncestors(context.Background(), repoID, headID, 10)

	require.NoError(t, err)
	require.Len(t, ancestors, 2)
	ids := make(map[string]struct{})
	for _, a := range ancestors {
		ids[a.ID] = struct{}{}
	}
	assert.Contains(t, ids, childID)
	assert.Contains(t, ids, rootID)
	assert.NotContains(t, ids, headID, "start commit must be excluded from its own ancestors")
}

func TestNeo4jGraphStore_GetAncestors_RootCommit_ReturnsEmpty(t *testing.T) {
	gs, driver := setupNeo4jGraphStore(t)
	repoID := uniqueID("repo")

	rootID := uniqueID("commit")
	createNode(t, driver, nodeProps(rootID, repoID, "a@t.com", "root", time.Now()))

	ancestors, err := gs.GetAncestors(context.Background(), repoID, rootID, 10)

	require.NoError(t, err)
	assert.Empty(t, ancestors)
}

func TestNeo4jGraphStore_GetAncestors_MergeCommit_ReturnsBothBranches(t *testing.T) {
	gs, driver := setupNeo4jGraphStore(t)
	repoID := uniqueID("repo")
	base := time.Now().UTC().Truncate(time.Second)

	baseID := uniqueID("commit")
	aID := uniqueID("commit")
	bID := uniqueID("commit")
	mergeID := uniqueID("commit")
	createNode(t, driver, nodeProps(baseID, repoID, "a@t.com", "base", base))
	createNode(t, driver, nodeProps(aID, repoID, "a@t.com", "branch a", base.Add(time.Second)))
	createNode(t, driver, nodeProps(bID, repoID, "a@t.com", "branch b", base.Add(time.Second)))
	createNode(t, driver, nodeProps(mergeID, repoID, "a@t.com", "merge", base.Add(2*time.Second)))
	createEdge(t, driver, aID, baseID)
	createEdge(t, driver, bID, baseID)
	createEdge(t, driver, mergeID, aID)
	createEdge(t, driver, mergeID, bID)

	ancestors, err := gs.GetAncestors(context.Background(), repoID, mergeID, 10)

	require.NoError(t, err)
	ids := make(map[string]struct{})
	for _, a := range ancestors {
		ids[a.ID] = struct{}{}
	}
	assert.Contains(t, ids, aID)
	assert.Contains(t, ids, bID)
	assert.Contains(t, ids, baseID)
	assert.NotContains(t, ids, mergeID)
}

func TestNeo4jGraphStore_GetAncestors_DefaultLimit_WhenZero(t *testing.T) {
	gs, driver := setupNeo4jGraphStore(t)
	repoID := uniqueID("repo")
	base := time.Now().UTC().Truncate(time.Second)

	ids := make([]string, 3)
	for i := range ids {
		ids[i] = uniqueID("commit")
		createNode(
			t,
			driver,
			nodeProps(
				ids[i],
				repoID,
				"a@t.com",
				fmt.Sprintf("c%d", i),
				base.Add(time.Duration(i)*time.Second),
			),
		)
		if i > 0 {
			createEdge(t, driver, ids[i], ids[i-1])
		}
	}

	// limit=0 -> default 20, all 2 ancestors returned
	ancestors, err := gs.GetAncestors(context.Background(), repoID, ids[2], 0)
	require.NoError(t, err)
	assert.Len(t, ancestors, 2)
}

// FindMergeBase

func TestNeo4jGraphStore_FindMergeBase_DivergedHistory_ReturnsLCA(t *testing.T) {
	gs, driver := setupNeo4jGraphStore(t)
	repoID := uniqueID("repo")
	base := time.Now().UTC().Truncate(time.Second)

	baseID := uniqueID("commit")
	aID := uniqueID("commit")
	bID := uniqueID("commit")
	createNode(t, driver, nodeProps(baseID, repoID, "a@t.com", "base", base))
	createNode(t, driver, nodeProps(aID, repoID, "a@t.com", "branch a", base.Add(time.Second)))
	createNode(t, driver, nodeProps(bID, repoID, "a@t.com", "branch b", base.Add(time.Second)))
	createEdge(t, driver, aID, baseID)
	createEdge(t, driver, bID, baseID)

	lca, err := gs.FindMergeBase(context.Background(), repoID, aID, bID)

	require.NoError(t, err)
	assert.Equal(t, baseID, lca.ID)
}

func TestNeo4jGraphStore_FindMergeBase_SameCommit_ReturnsThatCommit(t *testing.T) {
	gs, driver := setupNeo4jGraphStore(t)
	repoID := uniqueID("repo")

	id := uniqueID("commit")
	createNode(t, driver, nodeProps(id, repoID, "a@t.com", "root", time.Now()))

	lca, err := gs.FindMergeBase(context.Background(), repoID, id, id)

	require.NoError(t, err)
	assert.Equal(t, id, lca.ID)
}

func TestNeo4jGraphStore_FindMergeBase_NoCommonAncestor_ReturnsError(t *testing.T) {
	gs, driver := setupNeo4jGraphStore(t)
	repoID := uniqueID("repo")

	c1 := uniqueID("commit")
	c2 := uniqueID("commit")
	createNode(t, driver, nodeProps(c1, repoID, "a@t.com", "unrelated 1", time.Now()))
	createNode(t, driver, nodeProps(c2, repoID, "a@t.com", "unrelated 2", time.Now()))

	_, err := gs.FindMergeBase(context.Background(), repoID, c1, c2)

	require.Error(t, err)
}

func TestNeo4jGraphStore_FindMergeBase_OneIsAncestorOfOther_ReturnsAncestor(t *testing.T) {
	gs, driver := setupNeo4jGraphStore(t)
	repoID := uniqueID("repo")
	base := time.Now().UTC().Truncate(time.Second)

	rootID := uniqueID("commit")
	childID := uniqueID("commit")
	grandchildID := uniqueID("commit")
	createNode(t, driver, nodeProps(rootID, repoID, "a@t.com", "root", base))
	createNode(t, driver, nodeProps(childID, repoID, "a@t.com", "child", base.Add(time.Second)))
	createNode(
		t,
		driver,
		nodeProps(grandchildID, repoID, "a@t.com", "grandchild", base.Add(2*time.Second)),
	)
	createEdge(t, driver, childID, rootID)
	createEdge(t, driver, grandchildID, childID)

	// LCA of grandchild and root is root itself
	lca, err := gs.FindMergeBase(context.Background(), repoID, grandchildID, rootID)

	require.NoError(t, err)
	assert.Equal(t, rootID, lca.ID)
}

func TestNeo4jGraphStore_FindMergeBase_DeepDivergence_ReturnsMostRecentLCA(t *testing.T) {
	gs, driver := setupNeo4jGraphStore(t)
	repoID := uniqueID("repo")
	base := time.Now().UTC().Truncate(time.Second)

	rootID := uniqueID("commit")
	shared1ID := uniqueID("commit")
	shared2ID := uniqueID("commit")
	aID := uniqueID("commit")
	bID := uniqueID("commit")
	createNode(t, driver, nodeProps(rootID, repoID, "a@t.com", "root", base))
	createNode(t, driver, nodeProps(shared1ID, repoID, "a@t.com", "shared1", base.Add(time.Second)))
	createNode(
		t,
		driver,
		nodeProps(shared2ID, repoID, "a@t.com", "shared2", base.Add(2*time.Second)),
	)
	createNode(t, driver, nodeProps(aID, repoID, "a@t.com", "branch a", base.Add(3*time.Second)))
	createNode(t, driver, nodeProps(bID, repoID, "a@t.com", "branch b", base.Add(3*time.Second)))
	createEdge(t, driver, shared1ID, rootID)
	createEdge(t, driver, shared2ID, shared1ID)
	createEdge(t, driver, aID, shared2ID)
	createEdge(t, driver, bID, shared2ID)

	lca, err := gs.FindMergeBase(context.Background(), repoID, aID, bID)

	require.NoError(t, err)
	// most recent LCA is shared2
	assert.Equal(t, shared2ID, lca.ID)
}

func TestNeo4jGraphStore_FindMergeBase_RepoIsolation(t *testing.T) {
	gs, driver := setupNeo4jGraphStore(t)
	repo1 := uniqueID("repo")
	repo2 := uniqueID("repo")
	base := time.Now().UTC().Truncate(time.Second)

	// create commits in repo1 with a common ancestor
	baseID := uniqueID("commit")
	r1a := uniqueID("commit")
	r1b := uniqueID("commit")
	createNode(t, driver, nodeProps(baseID, repo1, "a@t.com", "base", base))
	createNode(t, driver, nodeProps(r1a, repo1, "a@t.com", "r1a", base.Add(time.Second)))
	createNode(t, driver, nodeProps(r1b, repo1, "a@t.com", "r1b", base.Add(time.Second)))
	createEdge(t, driver, r1a, baseID)
	createEdge(t, driver, r1b, baseID)

	// create unrelated commits in repo2
	r2a := uniqueID("commit")
	r2b := uniqueID("commit")
	createNode(t, driver, nodeProps(r2a, repo2, "a@t.com", "r2a", base))
	createNode(t, driver, nodeProps(r2b, repo2, "a@t.com", "r2b", base.Add(time.Second)))

	// query in repo2 scope - commits from repo1 must not appear as ancestors
	_, err := gs.FindMergeBase(context.Background(), repo2, r2a, r2b)
	require.Error(t, err, "no common ancestor in repo2 - cross-repo ancestors must not leak")
}

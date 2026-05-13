//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/testhelper"
)

func startCommitServer(t *testing.T) string {
	t.Helper()
	pool, cleanup := testhelper.SetupPostgres(t)
	t.Cleanup(cleanup)
	return startE2EServer(t, pool)
}

// POST /v1/repos/:repoID/commits

func TestCommits_Create_RootCommit_Returns201WithEmptyParentIDs(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)

	resp := doPost(t, base+"/repos/"+repo.ID+"/commits", map[string]any{
		"parent_ids": []string{},
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Initial commit",
		"author":  "test@example.com",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var got commitResponse
	decodeJSON(t, resp.Body, &got)
	assert.NotEmpty(t, got.ID)
	assert.Equal(t, repo.ID, got.RepoID)
	assert.Empty(t, got.ParentIDs)
	assert.Equal(t, "Initial commit", got.Message)
	assert.Equal(t, "test@example.com", got.Author)
	assert.False(t, got.Timestamp.IsZero())
}

func TestCommits_Create_RegularCommit_Returns201WithOneParent(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)
	parent := createCommit(t, base, repo.ID, []string{})

	resp := doPost(t, base+"/repos/"+repo.ID+"/commits", map[string]any{
		"parent_ids": []string{parent.ID},
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Add feature",
		"author":  "alice@example.com",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var got commitResponse
	decodeJSON(t, resp.Body, &got)
	require.Len(t, got.ParentIDs, 1)
	assert.Equal(t, parent.ID, got.ParentIDs[0])
}

func TestCommits_Create_TwoParentIDs_Returns400WithInvalidRequest(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)

	resp := doPost(t, base+"/repos/"+repo.ID+"/commits", map[string]any{
		"parent_ids": []string{"commit_1", "commit_2"},
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Test",
		"author":  "test@example.com",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "invalid_request", got.Error)
	assert.Contains(t, got.Message, "merge")
}

func TestCommits_Create_InvalidParent_Returns422WithInvalidParent(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)

	resp := doPost(t, base+"/repos/"+repo.ID+"/commits", map[string]any{
		"parent_ids": []string{"commit_nonexistent"},
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Test",
		"author":  "test@example.com",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "invalid_parent", got.Error)
}

func TestCommits_Create_IdempotencyKeyRepeat_Returns200SameCommit(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)
	idempotencyKey := "key_" + uuid.New().String()

	// first request
	resp1 := doPost(t, base+"/repos/"+repo.ID+"/commits", map[string]any{
		"idempotency_key": idempotencyKey,
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Test",
		"author":  "test@example.com",
	})
	defer resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode)

	var first commitResponse
	decodeJSON(t, resp1.Body, &first)

	// second request with same idempotency key
	resp2 := doPost(t, base+"/repos/"+repo.ID+"/commits", map[string]any{
		"idempotency_key": idempotencyKey,
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Different message",
		"author":  "different@example.com",
	})
	defer resp2.Body.Close()

	// should return 200 (not 201) with same commit
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var second commitResponse
	decodeJSON(t, resp2.Body, &second)
	assert.Equal(t, first.ID, second.ID, "should return same commit")
	assert.Equal(t, "Test", second.Message, "should preserve original message")
}

func TestCommits_Create_MissingDataPointer_Returns400(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)

	resp := doPost(t, base+"/repos/"+repo.ID+"/commits", map[string]any{
		"message": "Test",
		"author":  "test@example.com",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "invalid_request", got.Error)
}

func TestCommits_Create_RepoNotFound_Returns404(t *testing.T) {
	base := startCommitServer(t)

	resp := doPost(t, base+"/repos/repo_nonexistent/commits", map[string]any{
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Test",
		"author":  "test@example.com",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "repo_not_found", got.Error)
}

// GET /v1/repos/:repoID/commits/:commitID

func TestCommits_Get_Exists_Returns200WithFullCommit(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)
	commit := createCommit(t, base, repo.ID, []string{})

	resp := doGet(t, base+"/repos/"+repo.ID+"/commits/"+commit.ID)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got commitResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, commit.ID, got.ID)
	assert.Equal(t, repo.ID, got.RepoID)
	assert.Equal(t, "test commit", got.Message)
	assert.Equal(t, "test@example.com", got.Author)
	assert.NotNil(t, got.DataPointer)
	assert.Equal(t, "db", got.DataPointer.Type)
}

func TestCommits_Get_NotFound_Returns404WithCommitNotFound(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)

	resp := doGet(t, base+"/repos/"+repo.ID+"/commits/commit_nonexistent")
	defer resp.Body.Close()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "commit_not_found", got.Error)
}

// GET /v1/repos/:repoID/commits

func TestCommits_List_TraversalFlat_ReturnsInReverseChronologicalOrder(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)

	// create 3 commits with time delays
	c1 := createCommit(t, base, repo.ID, []string{})
	time.Sleep(50 * time.Millisecond)
	c2 := createCommit(t, base, repo.ID, []string{})
	time.Sleep(50 * time.Millisecond)
	c3 := createCommit(t, base, repo.ID, []string{})

	resp := doGet(t, base+"/repos/"+repo.ID+"/commits?traversal=flat")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got listCommitsResponse
	decodeJSON(t, resp.Body, &got)
	require.Len(t, got.Commits, 3)

	// should be reverse chronological (newest first)
	assert.Equal(t, c3.ID, got.Commits[0].ID)
	assert.Equal(t, c2.ID, got.Commits[1].ID)
	assert.Equal(t, c1.ID, got.Commits[2].ID)
}

func TestCommits_List_TraversalDAGWithBranch_FollowsParentLinks(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)

	// create linear history: root -> c1 -> c2
	root := createCommit(t, base, repo.ID, []string{})
	time.Sleep(50 * time.Millisecond)
	c1 := createCommit(t, base, repo.ID, []string{root.ID})
	time.Sleep(50 * time.Millisecond)
	c2 := createCommit(t, base, repo.ID, []string{c1.ID})

	// create branch pointing to c2
	branchResp := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"name":             "main",
		"source_commit_id": c2.ID,
	})
	branchResp.Body.Close()

	// create orphan commit (not on main)
	orphan := createCommit(t, base, repo.ID, []string{})

	resp := doGet(t, base+"/repos/"+repo.ID+"/commits?traversal=dag&branch=main")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got listCommitsResponse
	decodeJSON(t, resp.Body, &got)

	// should return c2, c1, root (ancestors of main)
	commitIDs := make(map[string]bool)
	for _, c := range got.Commits {
		commitIDs[c.ID] = true
	}
	assert.True(t, commitIDs[c2.ID])
	assert.True(t, commitIDs[c1.ID])
	assert.True(t, commitIDs[root.ID])
	assert.False(t, commitIDs[orphan.ID], "orphan should not be on main branch")
}

func TestCommits_List_TraversalDAGWithoutBranch_Returns400(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)

	resp := doGet(t, base+"/repos/"+repo.ID+"/commits?traversal=dag")
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "invalid_request", got.Error)
}

func TestCommits_List_FilterByAuthor_ReturnsOnlyMatchingCommits(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)

	// create commit by Alice
	aliceResp := doPost(t, base+"/repos/"+repo.ID+"/commits", map[string]any{
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Alice's commit",
		"author":  "alice@example.com",
	})
	var alice commitResponse
	decodeJSON(t, aliceResp.Body, &alice)
	aliceResp.Body.Close()

	// create commit by Bob
	bobResp := doPost(t, base+"/repos/"+repo.ID+"/commits", map[string]any{
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Bob's commit",
		"author":  "bob@example.com",
	})
	bobResp.Body.Close()

	// filter by Alice
	resp := doGet(t, base+"/repos/"+repo.ID+"/commits?author=alice@example.com")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got listCommitsResponse
	decodeJSON(t, resp.Body, &got)
	require.Len(t, got.Commits, 1)
	assert.Equal(t, alice.ID, got.Commits[0].ID)
	assert.Equal(t, "alice@example.com", got.Commits[0].Author)
}

func TestCommits_List_FilterBySinceAndUntil_ReturnsOnlyCommitsInRange(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)

	now := time.Now().UTC()
	past := now.Add(-2 * time.Hour)
	future := now.Add(1 * time.Hour)

	// create commits at different times
	pastResp := doPost(t, base+"/repos/"+repo.ID+"/commits", map[string]any{
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Past",
		"author":  "test@example.com",
	})
	var pastCommit commitResponse
	decodeJSON(t, pastResp.Body, &pastCommit)
	pastResp.Body.Close()

	recentResp := doPost(t, base+"/repos/"+repo.ID+"/commits", map[string]any{
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Recent",
		"author":  "test@example.com",
	})
	var recentCommit commitResponse
	decodeJSON(t, recentResp.Body, &recentCommit)
	recentResp.Body.Close()

	// filter by since and until (should get commits in that range)
	resp := doGet(t, fmt.Sprintf("%s/repos/%s/commits?since=%s&until=%s",
		base, repo.ID,
		past.Format(time.RFC3339),
		future.Format(time.RFC3339)))
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got listCommitsResponse
	decodeJSON(t, resp.Body, &got)
	assert.GreaterOrEqual(t, len(got.Commits), 1)
}

func TestCommits_List_Pagination_CursorWorksCorrectly(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)

	// create 5 commits
	for i := 0; i < 5; i++ {
		createCommit(t, base, repo.ID, []string{})
		time.Sleep(10 * time.Millisecond)
	}

	// page 1: limit=2
	resp1 := doGet(t, base+"/repos/"+repo.ID+"/commits?limit=2")
	defer resp1.Body.Close()
	var page1 listCommitsResponse
	decodeJSON(t, resp1.Body, &page1)

	require.Len(t, page1.Commits, 2)
	require.NotNil(t, page1.NextCursor)

	// page 2: use cursor
	resp2 := doGet(t, fmt.Sprintf("%s/repos/%s/commits?limit=2&cursor=%s",
		base, repo.ID, *page1.NextCursor))
	defer resp2.Body.Close()
	var page2 listCommitsResponse
	decodeJSON(t, resp2.Body, &page2)

	require.Len(t, page2.Commits, 2)

	// no duplicates
	seen := make(map[string]int)
	for _, c := range append(page1.Commits, page2.Commits...) {
		seen[c.ID]++
	}
	for id, count := range seen {
		assert.Equal(t, 1, count, "commit %s appeared more than once", id)
	}
}

// GET /v1/repos/:repoID/commits/:commitID/parents

func TestCommits_GetParents_RootCommit_ReturnsEmptyArray(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)
	commit := createCommit(t, base, repo.ID, []string{})

	resp := doGet(t, base+"/repos/"+repo.ID+"/commits/"+commit.ID+"/parents")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got parentsResponse
	decodeJSON(t, resp.Body, &got)
	assert.Empty(t, got.Parents)
}

func TestCommits_GetParents_RegularCommit_ReturnsOneParent(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)
	parent := createCommit(t, base, repo.ID, []string{})
	commit := createCommit(t, base, repo.ID, []string{parent.ID})

	resp := doGet(t, base+"/repos/"+repo.ID+"/commits/"+commit.ID+"/parents")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got parentsResponse
	decodeJSON(t, resp.Body, &got)
	require.Len(t, got.Parents, 1)
	assert.Equal(t, parent.ID, got.Parents[0].ID)
	assert.Equal(t, "test commit", got.Parents[0].Message)
}

func TestCommits_GetParents_CommitNotFound_Returns404(t *testing.T) {
	base := startCommitServer(t)
	repo := createRepo(t, base)

	resp := doGet(t, base+"/repos/"+repo.ID+"/commits/commit_nonexistent/parents")
	defer resp.Body.Close()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "commit_not_found", got.Error)
}

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

func startBranchServer(t *testing.T) string {
	t.Helper()
	pool, cleanup := testhelper.SetupPostgres(t)
	t.Cleanup(cleanup)
	return startE2EServer(t, pool)
}

// POST /v1/repos/:repoID/branches

func TestBranches_Create_ValidInput_Returns201WithFields(t *testing.T) {
	base := startBranchServer(t)
	repo := createRepo(t, base)
	commit := createCommit(t, base, repo.ID, []string{})

	resp := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"name":             "feature-x",
		"source_commit_id": commit.ID,
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var got branchResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "feature-x", got.Name)
	assert.Equal(t, repo.ID, got.RepoID)
	assert.Equal(t, commit.ID, got.CommitID)
	assert.False(t, got.CreatedAt.IsZero())
}

func TestBranches_Create_MissingName_Returns400WithInvalidRequest(t *testing.T) {
	base := startBranchServer(t)
	repo := createRepo(t, base)
	commit := createCommit(t, base, repo.ID, []string{})

	resp := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"source_commit_id": commit.ID,
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "invalid_request", got.Error)
}

func TestBranches_Create_MissingSourceCommitID_Returns400WithInvalidRequest(t *testing.T) {
	base := startBranchServer(t)
	repo := createRepo(t, base)

	resp := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"name": "feature-x",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "invalid_request", got.Error)
}

func TestBranches_Create_RepoNotFound_Returns404WithRepoNotFound(t *testing.T) {
	base := startBranchServer(t)

	resp := doPost(t, base+"/repos/repo_nonexistent/branches", map[string]string{
		"name":             "main",
		"source_commit_id": "commit_xyz",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "repo_not_found", got.Error)
}

func TestBranches_Create_CommitNotFound_Returns404WithCommitNotFound(t *testing.T) {
	base := startBranchServer(t)
	repo := createRepo(t, base)

	resp := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"name":             "main",
		"source_commit_id": "commit_nonexistent",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "commit_not_found", got.Error)
}

func TestBranches_Create_CommitNotInRepo_Returns404WithCommitNotFound(t *testing.T) {
	base := startBranchServer(t)
	repo1 := createRepo(t, base)
	repo2 := createRepo(t, base)

	// create commit in repo1
	commit := createCommit(t, base, repo1.ID, []string{})

	// try to create branch in repo2 using commit from repo1
	resp := doPost(t, base+"/repos/"+repo2.ID+"/branches", map[string]string{
		"name":             "main",
		"source_commit_id": commit.ID,
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "commit_not_found", got.Error, "should not allow cross-repo commit references")
}

func TestBranches_Create_DuplicateName_Returns409WithBranchAlreadyExists(t *testing.T) {
	base := startBranchServer(t)
	repo := createRepo(t, base)
	commit := createCommit(t, base, repo.ID, []string{})

	// create first branch
	resp1 := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"name":             "feature-x",
		"source_commit_id": commit.ID,
	})
	resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode)

	// try to create duplicate
	resp2 := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"name":             "feature-x",
		"source_commit_id": commit.ID,
	})
	defer resp2.Body.Close()

	require.Equal(t, http.StatusConflict, resp2.StatusCode)
	var got errResponse
	decodeJSON(t, resp2.Body, &got)
	assert.Equal(t, "branch_already_exists", got.Error)
}

// GET /v1/repos/:repoID/branches/:name

func TestBranches_Get_Exists_Returns200WithCorrectFields(t *testing.T) {
	base := startBranchServer(t)
	repo := createRepo(t, base)
	commit := createCommit(t, base, repo.ID, []string{})

	// create branch
	createResp := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"name":             "feature-x",
		"source_commit_id": commit.ID,
	})
	createResp.Body.Close()

	// get the branch
	getResp := doGet(t, base+"/repos/"+repo.ID+"/branches/feature-x")
	defer getResp.Body.Close()

	require.Equal(t, http.StatusOK, getResp.StatusCode)
	var got branchResponse
	decodeJSON(t, getResp.Body, &got)
	assert.Equal(t, "feature-x", got.Name)
	assert.Equal(t, repo.ID, got.RepoID)
	assert.Equal(t, commit.ID, got.CommitID)
	assert.False(t, got.CreatedAt.IsZero())
}

func TestBranches_Get_NotFound_Returns404WithBranchNotFound(t *testing.T) {
	base := startBranchServer(t)
	repo := createRepo(t, base)

	resp := doGet(t, base+"/repos/"+repo.ID+"/branches/nonexistent")
	defer resp.Body.Close()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "branch_not_found", got.Error)
}

// GET /v1/repos/:repoID/branches

func TestBranches_List_NoParams_Returns200(t *testing.T) {
	base := startBranchServer(t)
	repo := createRepo(t, base)

	resp := doGet(t, base+"/repos/"+repo.ID+"/branches")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestBranches_List_InvalidLimit_Returns400WithInvalidRequest(t *testing.T) {
	base := startBranchServer(t)
	repo := createRepo(t, base)

	resp := doGet(t, base+"/repos/"+repo.ID+"/branches?limit=abc")
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "invalid_request", got.Error)
}

func TestBranches_List_Pagination_CursorWorksCorrectly(t *testing.T) {
	base := startBranchServer(t)
	repo := createRepo(t, base)
	commit := createCommit(t, base, repo.ID, []string{})

	// create 3 branches
	for i := 0; i < 3; i++ {
		resp := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
			"name":             fmt.Sprintf("branch-%d", i),
			"source_commit_id": commit.ID,
		})
		resp.Body.Close()
		time.Sleep(10 * time.Millisecond) // ensure different created_at
	}

	// page 1: limit=2
	resp1 := doGet(t, base+"/repos/"+repo.ID+"/branches?limit=2")
	defer resp1.Body.Close()
	var page1 listBranchesResponse
	decodeJSON(t, resp1.Body, &page1)

	require.Len(t, page1.Branches, 2)
	require.NotNil(t, page1.NextCursor)
	require.NotEmpty(t, *page1.NextCursor)

	// page 2: use cursor
	resp2 := doGet(
		t,
		fmt.Sprintf("%s/repos/%s/branches?limit=2&cursor=%s", base, repo.ID, *page1.NextCursor),
	)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var page2 listBranchesResponse
	decodeJSON(t, resp2.Body, &page2)
	require.NotEmpty(t, page2.Branches)

	// no duplicates across pages
	seen := make(map[string]int)
	for _, b := range append(page1.Branches, page2.Branches...) {
		seen[b.Name]++
	}
	for name, count := range seen {
		assert.Equal(t, 1, count, "branch %s appeared more than once across pages", name)
	}
}

// PATCH /v1/repos/:repoID/branches/:name

func TestBranches_Advance_ValidInput_Returns200AndBranchPointsToNewCommit(t *testing.T) {
	base := startBranchServer(t)
	repo := createRepo(t, base)
	commit1 := createCommit(t, base, repo.ID, []string{})
	commit2 := createCommit(t, base, repo.ID, []string{commit1.ID})

	// create branch at commit1
	createResp := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"name":             "main",
		"source_commit_id": commit1.ID,
	})
	createResp.Body.Close()

	// advance to commit2
	advanceResp := doPatch(t, base+"/repos/"+repo.ID+"/branches/main", map[string]string{
		"commit_id":          commit2.ID,
		"expected_commit_id": commit1.ID,
	})
	defer advanceResp.Body.Close()

	require.Equal(t, http.StatusOK, advanceResp.StatusCode)

	var got branchResponse
	decodeJSON(t, advanceResp.Body, &got)
	assert.Equal(t, commit2.ID, got.CommitID)
}

func TestBranches_Advance_StaleExpectedCommitID_Returns409WithCurrentHead(t *testing.T) {
	base := startBranchServer(t)
	repo := createRepo(t, base)
	commit1 := createCommit(t, base, repo.ID, []string{})
	commit2 := createCommit(t, base, repo.ID, []string{commit1.ID})
	commit3 := createCommit(t, base, repo.ID, []string{commit2.ID})

	// create branch at commit2
	createResp := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"name":             "main",
		"source_commit_id": commit2.ID,
	})
	createResp.Body.Close()

	// try to advance with stale expected (commit1 instead of commit2)
	advanceResp := doPatch(t, base+"/repos/"+repo.ID+"/branches/main", map[string]string{
		"commit_id":          commit3.ID,
		"expected_commit_id": commit1.ID, // stale!
	})
	defer advanceResp.Body.Close()

	require.Equal(t, http.StatusConflict, advanceResp.StatusCode)

	var got struct {
		Error       string  `json:"error"`
		Message     string  `json:"message"`
		CurrentHead *string `json:"current_head"`
	}
	decodeJSON(t, advanceResp.Body, &got)
	assert.Equal(t, "branch_conflict", got.Error)
	require.NotNil(t, got.CurrentHead)
	assert.Equal(t, commit2.ID, *got.CurrentHead)
}

func TestBranches_Advance_BranchNotFound_Returns404(t *testing.T) {
	base := startBranchServer(t)
	repo := createRepo(t, base)
	commit := createCommit(t, base, repo.ID, []string{})

	resp := doPatch(t, base+"/repos/"+repo.ID+"/branches/nonexistent", map[string]string{
		"commit_id":          commit.ID,
		"expected_commit_id": "commit_old",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "branch_not_found", got.Error)
}

func TestBranches_Advance_CommitNotFound_Returns404(t *testing.T) {
	base := startBranchServer(t)
	repo := createRepo(t, base)
	commit1 := createCommit(t, base, repo.ID, []string{})

	// create branch
	createResp := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"name":             "main",
		"source_commit_id": commit1.ID,
	})
	createResp.Body.Close()

	// try to advance to nonexistent commit
	advanceResp := doPatch(t, base+"/repos/"+repo.ID+"/branches/main", map[string]string{
		"commit_id":          "commit_nonexistent",
		"expected_commit_id": commit1.ID,
	})
	defer advanceResp.Body.Close()

	require.Equal(t, http.StatusNotFound, advanceResp.StatusCode)
	var got errResponse
	decodeJSON(t, advanceResp.Body, &got)
	assert.Equal(t, "commit_not_found", got.Error)
}

// DELETE /v1/repos/:repoID/branches/:name

func TestBranches_Delete_Exists_Returns204AndBranchIsGone(t *testing.T) {
	base := startBranchServer(t)
	repo := createRepo(t, base)
	commit := createCommit(t, base, repo.ID, []string{})

	// create branch
	createResp := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"name":             "feature-x",
		"source_commit_id": commit.ID,
	})
	createResp.Body.Close()

	// delete it
	deleteResp := doDelete(t, base+"/repos/"+repo.ID+"/branches/feature-x")
	defer deleteResp.Body.Close()

	assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	// verify it's gone
	getResp := doGet(t, base+"/repos/"+repo.ID+"/branches")
	defer getResp.Body.Close()
	var list listBranchesResponse
	decodeJSON(t, getResp.Body, &list)

	for _, b := range list.Branches {
		assert.NotEqual(t, "feature-x", b.Name, "deleted branch should not appear in list")
	}
}

func TestBranches_Delete_NotFound_Returns404(t *testing.T) {
	base := startBranchServer(t)
	repo := createRepo(t, base)

	resp := doDelete(t, base+"/repos/"+repo.ID+"/branches/nonexistent")
	defer resp.Body.Close()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "branch_not_found", got.Error)
}

func TestBranches_Delete_DefaultBranch_Returns409WithCannotDeleteDefaultBranch(t *testing.T) {
	base := startBranchServer(t)

	// create repo with "main" as default branch
	createRepoResp := doPost(t, base+"/repos", map[string]string{
		"name":           "e2e-" + uuid.New().String(),
		"default_branch": "main",
	})
	var repo repoResponse
	decodeJSON(t, createRepoResp.Body, &repo)
	createRepoResp.Body.Close()

	commit := createCommit(t, base, repo.ID, []string{})

	// create the main branch
	createBranchResp := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"name":             "main",
		"source_commit_id": commit.ID,
	})
	createBranchResp.Body.Close()

	// try to delete main (the default branch)
	deleteResp := doDelete(t, base+"/repos/"+repo.ID+"/branches/main")
	defer deleteResp.Body.Close()

	require.Equal(t, http.StatusConflict, deleteResp.StatusCode)
	var got errResponse
	decodeJSON(t, deleteResp.Body, &got)
	assert.Equal(t, "cannot_delete_default_branch", got.Error)
}

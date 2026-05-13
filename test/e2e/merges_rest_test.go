//go:build e2e

package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/testhelper"
)

func startMergeServer(t *testing.T) string {
	t.Helper()
	pool, cleanup := testhelper.SetupPostgres(t)
	t.Cleanup(cleanup)
	return startE2EServer(t, pool)
}

// POST /v1/repos/:repoID/merges

func TestMerges_Create_ValidInput_Returns201AndBranchPointsToMergeCommit(t *testing.T) {
	base := startMergeServer(t)
	repo := createRepo(t, base)

	// create two separate commit histories
	// main: root -> commit_main
	root := createCommit(t, base, repo.ID, []string{})
	time.Sleep(10 * time.Millisecond)
	commitMain := createCommit(t, base, repo.ID, []string{root.ID})

	// feature: root -> commit_feature
	time.Sleep(10 * time.Millisecond)
	commitFeature := createCommit(t, base, repo.ID, []string{root.ID})

	// create main branch pointing to commit_main
	createBranchResp := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"name":             "main",
		"source_commit_id": commitMain.ID,
	})
	createBranchResp.Body.Close()

	// create merge
	mergeResp := doPost(t, base+"/repos/"+repo.ID+"/merges", map[string]any{
		"parent_ids":           []string{commitFeature.ID, commitMain.ID},
		"target_branch":        "main",
		"expected_target_head": commitMain.ID,
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Merge feature into main",
		"author":  "alice@example.com",
	})
	defer mergeResp.Body.Close()

	require.Equal(t, http.StatusCreated, mergeResp.StatusCode)

	var mergeCommit commitResponse
	decodeJSON(t, mergeResp.Body, &mergeCommit)

	// verify merge commit has two parents
	require.Len(t, mergeCommit.ParentIDs, 2)
	assert.Contains(t, mergeCommit.ParentIDs, commitFeature.ID)
	assert.Contains(t, mergeCommit.ParentIDs, commitMain.ID)

	// verify main branch now points to merge commit
	branchResp := doGet(t, base+"/repos/"+repo.ID+"/branches")
	defer branchResp.Body.Close()

	var branches listBranchesResponse
	decodeJSON(t, branchResp.Body, &branches)

	var mainBranch *branchResponse
	for i := range branches.Branches {
		if branches.Branches[i].Name == "main" {
			mainBranch = &branches.Branches[i]
			break
		}
	}
	require.NotNil(t, mainBranch, "main branch should exist")
	assert.Equal(t, mergeCommit.ID, mainBranch.CommitID, "main should point to merge commit")
}

func TestMerges_Create_NotExactlyTwoParents_Returns400WithInvalidRequest(t *testing.T) {
	base := startMergeServer(t)
	repo := createRepo(t, base)
	commit := createCommit(t, base, repo.ID, []string{})

	// create branch
	createBranchResp := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"name":             "main",
		"source_commit_id": commit.ID,
	})
	createBranchResp.Body.Close()

	// try merge with only one parent
	resp := doPost(t, base+"/repos/"+repo.ID+"/merges", map[string]any{
		"parent_ids":           []string{"commit_1"},
		"target_branch":        "main",
		"expected_target_head": commit.ID,
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Merge",
		"author":  "test@example.com",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "invalid_request", got.Error)
	assert.Contains(t, got.Message, "two")
}

func TestMerges_Create_TargetBranchNotFound_Returns404WithBranchNotFound(t *testing.T) {
	base := startMergeServer(t)
	repo := createRepo(t, base)
	commit1 := createCommit(t, base, repo.ID, []string{})
	commit2 := createCommit(t, base, repo.ID, []string{})

	resp := doPost(t, base+"/repos/"+repo.ID+"/merges", map[string]any{
		"parent_ids":           []string{commit1.ID, commit2.ID},
		"target_branch":        "nonexistent",
		"expected_target_head": "commit_old",
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Merge",
		"author":  "test@example.com",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "branch_not_found", got.Error)
}

func TestMerges_Create_InvalidParent_Returns422WithInvalidParent(t *testing.T) {
	base := startMergeServer(t)
	repo := createRepo(t, base)
	commit := createCommit(t, base, repo.ID, []string{})

	// create branch
	createBranchResp := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"name":             "main",
		"source_commit_id": commit.ID,
	})
	createBranchResp.Body.Close()

	// try merge with invalid parent
	resp := doPost(t, base+"/repos/"+repo.ID+"/merges", map[string]any{
		"parent_ids":           []string{commit.ID, "commit_nonexistent"},
		"target_branch":        "main",
		"expected_target_head": commit.ID,
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Merge",
		"author":  "test@example.com",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "invalid_parent", got.Error)
}

func TestMerges_Create_StaleExpectedTargetHead_Returns409WithCurrentHead(t *testing.T) {
	base := startMergeServer(t)
	repo := createRepo(t, base)

	// create commits
	commit1 := createCommit(t, base, repo.ID, []string{})
	time.Sleep(10 * time.Millisecond)
	commit2 := createCommit(t, base, repo.ID, []string{commit1.ID})
	time.Sleep(10 * time.Millisecond)
	commit3 := createCommit(t, base, repo.ID, []string{commit2.ID})

	// create branch pointing to commit2
	createBranchResp := doPost(t, base+"/repos/"+repo.ID+"/branches", map[string]string{
		"name":             "main",
		"source_commit_id": commit2.ID,
	})
	createBranchResp.Body.Close()

	// try to merge with stale expected head (commit1 instead of commit2)
	resp := doPost(t, base+"/repos/"+repo.ID+"/merges", map[string]any{
		"parent_ids":           []string{commit3.ID, commit2.ID},
		"target_branch":        "main",
		"expected_target_head": commit1.ID, // stale!
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Merge",
		"author":  "test@example.com",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusConflict, resp.StatusCode)

	var got struct {
		Error       string  `json:"error"`
		Message     string  `json:"message"`
		CurrentHead *string `json:"current_head"`
	}
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "stale_merge_target", got.Error)
	require.NotNil(t, got.CurrentHead)
	assert.Equal(t, commit2.ID, *got.CurrentHead)
}

func TestMerges_Create_MissingRequiredFields_Returns400(t *testing.T) {
	base := startMergeServer(t)
	repo := createRepo(t, base)

	cases := []struct {
		name string
		body map[string]any
	}{
		{
			name: "missing parent_ids",
			body: map[string]any{
				"target_branch":        "main",
				"expected_target_head": "commit_old",
				"data_pointer": map[string]string{
					"type":     "db",
					"location": "test/fixture",
				},
				"message": "Merge",
				"author":  "test@example.com",
			},
		},
		{
			name: "missing target_branch",
			body: map[string]any{
				"parent_ids":           []string{"commit_1", "commit_2"},
				"expected_target_head": "commit_old",
				"data_pointer": map[string]string{
					"type":     "db",
					"location": "test/fixture",
				},
				"message": "Merge",
				"author":  "test@example.com",
			},
		},
		{
			name: "missing expected_target_head",
			body: map[string]any{
				"parent_ids":    []string{"commit_1", "commit_2"},
				"target_branch": "main",
				"data_pointer": map[string]string{
					"type":     "db",
					"location": "test/fixture",
				},
				"message": "Merge",
				"author":  "test@example.com",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := doPost(t, base+"/repos/"+repo.ID+"/merges", tc.body)
			defer resp.Body.Close()

			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
			var got errResponse
			decodeJSON(t, resp.Body, &got)
			assert.Equal(t, "invalid_request", got.Error)
		})
	}
}

func TestMerges_Create_RepoNotFound_Returns404(t *testing.T) {
	base := startMergeServer(t)

	resp := doPost(t, base+"/repos/repo_nonexistent/merges", map[string]any{
		"parent_ids":           []string{"commit_1", "commit_2"},
		"target_branch":        "main",
		"expected_target_head": "commit_old",
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "Merge",
		"author":  "test@example.com",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "repo_not_found", got.Error)
}

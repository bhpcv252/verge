//go:build e2e && outbox

package e2e

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Commit

func TestMatrix_REST_CommitRoundTrip(t *testing.T) {
	for _, tier := range allTiers {
		tier := tier
		t.Run(tier.name, func(t *testing.T) {
			t.Parallel()
			env := startServerWithConfig(t, tier)
			repo := createRepo(t, env.restBase)

			// create a root commit
			created := createCommit(t, env.restBase, repo.ID, []string{})

			// wait for the outbox worker to propagate (no-op for postgres-only)
			env.waitForOutbox(t, 30*time.Second)

			resp := doGet(t, env.restBase+"/repos/"+repo.ID+"/commits/"+created.ID)
			defer resp.Body.Close()

			require.Equal(t, http.StatusOK, resp.StatusCode)

			var got commitResponse
			decodeJSON(t, resp.Body, &got)

			assert.Equal(t, created.ID, got.ID)
			assert.Equal(t, repo.ID, got.RepoID)
			assert.Equal(t, "test commit", got.Message)
			assert.Equal(t, "test@example.com", got.Author)
			assert.Empty(t, got.ParentIDs)
			assert.Equal(t, "db", got.DataPointer.Type)
			assert.Equal(t, "test/fixture", got.DataPointer.Location)
			assert.False(t, got.Timestamp.IsZero())
		})
	}
}

func TestMatrix_REST_CommitWithParent(t *testing.T) {
	for _, tier := range allTiers {
		tier := tier
		t.Run(tier.name, func(t *testing.T) {
			t.Parallel()
			env := startServerWithConfig(t, tier)
			repo := createRepo(t, env.restBase)

			parent := createCommit(t, env.restBase, repo.ID, []string{})
			child := createCommit(t, env.restBase, repo.ID, []string{parent.ID})

			env.waitForOutbox(t, 30*time.Second)

			// verify child commit fields
			commitResp := doGet(t, env.restBase+"/repos/"+repo.ID+"/commits/"+child.ID)
			defer commitResp.Body.Close()
			require.Equal(t, http.StatusOK, commitResp.StatusCode)

			var gotCommit commitResponse
			decodeJSON(t, commitResp.Body, &gotCommit)
			require.Len(t, gotCommit.ParentIDs, 1)
			assert.Equal(t, parent.ID, gotCommit.ParentIDs[0])

			// verify parents endpoint
			parentsResp := doGet(t, env.restBase+"/repos/"+repo.ID+"/commits/"+child.ID+"/parents")
			defer parentsResp.Body.Close()
			require.Equal(t, http.StatusOK, parentsResp.StatusCode)

			var gotParents parentsResponse
			decodeJSON(t, parentsResp.Body, &gotParents)
			require.Len(t, gotParents.Parents, 1)
			assert.Equal(t, parent.ID, gotParents.Parents[0].ID)
		})
	}
}

// Branch head

func TestMatrix_REST_BranchHeadAfterAdvance(t *testing.T) {
	for _, tier := range allTiers {
		tier := tier
		t.Run(tier.name, func(t *testing.T) {
			t.Parallel()
			env := startServerWithConfig(t, tier)
			repo := createRepo(t, env.restBase)

			commit1 := createCommit(t, env.restBase, repo.ID, []string{})
			commit2 := createCommit(t, env.restBase, repo.ID, []string{commit1.ID})

			createBranch(t, env.restBase, repo.ID, "main", commit1.ID)

			// advance the branch
			advResp := doPatch(
				t,
				env.restBase+"/repos/"+repo.ID+"/branches/main",
				map[string]string{
					"commit_id":          commit2.ID,
					"expected_commit_id": commit1.ID,
				},
			)
			advResp.Body.Close()
			require.Equal(t, http.StatusOK, advResp.StatusCode)

			env.waitForOutbox(t, 30*time.Second)

			// read back the branch head
			getResp := doGet(t, env.restBase+"/repos/"+repo.ID+"/branches/main")
			defer getResp.Body.Close()
			require.Equal(t, http.StatusOK, getResp.StatusCode)

			var got branchResponse
			decodeJSON(t, getResp.Body, &got)
			assert.Equal(t, "main", got.Name)
			assert.Equal(t, commit2.ID, got.CommitID,
				"branch head must point to the new commit after advance")
		})
	}
}

// Merge flow

func TestMatrix_REST_MergeFlow(t *testing.T) {
	for _, tier := range allTiers {
		tier := tier
		t.Run(tier.name, func(t *testing.T) {
			t.Parallel()
			env := startServerWithConfig(t, tier)
			repo := createRepo(t, env.restBase)

			root := createCommit(t, env.restBase, repo.ID, []string{})
			time.Sleep(10 * time.Millisecond)
			commitMain := createCommit(t, env.restBase, repo.ID, []string{root.ID})

			time.Sleep(10 * time.Millisecond)
			commitFeature := createCommit(t, env.restBase, repo.ID, []string{root.ID})

			createBranch(t, env.restBase, repo.ID, "main", commitMain.ID)

			mergeResp := doPost(t, env.restBase+"/repos/"+repo.ID+"/merges", map[string]any{
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

			env.waitForOutbox(t, 30*time.Second)

			// merge commit must have both parents
			require.Len(t, mergeCommit.ParentIDs, 2)
			assert.Contains(t, mergeCommit.ParentIDs, commitFeature.ID)
			assert.Contains(t, mergeCommit.ParentIDs, commitMain.ID)

			// branch must point to the merge commit
			branchResp := doGet(t, env.restBase+"/repos/"+repo.ID+"/branches/main")
			defer branchResp.Body.Close()
			require.Equal(t, http.StatusOK, branchResp.StatusCode)

			var branch branchResponse
			decodeJSON(t, branchResp.Body, &branch)
			assert.Equal(t, mergeCommit.ID, branch.CommitID,
				"main must point to the merge commit after merge")
		})
	}
}

// DAG traversal

func TestMatrix_REST_CommitListDAG(t *testing.T) {
	for _, tier := range allTiers {
		tier := tier
		t.Run(tier.name, func(t *testing.T) {
			t.Parallel()
			env := startServerWithConfig(t, tier)
			repo := createRepo(t, env.restBase)

			root := createCommit(t, env.restBase, repo.ID, []string{})
			time.Sleep(10 * time.Millisecond)
			c1 := createCommit(t, env.restBase, repo.ID, []string{root.ID})
			time.Sleep(10 * time.Millisecond)
			c2 := createCommit(t, env.restBase, repo.ID, []string{c1.ID})

			createBranch(t, env.restBase, repo.ID, "main", c2.ID)

			// orphan commit (not reachable from main)
			orphan := createCommit(t, env.restBase, repo.ID, []string{})

			// wait for outbox propagation (needed for Neo4j tier)
			env.waitForOutbox(t, 30*time.Second)

			resp := doGet(t, env.restBase+"/repos/"+repo.ID+"/commits?traversal=dag&branch=main")
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			var got listCommitsResponse
			decodeJSON(t, resp.Body, &got)

			ids := make(map[string]bool, len(got.Commits))
			for _, c := range got.Commits {
				ids[c.ID] = true
			}

			assert.True(t, ids[c2.ID], "tip should be in DAG results")
			assert.True(t, ids[c1.ID], "c1 should be in DAG results")
			assert.True(t, ids[root.ID], "root should be in DAG results")
			assert.False(t, ids[orphan.ID],
				"orphan commit must not appear in DAG traversal from main")
		})
	}
}

func TestMatrix_REST_CommitListFlat(t *testing.T) {
	for _, tier := range allTiers {
		tier := tier
		t.Run(tier.name, func(t *testing.T) {
			t.Parallel()
			env := startServerWithConfig(t, tier)
			repo := createRepo(t, env.restBase)

			c1 := createCommit(t, env.restBase, repo.ID, []string{})
			time.Sleep(15 * time.Millisecond)
			c2 := createCommit(t, env.restBase, repo.ID, []string{})
			time.Sleep(15 * time.Millisecond)
			c3 := createCommit(t, env.restBase, repo.ID, []string{})

			env.waitForOutbox(t, 30*time.Second)

			resp := doGet(t, env.restBase+"/repos/"+repo.ID+"/commits?traversal=flat")
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			var got listCommitsResponse
			decodeJSON(t, resp.Body, &got)
			require.Len(t, got.Commits, 3)

			// newest first
			assert.Equal(t, c3.ID, got.Commits[0].ID)
			assert.Equal(t, c2.ID, got.Commits[1].ID)
			assert.Equal(t, c1.ID, got.Commits[2].ID)
		})
	}
}

// Pagination

func TestMatrix_REST_PaginationNoDuplicates(t *testing.T) {
	for _, tier := range allTiers {
		tier := tier
		t.Run(tier.name, func(t *testing.T) {
			t.Parallel()
			env := startServerWithConfig(t, tier)
			repo := createRepo(t, env.restBase)

			for i := 0; i < 5; i++ {
				createCommit(t, env.restBase, repo.ID, []string{})
				time.Sleep(10 * time.Millisecond)
			}

			env.waitForOutbox(t, 30*time.Second)

			// page 1
			resp1 := doGet(t, env.restBase+"/repos/"+repo.ID+"/commits?limit=2")
			defer resp1.Body.Close()
			require.Equal(t, http.StatusOK, resp1.StatusCode)

			var page1 listCommitsResponse
			decodeJSON(t, resp1.Body, &page1)
			require.Len(t, page1.Commits, 2)
			require.NotNil(t, page1.NextCursor, "first page must have a cursor")
			require.NotEmpty(t, *page1.NextCursor)

			// page 2
			resp2 := doGet(t, fmt.Sprintf(
				"%s/repos/%s/commits?limit=2&cursor=%s",
				env.restBase, repo.ID, *page1.NextCursor,
			))
			defer resp2.Body.Close()
			require.Equal(t, http.StatusOK, resp2.StatusCode)

			var page2 listCommitsResponse
			decodeJSON(t, resp2.Body, &page2)
			require.Len(t, page2.Commits, 2)

			// no duplicates across both pages
			seen := make(map[string]int)
			for _, c := range append(page1.Commits, page2.Commits...) {
				seen[c.ID]++
			}
			for id, count := range seen {
				assert.Equal(t, 1, count,
					"commit %s appeared on more than one page in tier %s", id, tier.name)
			}
		})
	}
}

// Optimistic locking

func TestMatrix_REST_StaleAdvanceReturns409(t *testing.T) {
	for _, tier := range allTiers {
		tier := tier
		t.Run(tier.name, func(t *testing.T) {
			t.Parallel()
			env := startServerWithConfig(t, tier)
			repo := createRepo(t, env.restBase)

			commit1 := createCommit(t, env.restBase, repo.ID, []string{})
			commit2 := createCommit(t, env.restBase, repo.ID, []string{commit1.ID})
			commit3 := createCommit(t, env.restBase, repo.ID, []string{commit2.ID})

			createBranch(t, env.restBase, repo.ID, "main", commit2.ID)

			env.waitForOutbox(t, 30*time.Second)

			// advance with stale expected (commit1 instead of commit2)
			advResp := doPatch(
				t,
				env.restBase+"/repos/"+repo.ID+"/branches/main",
				map[string]string{
					"commit_id":          commit3.ID,
					"expected_commit_id": commit1.ID, // stale
				},
			)
			defer advResp.Body.Close()

			require.Equal(t, http.StatusConflict, advResp.StatusCode)

			var got struct {
				Error       string  `json:"error"`
				CurrentHead *string `json:"current_head"`
			}
			decodeJSON(t, advResp.Body, &got)
			assert.Equal(t, "branch_conflict", got.Error)
			require.NotNil(t, got.CurrentHead)
			assert.Equal(t, commit2.ID, *got.CurrentHead,
				"current_head must be authoritative (from Postgres) on tier %s", tier.name)
		})
	}
}

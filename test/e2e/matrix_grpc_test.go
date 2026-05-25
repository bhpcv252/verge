//go:build e2e && outbox

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	vergev1 "github.com/bhpcv252/verge/api/proto/verge/v1"
)

// Commit

func TestMatrix_GRPC_CommitRoundTrip(t *testing.T) {
	for _, tier := range allTiers {
		tier := tier
		t.Run(tier.name, func(t *testing.T) {
			t.Parallel()
			env := startServerWithConfig(t, tier)
			ctx := context.Background()

			repo := grpcCreateRepo(t, env.grpc)
			created := grpcCreateCommit(t, env.grpc, repo.Id, []string{})

			env.waitForOutbox(t, 5*time.Second)

			got, err := env.grpc.commit.GetCommit(ctx, &vergev1.GetCommitRequest{
				RepoId:   repo.Id,
				CommitId: created.Id,
			})
			require.NoError(t, err)

			assert.Equal(t, created.Id, got.Id)
			assert.Equal(t, repo.Id, got.RepoId)
			assert.Equal(t, "test commit", got.Message)
			assert.Equal(t, "test@example.com", got.Author)
			assert.Empty(t, got.ParentIds)
			assert.NotEmpty(t, got.Timestamp)
		})
	}
}

func TestMatrix_GRPC_CommitWithParent(t *testing.T) {
	for _, tier := range allTiers {
		tier := tier
		t.Run(tier.name, func(t *testing.T) {
			t.Parallel()
			env := startServerWithConfig(t, tier)
			ctx := context.Background()

			repo := grpcCreateRepo(t, env.grpc)
			parent := grpcCreateCommit(t, env.grpc, repo.Id, []string{})
			child := grpcCreateCommit(t, env.grpc, repo.Id, []string{parent.Id})

			env.waitForOutbox(t, 5*time.Second)

			// GetCommit must show parent linkage
			got, err := env.grpc.commit.GetCommit(ctx, &vergev1.GetCommitRequest{
				RepoId:   repo.Id,
				CommitId: child.Id,
			})
			require.NoError(t, err)
			require.Len(t, got.ParentIds, 1)
			assert.Equal(t, parent.Id, got.ParentIds[0])

			// GetParents must return the parent commit
			parents, err := env.grpc.commit.GetParents(ctx, &vergev1.GetParentsRequest{
				RepoId:   repo.Id,
				CommitId: child.Id,
			})
			require.NoError(t, err)
			require.Len(t, parents.Parents, 1)
			assert.Equal(t, parent.Id, parents.Parents[0].Id)
		})
	}
}

// Branch head

func TestMatrix_GRPC_BranchHeadAfterAdvance(t *testing.T) {
	for _, tier := range allTiers {
		tier := tier
		t.Run(tier.name, func(t *testing.T) {
			t.Parallel()
			env := startServerWithConfig(t, tier)
			ctx := context.Background()

			repo := grpcCreateRepo(t, env.grpc)
			commit1 := grpcCreateCommit(t, env.grpc, repo.Id, []string{})
			commit2 := grpcCreateCommit(t, env.grpc, repo.Id, []string{commit1.Id})

			branchName := uniqueGRPCBranchName()
			grpcCreateBranch(t, env.grpc, repo.Id, branchName, commit1.Id)

			_, err := env.grpc.branch.AdvanceBranch(ctx, &vergev1.AdvanceBranchRequest{
				RepoId:           repo.Id,
				Name:             branchName,
				CommitId:         commit2.Id,
				ExpectedCommitId: commit1.Id,
			})
			require.NoError(t, err)

			env.waitForOutbox(t, 5*time.Second)

			got, err := env.grpc.branch.GetBranch(ctx, &vergev1.GetBranchRequest{
				RepoId: repo.Id,
				Name:   branchName,
			})
			require.NoError(t, err)
			assert.Equal(t, commit2.Id, got.CommitId,
				"branch head must reflect the new commit on tier %s", tier.name)
		})
	}
}

// Merge flow

func TestMatrix_GRPC_MergeFlow(t *testing.T) {
	for _, tier := range allTiers {
		tier := tier
		t.Run(tier.name, func(t *testing.T) {
			t.Parallel()
			env := startServerWithConfig(t, tier)
			ctx := context.Background()

			repo := grpcCreateRepo(t, env.grpc)

			root := grpcCreateCommit(t, env.grpc, repo.Id, []string{})
			time.Sleep(10 * time.Millisecond)
			commitMain := grpcCreateCommit(t, env.grpc, repo.Id, []string{root.Id})
			time.Sleep(10 * time.Millisecond)
			commitFeature := grpcCreateCommit(t, env.grpc, repo.Id, []string{root.Id})

			grpcCreateBranch(t, env.grpc, repo.Id, "main", commitMain.Id)

			mergeResp, err := env.grpc.merge.CreateMerge(ctx, &vergev1.CreateMergeRequest{
				RepoId:             repo.Id,
				ParentIds:          []string{commitFeature.Id, commitMain.Id},
				TargetBranch:       "main",
				ExpectedTargetHead: commitMain.Id,
				DataPointer: &vergev1.DataPointer{
					Type:     "db",
					Location: "test/fixture",
				},
				Message: "Merge feature into main",
				Author:  "alice@example.com",
			})
			require.NoError(t, err)

			env.waitForOutbox(t, 5*time.Second)

			// merge commit has both parents
			require.Len(t, mergeResp.ParentIds, 2)
			assert.Contains(t, mergeResp.ParentIds, commitFeature.Id)
			assert.Contains(t, mergeResp.ParentIds, commitMain.Id)

			// main now points to the merge commit
			branch, err := env.grpc.branch.GetBranch(ctx, &vergev1.GetBranchRequest{
				RepoId: repo.Id,
				Name:   "main",
			})
			require.NoError(t, err)
			assert.Equal(t, mergeResp.Id, branch.CommitId,
				"main must point to the merge commit on tier %s", tier.name)
		})
	}
}

// DAG traversal

func TestMatrix_GRPC_CommitListDAG(t *testing.T) {
	for _, tier := range allTiers {
		tier := tier
		t.Run(tier.name, func(t *testing.T) {
			t.Parallel()
			env := startServerWithConfig(t, tier)
			ctx := context.Background()

			repo := grpcCreateRepo(t, env.grpc)

			root := grpcCreateCommit(t, env.grpc, repo.Id, []string{})
			time.Sleep(10 * time.Millisecond)
			c1 := grpcCreateCommit(t, env.grpc, repo.Id, []string{root.Id})
			time.Sleep(10 * time.Millisecond)
			c2 := grpcCreateCommit(t, env.grpc, repo.Id, []string{c1.Id})

			grpcCreateBranch(t, env.grpc, repo.Id, "main", c2.Id)

			// orphan: not reachable from main
			orphan := grpcCreateCommit(t, env.grpc, repo.Id, []string{})

			// wait for Neo4j propagation (no-op on tiers without Neo4j)
			env.waitForOutbox(t, 10*time.Second)

			resp, err := env.grpc.commit.ListCommits(ctx, &vergev1.ListCommitsRequest{
				RepoId: repo.Id,
				Branch: "main",
			})
			require.NoError(t, err)

			ids := make(map[string]bool, len(resp.Commits))
			for _, c := range resp.Commits {
				ids[c.Id] = true
			}

			assert.True(t, ids[c2.Id], "tip should be in results on tier %s", tier.name)
			assert.True(t, ids[c1.Id], "c1 should be in results on tier %s", tier.name)
			assert.True(t, ids[root.Id], "root should be in results on tier %s", tier.name)
			assert.False(t, ids[orphan.Id],
				"orphan must not appear in DAG traversal on tier %s", tier.name)
		})
	}
}

func TestMatrix_GRPC_CommitListByAuthor(t *testing.T) {
	for _, tier := range allTiers {
		tier := tier
		t.Run(tier.name, func(t *testing.T) {
			t.Parallel()
			env := startServerWithConfig(t, tier)
			ctx := context.Background()

			repo := grpcCreateRepo(t, env.grpc)

			_, err := env.grpc.commit.CreateCommit(ctx, &vergev1.CreateCommitRequest{
				RepoId:      repo.Id,
				ParentIds:   []string{},
				DataPointer: &vergev1.DataPointer{Type: "db", Location: "test/fixture"},
				Message:     "Alice's commit",
				Author:      "alice@example.com",
			})
			require.NoError(t, err)

			_, err = env.grpc.commit.CreateCommit(ctx, &vergev1.CreateCommitRequest{
				RepoId:      repo.Id,
				ParentIds:   []string{},
				DataPointer: &vergev1.DataPointer{Type: "db", Location: "test/fixture"},
				Message:     "Bob's commit",
				Author:      "bob@example.com",
			})
			require.NoError(t, err)

			env.waitForOutbox(t, 5*time.Second)

			resp, err := env.grpc.commit.ListCommits(ctx, &vergev1.ListCommitsRequest{
				RepoId: repo.Id,
				Author: "alice@example.com",
			})
			require.NoError(t, err)
			assert.NotEmpty(t, resp.Commits)
			for _, c := range resp.Commits {
				assert.Equal(t, "alice@example.com", c.Author,
					"author filter must be respected on tier %s", tier.name)
			}
		})
	}
}

// Pagination

func TestMatrix_GRPC_PaginationNoDuplicates(t *testing.T) {
	for _, tier := range allTiers {
		tier := tier
		t.Run(tier.name, func(t *testing.T) {
			t.Parallel()
			env := startServerWithConfig(t, tier)
			ctx := context.Background()

			repo := grpcCreateRepo(t, env.grpc)
			for i := 0; i < 5; i++ {
				grpcCreateCommit(t, env.grpc, repo.Id, []string{})
				time.Sleep(10 * time.Millisecond)
			}

			env.waitForOutbox(t, 5*time.Second)

			page1, err := env.grpc.commit.ListCommits(ctx, &vergev1.ListCommitsRequest{
				RepoId: repo.Id,
				Limit:  2,
			})
			require.NoError(t, err)
			require.Len(t, page1.Commits, 2)
			require.NotEmpty(t, page1.NextCursor)

			page2, err := env.grpc.commit.ListCommits(ctx, &vergev1.ListCommitsRequest{
				RepoId: repo.Id,
				Limit:  2,
				Cursor: page1.NextCursor,
			})
			require.NoError(t, err)
			require.Len(t, page2.Commits, 2)

			seen := make(map[string]int)
			for _, c := range append(page1.Commits, page2.Commits...) {
				seen[c.Id]++
			}
			for id, count := range seen {
				assert.Equal(t, 1, count,
					"commit %s duplicated across pages on tier %s", id, tier.name)
			}
		})
	}
}

// Optimistic locking

func TestMatrix_GRPC_StaleAdvanceReturnsAborted(t *testing.T) {
	for _, tier := range allTiers {
		tier := tier
		t.Run(tier.name, func(t *testing.T) {
			t.Parallel()
			env := startServerWithConfig(t, tier)
			ctx := context.Background()

			repo := grpcCreateRepo(t, env.grpc)
			commit1 := grpcCreateCommit(t, env.grpc, repo.Id, []string{})
			commit2 := grpcCreateCommit(t, env.grpc, repo.Id, []string{commit1.Id})
			commit3 := grpcCreateCommit(t, env.grpc, repo.Id, []string{commit2.Id})

			branchName := uniqueGRPCBranchName()
			grpcCreateBranch(t, env.grpc, repo.Id, branchName, commit2.Id)

			env.waitForOutbox(t, 5*time.Second)

			_, err := env.grpc.branch.AdvanceBranch(ctx, &vergev1.AdvanceBranchRequest{
				RepoId:           repo.Id,
				Name:             branchName,
				CommitId:         commit3.Id,
				ExpectedCommitId: commit1.Id, // stale
			})
			require.Error(t, err)
			assert.Equal(t, codes.Aborted, grpcCode(err),
				"stale advance must return Aborted on tier %s", tier.name)
		})
	}
}

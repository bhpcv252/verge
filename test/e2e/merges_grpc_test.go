//go:build e2e

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

// CreateMerge

func TestGRPC_CreateMerge_ValidInput_Returns201AndBranchPointsToMergeCommit(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)

	// main: root -> commitMain
	root := grpcCreateCommit(t, clients, repo.Id, []string{})
	time.Sleep(10 * time.Millisecond)
	commitMain := grpcCreateCommit(t, clients, repo.Id, []string{root.Id})

	// feature: root -> commitFeature
	time.Sleep(10 * time.Millisecond)
	commitFeature := grpcCreateCommit(t, clients, repo.Id, []string{root.Id})

	grpcCreateBranch(t, clients, repo.Id, "main", commitMain.Id)

	mergeResp, err := clients.merge.CreateMerge(ctx, &vergev1.CreateMergeRequest{
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
	require.Len(t, mergeResp.ParentIds, 2)
	assert.Contains(t, mergeResp.ParentIds, commitFeature.Id)
	assert.Contains(t, mergeResp.ParentIds, commitMain.Id)

	branchesResp, err := clients.branch.ListBranches(ctx, &vergev1.ListBranchesRequest{
		RepoId: repo.Id,
	})
	require.NoError(t, err)

	var mainBranch *vergev1.Branch
	for _, b := range branchesResp.Branches {
		if b.Name == "main" {
			mainBranch = b
			break
		}
	}
	require.NotNil(t, mainBranch, "main branch should exist")
	assert.Equal(t, mergeResp.Id, mainBranch.CommitId, "main should point to merge commit")
}

func TestGRPC_CreateMerge_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.merge.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		Message:            "Merge",
		Author:             "test@example.com",
		ParentIds:          []string{"commit_1", "commit_2"},
		ExpectedTargetHead: "commit_1",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateMerge_MissingMessage_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.merge.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_abc",
		Author:             "test@example.com",
		ParentIds:          []string{"commit_1", "commit_2"},
		ExpectedTargetHead: "commit_1",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateMerge_MissingAuthor_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.merge.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_abc",
		Message:            "Merge",
		ParentIds:          []string{"commit_1", "commit_2"},
		ExpectedTargetHead: "commit_1",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateMerge_MissingExpectedTargetHead_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.merge.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:       "repo_abc",
		Message:      "Merge",
		Author:       "test@example.com",
		ParentIds:    []string{"commit_1", "commit_2"},
		TargetBranch: "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateMerge_MissingTargetBranch_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.merge.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_abc",
		Message:            "Merge",
		Author:             "test@example.com",
		ParentIds:          []string{"commit_1", "commit_2"},
		ExpectedTargetHead: "commit_1",
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateMerge_NotExactlyTwoParents_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	commit := grpcCreateCommit(t, clients, repo.Id, []string{})
	grpcCreateBranch(t, clients, repo.Id, "main", commit.Id)

	_, err := clients.merge.CreateMerge(ctx, &vergev1.CreateMergeRequest{
		RepoId:             repo.Id,
		ParentIds:          []string{"commit_1"},
		TargetBranch:       "main",
		ExpectedTargetHead: commit.Id,
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Merge",
		Author:  "test@example.com",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateMerge_MissingDataPointer_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.merge.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_abc",
		Message:            "Merge",
		Author:             "test@example.com",
		ParentIds:          []string{"commit_1", "commit_2"},
		ExpectedTargetHead: "commit_1",
		TargetBranch:       "main",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateMerge_TargetBranchNotFound_ReturnsNotFound(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	commit1 := grpcCreateCommit(t, clients, repo.Id, []string{})
	commit2 := grpcCreateCommit(t, clients, repo.Id, []string{})

	_, err := clients.merge.CreateMerge(ctx, &vergev1.CreateMergeRequest{
		RepoId:             repo.Id,
		ParentIds:          []string{commit1.Id, commit2.Id},
		TargetBranch:       "nonexistent",
		ExpectedTargetHead: "commit_old",
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Merge",
		Author:  "test@example.com",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcCode(err))
}

func TestGRPC_CreateMerge_InvalidParent_ReturnsFailedPrecondition(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	commit := grpcCreateCommit(t, clients, repo.Id, []string{})
	grpcCreateBranch(t, clients, repo.Id, "main", commit.Id)

	_, err := clients.merge.CreateMerge(ctx, &vergev1.CreateMergeRequest{
		RepoId:             repo.Id,
		ParentIds:          []string{"commit_nonexistent1", "commit_nonexistent2"},
		TargetBranch:       "main",
		ExpectedTargetHead: commit.Id,
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Merge",
		Author:  "test@example.com",
	})

	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, grpcCode(err))
}

func TestGRPC_CreateMerge_StaleTargetHead_ReturnsAborted(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	root := grpcCreateCommit(t, clients, repo.Id, []string{})
	time.Sleep(10 * time.Millisecond)
	commit1 := grpcCreateCommit(t, clients, repo.Id, []string{root.Id})
	commit2 := grpcCreateCommit(t, clients, repo.Id, []string{root.Id})

	grpcCreateBranch(t, clients, repo.Id, "main", commit2.Id)

	_, err := clients.merge.CreateMerge(ctx, &vergev1.CreateMergeRequest{
		RepoId:             repo.Id,
		ParentIds:          []string{commit1.Id, commit2.Id},
		TargetBranch:       "main",
		ExpectedTargetHead: commit1.Id, // stale!
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Merge",
		Author:  "test@example.com",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Aborted, grpcCode(err))
}

func TestGRPC_CreateMerge_RepoNotFound_ReturnsNotFound(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.merge.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_nonexistent",
		ParentIds:          []string{"commit_1", "commit_2"},
		TargetBranch:       "main",
		ExpectedTargetHead: "commit_1",
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Merge",
		Author:  "test@example.com",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcCode(err))
}

//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	vergev1 "github.com/bhpcv252/verge/api/proto/verge/v1"
)

// CreateBranch

func TestGRPC_CreateBranch_ValidInput_ReturnsCreatedBranch(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	commit := grpcCreateCommit(t, clients, repo.Id, []string{})

	branchName := uniqueGRPCBranchName()
	resp, err := clients.branch.CreateBranch(ctx, &vergev1.CreateBranchRequest{
		RepoId:         repo.Id,
		Name:           branchName,
		SourceCommitId: commit.Id,
	})

	require.NoError(t, err)
	assert.Equal(t, branchName, resp.Name)
	assert.Equal(t, repo.Id, resp.RepoId)
	assert.Equal(t, commit.Id, resp.CommitId)
	assert.NotEmpty(t, resp.CreatedAt)

	_, parseErr := time.Parse("2006-01-02T15:04:05Z", resp.CreatedAt)
	assert.NoError(t, parseErr)
}

func TestGRPC_CreateBranch_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.branch.CreateBranch(context.Background(), &vergev1.CreateBranchRequest{
		Name:           "feature-x",
		SourceCommitId: "commit_xyz",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateBranch_MissingName_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.branch.CreateBranch(context.Background(), &vergev1.CreateBranchRequest{
		RepoId:         "repo_abc",
		SourceCommitId: "commit_xyz",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateBranch_MissingSourceCommitID_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.branch.CreateBranch(context.Background(), &vergev1.CreateBranchRequest{
		RepoId: "repo_abc",
		Name:   "feature-x",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateBranch_RepoNotFound_ReturnsNotFound(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.branch.CreateBranch(context.Background(), &vergev1.CreateBranchRequest{
		RepoId:         "repo_nonexistent",
		Name:           "main",
		SourceCommitId: "commit_xyz",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcCode(err))
}

func TestGRPC_CreateBranch_CommitNotFound_ReturnsNotFound(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)

	_, err := clients.branch.CreateBranch(ctx, &vergev1.CreateBranchRequest{
		RepoId:         repo.Id,
		Name:           "main",
		SourceCommitId: "commit_nonexistent",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcCode(err))
}

func TestGRPC_CreateBranch_DuplicateName_ReturnsAlreadyExists(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	commit := grpcCreateCommit(t, clients, repo.Id, []string{})

	branchName := uniqueGRPCBranchName()
	grpcCreateBranch(t, clients, repo.Id, branchName, commit.Id)

	_, err := clients.branch.CreateBranch(ctx, &vergev1.CreateBranchRequest{
		RepoId:         repo.Id,
		Name:           branchName,
		SourceCommitId: commit.Id,
	})

	require.Error(t, err)
	assert.Equal(t, codes.AlreadyExists, grpcCode(err))
}

// GetBranch

func TestGRPC_GetBranch_Exists_ReturnsCorrectBranch(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	commit := grpcCreateCommit(t, clients, repo.Id, []string{})
	branchName := uniqueGRPCBranchName()
	grpcCreateBranch(t, clients, repo.Id, branchName, commit.Id)

	resp, err := clients.branch.GetBranch(ctx, &vergev1.GetBranchRequest{
		RepoId: repo.Id,
		Name:   branchName,
	})

	require.NoError(t, err)
	assert.Equal(t, branchName, resp.Name)
	assert.Equal(t, repo.Id, resp.RepoId)
	assert.Equal(t, commit.Id, resp.CommitId)
	assert.NotEmpty(t, resp.CreatedAt)

	_, parseErr := time.Parse("2006-01-02T15:04:05Z", resp.CreatedAt)
	assert.NoError(t, parseErr)
}

func TestGRPC_GetBranch_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.branch.GetBranch(context.Background(), &vergev1.GetBranchRequest{
		Name: "feature-x",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_GetBranch_MissingName_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.branch.GetBranch(context.Background(), &vergev1.GetBranchRequest{
		RepoId: "repo_abc",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_GetBranch_NotFound_ReturnsNotFound(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)

	_, err := clients.branch.GetBranch(ctx, &vergev1.GetBranchRequest{
		RepoId: repo.Id,
		Name:   "nonexistent",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcCode(err))
}

func TestGRPC_GetBranch_RepoNotFound_ReturnsNotFound(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.branch.GetBranch(context.Background(), &vergev1.GetBranchRequest{
		RepoId: "repo_nonexistent",
		Name:   "main",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcCode(err))
}

// ListBranches

func TestGRPC_ListBranches_NoParams_ReturnsSuccess(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)

	resp, err := clients.branch.ListBranches(ctx, &vergev1.ListBranchesRequest{
		RepoId: repo.Id,
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestGRPC_ListBranches_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.branch.ListBranches(context.Background(), &vergev1.ListBranchesRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_ListBranches_InvalidLimit_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)

	_, err := clients.branch.ListBranches(ctx, &vergev1.ListBranchesRequest{
		RepoId: repo.Id,
		Limit:  200,
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_ListBranches_Pagination_CursorWorksCorrectly(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	commit := grpcCreateCommit(t, clients, repo.Id, []string{})

	for i := 0; i < 3; i++ {
		grpcCreateBranch(t, clients, repo.Id, fmt.Sprintf("branch-%d", i), commit.Id)
		time.Sleep(10 * time.Millisecond)
	}

	page1, err := clients.branch.ListBranches(ctx, &vergev1.ListBranchesRequest{
		RepoId: repo.Id,
		Limit:  2,
	})
	require.NoError(t, err)
	require.Len(t, page1.Branches, 2)
	require.NotEmpty(t, page1.NextCursor)

	page2, err := clients.branch.ListBranches(ctx, &vergev1.ListBranchesRequest{
		RepoId: repo.Id,
		Limit:  2,
		Cursor: page1.NextCursor,
	})
	require.NoError(t, err)
	require.NotEmpty(t, page2.Branches)

	seen := make(map[string]int)
	for _, b := range append(page1.Branches, page2.Branches...) {
		seen[b.Name]++
	}
	for name, count := range seen {
		assert.Equal(t, 1, count, "branch %s appeared more than once", name)
	}
}

// AdvanceBranch

func TestGRPC_AdvanceBranch_ValidInput_BranchPointsToNewCommit(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	commit1 := grpcCreateCommit(t, clients, repo.Id, []string{})
	commit2 := grpcCreateCommit(t, clients, repo.Id, []string{commit1.Id})

	branchName := uniqueGRPCBranchName()
	grpcCreateBranch(t, clients, repo.Id, branchName, commit1.Id)

	resp, err := clients.branch.AdvanceBranch(ctx, &vergev1.AdvanceBranchRequest{
		RepoId:           repo.Id,
		Name:             branchName,
		CommitId:         commit2.Id,
		ExpectedCommitId: commit1.Id,
	})

	require.NoError(t, err)
	assert.Equal(t, commit2.Id, resp.CommitId)
}

func TestGRPC_AdvanceBranch_StaleExpectedCommitID_ReturnsAborted(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	commit1 := grpcCreateCommit(t, clients, repo.Id, []string{})
	commit2 := grpcCreateCommit(t, clients, repo.Id, []string{commit1.Id})
	commit3 := grpcCreateCommit(t, clients, repo.Id, []string{commit2.Id})

	branchName := uniqueGRPCBranchName()
	grpcCreateBranch(t, clients, repo.Id, branchName, commit2.Id)

	_, err := clients.branch.AdvanceBranch(ctx, &vergev1.AdvanceBranchRequest{
		RepoId:           repo.Id,
		Name:             branchName,
		CommitId:         commit3.Id,
		ExpectedCommitId: commit1.Id, // stale!
	})

	require.Error(t, err)
	assert.Equal(t, codes.Aborted, grpcCode(err))
}

func TestGRPC_AdvanceBranch_BranchNotFound_ReturnsNotFound(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	commit := grpcCreateCommit(t, clients, repo.Id, []string{})

	_, err := clients.branch.AdvanceBranch(ctx, &vergev1.AdvanceBranchRequest{
		RepoId:           repo.Id,
		Name:             "nonexistent",
		CommitId:         commit.Id,
		ExpectedCommitId: "commit_old",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcCode(err))
}

// DeleteBranch

func TestGRPC_DeleteBranch_Exists_ReturnsSuccessAndBranchIsGone(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	commit := grpcCreateCommit(t, clients, repo.Id, []string{})
	branchName := uniqueGRPCBranchName()
	grpcCreateBranch(t, clients, repo.Id, branchName, commit.Id)

	_, err := clients.branch.DeleteBranch(ctx, &vergev1.DeleteBranchRequest{
		RepoId: repo.Id,
		Name:   branchName,
	})

	require.NoError(t, err)

	list, err := clients.branch.ListBranches(ctx, &vergev1.ListBranchesRequest{RepoId: repo.Id})
	require.NoError(t, err)
	for _, b := range list.Branches {
		assert.NotEqual(t, branchName, b.Name, "deleted branch should not appear in list")
	}
}

func TestGRPC_DeleteBranch_NotFound_ReturnsNotFound(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)

	_, err := clients.branch.DeleteBranch(ctx, &vergev1.DeleteBranchRequest{
		RepoId: repo.Id,
		Name:   "nonexistent",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcCode(err))
}

func TestGRPC_DeleteBranch_DefaultBranch_ReturnsFailedPrecondition(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	commit := grpcCreateCommit(t, clients, repo.Id, []string{})
	grpcCreateBranch(t, clients, repo.Id, "main", commit.Id)

	_, err := clients.branch.DeleteBranch(ctx, &vergev1.DeleteBranchRequest{
		RepoId: repo.Id,
		Name:   "main",
	})

	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, grpcCode(err))
}

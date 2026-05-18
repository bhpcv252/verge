//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	vergev1 "github.com/bhpcv252/verge/api/proto/verge/v1"
)

// CreateCommit

func TestGRPC_CreateCommit_RootCommit_Returns201WithEmptyParentIDs(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)

	resp, err := clients.commit.CreateCommit(ctx, &vergev1.CreateCommitRequest{
		RepoId:    repo.Id,
		ParentIds: []string{},
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Initial commit",
		Author:  "test@example.com",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Commit.Id)
	assert.Equal(t, repo.Id, resp.Commit.RepoId)
	assert.Empty(t, resp.Commit.ParentIds)
	assert.Equal(t, "Initial commit", resp.Commit.Message)
	assert.Equal(t, "test@example.com", resp.Commit.Author)
	assert.NotEmpty(t, resp.Commit.Timestamp)
	assert.False(t, resp.Existing)

	_, parseErr := time.Parse("2006-01-02T15:04:05Z", resp.Commit.Timestamp)
	assert.NoError(t, parseErr)
}

func TestGRPC_CreateCommit_RegularCommit_Returns201WithOneParent(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	parent := grpcCreateCommit(t, clients, repo.Id, []string{})

	resp, err := clients.commit.CreateCommit(ctx, &vergev1.CreateCommitRequest{
		RepoId:    repo.Id,
		ParentIds: []string{parent.Id},
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Add feature",
		Author:  "alice@example.com",
	})

	require.NoError(t, err)
	require.Len(t, resp.Commit.ParentIds, 1)
	assert.Equal(t, parent.Id, resp.Commit.ParentIds[0])
}

func TestGRPC_CreateCommit_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.commit.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		Message: "Test",
		Author:  "test@example.com",
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateCommit_MissingMessage_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.commit.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		RepoId: "repo_abc",
		Author: "test@example.com",
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateCommit_MissingAuthor_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.commit.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		RepoId:  "repo_abc",
		Message: "Test",
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateCommit_MissingDataPointer_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.commit.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		RepoId:  "repo_abc",
		Message: "Test",
		Author:  "test@example.com",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateCommit_TwoParentIDs_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)

	_, err := clients.commit.CreateCommit(ctx, &vergev1.CreateCommitRequest{
		RepoId:    repo.Id,
		ParentIds: []string{"commit_1", "commit_2"},
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Test",
		Author:  "test@example.com",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateCommit_InvalidParent_ReturnsFailedPrecondition(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)

	_, err := clients.commit.CreateCommit(ctx, &vergev1.CreateCommitRequest{
		RepoId:    repo.Id,
		ParentIds: []string{"commit_nonexistent"},
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Test",
		Author:  "test@example.com",
	})

	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, grpcCode(err))
}

func TestGRPC_CreateCommit_IdempotencyKeyRepeat_Returns200SameCommit(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	idempotencyKey := "key_" + uuid.New().String()

	resp1, err := clients.commit.CreateCommit(ctx, &vergev1.CreateCommitRequest{
		RepoId:         repo.Id,
		IdempotencyKey: idempotencyKey,
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Test",
		Author:  "test@example.com",
	})
	require.NoError(t, err)
	assert.False(t, resp1.Existing)

	resp2, err := clients.commit.CreateCommit(ctx, &vergev1.CreateCommitRequest{
		RepoId:         repo.Id,
		IdempotencyKey: idempotencyKey,
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Different message",
		Author:  "different@example.com",
	})

	require.NoError(t, err)
	assert.True(t, resp2.Existing)
	assert.Equal(t, resp1.Commit.Id, resp2.Commit.Id)
}

func TestGRPC_CreateCommit_RepoNotFound_ReturnsNotFound(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.commit.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		RepoId: "repo_nonexistent",
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Test",
		Author:  "test@example.com",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcCode(err))
}

// GetCommit

func TestGRPC_GetCommit_Exists_ReturnsCorrectCommit(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	commit := grpcCreateCommit(t, clients, repo.Id, []string{})

	resp, err := clients.commit.GetCommit(ctx, &vergev1.GetCommitRequest{
		RepoId:   repo.Id,
		CommitId: commit.Id,
	})

	require.NoError(t, err)
	assert.Equal(t, commit.Id, resp.Id)
	assert.Equal(t, repo.Id, resp.RepoId)
	assert.Equal(t, "test commit", resp.Message)
	assert.Equal(t, "test@example.com", resp.Author)
}

func TestGRPC_GetCommit_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.commit.GetCommit(context.Background(), &vergev1.GetCommitRequest{
		CommitId: "commit_abc",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_GetCommit_MissingCommitID_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.commit.GetCommit(context.Background(), &vergev1.GetCommitRequest{
		RepoId: "repo_abc",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_GetCommit_NotFound_ReturnsNotFound(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)

	_, err := clients.commit.GetCommit(ctx, &vergev1.GetCommitRequest{
		RepoId:   repo.Id,
		CommitId: "commit_nonexistent",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcCode(err))
}

// ListCommits

func TestGRPC_ListCommits_NoParams_ReturnsSuccess(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)

	resp, err := clients.commit.ListCommits(ctx, &vergev1.ListCommitsRequest{
		RepoId: repo.Id,
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestGRPC_ListCommits_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.commit.ListCommits(context.Background(), &vergev1.ListCommitsRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_ListCommits_InvalidLimit_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)

	_, err := clients.commit.ListCommits(ctx, &vergev1.ListCommitsRequest{
		RepoId: repo.Id,
		Limit:  200,
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_ListCommits_WithFilters_ReturnsFilteredResults(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)

	_, err := clients.commit.CreateCommit(ctx, &vergev1.CreateCommitRequest{
		RepoId:    repo.Id,
		ParentIds: []string{},
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Commit by Alice",
		Author:  "alice@example.com",
	})
	require.NoError(t, err)

	_, err = clients.commit.CreateCommit(ctx, &vergev1.CreateCommitRequest{
		RepoId:    repo.Id,
		ParentIds: []string{},
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Commit by Bob",
		Author:  "bob@example.com",
	})
	require.NoError(t, err)

	resp, err := clients.commit.ListCommits(ctx, &vergev1.ListCommitsRequest{
		RepoId: repo.Id,
		Author: "alice@example.com",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Commits)
	for _, c := range resp.Commits {
		assert.Equal(t, "alice@example.com", c.Author)
	}
}

// GetParents

func TestGRPC_GetParents_CommitWithParent_ReturnsParents(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	parent := grpcCreateCommit(t, clients, repo.Id, []string{})
	child := grpcCreateCommit(t, clients, repo.Id, []string{parent.Id})

	resp, err := clients.commit.GetParents(ctx, &vergev1.GetParentsRequest{
		RepoId:   repo.Id,
		CommitId: child.Id,
	})

	require.NoError(t, err)
	assert.Equal(t, child.Id, resp.CommitId)
	require.Len(t, resp.Parents, 1)
	assert.Equal(t, parent.Id, resp.Parents[0].Id)
}

func TestGRPC_GetParents_RootCommit_ReturnsEmptyList(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)
	commit := grpcCreateCommit(t, clients, repo.Id, []string{})

	resp, err := clients.commit.GetParents(ctx, &vergev1.GetParentsRequest{
		RepoId:   repo.Id,
		CommitId: commit.Id,
	})

	require.NoError(t, err)
	assert.Len(t, resp.Parents, 0)
}

func TestGRPC_GetParents_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.commit.GetParents(context.Background(), &vergev1.GetParentsRequest{
		CommitId: "commit_abc",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_GetParents_MissingCommitID_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.commit.GetParents(context.Background(), &vergev1.GetParentsRequest{
		RepoId: "repo_abc",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_GetParents_CommitNotFound_ReturnsNotFound(t *testing.T) {
	clients := startGRPCServer(t)
	ctx := context.Background()

	repo := grpcCreateRepo(t, clients)

	_, err := clients.commit.GetParents(ctx, &vergev1.GetParentsRequest{
		RepoId:   repo.Id,
		CommitId: "commit_nonexistent",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcCode(err))
}

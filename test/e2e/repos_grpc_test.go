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

// CreateRepo

func TestGRPC_CreateRepo_ValidInput_Returns201WithAllFields(t *testing.T) {
	clients := startGRPCServer(t)
	name := uniqueGRPCRepoName()

	resp, err := clients.repo.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
		Name:          name,
		DefaultBranch: "main",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
	assert.Equal(t, name, resp.Name)
	assert.Equal(t, "main", resp.DefaultBranch)
	assert.NotEmpty(t, resp.CreatedAt)

	_, parseErr := time.Parse("2006-01-02T15:04:05Z", resp.CreatedAt)
	assert.NoError(t, parseErr, "CreatedAt should be valid ISO 8601: %s", resp.CreatedAt)
}

func TestGRPC_CreateRepo_MissingName_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.repo.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
		DefaultBranch: "main",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateRepo_MissingDefaultBranch_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.repo.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
		Name: uniqueGRPCRepoName(),
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

// GetRepo

func TestGRPC_GetRepo_Exists_ReturnsCorrectFields(t *testing.T) {
	clients := startGRPCServer(t)
	name := uniqueGRPCRepoName()

	created, err := clients.repo.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
		Name:          name,
		DefaultBranch: "main",
	})
	require.NoError(t, err)

	got, err := clients.repo.GetRepo(
		context.Background(),
		&vergev1.GetRepoRequest{RepoId: created.Id},
	)

	require.NoError(t, err)
	assert.Equal(t, created.Id, got.Id)
	assert.Equal(t, name, got.Name)
	assert.Equal(t, "main", got.DefaultBranch)
}

func TestGRPC_GetRepo_NotFound_ReturnsNotFound(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.repo.GetRepo(context.Background(), &vergev1.GetRepoRequest{
		RepoId: "repo_does-not-exist",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcCode(err))
}

func TestGRPC_GetRepo_EmptyRepoID_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.repo.GetRepo(context.Background(), &vergev1.GetRepoRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

// ListRepos

func TestGRPC_ListRepos_NoParams_Returns200(t *testing.T) {
	clients := startGRPCServer(t)

	resp, err := clients.repo.ListRepos(context.Background(), &vergev1.ListReposRequest{})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestGRPC_ListRepos_InvalidLimit_ReturnsInvalidArgument(t *testing.T) {
	clients := startGRPCServer(t)

	_, err := clients.repo.ListRepos(context.Background(), &vergev1.ListReposRequest{Limit: 200})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_ListRepos_Pagination_CursorWorksCorrectly(t *testing.T) {
	clients := startGRPCServer(t)

	for i := 0; i < 3; i++ {
		_, err := clients.repo.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
			Name:          uniqueGRPCRepoName(),
			DefaultBranch: "main",
		})
		require.NoError(t, err)
	}

	page1, err := clients.repo.ListRepos(context.Background(), &vergev1.ListReposRequest{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page1.Repos, 2)
	require.NotEmpty(t, page1.NextCursor)

	page2, err := clients.repo.ListRepos(context.Background(), &vergev1.ListReposRequest{
		Limit:  2,
		Cursor: page1.NextCursor,
	})
	require.NoError(t, err)
	require.NotEmpty(t, page2.Repos)

	seen := make(map[string]int)
	for _, r := range append(page1.Repos, page2.Repos...) {
		seen[r.Id]++
	}
	for id, count := range seen {
		assert.Equal(
			t,
			1,
			count,
			"repo %s appeared more than once: %s",
			id,
			fmt.Sprintf("%d times", count),
		)
	}
}

func TestGRPC_ListRepos_LastPageHasEmptyNextCursor(t *testing.T) {
	clients := startGRPCServer(t)

	for i := 0; i < 2; i++ {
		_, err := clients.repo.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
			Name:          uniqueGRPCRepoName(),
			DefaultBranch: "main",
		})
		require.NoError(t, err)
	}

	resp, err := clients.repo.ListRepos(context.Background(), &vergev1.ListReposRequest{Limit: 100})
	require.NoError(t, err)
	assert.Empty(t, resp.NextCursor)
}

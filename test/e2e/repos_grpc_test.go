//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pgmodule "github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	vergev1 "github.com/bhpcv252/verge/api/proto/verge/v1"
	grpcv1 "github.com/bhpcv252/verge/internal/api/grpc/v1"
	"github.com/bhpcv252/verge/internal/service"
	pgstore "github.com/bhpcv252/verge/internal/storage/postgres"
	"github.com/bhpcv252/verge/testhelper"
)

// startGRPCServer spins up a real Postgres container, runs all migrations,
// wires the full gRPC stack, and returns a connected client.
func startGRPCServer(t *testing.T) vergev1.RepositoryServiceClient {
	t.Helper()
	ctx := context.Background()

	ctr, err := pgmodule.Run(ctx,
		"postgres:16",
		pgmodule.WithDatabase("verge"),
		pgmodule.WithUsername("verge"),
		pgmodule.WithPassword("changeme"),
		pgmodule.BasicWaitStrategies(),
	)
	require.NoError(t, err, "failed to start postgres container")
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	testhelper.RunMigrations(t, connStr)

	pool, err := pgstore.NewPool(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	repoStore := pgstore.NewRepoStore(pool)
	repoSvc := service.NewRepoService(repoStore)
	repoServer := grpcv1.NewRepoServer(repoSvc)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer()
	vergev1.RegisterRepositoryServiceServer(srv, repoServer)

	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.GracefulStop() })

	cc, err := grpc.NewClient(
		ln.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cc.Close() })

	return vergev1.NewRepositoryServiceClient(cc)
}

func uniqueGRPCRepoName() string { return "grpc-e2e-" + uuid.New().String() }

func grpcCode(err error) codes.Code {
	s, _ := status.FromError(err)
	return s.Code()
}

// CreateRepo

func TestGRPC_CreateRepo_ValidInput_Returns201WithAllFields(t *testing.T) {
	client := startGRPCServer(t)
	name := uniqueGRPCRepoName()

	resp, err := client.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
		Name:          name,
		DefaultBranch: "main",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
	assert.Equal(t, name, resp.Name)
	assert.Equal(t, "main", resp.DefaultBranch)
	assert.NotEmpty(t, resp.CreatedAt)

	// CreatedAt must be parseable ISO 8601
	_, parseErr := time.Parse("2006-01-02T15:04:05Z", resp.CreatedAt)
	assert.NoError(t, parseErr, "CreatedAt should be valid ISO 8601: %s", resp.CreatedAt)
}

func TestGRPC_CreateRepo_MissingName_ReturnsInvalidArgument(t *testing.T) {
	client := startGRPCServer(t)

	_, err := client.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
		DefaultBranch: "main",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_CreateRepo_MissingDefaultBranch_ReturnsInvalidArgument(t *testing.T) {
	client := startGRPCServer(t)

	_, err := client.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
		Name: uniqueGRPCRepoName(),
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

// GetRepo

func TestGRPC_GetRepo_Exists_ReturnsCorrectFields(t *testing.T) {
	client := startGRPCServer(t)
	name := uniqueGRPCRepoName()

	created, err := client.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
		Name:          name,
		DefaultBranch: "main",
	})
	require.NoError(t, err)

	got, err := client.GetRepo(context.Background(), &vergev1.GetRepoRequest{RepoId: created.Id})

	require.NoError(t, err)
	assert.Equal(t, created.Id, got.Id)
	assert.Equal(t, name, got.Name)
	assert.Equal(t, "main", got.DefaultBranch)
}

func TestGRPC_GetRepo_NotFound_ReturnsNotFound(t *testing.T) {
	client := startGRPCServer(t)

	_, err := client.GetRepo(context.Background(), &vergev1.GetRepoRequest{
		RepoId: "repo_does-not-exist",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcCode(err))
}

func TestGRPC_GetRepo_EmptyRepoID_ReturnsInvalidArgument(t *testing.T) {
	client := startGRPCServer(t)

	_, err := client.GetRepo(context.Background(), &vergev1.GetRepoRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

// ListRepos

func TestGRPC_ListRepos_NoParams_Returns200(t *testing.T) {
	client := startGRPCServer(t)

	resp, err := client.ListRepos(context.Background(), &vergev1.ListReposRequest{})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestGRPC_ListRepos_InvalidLimit_ReturnsInvalidArgument(t *testing.T) {
	client := startGRPCServer(t)

	_, err := client.ListRepos(context.Background(), &vergev1.ListReposRequest{Limit: 200})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestGRPC_ListRepos_Pagination_CursorWorksCorrectly(t *testing.T) {
	client := startGRPCServer(t)

	// insert 3 repos
	for i := 0; i < 3; i++ {
		_, err := client.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
			Name:          uniqueGRPCRepoName(),
			DefaultBranch: "main",
		})
		require.NoError(t, err)
	}

	// page 1: limit=2
	page1, err := client.ListRepos(context.Background(), &vergev1.ListReposRequest{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page1.Repos, 2)
	require.NotEmpty(t, page1.NextCursor)

	// page 2: use cursor from page 1
	page2, err := client.ListRepos(context.Background(), &vergev1.ListReposRequest{
		Limit:  2,
		Cursor: page1.NextCursor,
	})
	require.NoError(t, err)
	require.NotEmpty(t, page2.Repos)

	// no duplicates across pages
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
	client := startGRPCServer(t)

	// insert exactly 2 repos
	for i := 0; i < 2; i++ {
		_, err := client.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
			Name:          uniqueGRPCRepoName(),
			DefaultBranch: "main",
		})
		require.NoError(t, err)
	}

	resp, err := client.ListRepos(context.Background(), &vergev1.ListReposRequest{Limit: 100})
	require.NoError(t, err)
	assert.Empty(t, resp.NextCursor)
}

//go:build e2e

package e2e

import (
	"context"
	"net"
	"testing"

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

// grpcClients holds connected clients for every service.
// All gRPC tests share this single type; each test just uses the fields it needs.
type grpcClients struct {
	repo   vergev1.RepositoryServiceClient
	branch vergev1.BranchServiceClient
	commit vergev1.CommitServiceClient
	merge  vergev1.MergeServiceClient
}

// startGRPCServer spins up a Postgres container, runs migrations, wires the
// complete gRPC stack (all four services), and returns connected clients.
// It registers t.Cleanup for every resource it creates.
func startGRPCServer(t *testing.T) *grpcClients {
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
	branchStore := pgstore.NewBranchStore(pool)
	commitStore := pgstore.NewCommitStore(pool)

	repoSvc := service.NewRepoService(repoStore)
	branchSvc := service.NewBranchService(branchStore, repoStore, commitStore)
	commitSvc := service.NewCommitService(commitStore, repoStore)
	mergeSvc := service.NewMergeService(commitStore, repoStore, commitStore, branchStore)

	srv := grpc.NewServer()
	vergev1.RegisterRepositoryServiceServer(srv, grpcv1.NewRepoServer(repoSvc))
	vergev1.RegisterBranchServiceServer(srv, grpcv1.NewBranchServer(branchSvc))
	vergev1.RegisterCommitServiceServer(srv, grpcv1.NewCommitServer(commitSvc))
	vergev1.RegisterMergeServiceServer(srv, grpcv1.NewMergeServer(mergeSvc))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.GracefulStop() })

	cc, err := grpc.NewClient(
		ln.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cc.Close() })

	return &grpcClients{
		repo:   vergev1.NewRepositoryServiceClient(cc),
		branch: vergev1.NewBranchServiceClient(cc),
		commit: vergev1.NewCommitServiceClient(cc),
		merge:  vergev1.NewMergeServiceClient(cc),
	}
}

// grpcCode extracts the gRPC status code from an error.
func grpcCode(err error) codes.Code {
	s, _ := status.FromError(err)
	return s.Code()
}

// factory helpers

func uniqueGRPCRepoName() string   { return testhelper.UniqueName("grpc-e2e") }
func uniqueGRPCBranchName() string { return testhelper.UniqueName("grpc-branch") }

func grpcCreateRepo(t *testing.T, clients *grpcClients) *vergev1.Repository {
	t.Helper()
	resp, err := clients.repo.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
		Name:          uniqueGRPCRepoName(),
		DefaultBranch: "main",
	})
	require.NoError(t, err, "grpcCreateRepo setup failed")
	return resp
}

func grpcCreateCommit(
	t *testing.T,
	clients *grpcClients,
	repoID string,
	parentIDs []string,
) *vergev1.Commit {
	t.Helper()
	resp, err := clients.commit.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		RepoId:    repoID,
		ParentIds: parentIDs,
		DataPointer: &vergev1.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "test commit",
		Author:  "test@example.com",
	})
	require.NoError(t, err, "grpcCreateCommit setup failed")
	return resp.Commit
}

func grpcCreateBranch(
	t *testing.T,
	clients *grpcClients,
	repoID, name, commitID string,
) *vergev1.Branch {
	t.Helper()
	resp, err := clients.branch.CreateBranch(context.Background(), &vergev1.CreateBranchRequest{
		RepoId:         repoID,
		Name:           name,
		SourceCommitId: commitID,
	})
	require.NoError(t, err, "grpcCreateBranch setup failed")
	return resp
}

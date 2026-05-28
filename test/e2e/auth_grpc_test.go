//go:build e2e

package e2e

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	vergev1 "github.com/bhpcv252/verge/api/proto/verge/v1"
	grpcv1 "github.com/bhpcv252/verge/internal/api/grpc/v1"
	"github.com/bhpcv252/verge/internal/auth"
	"github.com/bhpcv252/verge/internal/observability"
	"github.com/bhpcv252/verge/internal/service"
	pgstore "github.com/bhpcv252/verge/internal/storage/postgres"
	"github.com/bhpcv252/verge/testhelper"
)

type authGRPCEnv struct {
	cc      *grpc.ClientConn
	testKey string
}

func startAuthGRPCServer(t *testing.T) authGRPCEnv {
	t.Helper()

	const testKey = "grpc-e2e-test-key-do-not-use-in-prod"

	pool, cleanup := testhelper.SetupPostgres(t)
	t.Cleanup(cleanup)

	repoStore := pgstore.NewRepoStore(pool)
	branchStore := pgstore.NewBranchStore(pool)
	commitStore := pgstore.NewCommitStore(pool)

	repoSvc := service.NewRepoService(repoStore)
	branchSvc := service.NewBranchService(branchStore, repoStore, commitStore)
	commitSvc := service.NewCommitService(commitStore, repoStore)
	mergeSvc := service.NewMergeService(commitStore, repoStore, commitStore, branchStore)

	validator, err := auth.NewValidator([]string{testKey})
	require.NoError(t, err, "build auth validator")

	srv := grpcv1.NewServer(
		observability.Noop(),
		validator,
		grpcv1.NewRepoServer(repoSvc),
		grpcv1.NewBranchServer(branchSvc),
		grpcv1.NewCommitServer(commitSvc),
		grpcv1.NewMergeServer(mergeSvc),
	)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "get free port for auth gRPC server")

	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.GracefulStop() })

	cc, err := grpc.NewClient(
		ln.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err, "dial auth gRPC server")
	t.Cleanup(func() { _ = cc.Close() })

	return authGRPCEnv{cc: cc, testKey: testKey}
}

func ctxWithKey(key string) context.Context {
	if key == "" {
		return context.Background()
	}
	return metadata.AppendToOutgoingContext(
		context.Background(),
		"authorization", "Bearer "+key,
	)
}

// no-key calls

func TestAuthGRPC_NoKey_Returns_Unauthenticated(t *testing.T) {
	env := startAuthGRPCServer(t)
	client := vergev1.NewRepositoryServiceClient(env.cc)

	_, err := client.ListRepos(ctxWithKey(""), &vergev1.ListReposRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, grpcCode(err))
}

func TestAuthGRPC_NoKey_AllServices_Return_Unauthenticated(t *testing.T) {
	env := startAuthGRPCServer(t)

	calls := []struct {
		name string
		fn   func() error
	}{
		{
			"RepoService/ListRepos",
			func() error {
				_, err := vergev1.NewRepositoryServiceClient(env.cc).
					ListRepos(ctxWithKey(""), &vergev1.ListReposRequest{})
				return err
			},
		},
		{
			"BranchService/ListBranches",
			func() error {
				_, err := vergev1.NewBranchServiceClient(env.cc).
					ListBranches(ctxWithKey(""), &vergev1.ListBranchesRequest{RepoId: "any"})
				return err
			},
		},
		{
			"CommitService/ListCommits",
			func() error {
				_, err := vergev1.NewCommitServiceClient(env.cc).
					ListCommits(ctxWithKey(""), &vergev1.ListCommitsRequest{RepoId: "any"})
				return err
			},
		},
		{
			"MergeService/CreateMerge",
			func() error {
				_, err := vergev1.NewMergeServiceClient(env.cc).
					CreateMerge(ctxWithKey(""), &vergev1.CreateMergeRequest{RepoId: "any"})
				return err
			},
		},
	}

	for _, tc := range calls {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			require.Error(t, err)
			assert.Equal(t, codes.Unauthenticated, grpcCode(err),
				"%s must return Unauthenticated when no key is provided", tc.name)
		})
	}
}

// wrong-key calls

func TestAuthGRPC_WrongKey_Returns_Unauthenticated(t *testing.T) {
	env := startAuthGRPCServer(t)
	client := vergev1.NewRepositoryServiceClient(env.cc)

	_, err := client.ListRepos(ctxWithKey("totally-wrong-key"), &vergev1.ListReposRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, grpcCode(err))
}

func TestAuthGRPC_WrongAndMissingKeyReturnSameCodeAndMessage(t *testing.T) {
	env := startAuthGRPCServer(t)
	client := vergev1.NewRepositoryServiceClient(env.cc)

	_, errNoKey := client.ListRepos(ctxWithKey(""), &vergev1.ListReposRequest{})
	_, errBadKey := client.ListRepos(ctxWithKey("bad-key"), &vergev1.ListReposRequest{})

	require.Error(t, errNoKey)
	require.Error(t, errBadKey)

	stNoKey, _ := status.FromError(errNoKey)
	stBadKey, _ := status.FromError(errBadKey)

	assert.Equal(t, stNoKey.Code(), stBadKey.Code())
	assert.Equal(t, stNoKey.Message(), stBadKey.Message())
}

// valid-key calls

func TestAuthGRPC_ValidKey_PassesThroughToHandler(t *testing.T) {
	env := startAuthGRPCServer(t)
	client := vergev1.NewRepositoryServiceClient(env.cc)

	resp, err := client.ListRepos(ctxWithKey(env.testKey), &vergev1.ListReposRequest{})

	require.NoError(t, err, "valid key must reach the handler")
	assert.NotNil(t, resp)
}

func TestAuthGRPC_ValidKey_CreateAndFetch(t *testing.T) {
	env := startAuthGRPCServer(t)
	client := vergev1.NewRepositoryServiceClient(env.cc)

	created, err := client.CreateRepo(
		ctxWithKey(env.testKey),
		&vergev1.CreateRepoRequest{
			Name:          testhelper.UniqueName("grpc-auth"),
			DefaultBranch: "main",
		},
	)
	require.NoError(t, err)
	require.NotEmpty(t, created.GetId())

	fetched, err := client.GetRepo(
		ctxWithKey(env.testKey),
		&vergev1.GetRepoRequest{RepoId: created.GetId()},
	)
	require.NoError(t, err)
	assert.Equal(t, created.GetId(), fetched.GetId())
}

// key rotation

func TestAuthGRPC_KeyRotation_BothKeysAccepted(t *testing.T) {
	const (
		oldKey = "grpc-old-key-being-rotated-out"
		newKey = "grpc-new-key-just-added"
	)

	pool, cleanup := testhelper.SetupPostgres(t)
	t.Cleanup(cleanup)

	repoStore := pgstore.NewRepoStore(pool)
	branchStore := pgstore.NewBranchStore(pool)
	commitStore := pgstore.NewCommitStore(pool)

	repoSvc := service.NewRepoService(repoStore)
	branchSvc := service.NewBranchService(branchStore, repoStore, commitStore)
	commitSvc := service.NewCommitService(commitStore, repoStore)
	mergeSvc := service.NewMergeService(commitStore, repoStore, commitStore, branchStore)

	validator, err := auth.NewValidator([]string{oldKey, newKey})
	require.NoError(t, err)

	srv := grpcv1.NewServer(
		observability.Noop(),
		validator,
		grpcv1.NewRepoServer(repoSvc),
		grpcv1.NewBranchServer(branchSvc),
		grpcv1.NewCommitServer(commitSvc),
		grpcv1.NewMergeServer(mergeSvc),
	)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.GracefulStop() })

	cc, err := grpc.NewClient(ln.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cc.Close() })

	client := vergev1.NewRepositoryServiceClient(cc)

	for _, key := range []string{oldKey, newKey} {
		key := key
		t.Run("key="+key[:8]+"...", func(t *testing.T) {
			_, err := client.ListRepos(ctxWithKey(key), &vergev1.ListReposRequest{})
			assert.NoError(t, err, "key must be accepted during rotation window")
		})
	}

	_, err = client.ListRepos(ctxWithKey("revoked-key"), &vergev1.ListReposRequest{})
	assert.Equal(t, codes.Unauthenticated, grpcCode(err),
		"revoked key must be rejected even during rotation window")
}

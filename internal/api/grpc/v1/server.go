package v1

import (
	"context"
	"runtime/debug"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	vergev1 "github.com/bhpcv252/verge/api/proto/verge/v1"
	"github.com/bhpcv252/verge/internal/observability"
)

func NewServer(
	obs *observability.Provider,
	repoSvr *RepoServer,
	branchSvr *BranchServer,
	commitSvr *CommitServer,
	mergeSvr *MergeServer,
) *grpc.Server {
	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			recoveryInterceptor,
			observability.GRPCUnaryInterceptor(obs),
		),
	)

	vergev1.RegisterRepositoryServiceServer(s, repoSvr)
	vergev1.RegisterBranchServiceServer(s, branchSvr)
	vergev1.RegisterCommitServiceServer(s, commitSvr)
	vergev1.RegisterMergeServiceServer(s, mergeSvr)

	return s
}

// recoveryInterceptor catches panics in any RPC handler and converts them to
// a codes.Internal gRPC status error. The stack trace is printed to stderr.
func recoveryInterceptor(
	ctx context.Context,
	req any,
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp any, err error) {
	defer func() {
		if r := recover(); r != nil {
			debug.PrintStack()
			err = status.Error(codes.Internal, "an unexpected error occurred")
		}
	}()
	return handler(ctx, req)
}

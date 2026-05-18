package v1

import (
	"context"
	"runtime/debug"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	vergev1 "github.com/bhpcv252/verge/api/proto/verge/v1"
)

func NewServer(
	repoSvr *RepoServer,
	branchSvr *BranchServer,
	commitSvr *CommitServer,
	mergeSvr *MergeServer,
) *grpc.Server {
	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			recoveryInterceptor,
			// TODO: Add logging, tracing, auth interceptors here as the project grows.
		),
	)

	vergev1.RegisterRepositoryServiceServer(s, repoSvr)
	vergev1.RegisterBranchServiceServer(s, branchSvr)
	vergev1.RegisterCommitServiceServer(s, commitSvr)
	vergev1.RegisterMergeServiceServer(s, mergeSvr)

	return s
}

// recoveryInterceptor catches panics in any
// RPC handler and returns an INTERNAL status error
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

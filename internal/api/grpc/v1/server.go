package v1

import (
	"context"
	"runtime/debug"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	vergev1 "github.com/bhpcv252/verge/api/proto/verge/v1"
	"github.com/bhpcv252/verge/internal/auth"
	"github.com/bhpcv252/verge/internal/observability"
)

func NewServer(
	obs *observability.Provider,
	validator *auth.Validator, // nil = auth disabled
	repoSvr *RepoServer,
	branchSvr *BranchServer,
	commitSvr *CommitServer,
	mergeSvr *MergeServer,
) *grpc.Server {
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		recoveryInterceptor,
		auth.UnaryInterceptor(validator), // no-op when validator is nil
		observability.GRPCUnaryInterceptor(obs),
	}

	streamInterceptors := []grpc.StreamServerInterceptor{
		auth.StreamInterceptor(validator), // no-op when validator is nil
	}

	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	)

	vergev1.RegisterRepositoryServiceServer(s, repoSvr)
	vergev1.RegisterBranchServiceServer(s, branchSvr)
	vergev1.RegisterCommitServiceServer(s, commitSvr)
	vergev1.RegisterMergeServiceServer(s, mergeSvr)

	return s
}

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

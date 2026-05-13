package v1

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	vergev1 "github.com/bhpcv252/verge/api/proto/verge/v1"
	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/service"
)

// RepoServer implements vergev1.RepositoryServiceServer.
type RepoServer struct {
	vergev1.UnimplementedRepositoryServiceServer
	svc RepoService
}

type RepoService interface {
	CreateRepo(ctx context.Context, in service.CreateRepoInput) (*domain.Repo, error)
	GetRepo(ctx context.Context, id string) (*domain.Repo, error)
	ListRepos(ctx context.Context, in service.ListReposInput) (*service.ListReposResult, error)
}

func NewRepoServer(svc RepoService) *RepoServer {
	return &RepoServer{svc: svc}
}

func (s *RepoServer) CreateRepo(
	ctx context.Context,
	req *vergev1.CreateRepoRequest,
) (*vergev1.Repository, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "'name' is required and must not be empty.")
	}
	if req.DefaultBranch == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"'default_branch' is required and must not be empty.",
		)
	}

	repo, err := s.svc.CreateRepo(ctx, service.CreateRepoInput{
		Name:          req.Name,
		DefaultBranch: req.DefaultBranch,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "an unexpected error occurred")
	}

	return toProtoRepo(repo), nil
}

func (s *RepoServer) GetRepo(
	ctx context.Context,
	req *vergev1.GetRepoRequest,
) (*vergev1.Repository, error) {
	if req.RepoId == "" {
		return nil, status.Error(codes.InvalidArgument, "'repo_id' is required.")
	}

	repo, err := s.svc.GetRepo(ctx, req.RepoId)
	if err != nil {
		if errors.Is(err, domain.ErrRepoNotFound) {
			return nil, status.Error(codes.NotFound,
				fmt.Sprintf("Repository %q does not exist.", req.RepoId))
		}
		return nil, status.Error(codes.Internal, "an unexpected error occurred")
	}

	return toProtoRepo(repo), nil
}

func (s *RepoServer) ListRepos(
	ctx context.Context,
	req *vergev1.ListReposRequest,
) (*vergev1.ListReposResponse, error) {
	limit := int(req.Limit)
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return nil, status.Error(codes.InvalidArgument, "'limit' must be between 1 and 100.")
	}

	result, err := s.svc.ListRepos(ctx, service.ListReposInput{
		Limit:  limit,
		Cursor: req.Cursor,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "an unexpected error occurred")
	}

	resp := &vergev1.ListReposResponse{
		Repos: make([]*vergev1.Repository, 0, len(result.Repos)),
	}
	for _, r := range result.Repos {
		resp.Repos = append(resp.Repos, toProtoRepo(r))
	}
	resp.NextCursor = result.NextCursor

	return resp, nil
}

// Mapping

func toProtoRepo(r *domain.Repo) *vergev1.Repository {
	return &vergev1.Repository{
		Id:            r.ID,
		Name:          r.Name,
		DefaultBranch: r.DefaultBranch,
		CreatedAt:     r.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

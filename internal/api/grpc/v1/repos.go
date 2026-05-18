package v1

import (
	"context"
	"strings"

	vergev1 "github.com/bhpcv252/verge/api/proto/verge/v1"
	"github.com/bhpcv252/verge/internal/api/core"
	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/service"
)

type RepoServer struct {
	vergev1.UnimplementedRepositoryServiceServer
	svc core.RepoService
}

func NewRepoServer(svc core.RepoService) *RepoServer {
	return &RepoServer{svc: svc}
}

func (s *RepoServer) CreateRepo(
	ctx context.Context,
	req *vergev1.CreateRepoRequest,
) (*vergev1.Repository, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, invalidArg("'name' is required and must not be empty.")
	}
	if strings.TrimSpace(req.DefaultBranch) == "" {
		return nil, invalidArg("'default_branch' is required and must not be empty.")
	}

	repo, err := s.svc.CreateRepo(ctx, service.CreateRepoInput{
		Name:          req.Name,
		DefaultBranch: req.DefaultBranch,
	})
	if err != nil {
		return nil, toGRPCError(core.MapDomainError(err))
	}

	return toProtoRepo(repo), nil
}

func (s *RepoServer) GetRepo(
	ctx context.Context,
	req *vergev1.GetRepoRequest,
) (*vergev1.Repository, error) {
	if strings.TrimSpace(req.RepoId) == "" {
		return nil, invalidArg("'repo_id' is required and must not be empty.")
	}

	repo, err := s.svc.GetRepo(ctx, req.RepoId)
	if err != nil {
		return nil, toGRPCError(core.MapDomainError(err))
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
		return nil, invalidArg("'limit' must be between 1 and 100.")
	}

	result, err := s.svc.ListRepos(ctx, service.ListReposInput{
		Limit:  limit,
		Cursor: req.Cursor,
	})
	if err != nil {
		return nil, toGRPCError(core.MapDomainError(err))
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

func toProtoRepo(r *domain.Repo) *vergev1.Repository {
	return &vergev1.Repository{
		Id:            r.ID,
		Name:          r.Name,
		DefaultBranch: r.DefaultBranch,
		CreatedAt:     r.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

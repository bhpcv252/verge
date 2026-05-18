package v1

import (
	"context"
	"strings"

	vergev1 "github.com/bhpcv252/verge/api/proto/verge/v1"
	"github.com/bhpcv252/verge/internal/api/core"
	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/service"
)

type BranchServer struct {
	vergev1.UnimplementedBranchServiceServer
	svc core.BranchService
}

func NewBranchServer(svc core.BranchService) *BranchServer {
	return &BranchServer{svc: svc}
}

func (s *BranchServer) CreateBranch(
	ctx context.Context,
	req *vergev1.CreateBranchRequest,
) (*vergev1.Branch, error) {
	repoID := strings.TrimSpace(req.RepoId)
	name := strings.TrimSpace(req.Name)
	sourceCommitID := strings.TrimSpace(req.SourceCommitId)

	if repoID == "" {
		return nil, invalidArg("'repo_id' is required and must not be empty.")
	}
	if name == "" {
		return nil, invalidArg("'name' is required and must not be empty.")
	}
	if sourceCommitID == "" {
		return nil, invalidArg("'source_commit_id' is required and must not be empty.")
	}

	branch, err := s.svc.CreateBranch(ctx, service.CreateBranchInput{
		RepoID:         repoID,
		Name:           name,
		SourceCommitID: sourceCommitID,
	})
	if err != nil {
		return nil, toGRPCError(core.MapDomainError(err))
	}

	return toBranchProto(branch), nil
}

func (s *BranchServer) GetBranch(
	ctx context.Context,
	req *vergev1.GetBranchRequest,
) (*vergev1.Branch, error) {
	repoID := strings.TrimSpace(req.RepoId)
	name := strings.TrimSpace(req.Name)

	if repoID == "" {
		return nil, invalidArg("'repo_id' is required and must not be empty.")
	}
	if name == "" {
		return nil, invalidArg("'name' is required and must not be empty.")
	}

	branch, err := s.svc.GetBranch(ctx, repoID, name)
	if err != nil {
		return nil, toGRPCError(core.MapDomainError(err))
	}

	return toBranchProto(branch), nil
}

func (s *BranchServer) ListBranches(
	ctx context.Context,
	req *vergev1.ListBranchesRequest,
) (*vergev1.ListBranchesResponse, error) {
	repoID := strings.TrimSpace(req.RepoId)
	if repoID == "" {
		return nil, invalidArg("'repo_id' is required and must not be empty.")
	}

	limit := int(req.Limit)
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return nil, invalidArg("'limit' must be between 1 and 100.")
	}

	result, err := s.svc.ListBranches(ctx, service.ListBranchesInput{
		RepoID: repoID,
		Limit:  limit,
		Cursor: req.Cursor,
	})
	if err != nil {
		return nil, toGRPCError(core.MapDomainError(err))
	}

	resp := &vergev1.ListBranchesResponse{
		Branches:   make([]*vergev1.Branch, 0, len(result.Branches)),
		NextCursor: result.NextCursor,
	}
	for _, b := range result.Branches {
		resp.Branches = append(resp.Branches, toBranchProto(b))
	}

	return resp, nil
}

func (s *BranchServer) AdvanceBranch(
	ctx context.Context,
	req *vergev1.AdvanceBranchRequest,
) (*vergev1.Branch, error) {
	repoID := strings.TrimSpace(req.RepoId)
	name := strings.TrimSpace(req.Name)
	commitID := strings.TrimSpace(req.CommitId)
	expectedCommitID := strings.TrimSpace(req.ExpectedCommitId)

	if repoID == "" {
		return nil, invalidArg("'repo_id' is required and must not be empty.")
	}
	if name == "" {
		return nil, invalidArg("'name' is required and must not be empty.")
	}
	if commitID == "" {
		return nil, invalidArg("'commit_id' is required and must not be empty.")
	}
	if expectedCommitID == "" {
		return nil, invalidArg("'expected_commit_id' is required and must not be empty.")
	}

	branch, err := s.svc.AdvanceBranch(ctx, service.AdvanceBranchInput{
		RepoID:           repoID,
		Name:             name,
		CommitID:         commitID,
		ExpectedCommitID: expectedCommitID,
	})
	if err != nil {
		return nil, toGRPCError(core.MapDomainError(err))
	}

	return toBranchProto(branch), nil
}

func (s *BranchServer) DeleteBranch(
	ctx context.Context,
	req *vergev1.DeleteBranchRequest,
) (*vergev1.DeleteBranchResponse, error) {
	repoID := strings.TrimSpace(req.RepoId)
	name := strings.TrimSpace(req.Name)

	if repoID == "" {
		return nil, invalidArg("'repo_id' is required and must not be empty.")
	}
	if name == "" {
		return nil, invalidArg("'name' is required and must not be empty.")
	}

	if err := s.svc.DeleteBranch(ctx, repoID, name); err != nil {
		return nil, toGRPCError(core.MapDomainError(err))
	}

	return &vergev1.DeleteBranchResponse{}, nil
}

func toBranchProto(b *domain.Branch) *vergev1.Branch {
	return &vergev1.Branch{
		Name:      b.Name,
		RepoId:    b.RepoID,
		CommitId:  b.CommitID,
		CreatedAt: b.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

package v1

import (
	"context"
	"encoding/json"
	"strings"

	vergev1 "github.com/bhpcv252/verge/api/proto/verge/v1"
	"github.com/bhpcv252/verge/internal/api/core"
	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/service"
)

type CommitServer struct {
	vergev1.UnimplementedCommitServiceServer
	svc core.CommitService
}

func NewCommitServer(svc core.CommitService) *CommitServer {
	return &CommitServer{svc: svc}
}

func (s *CommitServer) CreateCommit(
	ctx context.Context,
	req *vergev1.CreateCommitRequest,
) (*vergev1.CreateCommitResponse, error) {
	repoID := strings.TrimSpace(req.RepoId)
	message := strings.TrimSpace(req.Message)
	author := strings.TrimSpace(req.Author)

	if repoID == "" {
		return nil, invalidArg("'repo_id' is required and must not be empty.")
	}
	if message == "" {
		return nil, invalidArg("'message' is required and must not be empty.")
	}
	if author == "" {
		return nil, invalidArg("'author' is required and must not be empty.")
	}
	if req.DataPointer == nil {
		return nil, invalidArg("'data_pointer' is required.")
	}
	if req.DataPointer.Type == "" || req.DataPointer.Location == "" {
		return nil, invalidArg("'data_pointer' must have 'type' and 'location'.")
	}

	dataPointer := domain.DataPointer{
		Type:     req.DataPointer.Type,
		Location: req.DataPointer.Location,
		Hash:     req.DataPointer.Hash,
	}
	if len(req.DataPointer.Metadata) > 0 {
		dataPointer.Metadata = json.RawMessage(req.DataPointer.Metadata)
	}

	result, err := s.svc.CreateCommit(ctx, service.CreateCommitInput{
		RepoID:         repoID,
		ParentIDs:      req.ParentIds,
		DataPointer:    dataPointer,
		Message:        message,
		Author:         author,
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
	})
	if err != nil {
		return nil, toGRPCError(core.MapDomainError(err))
	}

	return &vergev1.CreateCommitResponse{
		Commit:   toCommitProto(result.Commit),
		Existing: result.Existing,
	}, nil
}

func (s *CommitServer) GetCommit(
	ctx context.Context,
	req *vergev1.GetCommitRequest,
) (*vergev1.Commit, error) {
	repoID := strings.TrimSpace(req.RepoId)
	commitID := strings.TrimSpace(req.CommitId)

	if repoID == "" {
		return nil, invalidArg("'repo_id' is required and must not be empty.")
	}
	if commitID == "" {
		return nil, invalidArg("'commit_id' is required and must not be empty.")
	}

	commit, err := s.svc.GetCommit(ctx, repoID, commitID)
	if err != nil {
		return nil, toGRPCError(core.MapDomainError(err))
	}

	return toCommitProto(commit), nil
}

func (s *CommitServer) ListCommits(
	ctx context.Context,
	req *vergev1.ListCommitsRequest,
) (*vergev1.ListCommitsResponse, error) {
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

	result, err := s.svc.ListCommits(ctx, service.ListCommitsInput{
		RepoID:    repoID,
		Branch:    req.Branch,
		Author:    req.Author,
		Since:     req.Since,
		Until:     req.Until,
		Traversal: req.Traversal,
		Limit:     limit,
		Cursor:    req.Cursor,
	})
	if err != nil {
		return nil, toGRPCError(core.MapDomainError(err))
	}

	resp := &vergev1.ListCommitsResponse{
		Commits:    make([]*vergev1.Commit, 0, len(result.Commits)),
		NextCursor: result.NextCursor,
	}
	for _, c := range result.Commits {
		resp.Commits = append(resp.Commits, toCommitProto(c))
	}

	return resp, nil
}

func (s *CommitServer) GetParents(
	ctx context.Context,
	req *vergev1.GetParentsRequest,
) (*vergev1.GetParentsResponse, error) {
	repoID := strings.TrimSpace(req.RepoId)
	commitID := strings.TrimSpace(req.CommitId)

	if repoID == "" {
		return nil, invalidArg("'repo_id' is required and must not be empty.")
	}
	if commitID == "" {
		return nil, invalidArg("'commit_id' is required and must not be empty.")
	}

	parents, err := s.svc.GetParents(ctx, repoID, commitID)
	if err != nil {
		return nil, toGRPCError(core.MapDomainError(err))
	}

	resp := &vergev1.GetParentsResponse{
		CommitId: commitID,
		Parents:  make([]*vergev1.Commit, 0, len(parents)),
	}
	for _, p := range parents {
		resp.Parents = append(resp.Parents, toCommitProto(p))
	}

	return resp, nil
}

func toCommitProto(c *domain.Commit) *vergev1.Commit {
	var metadata []byte
	if c.DataPointer.Metadata != nil {
		metadata = []byte(c.DataPointer.Metadata)
	}

	return &vergev1.Commit{
		Id:     c.ID,
		RepoId: c.RepoID,
		DataPointer: &vergev1.DataPointer{
			Type:     c.DataPointer.Type,
			Location: c.DataPointer.Location,
			Hash:     c.DataPointer.Hash,
			Metadata: metadata,
		},
		ParentIds: c.ParentIDs,
		Message:   c.Message,
		Author:    c.Author,
		Timestamp: c.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

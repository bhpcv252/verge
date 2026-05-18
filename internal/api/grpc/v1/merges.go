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

type MergeServer struct {
	vergev1.UnimplementedMergeServiceServer
	svc core.MergeService
}

func NewMergeServer(svc core.MergeService) *MergeServer {
	return &MergeServer{svc: svc}
}

func (s *MergeServer) CreateMerge(
	ctx context.Context,
	req *vergev1.CreateMergeRequest,
) (*vergev1.Commit, error) {
	repoID := strings.TrimSpace(req.RepoId)
	message := strings.TrimSpace(req.Message)
	author := strings.TrimSpace(req.Author)
	expectedTargetHead := strings.TrimSpace(req.ExpectedTargetHead)
	targetBranch := strings.TrimSpace(req.TargetBranch)

	if repoID == "" {
		return nil, invalidArg("'repo_id' is required and must not be empty.")
	}
	if message == "" {
		return nil, invalidArg("'message' is required and must not be empty.")
	}
	if author == "" {
		return nil, invalidArg("'author' is required and must not be empty.")
	}
	if expectedTargetHead == "" {
		return nil, invalidArg("'expected_target_head' is required and must not be empty.")
	}
	if targetBranch == "" {
		return nil, invalidArg("'target_branch' is required and must not be empty.")
	}
	if len(req.ParentIds) != 2 {
		return nil, invalidArg("merge commits require exactly two parent_ids.")
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

	mergeCommit, err := s.svc.CreateMerge(ctx, service.CreateMergeInput{
		RepoID:             repoID,
		ParentIDs:          req.ParentIds,
		ExpectedTargetHead: expectedTargetHead,
		TargetBranch:       targetBranch,
		DataPointer:        dataPointer,
		Message:            message,
		Author:             author,
	})
	if err != nil {
		return nil, toGRPCError(core.MapDomainError(err))
	}

	return toMergeCommitProto(mergeCommit), nil
}

func toMergeCommitProto(c *domain.Commit) *vergev1.Commit {
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

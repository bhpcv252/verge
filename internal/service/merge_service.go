package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/bhpcv252/verge/internal/domain"
)

type MergeStore interface {
	CreateMerge(
		ctx context.Context,
		commit *domain.Commit,
		parentIDs []string,
		targetBranch, expectedTargetHead string,
	) (*domain.Commit, error)
}

type MergeService struct {
	store       MergeStore
	repoStore   RepoStore
	commitStore CommitStore
	branchStore BranchStore
}

func NewMergeService(
	store MergeStore,
	repoStore RepoStore,
	commitStore CommitStore,
	branchStore BranchStore,
) *MergeService {
	return &MergeService{
		store:       store,
		repoStore:   repoStore,
		commitStore: commitStore,
		branchStore: branchStore,
	}
}

type CreateMergeInput struct {
	RepoID             string
	ParentIDs          []string // must be exactly 2
	ExpectedTargetHead string   // required - optimistic lock
	TargetBranch       string
	DataPointer        domain.DataPointer
	Message            string
	Author             string
}

func (s *MergeService) CreateMerge(
	ctx context.Context,
	in CreateMergeInput,
) (*domain.Commit, error) {
	// validate parent_ids count first,
	// must be EXACTLY 2 for merge commits
	if len(in.ParentIDs) != 2 {
		return nil, fmt.Errorf("merge commits require exactly two parent_ids")
	}

	if err := in.DataPointer.Validate(); err != nil {
		return nil, fmt.Errorf("service: create merge: %w", err)
	}

	_, err := s.repoStore.GetByID(ctx, in.RepoID)
	if err != nil {
		return nil, fmt.Errorf("service: create merge: %w", err)
	}

	// validate both parent commits exist in this repo
	if err := s.commitStore.ValidateParentsExist(ctx, in.RepoID, in.ParentIDs); err != nil {
		return nil, fmt.Errorf("service: create merge: %w", err)
	}

	// validate target branch exists
	_, err = s.branchStore.GetByName(ctx, in.RepoID, in.TargetBranch)
	if err != nil {
		return nil, fmt.Errorf("service: create merge: %w", err)
	}

	// create merge commit with 2 parents
	mergeCommit := &domain.Commit{
		ID:          "commit_" + uuid.New().String(),
		RepoID:      in.RepoID,
		ParentIDs:   in.ParentIDs,
		DataPointer: in.DataPointer,
		Message:     in.Message,
		Author:      in.Author,
		Timestamp:   time.Now().UTC(),
	}

	createdCommit, err := s.store.CreateMerge(
		ctx,
		mergeCommit,
		in.ParentIDs,
		in.TargetBranch,
		in.ExpectedTargetHead,
	)
	if err != nil {
		return nil, fmt.Errorf("service: create merge: %w", err)
	}

	return createdCommit, nil
}

// StaleMergeTargetError is returned when the target
// branch has moved since expected_target_head was read
type StaleMergeTargetError struct {
	BranchName   string
	CurrentHead  string
	ExpectedHead string
}

func (e *StaleMergeTargetError) Error() string {
	return fmt.Sprintf("stale merge target: branch %q is at %q but expected %q",
		e.BranchName, e.CurrentHead, e.ExpectedHead)
}

func (e *StaleMergeTargetError) Is(target error) bool {
	return target == domain.ErrStaleMergeTarget
}

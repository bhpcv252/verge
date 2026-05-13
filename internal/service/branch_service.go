package service

import (
	"context"
	"fmt"
	"time"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/storage/postgres"
)

type BranchStore interface {
	Create(ctx context.Context, branch *domain.Branch) error
	GetByName(ctx context.Context, repoID, name string) (*domain.Branch, error)
	List(
		ctx context.Context,
		repoID string,
		limit int,
		cursor string,
	) (*postgres.ListBranchesPage, error)
	Advance(
		ctx context.Context,
		repoID, name, commitID, expectedCommitID string,
	) (*domain.Branch, error)
	Delete(ctx context.Context, repoID, name string) error
}

type BranchService struct {
	store       BranchStore
	repoStore   RepoStore
	commitStore CommitStore
}

func NewBranchService(
	store BranchStore,
	repoStore RepoStore,
	commitStore CommitStore,
) *BranchService {
	return &BranchService{
		store:       store,
		repoStore:   repoStore,
		commitStore: commitStore,
	}
}

type CreateBranchInput struct {
	RepoID         string
	Name           string
	SourceCommitID string
}

type ListBranchesInput struct {
	RepoID string
	Limit  int
	Cursor string
}

type ListBranchesResult struct {
	Branches   []*domain.Branch
	NextCursor string
}

type AdvanceBranchInput struct {
	RepoID           string
	Name             string
	CommitID         string
	ExpectedCommitID string
}

func (s *BranchService) CreateBranch(
	ctx context.Context,
	in CreateBranchInput,
) (*domain.Branch, error) {
	// validate repo exists
	_, err := s.repoStore.GetByID(ctx, in.RepoID)
	if err != nil {
		return nil, fmt.Errorf("service: create branch: %w", err)
	}

	// validate source_commit_id exists in this repo
	_, err = s.commitStore.GetByID(ctx, in.RepoID, in.SourceCommitID)
	if err != nil {
		return nil, fmt.Errorf("service: create branch: %w", err)
	}

	branch := &domain.Branch{
		Name:      in.Name,
		RepoID:    in.RepoID,
		CommitID:  in.SourceCommitID,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.store.Create(ctx, branch); err != nil {
		return nil, fmt.Errorf("service: create branch: %w", err)
	}

	return branch, nil
}

func (s *BranchService) GetBranch(
	ctx context.Context,
	repoID, name string,
) (*domain.Branch, error) {
	_, err := s.repoStore.GetByID(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("service: get branch: %w", err)
	}

	branch, err := s.store.GetByName(ctx, repoID, name)
	if err != nil {
		return nil, fmt.Errorf("service: get branch: %w", err)
	}

	return branch, nil
}

func (s *BranchService) ListBranches(
	ctx context.Context,
	in ListBranchesInput,
) (*ListBranchesResult, error) {
	_, err := s.repoStore.GetByID(ctx, in.RepoID)
	if err != nil {
		return nil, fmt.Errorf("service: list branches: %w", err)
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	page, err := s.store.List(ctx, in.RepoID, limit, in.Cursor)
	if err != nil {
		return nil, fmt.Errorf("service: list branches: %w", err)
	}

	return &ListBranchesResult{
		Branches:   page.Branches,
		NextCursor: page.NextCursor,
	}, nil
}

func (s *BranchService) AdvanceBranch(
	ctx context.Context,
	in AdvanceBranchInput,
) (*domain.Branch, error) {
	_, err := s.repoStore.GetByID(ctx, in.RepoID)
	if err != nil {
		return nil, fmt.Errorf("service: advance branch: %w", err)
	}

	// validate expected_commit_id is provided
	if in.ExpectedCommitID == "" {
		return nil, fmt.Errorf(
			"service: advance branch: expected_commit_id is required for optimistic locking",
		)
	}

	// validate commit_id exists in this repo
	_, err = s.commitStore.GetByID(ctx, in.RepoID, in.CommitID)
	if err != nil {
		return nil, fmt.Errorf("service: advance branch: %w", err)
	}

	branch, err := s.store.Advance(ctx, in.RepoID, in.Name, in.CommitID, in.ExpectedCommitID)
	if err != nil {
		return nil, fmt.Errorf("service: advance branch: %w", err)
	}

	return branch, nil
}

func (s *BranchService) DeleteBranch(ctx context.Context, repoID, name string) error {
	repo, err := s.repoStore.GetByID(ctx, repoID)
	if err != nil {
		return fmt.Errorf("service: delete branch: %w", err)
	}

	if name == repo.DefaultBranch {
		return domain.ErrCannotDeleteDefaultBranch
	}

	if err := s.store.Delete(ctx, repoID, name); err != nil {
		return fmt.Errorf("service: delete branch: %w", err)
	}

	return nil
}

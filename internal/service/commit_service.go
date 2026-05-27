package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/storage/postgres"
)

type CommitStore interface {
	Create(ctx context.Context, commit *domain.Commit, parentIDs []string) (*domain.Commit, error)
	GetByID(ctx context.Context, repoID, commitID string) (*domain.Commit, error)
	GetByIdempotencyKey(ctx context.Context, repoID, idempotencyKey string) (*domain.Commit, error)
	List(ctx context.Context, in postgres.ListCommitsFilter) (*postgres.ListCommitsPage, error)
	GetParents(ctx context.Context, repoID, commitID string) ([]*domain.Commit, error)
	ValidateParentsExist(ctx context.Context, repoID string, parentIDs []string) error
}

type CommitService struct {
	store     CommitStore
	repoStore RepoStore
}

func NewCommitService(store CommitStore, repoStore RepoStore) *CommitService {
	return &CommitService{
		store:     store,
		repoStore: repoStore,
	}
}

type CreateCommitInput struct {
	RepoID         string
	ParentIDs      []string
	DataPointer    domain.DataPointer
	Message        string
	Author         string
	IdempotencyKey string // optional
}

type CreateCommitResult struct {
	Commit   *domain.Commit
	Existing bool // true if idempotency_key matched an existing commit
}

type ListCommitsInput struct {
	RepoID    string
	Branch    string // filter by branch
	Author    string // filter by author
	Since     string // ISO 8601 timestamp
	Until     string // ISO 8601 timestamp
	Traversal string // "flat" | "dag"
	Limit     int
	Cursor    string
}

type ListCommitsResult struct {
	Commits    []*domain.Commit
	NextCursor string
}

func (s *CommitService) CreateCommit(
	ctx context.Context,
	in CreateCommitInput,
) (*CreateCommitResult, error) {
	// validate repo exists
	_, err := s.repoStore.GetByID(ctx, in.RepoID)
	if err != nil {
		return nil, fmt.Errorf("service: create commit: %w", err)
	}

	// validate DataPointer fields
	if err := in.DataPointer.Validate(); err != nil {
		return nil, &ValidationError{Msg: err.Error()}
	}

	// commits accept 0 or 1 parent
	if len(in.ParentIDs) > 1 {
		return nil, &ValidationError{
			Msg: "commits accept zero or one parent_ids; use the merges endpoint for merge commits with two parents",
		}
	}

	// idempotency: return existing commit when key matches
	if in.IdempotencyKey != "" {
		existing, err := s.store.GetByIdempotencyKey(ctx, in.RepoID, in.IdempotencyKey)
		if err == nil {
			return &CreateCommitResult{Commit: existing, Existing: true}, nil
		}
		if err != domain.ErrCommitNotFound {
			return nil, fmt.Errorf("service: create commit: check idempotency: %w", err)
		}
		// not found, proceed with creation
	}

	// validate parent_ids exist in this repo.
	if len(in.ParentIDs) > 0 {
		if err := s.store.ValidateParentsExist(ctx, in.RepoID, in.ParentIDs); err != nil {
			return nil, fmt.Errorf("service: create commit: %w", err)
		}
	}

	commit := &domain.Commit{
		ID:             "commit_" + uuid.New().String(),
		RepoID:         in.RepoID,
		ParentIDs:      in.ParentIDs,
		DataPointer:    in.DataPointer,
		Message:        in.Message,
		Author:         in.Author,
		Timestamp:      time.Now().UTC(),
		IdempotencyKey: in.IdempotencyKey,
	}

	createdCommit, err := s.store.Create(ctx, commit, in.ParentIDs)
	if err != nil {
		return nil, fmt.Errorf("service: create commit: %w", err)
	}

	return &CreateCommitResult{Commit: createdCommit, Existing: false}, nil
}

func (s *CommitService) GetCommit(
	ctx context.Context,
	repoID, commitID string,
) (*domain.Commit, error) {
	_, err := s.repoStore.GetByID(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("service: get commit: %w", err)
	}

	commit, err := s.store.GetByID(ctx, repoID, commitID)
	if err != nil {
		return nil, fmt.Errorf("service: get commit: %w", err)
	}

	return commit, nil
}

func (s *CommitService) ListCommits(
	ctx context.Context,
	in ListCommitsInput,
) (*ListCommitsResult, error) {
	_, err := s.repoStore.GetByID(ctx, in.RepoID)
	if err != nil {
		return nil, fmt.Errorf("service: list commits: %w", err)
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var since, until *time.Time
	if in.Since != "" {
		t, err := time.Parse(time.RFC3339, in.Since)
		if err != nil {
			return nil, &ValidationError{
				Msg: fmt.Sprintf("'since' must be a valid ISO 8601 timestamp, got: %q", in.Since),
			}
		}
		since = &t
	}
	if in.Until != "" {
		t, err := time.Parse(time.RFC3339, in.Until)
		if err != nil {
			return nil, &ValidationError{
				Msg: fmt.Sprintf("'until' must be a valid ISO 8601 timestamp, got: %q", in.Until),
			}
		}
		until = &t
	}

	traversal := in.Traversal
	if traversal == "" {
		traversal = "flat"
	}
	if traversal != "flat" && traversal != "dag" {
		return nil, &ValidationError{Msg: "'traversal' must be 'flat' or 'dag'"}
	}

	// DAG traversal requires a branch as starting point
	if traversal == "dag" && in.Branch == "" {
		return nil, &ValidationError{Msg: "'traversal=dag' requires a 'branch' parameter"}
	}

	page, err := s.store.List(ctx, postgres.ListCommitsFilter{
		RepoID:    in.RepoID,
		Branch:    in.Branch,
		Author:    in.Author,
		Since:     since,
		Until:     until,
		Traversal: traversal,
		Limit:     limit,
		Cursor:    in.Cursor,
	})
	if err != nil {
		return nil, fmt.Errorf("service: list commits: %w", err)
	}

	return &ListCommitsResult{
		Commits:    page.Commits,
		NextCursor: page.NextCursor,
	}, nil
}

func (s *CommitService) GetParents(
	ctx context.Context,
	repoID, commitID string,
) ([]*domain.Commit, error) {
	_, err := s.repoStore.GetByID(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("service: get parents: %w", err)
	}

	_, err = s.store.GetByID(ctx, repoID, commitID)
	if err != nil {
		return nil, fmt.Errorf("service: get parents: %w", err)
	}

	parents, err := s.store.GetParents(ctx, repoID, commitID)
	if err != nil {
		return nil, fmt.Errorf("service: get parents: %w", err)
	}

	return parents, nil
}

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/storage/postgres"
)

type RepoStore interface {
	Create(ctx context.Context, repo *domain.Repo) error
	GetByID(ctx context.Context, id string) (*domain.Repo, error)
	List(ctx context.Context, limit int, cursor string) (*postgres.ListReposPage, error)
}

type RepoService struct {
	store RepoStore
}

func NewRepoService(store RepoStore) *RepoService {
	return &RepoService{store: store}
}

type CreateRepoInput struct {
	Name          string
	DefaultBranch string
}

type ListReposInput struct {
	Limit  int
	Cursor string
}

type ListReposResult struct {
	Repos      []*domain.Repo
	NextCursor string
}

func (s *RepoService) CreateRepo(ctx context.Context, in CreateRepoInput) (*domain.Repo, error) {
	repo := &domain.Repo{
		ID:            "repo_" + uuid.New().String(),
		Name:          in.Name,
		DefaultBranch: in.DefaultBranch,
		CreatedAt:     time.Now().UTC(),
	}

	if err := s.store.Create(ctx, repo); err != nil {
		return nil, fmt.Errorf("service: create repo: %w", err)
	}

	return repo, nil
}

func (s *RepoService) GetRepo(ctx context.Context, id string) (*domain.Repo, error) {
	repo, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("service: get repo: %w", err)
	}
	return repo, nil
}

// limit is clamped to [1, 100]
// defaults to 20
func (s *RepoService) ListRepos(ctx context.Context, in ListReposInput) (*ListReposResult, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	page, err := s.store.List(ctx, limit, in.Cursor)
	if err != nil {
		return nil, fmt.Errorf("service: list repos: %w", err)
	}

	return &ListReposResult{
		Repos:      page.Repos,
		NextCursor: page.NextCursor,
	}, nil
}

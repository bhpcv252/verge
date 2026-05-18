package core

import (
	"context"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/service"
)

type RepoService interface {
	CreateRepo(ctx context.Context, in service.CreateRepoInput) (*domain.Repo, error)
	GetRepo(ctx context.Context, id string) (*domain.Repo, error)
	ListRepos(ctx context.Context, in service.ListReposInput) (*service.ListReposResult, error)
}

type BranchService interface {
	CreateBranch(ctx context.Context, in service.CreateBranchInput) (*domain.Branch, error)
	GetBranch(ctx context.Context, repoID, name string) (*domain.Branch, error)
	ListBranches(
		ctx context.Context,
		in service.ListBranchesInput,
	) (*service.ListBranchesResult, error)
	AdvanceBranch(ctx context.Context, in service.AdvanceBranchInput) (*domain.Branch, error)
	DeleteBranch(ctx context.Context, repoID, name string) error
}

type CommitService interface {
	CreateCommit(
		ctx context.Context,
		in service.CreateCommitInput,
	) (*service.CreateCommitResult, error)
	GetCommit(ctx context.Context, repoID, commitID string) (*domain.Commit, error)
	ListCommits(
		ctx context.Context,
		in service.ListCommitsInput,
	) (*service.ListCommitsResult, error)
	GetParents(ctx context.Context, repoID, commitID string) ([]*domain.Commit, error)
}

type MergeService interface {
	CreateMerge(ctx context.Context, in service.CreateMergeInput) (*domain.Commit, error)
}

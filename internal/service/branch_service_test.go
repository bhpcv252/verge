package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/storage/postgres"
)

// mocks

type mockBranchStore struct {
	createFn    func(ctx context.Context, branch *domain.Branch) error
	getByNameFn func(ctx context.Context, repoID, name string) (*domain.Branch, error)
	listFn      func(ctx context.Context, repoID string, limit int, cursor string) (*postgres.ListBranchesPage, error)
	advanceFn   func(ctx context.Context, repoID, name, commitID, expectedCommitID string) (*domain.Branch, error)
	deleteFn    func(ctx context.Context, repoID, name string) error
}

func (m *mockBranchStore) Create(ctx context.Context, branch *domain.Branch) error {
	return m.createFn(ctx, branch)
}

func (m *mockBranchStore) GetByName(
	ctx context.Context,
	repoID, name string,
) (*domain.Branch, error) {
	return m.getByNameFn(ctx, repoID, name)
}

func (m *mockBranchStore) List(
	ctx context.Context,
	repoID string,
	limit int,
	cursor string,
) (*postgres.ListBranchesPage, error) {
	return m.listFn(ctx, repoID, limit, cursor)
}

func (m *mockBranchStore) Advance(
	ctx context.Context,
	repoID, name, commitID, expectedCommitID string,
) (*domain.Branch, error) {
	return m.advanceFn(ctx, repoID, name, commitID, expectedCommitID)
}

func (m *mockBranchStore) Delete(ctx context.Context, repoID, name string) error {
	return m.deleteFn(ctx, repoID, name)
}

// Create

func TestCreateBranch_RepoNotFound_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, _ string) (*domain.Repo, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	svc := NewBranchService(nil, repoStore, nil)
	_, err := svc.CreateBranch(context.Background(), CreateBranchInput{
		RepoID:         "repo_missing",
		Name:           "main",
		SourceCommitID: "commit_abc",
	})

	assert.ErrorIs(t, err, domain.ErrRepoNotFound)
}

func TestCreateBranch_CommitNotFound_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		getByIDFn: func(_ context.Context, _, _ string) (*domain.Commit, error) {
			return nil, domain.ErrCommitNotFound
		},
	}

	svc := NewBranchService(nil, repoStore, commitStore)
	_, err := svc.CreateBranch(context.Background(), CreateBranchInput{
		RepoID:         "repo_xyz",
		Name:           "main",
		SourceCommitID: "commit_missing",
	})

	assert.ErrorIs(t, err, domain.ErrCommitNotFound)
}

func TestCreateBranch_BranchAlreadyExists_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		getByIDFn: func(_ context.Context, repoID, commitID string) (*domain.Commit, error) {
			return &domain.Commit{ID: commitID, RepoID: repoID}, nil
		},
	}

	branchStore := &mockBranchStore{
		createFn: func(_ context.Context, branch *domain.Branch) error {
			return domain.ErrBranchAlreadyExists
		},
	}

	svc := NewBranchService(branchStore, repoStore, commitStore)
	_, err := svc.CreateBranch(context.Background(), CreateBranchInput{
		RepoID:         "repo_xyz",
		Name:           "existing-branch",
		SourceCommitID: "commit_abc",
	})

	assert.ErrorIs(t, err, domain.ErrBranchAlreadyExists)
}

func TestCreateBranch_ValidInput_CallsStoreCreate(t *testing.T) {
	var capturedBranch *domain.Branch

	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		getByIDFn: func(_ context.Context, repoID, commitID string) (*domain.Commit, error) {
			return &domain.Commit{ID: commitID, RepoID: repoID}, nil
		},
	}

	branchStore := &mockBranchStore{
		createFn: func(_ context.Context, branch *domain.Branch) error {
			capturedBranch = branch
			return nil
		},
	}

	svc := NewBranchService(branchStore, repoStore, commitStore)
	branch, err := svc.CreateBranch(context.Background(), CreateBranchInput{
		RepoID:         "repo_xyz",
		Name:           "feature-x",
		SourceCommitID: "commit_abc",
	})

	require.NoError(t, err)
	assert.Equal(t, "feature-x", branch.Name)
	assert.Equal(t, "repo_xyz", branch.RepoID)
	assert.Equal(t, "commit_abc", branch.CommitID)
	assert.False(t, branch.CreatedAt.IsZero())

	require.NotNil(t, capturedBranch)
	assert.Equal(t, "feature-x", capturedBranch.Name)
	assert.Equal(t, "commit_abc", capturedBranch.CommitID)
}

// GetBranch

func TestGetBranch_RepoNotFound_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, _ string) (*domain.Repo, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	svc := NewBranchService(nil, repoStore, nil)
	_, err := svc.GetBranch(context.Background(), "repo_missing", "main")

	assert.ErrorIs(t, err, domain.ErrRepoNotFound)
}

func TestGetBranch_BranchNotFound_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	branchStore := &mockBranchStore{
		getByNameFn: func(_ context.Context, _, _ string) (*domain.Branch, error) {
			return nil, domain.ErrBranchNotFound
		},
	}

	svc := NewBranchService(branchStore, repoStore, nil)
	_, err := svc.GetBranch(context.Background(), "repo_xyz", "missing")

	assert.ErrorIs(t, err, domain.ErrBranchNotFound)
}

// ListBranches

func TestListBranches_RepoNotFound_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, _ string) (*domain.Repo, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	svc := NewBranchService(nil, repoStore, nil)
	_, err := svc.ListBranches(context.Background(), ListBranchesInput{
		RepoID: "repo_missing",
	})

	assert.ErrorIs(t, err, domain.ErrRepoNotFound)
}

func TestListBranches_DefaultLimit_PassedToStore(t *testing.T) {
	var capturedLimit int

	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	branchStore := &mockBranchStore{
		listFn: func(_ context.Context, _ string, limit int, _ string) (*postgres.ListBranchesPage, error) {
			capturedLimit = limit
			return &postgres.ListBranchesPage{Branches: []*domain.Branch{}}, nil
		},
	}

	svc := NewBranchService(branchStore, repoStore, nil)
	_, err := svc.ListBranches(context.Background(), ListBranchesInput{
		RepoID: "repo_xyz",
		Limit:  0, // should default to 20
	})

	require.NoError(t, err)
	assert.Equal(t, 20, capturedLimit)
}

func TestListBranches_ExplicitLimit_PassedToStore(t *testing.T) {
	var capturedLimit int

	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	branchStore := &mockBranchStore{
		listFn: func(_ context.Context, _ string, limit int, _ string) (*postgres.ListBranchesPage, error) {
			capturedLimit = limit
			return &postgres.ListBranchesPage{Branches: []*domain.Branch{}}, nil
		},
	}

	svc := NewBranchService(branchStore, repoStore, nil)
	_, err := svc.ListBranches(context.Background(), ListBranchesInput{
		RepoID: "repo_xyz",
		Limit:  50,
	})

	require.NoError(t, err)
	assert.Equal(t, 50, capturedLimit)
}

func TestListBranches_LimitOverMax_CappedAt100(t *testing.T) {
	var capturedLimit int

	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	branchStore := &mockBranchStore{
		listFn: func(_ context.Context, _ string, limit int, _ string) (*postgres.ListBranchesPage, error) {
			capturedLimit = limit
			return &postgres.ListBranchesPage{Branches: []*domain.Branch{}}, nil
		},
	}

	svc := NewBranchService(branchStore, repoStore, nil)
	_, err := svc.ListBranches(context.Background(), ListBranchesInput{
		RepoID: "repo_xyz",
		Limit:  200, // over max
	})

	require.NoError(t, err)
	assert.Equal(t, 100, capturedLimit, "limit should be capped at 100")
}

// AdvanceBranch

func TestAdvanceBranch_RepoNotFound_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, _ string) (*domain.Repo, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	svc := NewBranchService(nil, repoStore, nil)
	_, err := svc.AdvanceBranch(context.Background(), AdvanceBranchInput{
		RepoID:           "repo_missing",
		Name:             "main",
		CommitID:         "commit_new",
		ExpectedCommitID: "commit_old",
	})

	assert.ErrorIs(t, err, domain.ErrRepoNotFound)
}

func TestAdvanceBranch_CommitNotFound_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		getByIDFn: func(_ context.Context, _, _ string) (*domain.Commit, error) {
			return nil, domain.ErrCommitNotFound
		},
	}

	svc := NewBranchService(nil, repoStore, commitStore)
	_, err := svc.AdvanceBranch(context.Background(), AdvanceBranchInput{
		RepoID:           "repo_xyz",
		Name:             "main",
		CommitID:         "commit_missing",
		ExpectedCommitID: "commit_old",
	})

	assert.ErrorIs(t, err, domain.ErrCommitNotFound)
}

func TestAdvanceBranch_ExpectedCommitIDMissing_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	svc := NewBranchService(nil, repoStore, nil)
	_, err := svc.AdvanceBranch(context.Background(), AdvanceBranchInput{
		RepoID:           "repo_xyz",
		Name:             "main",
		CommitID:         "commit_new",
		ExpectedCommitID: "", // missing expected commit ID
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected_commit_id")
}

func TestAdvanceBranch_OptimisticLockFails_ReturnsBranchConflict(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		getByIDFn: func(_ context.Context, repoID, commitID string) (*domain.Commit, error) {
			return &domain.Commit{ID: commitID, RepoID: repoID}, nil
		},
	}

	branchStore := &mockBranchStore{
		advanceFn: func(_ context.Context, repoID, name, commitID, expectedCommitID string) (*domain.Branch, error) {
			// simulate optimistic lock failure (0 rows updated)
			return nil, &postgres.BranchConflictError{CurrentHead: "commit_actual"}
		},
	}

	svc := NewBranchService(branchStore, repoStore, commitStore)
	_, err := svc.AdvanceBranch(context.Background(), AdvanceBranchInput{
		RepoID:           "repo_xyz",
		Name:             "main",
		CommitID:         "commit_new",
		ExpectedCommitID: "commit_stale",
	})

	assert.ErrorIs(t, err, domain.ErrBranchConflict)

	var conflictErr *postgres.BranchConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Equal(t, "commit_actual", conflictErr.CurrentHead)
}

func TestAdvanceBranch_ValidInput_CallsStoreAdvance(t *testing.T) {
	var capturedRepoID, capturedName, capturedCommitID, capturedExpectedCommitID string

	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		getByIDFn: func(_ context.Context, repoID, commitID string) (*domain.Commit, error) {
			return &domain.Commit{ID: commitID, RepoID: repoID}, nil
		},
	}

	branchStore := &mockBranchStore{
		advanceFn: func(_ context.Context, repoID, name, commitID, expectedCommitID string) (*domain.Branch, error) {
			capturedRepoID = repoID
			capturedName = name
			capturedCommitID = commitID
			capturedExpectedCommitID = expectedCommitID
			return &domain.Branch{
				Name:      name,
				RepoID:    repoID,
				CommitID:  commitID,
				CreatedAt: time.Now(),
			}, nil
		},
	}

	svc := NewBranchService(branchStore, repoStore, commitStore)
	branch, err := svc.AdvanceBranch(context.Background(), AdvanceBranchInput{
		RepoID:           "repo_xyz",
		Name:             "main",
		CommitID:         "commit_new",
		ExpectedCommitID: "commit_old",
	})

	require.NoError(t, err)
	assert.Equal(t, "commit_new", branch.CommitID)

	assert.Equal(t, "repo_xyz", capturedRepoID)
	assert.Equal(t, "main", capturedName)
	assert.Equal(t, "commit_new", capturedCommitID)
	assert.Equal(t, "commit_old", capturedExpectedCommitID)
}

// DeleteBranch

func TestDeleteBranch_RepoNotFound_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, _ string) (*domain.Repo, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	svc := NewBranchService(nil, repoStore, nil)
	err := svc.DeleteBranch(context.Background(), "repo_missing", "feature-x")

	assert.ErrorIs(t, err, domain.ErrRepoNotFound)
}

func TestDeleteBranch_DefaultBranch_ReturnsCannotDeleteDefaultBranch(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id, DefaultBranch: "main"}, nil
		},
	}

	svc := NewBranchService(nil, repoStore, nil)
	err := svc.DeleteBranch(context.Background(), "repo_xyz", "main")

	assert.ErrorIs(t, err, domain.ErrCannotDeleteDefaultBranch)
}

func TestDeleteBranch_NonDefaultBranch_CallsStoreDelete(t *testing.T) {
	var capturedRepoID, capturedName string

	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id, DefaultBranch: "main"}, nil
		},
	}

	branchStore := &mockBranchStore{
		deleteFn: func(_ context.Context, repoID, name string) error {
			capturedRepoID = repoID
			capturedName = name
			return nil
		},
	}

	svc := NewBranchService(branchStore, repoStore, nil)
	err := svc.DeleteBranch(context.Background(), "repo_xyz", "feature-x")

	require.NoError(t, err)
	assert.Equal(t, "repo_xyz", capturedRepoID)
	assert.Equal(t, "feature-x", capturedName)
}

func TestDeleteBranch_BranchNotFound_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id, DefaultBranch: "main"}, nil
		},
	}

	branchStore := &mockBranchStore{
		deleteFn: func(_ context.Context, _, _ string) error {
			return domain.ErrBranchNotFound
		},
	}

	svc := NewBranchService(branchStore, repoStore, nil)
	err := svc.DeleteBranch(context.Background(), "repo_xyz", "missing")

	assert.ErrorIs(t, err, domain.ErrBranchNotFound)
}

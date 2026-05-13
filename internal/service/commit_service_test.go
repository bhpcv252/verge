package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/storage/postgres"
)

// mocks

type mockCommitStore struct {
	createFn               func(ctx context.Context, commit *domain.Commit, parentIDs []string) (*domain.Commit, error)
	getByIDFn              func(ctx context.Context, repoID, commitID string) (*domain.Commit, error)
	getByIdempotencyKeyFn  func(ctx context.Context, repoID, idempotencyKey string) (*domain.Commit, error)
	listFn                 func(ctx context.Context, filter postgres.ListCommitsFilter) (*postgres.ListCommitsPage, error)
	getParentsFn           func(ctx context.Context, repoID, commitID string) ([]*domain.Commit, error)
	validateParentsExistFn func(ctx context.Context, repoID string, parentIDs []string) error
}

func (m *mockCommitStore) Create(
	ctx context.Context,
	commit *domain.Commit,
	parentIDs []string,
) (*domain.Commit, error) {
	if m.createFn != nil {
		return m.createFn(ctx, commit, parentIDs)
	}
	return nil, nil
}

func (m *mockCommitStore) GetByID(
	ctx context.Context,
	repoID, commitID string,
) (*domain.Commit, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, repoID, commitID)
	}
	return nil, nil
}

func (m *mockCommitStore) GetByIdempotencyKey(
	ctx context.Context,
	repoID, idempotencyKey string,
) (*domain.Commit, error) {
	if m.getByIdempotencyKeyFn != nil {
		return m.getByIdempotencyKeyFn(ctx, repoID, idempotencyKey)
	}
	return nil, nil
}

func (m *mockCommitStore) List(
	ctx context.Context,
	filter postgres.ListCommitsFilter,
) (*postgres.ListCommitsPage, error) {
	if m.listFn != nil {
		return m.listFn(ctx, filter)
	}
	return nil, nil
}

func (m *mockCommitStore) GetParents(
	ctx context.Context,
	repoID, commitID string,
) ([]*domain.Commit, error) {
	if m.getParentsFn != nil {
		return m.getParentsFn(ctx, repoID, commitID)
	}
	return nil, nil
}

func (m *mockCommitStore) ValidateParentsExist(
	ctx context.Context,
	repoID string,
	parentIDs []string,
) error {
	if m.validateParentsExistFn != nil {
		return m.validateParentsExistFn(ctx, repoID, parentIDs)
	}
	return nil
}

// Create

func TestCreateCommit_RepoNotFound_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, _ string) (*domain.Repo, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	svc := NewCommitService(nil, repoStore)
	_, err := svc.CreateCommit(context.Background(), CreateCommitInput{
		RepoID: "repo_missing",
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "test",
		Author:  "test@example.com",
	})

	assert.ErrorIs(t, err, domain.ErrRepoNotFound)
}

func TestCreateCommit_InvalidDataPointer_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	svc := NewCommitService(nil, repoStore)
	_, err := svc.CreateCommit(context.Background(), CreateCommitInput{
		RepoID: "repo_xyz",
		DataPointer: domain.DataPointer{
			Type:     "invalid-type",
			Location: "test/fixture",
		},
		Message: "test",
		Author:  "test@example.com",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "data_pointer")
}

func TestCreateCommit_TwoParents_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	svc := NewCommitService(nil, repoStore)
	_, err := svc.CreateCommit(context.Background(), CreateCommitInput{
		RepoID:    "repo_xyz",
		ParentIDs: []string{"commit_1", "commit_2"}, // 2 parents
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "test",
		Author:  "test@example.com",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "merge")
}

func TestCreateCommit_IdempotencyKeyMatch_ReturnsExistingCommit(t *testing.T) {
	existingCommit := &domain.Commit{
		ID:             "commit_existing",
		RepoID:         "repo_xyz",
		IdempotencyKey: "key_123",
	}

	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		getByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.Commit, error) {
			return existingCommit, nil
		},
	}

	svc := NewCommitService(commitStore, repoStore)
	result, err := svc.CreateCommit(context.Background(), CreateCommitInput{
		RepoID:         "repo_xyz",
		IdempotencyKey: "key_123",
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "test",
		Author:  "test@example.com",
	})

	require.NoError(t, err)
	assert.True(t, result.Existing)
	assert.Equal(t, "commit_existing", result.Commit.ID)
}

func TestCreateCommit_InvalidParent_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		getByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.Commit, error) {
			return nil, domain.ErrCommitNotFound
		},
		validateParentsExistFn: func(_ context.Context, _ string, _ []string) error {
			return domain.ErrInvalidParent
		},
	}

	svc := NewCommitService(commitStore, repoStore)
	_, err := svc.CreateCommit(context.Background(), CreateCommitInput{
		RepoID:    "repo_xyz",
		ParentIDs: []string{"commit_nonexistent"},
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "test",
		Author:  "test@example.com",
	})

	assert.ErrorIs(t, err, domain.ErrInvalidParent)
}

func TestCreateCommit_RootCommit_CallsStoreCreate(t *testing.T) {
	var capturedCommit *domain.Commit
	var capturedParentIDs []string

	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		getByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.Commit, error) {
			return nil, domain.ErrCommitNotFound
		},
		createFn: func(_ context.Context, commit *domain.Commit, parentIDs []string) (*domain.Commit, error) {
			capturedCommit = commit
			capturedParentIDs = parentIDs
			return commit, nil
		},
	}

	svc := NewCommitService(commitStore, repoStore)
	result, err := svc.CreateCommit(context.Background(), CreateCommitInput{
		RepoID:    "repo_xyz",
		ParentIDs: []string{}, // root commit
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Initial commit",
		Author:  "test@example.com",
	})

	require.NoError(t, err)
	assert.False(t, result.Existing)
	assert.Equal(t, "repo_xyz", capturedCommit.RepoID)
	assert.Equal(t, "Initial commit", capturedCommit.Message)
	assert.Len(t, capturedParentIDs, 0)
}

func TestCreateCommit_RegularCommit_CallsStoreCreate(t *testing.T) {
	var capturedCommit *domain.Commit
	var capturedParentIDs []string

	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		getByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.Commit, error) {
			return nil, domain.ErrCommitNotFound
		},
		validateParentsExistFn: func(_ context.Context, _ string, _ []string) error {
			return nil
		},
		createFn: func(_ context.Context, commit *domain.Commit, parentIDs []string) (*domain.Commit, error) {
			capturedCommit = commit
			capturedParentIDs = parentIDs
			return commit, nil
		},
	}

	svc := NewCommitService(commitStore, repoStore)
	result, err := svc.CreateCommit(context.Background(), CreateCommitInput{
		RepoID:    "repo_xyz",
		ParentIDs: []string{"commit_parent"},
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Add feature",
		Author:  "test@example.com",
	})

	require.NoError(t, err)
	assert.False(t, result.Existing)
	assert.Equal(t, "repo_xyz", capturedCommit.RepoID)
	assert.Equal(t, []string{"commit_parent"}, capturedParentIDs)
}

// GetCommit

func TestGetCommit_RepoNotFound_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, _ string) (*domain.Repo, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	svc := NewCommitService(nil, repoStore)
	_, err := svc.GetCommit(context.Background(), "repo_missing", "commit_abc")

	assert.ErrorIs(t, err, domain.ErrRepoNotFound)
}

func TestGetCommit_CommitNotFound_ReturnsError(t *testing.T) {
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

	svc := NewCommitService(commitStore, repoStore)
	_, err := svc.GetCommit(context.Background(), "repo_xyz", "commit_missing")

	assert.ErrorIs(t, err, domain.ErrCommitNotFound)
}

func TestGetCommit_WrongRepo_ReturnsCommitNotFound(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		getByIDFn: func(_ context.Context, repoID, commitID string) (*domain.Commit, error) {
			// commit exists but in a different repo
			if repoID == "repo_xyz" {
				return nil, domain.ErrCommitNotFound
			}
			return &domain.Commit{ID: commitID, RepoID: "repo_other"}, nil
		},
	}

	svc := NewCommitService(commitStore, repoStore)
	_, err := svc.GetCommit(context.Background(), "repo_xyz", "commit_abc")

	assert.ErrorIs(t, err, domain.ErrCommitNotFound, "should not leak cross-repo data")
}

// ListCommits

func TestListCommits_RepoNotFound_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, _ string) (*domain.Repo, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	svc := NewCommitService(nil, repoStore)
	_, err := svc.ListCommits(context.Background(), ListCommitsInput{
		RepoID: "repo_missing",
	})

	assert.ErrorIs(t, err, domain.ErrRepoNotFound)
}

func TestListCommits_DefaultLimit_PassedToStore(t *testing.T) {
	var capturedFilter postgres.ListCommitsFilter

	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		listFn: func(_ context.Context, filter postgres.ListCommitsFilter) (*postgres.ListCommitsPage, error) {
			capturedFilter = filter
			return &postgres.ListCommitsPage{Commits: []*domain.Commit{}}, nil
		},
	}

	svc := NewCommitService(commitStore, repoStore)
	_, err := svc.ListCommits(context.Background(), ListCommitsInput{
		RepoID: "repo_xyz",
		Limit:  0, // should default to 20
	})

	require.NoError(t, err)
	assert.Equal(t, 20, capturedFilter.Limit)
}

func TestListCommits_LimitOverMax_CappedAt100(t *testing.T) {
	var capturedFilter postgres.ListCommitsFilter

	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		listFn: func(_ context.Context, filter postgres.ListCommitsFilter) (*postgres.ListCommitsPage, error) {
			capturedFilter = filter
			return &postgres.ListCommitsPage{Commits: []*domain.Commit{}}, nil
		},
	}

	svc := NewCommitService(commitStore, repoStore)
	_, err := svc.ListCommits(context.Background(), ListCommitsInput{
		RepoID: "repo_xyz",
		Limit:  200, // over max
	})

	require.NoError(t, err)
	assert.Equal(t, 100, capturedFilter.Limit)
}

func TestListCommits_InvalidTimestamp_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	svc := NewCommitService(nil, repoStore)
	_, err := svc.ListCommits(context.Background(), ListCommitsInput{
		RepoID: "repo_xyz",
		Since:  "invalid-timestamp",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "timestamp")
}

func TestListCommits_InvalidTraversal_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	svc := NewCommitService(nil, repoStore)
	_, err := svc.ListCommits(context.Background(), ListCommitsInput{
		RepoID:    "repo_xyz",
		Traversal: "invalid",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "traversal")
}

func TestListCommits_TraversalDAGWithoutBranch_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	svc := NewCommitService(nil, repoStore)
	_, err := svc.ListCommits(context.Background(), ListCommitsInput{
		RepoID:    "repo_xyz",
		Traversal: "dag",
		// branch is missing
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a 'branch'")
}

// GetParents

func TestGetParents_RepoNotFound_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, _ string) (*domain.Repo, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	svc := NewCommitService(nil, repoStore)
	_, err := svc.GetParents(context.Background(), "repo_missing", "commit_abc")

	assert.ErrorIs(t, err, domain.ErrRepoNotFound)
}

func TestGetParents_CommitNotFound_ReturnsError(t *testing.T) {
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

	svc := NewCommitService(commitStore, repoStore)
	_, err := svc.GetParents(context.Background(), "repo_xyz", "commit_missing")

	assert.ErrorIs(t, err, domain.ErrCommitNotFound)
}

func TestGetParents_RootCommit_ReturnsEmptyArray(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		getByIDFn: func(_ context.Context, repoID, commitID string) (*domain.Commit, error) {
			return &domain.Commit{
				ID:        commitID,
				RepoID:    repoID,
				ParentIDs: []string{}, // root commit
			}, nil
		},
		getParentsFn: func(_ context.Context, _, _ string) ([]*domain.Commit, error) {
			return []*domain.Commit{}, nil
		},
	}

	svc := NewCommitService(commitStore, repoStore)
	parents, err := svc.GetParents(context.Background(), "repo_xyz", "commit_root")

	require.NoError(t, err)
	assert.Len(t, parents, 0, "root commit should have no parents")
}

func TestGetParents_MergeCommit_ReturnsTwoParents(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		getByIDFn: func(_ context.Context, repoID, commitID string) (*domain.Commit, error) {
			return &domain.Commit{
				ID:        commitID,
				RepoID:    repoID,
				ParentIDs: []string{"commit_p1", "commit_p2"}, // merge commit
			}, nil
		},
		getParentsFn: func(_ context.Context, _, _ string) ([]*domain.Commit, error) {
			return []*domain.Commit{
				{ID: "commit_p1", RepoID: "repo_xyz", Message: "Parent 1"},
				{ID: "commit_p2", RepoID: "repo_xyz", Message: "Parent 2"},
			}, nil
		},
	}

	svc := NewCommitService(commitStore, repoStore)
	parents, err := svc.GetParents(context.Background(), "repo_xyz", "commit_merge")

	require.NoError(t, err)
	assert.Len(t, parents, 2, "merge commit should have exactly two parents")
	assert.Equal(t, "commit_p1", parents[0].ID)
	assert.Equal(t, "commit_p2", parents[1].ID)
}

func TestGetParents_ValidCommit_CallsStore(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		getByIDFn: func(_ context.Context, repoID, commitID string) (*domain.Commit, error) {
			return &domain.Commit{ID: commitID, RepoID: repoID}, nil
		},
		getParentsFn: func(_ context.Context, _, _ string) ([]*domain.Commit, error) {
			return []*domain.Commit{
				{ID: "commit_parent1"},
				{ID: "commit_parent2"},
			}, nil
		},
	}

	svc := NewCommitService(commitStore, repoStore)
	parents, err := svc.GetParents(context.Background(), "repo_xyz", "commit_abc")

	require.NoError(t, err)
	assert.Len(t, parents, 2)
}

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/storage/postgres"
)

// mock
type mockMergeStore struct {
	createMergeFn func(ctx context.Context, commit *domain.Commit, parentIDs []string, targetBranch, expectedTargetHead string) (*domain.Commit, error)
}

func (m *mockMergeStore) CreateMerge(
	ctx context.Context,
	commit *domain.Commit,
	parentIDs []string,
	targetBranch, expectedTargetHead string,
) (*domain.Commit, error) {
	if m.createMergeFn != nil {
		return m.createMergeFn(ctx, commit, parentIDs, targetBranch, expectedTargetHead)
	}
	return nil, nil
}

// Create

func TestCreateMerge_NotExactlyTwoParents_ReturnsError(t *testing.T) {
	cases := []struct {
		name      string
		parentIDs []string
	}{
		{"zero parents", []string{}},
		{"one parent", []string{"commit_1"}},
		{"three parents", []string{"commit_1", "commit_2", "commit_3"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewMergeService(nil, nil, nil, nil)
			_, err := svc.CreateMerge(context.Background(), CreateMergeInput{
				RepoID:             "repo_xyz",
				ParentIDs:          tc.parentIDs,
				TargetBranch:       "main",
				ExpectedTargetHead: "commit_old",
				DataPointer: domain.DataPointer{
					Type:     "db",
					Location: "test/fixture",
				},
				Message: "Merge",
				Author:  "test@example.com",
			})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "exactly two")
		})
	}
}

func TestCreateMerge_RepoNotFound_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, _ string) (*domain.Repo, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	svc := NewMergeService(nil, repoStore, nil, nil)
	_, err := svc.CreateMerge(context.Background(), CreateMergeInput{
		RepoID:             "repo_missing",
		ParentIDs:          []string{"commit_1", "commit_2"},
		TargetBranch:       "main",
		ExpectedTargetHead: "commit_old",
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Merge",
		Author:  "test@example.com",
	})

	assert.ErrorIs(t, err, domain.ErrRepoNotFound)
}

func TestCreateMerge_InvalidDataPointer_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	svc := NewMergeService(nil, repoStore, nil, nil)
	_, err := svc.CreateMerge(context.Background(), CreateMergeInput{
		RepoID:             "repo_xyz",
		ParentIDs:          []string{"commit_1", "commit_2"},
		TargetBranch:       "main",
		ExpectedTargetHead: "commit_old",
		DataPointer: domain.DataPointer{
			Type:     "invalid-type",
			Location: "test/fixture",
		},
		Message: "Merge",
		Author:  "test@example.com",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "data_pointer")
}

func TestCreateMerge_InvalidParent_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		validateParentsExistFn: func(_ context.Context, _ string, _ []string) error {
			return domain.ErrInvalidParent
		},
	}

	svc := NewMergeService(nil, repoStore, commitStore, nil)
	_, err := svc.CreateMerge(context.Background(), CreateMergeInput{
		RepoID:             "repo_xyz",
		ParentIDs:          []string{"commit_nonexistent", "commit_2"},
		TargetBranch:       "main",
		ExpectedTargetHead: "commit_old",
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Merge",
		Author:  "test@example.com",
	})

	assert.ErrorIs(t, err, domain.ErrInvalidParent)
}

func TestCreateMerge_TargetBranchNotFound_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		validateParentsExistFn: func(_ context.Context, _ string, _ []string) error {
			return nil
		},
	}

	branchStore := &mockBranchStore{
		getByNameFn: func(_ context.Context, _, _ string) (*domain.Branch, error) {
			return nil, domain.ErrBranchNotFound
		},
	}

	svc := NewMergeService(nil, repoStore, commitStore, branchStore)
	_, err := svc.CreateMerge(context.Background(), CreateMergeInput{
		RepoID:             "repo_xyz",
		ParentIDs:          []string{"commit_1", "commit_2"},
		TargetBranch:       "missing",
		ExpectedTargetHead: "commit_old",
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Merge",
		Author:  "test@example.com",
	})

	assert.ErrorIs(t, err, domain.ErrBranchNotFound)
}

func TestCreateMerge_StaleExpectedTargetHead_ReturnsError(t *testing.T) {
	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		validateParentsExistFn: func(_ context.Context, _ string, _ []string) error {
			return nil
		},
	}

	branchStore := &mockBranchStore{
		getByNameFn: func(_ context.Context, _, _ string) (*domain.Branch, error) {
			return &domain.Branch{CommitID: "commit_actual"}, nil
		},
	}

	mergeStore := &mockMergeStore{
		createMergeFn: func(_ context.Context, _ *domain.Commit, _ []string, _, _ string) (*domain.Commit, error) {
			return nil, &postgres.MergeBranchConflictError{CurrentHead: "commit_actual"}
		},
	}

	svc := NewMergeService(mergeStore, repoStore, commitStore, branchStore)
	_, err := svc.CreateMerge(context.Background(), CreateMergeInput{
		RepoID:             "repo_xyz",
		ParentIDs:          []string{"commit_1", "commit_2"},
		TargetBranch:       "main",
		ExpectedTargetHead: "commit_stale",
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Merge",
		Author:  "test@example.com",
	})

	assert.ErrorIs(t, err, domain.ErrStaleMergeTarget)

	var conflictErr *postgres.MergeBranchConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Equal(t, "commit_actual", conflictErr.CurrentHead)
}

func TestCreateMerge_ValidInput_CallsStoreCreateMerge(t *testing.T) {
	var capturedCommit *domain.Commit
	var capturedParentIDs []string
	var capturedTargetBranch, capturedExpectedHead string

	repoStore := &mockRepoStore{
		getByIDFn: func(_ context.Context, id string) (*domain.Repo, error) {
			return &domain.Repo{ID: id}, nil
		},
	}

	commitStore := &mockCommitStore{
		validateParentsExistFn: func(_ context.Context, _ string, _ []string) error {
			return nil
		},
	}

	branchStore := &mockBranchStore{
		getByNameFn: func(_ context.Context, _, _ string) (*domain.Branch, error) {
			return &domain.Branch{CommitID: "commit_old"}, nil
		},
	}

	mergeStore := &mockMergeStore{
		createMergeFn: func(
			_ context.Context,
			commit *domain.Commit,
			parentIDs []string,
			targetBranch, expectedHead string,
		) (*domain.Commit, error) {
			capturedCommit = commit
			capturedParentIDs = parentIDs
			capturedTargetBranch = targetBranch
			capturedExpectedHead = expectedHead
			return commit, nil
		},
	}

	svc := NewMergeService(mergeStore, repoStore, commitStore, branchStore)
	result, err := svc.CreateMerge(context.Background(), CreateMergeInput{
		RepoID:             "repo_xyz",
		ParentIDs:          []string{"commit_1", "commit_2"},
		TargetBranch:       "main",
		ExpectedTargetHead: "commit_old",
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message: "Merge feature into main",
		Author:  "alice@example.com",
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "repo_xyz", capturedCommit.RepoID)
	assert.Equal(t, "Merge feature into main", capturedCommit.Message)
	assert.Equal(t, "alice@example.com", capturedCommit.Author)
	assert.Equal(t, []string{"commit_1", "commit_2"}, capturedParentIDs)
	assert.Equal(t, "main", capturedTargetBranch)
	assert.Equal(t, "commit_old", capturedExpectedHead)
}

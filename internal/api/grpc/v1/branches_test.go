package v1

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	vergev1 "github.com/bhpcv252/verge/api/proto/verge/v1"
	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/service"
	"github.com/bhpcv252/verge/testhelper"
)

// mock

type mockBranchService struct {
	createFn  func(ctx context.Context, in service.CreateBranchInput) (*domain.Branch, error)
	getBranch func(ctx context.Context, repoID, name string) (*domain.Branch, error)
	listFn    func(ctx context.Context, in service.ListBranchesInput) (*service.ListBranchesResult, error)
	advanceFn func(ctx context.Context, in service.AdvanceBranchInput) (*domain.Branch, error)
	deleteFn  func(ctx context.Context, repoID, name string) error
}

func (m *mockBranchService) CreateBranch(
	ctx context.Context,
	in service.CreateBranchInput,
) (*domain.Branch, error) {
	return m.createFn(ctx, in)
}

func (m *mockBranchService) GetBranch(
	ctx context.Context,
	repoID, name string,
) (*domain.Branch, error) {
	return m.getBranch(ctx, repoID, name)
}

func (m *mockBranchService) ListBranches(
	ctx context.Context,
	in service.ListBranchesInput,
) (*service.ListBranchesResult, error) {
	return m.listFn(ctx, in)
}

func (m *mockBranchService) AdvanceBranch(
	ctx context.Context,
	in service.AdvanceBranchInput,
) (*domain.Branch, error) {
	return m.advanceFn(ctx, in)
}

func (m *mockBranchService) DeleteBranch(ctx context.Context, repoID, name string) error {
	return m.deleteFn(ctx, repoID, name)
}

// CreateBranch

func TestCreateBranch_ValidRequest_CallsServiceWithCorrectInputAndReturnsBranch(t *testing.T) {
	var captured service.CreateBranchInput

	srv := NewBranchServer(&mockBranchService{
		createFn: func(_ context.Context, in service.CreateBranchInput) (*domain.Branch, error) {
			captured = in
			return testhelper.FixedBranch(), nil
		},
	})

	resp, err := srv.CreateBranch(context.Background(), &vergev1.CreateBranchRequest{
		RepoId:         "repo_abc123",
		Name:           "main",
		SourceCommitId: "commit_xyz789",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "repo_abc123", resp.RepoId)
	assert.Equal(t, "main", resp.Name)
	assert.Equal(t, "commit_xyz789", resp.CommitId)
	assert.NotEmpty(t, resp.CreatedAt)
	assert.Equal(t, "repo_abc123", captured.RepoID)
	assert.Equal(t, "main", captured.Name)
	assert.Equal(t, "commit_xyz789", captured.SourceCommitID)
}

func TestCreateBranch_MissingFields_ReturnsInvalidArgument(t *testing.T) {
	cases := []struct {
		name    string
		request *vergev1.CreateBranchRequest
	}{
		{
			name: "missing_repo_id",
			request: &vergev1.CreateBranchRequest{
				Name:           "feature-x",
				SourceCommitId: "commit_xyz789",
			},
		},
		{
			name: "missing_name",
			request: &vergev1.CreateBranchRequest{
				RepoId:         "repo_abc123",
				SourceCommitId: "commit_xyz789",
			},
		},
		{
			name:    "missing_source_commit_id",
			request: &vergev1.CreateBranchRequest{RepoId: "repo_abc123", Name: "feature-x"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			srv := NewBranchServer(&mockBranchService{
				createFn: func(_ context.Context, _ service.CreateBranchInput) (*domain.Branch, error) {
					called = true
					return nil, nil
				},
			})

			_, err := srv.CreateBranch(context.Background(), tc.request)

			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
			assert.False(t, called)
		})
	}
}

func TestCreateBranch_ServiceReturnsRepoNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		createFn: func(_ context.Context, _ service.CreateBranchInput) (*domain.Branch, error) {
			return nil, domain.ErrRepoNotFound
		},
	})

	_, err := srv.CreateBranch(context.Background(), &vergev1.CreateBranchRequest{
		RepoId:         "repo_missing",
		Name:           "feature-x",
		SourceCommitId: "commit_xyz789",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestCreateBranch_ServiceReturnsBranchAlreadyExists_ReturnsAlreadyExists(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		createFn: func(_ context.Context, _ service.CreateBranchInput) (*domain.Branch, error) {
			return nil, domain.ErrBranchAlreadyExists
		},
	})

	_, err := srv.CreateBranch(context.Background(), &vergev1.CreateBranchRequest{
		RepoId:         "repo_abc123",
		Name:           "existing-branch",
		SourceCommitId: "commit_xyz789",
	})

	require.Error(t, err)
	assert.Equal(t, codes.AlreadyExists, testhelper.GRPCCode(err))
}

func TestCreateBranch_ServiceReturnsCommitNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		createFn: func(_ context.Context, _ service.CreateBranchInput) (*domain.Branch, error) {
			return nil, domain.ErrCommitNotFound
		},
	})

	_, err := srv.CreateBranch(context.Background(), &vergev1.CreateBranchRequest{
		RepoId:         "repo_abc123",
		Name:           "feature-x",
		SourceCommitId: "commit_missing",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestCreateBranch_ServiceReturnsUnexpectedError_ReturnsInternal(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		createFn: func(_ context.Context, _ service.CreateBranchInput) (*domain.Branch, error) {
			return nil, errors.New("db connection failed")
		},
	})

	_, err := srv.CreateBranch(context.Background(), &vergev1.CreateBranchRequest{
		RepoId:         "repo_abc123",
		Name:           "feature-x",
		SourceCommitId: "commit_xyz789",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, testhelper.GRPCCode(err))
}

func TestCreateBranch_CreatedAtIsISO8601(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		createFn: func(_ context.Context, _ service.CreateBranchInput) (*domain.Branch, error) {
			return testhelper.FixedBranch(), nil
		},
	})

	resp, err := srv.CreateBranch(context.Background(), &vergev1.CreateBranchRequest{
		RepoId:         "repo_abc123",
		Name:           "feature-x",
		SourceCommitId: "commit_xyz789",
	})

	require.NoError(t, err)
	assert.Equal(t, "2024-04-05T10:00:00Z", resp.CreatedAt)
}

// GetBranch

func TestGetBranch_ValidRequest_ReturnsCorrectBranch(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		getBranch: func(_ context.Context, repoID, name string) (*domain.Branch, error) {
			assert.Equal(t, "repo_abc123", repoID)
			assert.Equal(t, "main", name)
			return testhelper.FixedBranch(), nil
		},
	})

	resp, err := srv.GetBranch(context.Background(), &vergev1.GetBranchRequest{
		RepoId: "repo_abc123",
		Name:   "main",
	})

	require.NoError(t, err)
	assert.Equal(t, "main", resp.Name)
	assert.Equal(t, "repo_abc123", resp.RepoId)
	assert.Equal(t, "commit_xyz789", resp.CommitId)
	assert.NotEmpty(t, resp.CreatedAt)
}

func TestGetBranch_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewBranchServer(&mockBranchService{
		getBranch: func(_ context.Context, _, _ string) (*domain.Branch, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.GetBranch(context.Background(), &vergev1.GetBranchRequest{
		Name: "feature-x",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestGetBranch_MissingName_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewBranchServer(&mockBranchService{
		getBranch: func(_ context.Context, _, _ string) (*domain.Branch, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.GetBranch(context.Background(), &vergev1.GetBranchRequest{
		RepoId: "repo_abc123",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestGetBranch_WhitespaceOnlyFields_ReturnsInvalidArgument(t *testing.T) {
	cases := []struct {
		name    string
		request *vergev1.GetBranchRequest
	}{
		{
			name: "whitespace_repo_id",
			request: &vergev1.GetBranchRequest{
				RepoId: "   ",
				Name:   "feature-x",
			},
		},
		{
			name: "whitespace_name",
			request: &vergev1.GetBranchRequest{
				RepoId: "repo_abc123",
				Name:   "  ",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			srv := NewBranchServer(&mockBranchService{
				getBranch: func(_ context.Context, _, _ string) (*domain.Branch, error) {
					called = true
					return nil, nil
				},
			})

			_, err := srv.GetBranch(context.Background(), tc.request)

			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
			assert.False(t, called)
		})
	}
}

func TestGetBranch_ServiceReturnsBranchNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		getBranch: func(_ context.Context, _, _ string) (*domain.Branch, error) {
			return nil, domain.ErrBranchNotFound
		},
	})

	_, err := srv.GetBranch(context.Background(), &vergev1.GetBranchRequest{
		RepoId: "repo_abc123",
		Name:   "nonexistent",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestGetBranch_ServiceReturnsRepoNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		getBranch: func(_ context.Context, _, _ string) (*domain.Branch, error) {
			return nil, domain.ErrRepoNotFound
		},
	})

	_, err := srv.GetBranch(context.Background(), &vergev1.GetBranchRequest{
		RepoId: "repo_missing",
		Name:   "feature-x",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestGetBranch_ServiceReturnsUnexpectedError_ReturnsInternal(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		getBranch: func(_ context.Context, _, _ string) (*domain.Branch, error) {
			return nil, errors.New("database connection lost")
		},
	})

	_, err := srv.GetBranch(context.Background(), &vergev1.GetBranchRequest{
		RepoId: "repo_abc123",
		Name:   "feature-x",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, testhelper.GRPCCode(err))
}

func TestGetBranch_CreatedAtIsISO8601(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		getBranch: func(_ context.Context, _, _ string) (*domain.Branch, error) {
			return testhelper.FixedBranch(), nil
		},
	})

	resp, err := srv.GetBranch(context.Background(), &vergev1.GetBranchRequest{
		RepoId: "repo_abc123",
		Name:   "feature-x",
	})

	require.NoError(t, err)
	assert.Equal(t, "2024-04-05T10:00:00Z", resp.CreatedAt)
}

// ListBranches

func TestListBranches_ValidRequest_CallsServiceWithCorrectInput(t *testing.T) {
	var captured service.ListBranchesInput

	srv := NewBranchServer(&mockBranchService{
		listFn: func(_ context.Context, in service.ListBranchesInput) (*service.ListBranchesResult, error) {
			captured = in
			return &service.ListBranchesResult{
				Branches: []*domain.Branch{testhelper.FixedBranch()},
			}, nil
		},
	})

	resp, err := srv.ListBranches(context.Background(), &vergev1.ListBranchesRequest{
		RepoId: "repo_abc123",
		Limit:  10,
		Cursor: "cursor123",
	})

	require.NoError(t, err)
	require.Len(t, resp.Branches, 1)
	assert.Equal(t, "repo_abc123", captured.RepoID)
	assert.Equal(t, 10, captured.Limit)
	assert.Equal(t, "cursor123", captured.Cursor)
}

func TestListBranches_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewBranchServer(&mockBranchService{
		listFn: func(_ context.Context, _ service.ListBranchesInput) (*service.ListBranchesResult, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.ListBranches(context.Background(), &vergev1.ListBranchesRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestListBranches_ServiceReturnsUnexpectedError_ReturnsInternal(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		listFn: func(_ context.Context, _ service.ListBranchesInput) (*service.ListBranchesResult, error) {
			return nil, errors.New("unexpected")
		},
	})

	_, err := srv.ListBranches(context.Background(), &vergev1.ListBranchesRequest{
		RepoId: "repo_abc123",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, testhelper.GRPCCode(err))
}

func TestListBranches_WithNextCursor_IncludedInResponse(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		listFn: func(_ context.Context, _ service.ListBranchesInput) (*service.ListBranchesResult, error) {
			return &service.ListBranchesResult{
				Branches:   []*domain.Branch{testhelper.FixedBranch()},
				NextCursor: "next-page-abc",
			}, nil
		},
	})

	resp, err := srv.ListBranches(context.Background(), &vergev1.ListBranchesRequest{
		RepoId: "repo_abc123",
	})

	require.NoError(t, err)
	assert.Equal(t, "next-page-abc", resp.NextCursor)
}

func TestListBranches_BranchesAreMappedCorrectly(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		listFn: func(_ context.Context, _ service.ListBranchesInput) (*service.ListBranchesResult, error) {
			return &service.ListBranchesResult{
				Branches: []*domain.Branch{testhelper.FixedBranch()},
			}, nil
		},
	})

	resp, err := srv.ListBranches(context.Background(), &vergev1.ListBranchesRequest{
		RepoId: "repo_abc123",
	})

	require.NoError(t, err)
	require.Len(t, resp.Branches, 1)
	assert.Equal(t, "repo_abc123", resp.Branches[0].RepoId)
	assert.Equal(t, "main", resp.Branches[0].Name)
	assert.Equal(t, "commit_xyz789", resp.Branches[0].CommitId)
	assert.NotEmpty(t, resp.Branches[0].CreatedAt)
}

// AdvanceBranch

func TestAdvanceBranch_ValidRequest_CallsServiceWithCorrectInput(t *testing.T) {
	var captured service.AdvanceBranchInput

	srv := NewBranchServer(&mockBranchService{
		advanceFn: func(_ context.Context, in service.AdvanceBranchInput) (*domain.Branch, error) {
			captured = in
			return testhelper.FixedBranch(), nil
		},
	})

	resp, err := srv.AdvanceBranch(context.Background(), &vergev1.AdvanceBranchRequest{
		RepoId:           "repo_abc123",
		Name:             "feature-x",
		CommitId:         "commit_new789",
		ExpectedCommitId: "commit_xyz789",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "repo_abc123", captured.RepoID)
	assert.Equal(t, "feature-x", captured.Name)
	assert.Equal(t, "commit_new789", captured.CommitID)
	assert.Equal(t, "commit_xyz789", captured.ExpectedCommitID)
}

func TestAdvanceBranch_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewBranchServer(&mockBranchService{
		advanceFn: func(_ context.Context, _ service.AdvanceBranchInput) (*domain.Branch, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.AdvanceBranch(context.Background(), &vergev1.AdvanceBranchRequest{
		Name:             "feature-x",
		CommitId:         "commit_new789",
		ExpectedCommitId: "commit_xyz789",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestAdvanceBranch_MissingName_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewBranchServer(&mockBranchService{
		advanceFn: func(_ context.Context, _ service.AdvanceBranchInput) (*domain.Branch, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.AdvanceBranch(context.Background(), &vergev1.AdvanceBranchRequest{
		RepoId:           "repo_abc123",
		CommitId:         "commit_new789",
		ExpectedCommitId: "commit_xyz789",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestAdvanceBranch_MissingCommitID_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewBranchServer(&mockBranchService{
		advanceFn: func(_ context.Context, _ service.AdvanceBranchInput) (*domain.Branch, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.AdvanceBranch(context.Background(), &vergev1.AdvanceBranchRequest{
		RepoId:           "repo_abc123",
		Name:             "feature-x",
		ExpectedCommitId: "commit_xyz789",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestAdvanceBranch_MissingExpectedCommitID_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewBranchServer(&mockBranchService{
		advanceFn: func(_ context.Context, _ service.AdvanceBranchInput) (*domain.Branch, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.AdvanceBranch(context.Background(), &vergev1.AdvanceBranchRequest{
		RepoId:   "repo_abc123",
		Name:     "feature-x",
		CommitId: "commit_new789",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestAdvanceBranch_ServiceReturnsRepoNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		advanceFn: func(_ context.Context, _ service.AdvanceBranchInput) (*domain.Branch, error) {
			return nil, domain.ErrRepoNotFound
		},
	})

	_, err := srv.AdvanceBranch(context.Background(), &vergev1.AdvanceBranchRequest{
		RepoId:           "repo_missing",
		Name:             "feature-x",
		CommitId:         "commit_new789",
		ExpectedCommitId: "commit_xyz789",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestAdvanceBranch_ServiceReturnsBranchNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		advanceFn: func(_ context.Context, _ service.AdvanceBranchInput) (*domain.Branch, error) {
			return nil, domain.ErrBranchNotFound
		},
	})

	_, err := srv.AdvanceBranch(context.Background(), &vergev1.AdvanceBranchRequest{
		RepoId:           "repo_abc123",
		Name:             "missing-branch",
		CommitId:         "commit_new789",
		ExpectedCommitId: "commit_xyz789",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestAdvanceBranch_ServiceReturnsCommitNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		advanceFn: func(_ context.Context, _ service.AdvanceBranchInput) (*domain.Branch, error) {
			return nil, domain.ErrCommitNotFound
		},
	})

	_, err := srv.AdvanceBranch(context.Background(), &vergev1.AdvanceBranchRequest{
		RepoId:           "repo_abc123",
		Name:             "feature-x",
		CommitId:         "commit_missing",
		ExpectedCommitId: "commit_xyz789",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestAdvanceBranch_ServiceReturnsBranchConflict_ReturnsAborted(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		advanceFn: func(_ context.Context, _ service.AdvanceBranchInput) (*domain.Branch, error) {
			return nil, &service.BranchConflictError{
				BranchName:   "feature-x",
				CurrentHead:  "commit_changed",
				ExpectedHead: "commit_xyz789",
			}
		},
	})

	_, err := srv.AdvanceBranch(context.Background(), &vergev1.AdvanceBranchRequest{
		RepoId:           "repo_abc123",
		Name:             "feature-x",
		CommitId:         "commit_new789",
		ExpectedCommitId: "commit_xyz789",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Aborted, testhelper.GRPCCode(err))
}

func TestAdvanceBranch_ServiceReturnsUnexpectedError_ReturnsInternal(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		advanceFn: func(_ context.Context, _ service.AdvanceBranchInput) (*domain.Branch, error) {
			return nil, errors.New("unexpected error")
		},
	})

	_, err := srv.AdvanceBranch(context.Background(), &vergev1.AdvanceBranchRequest{
		RepoId:           "repo_abc123",
		Name:             "feature-x",
		CommitId:         "commit_new789",
		ExpectedCommitId: "commit_xyz789",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, testhelper.GRPCCode(err))
}

// DeleteBranch

func TestDeleteBranch_ValidRequest_CallsServiceWithCorrectParams(t *testing.T) {
	var capturedRepoID, capturedName string

	srv := NewBranchServer(&mockBranchService{
		deleteFn: func(_ context.Context, repoID, name string) error {
			capturedRepoID = repoID
			capturedName = name
			return nil
		},
	})

	resp, err := srv.DeleteBranch(context.Background(), &vergev1.DeleteBranchRequest{
		RepoId: "repo_abc123",
		Name:   "feature-x",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "repo_abc123", capturedRepoID)
	assert.Equal(t, "feature-x", capturedName)
}

func TestDeleteBranch_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewBranchServer(&mockBranchService{
		deleteFn: func(_ context.Context, _, _ string) error {
			called = true
			return nil
		},
	})

	_, err := srv.DeleteBranch(context.Background(), &vergev1.DeleteBranchRequest{
		Name: "feature-x",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestDeleteBranch_MissingName_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewBranchServer(&mockBranchService{
		deleteFn: func(_ context.Context, _, _ string) error {
			called = true
			return nil
		},
	})

	_, err := srv.DeleteBranch(context.Background(), &vergev1.DeleteBranchRequest{
		RepoId: "repo_abc123",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestDeleteBranch_ServiceReturnsRepoNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		deleteFn: func(_ context.Context, _, _ string) error {
			return domain.ErrRepoNotFound
		},
	})

	_, err := srv.DeleteBranch(context.Background(), &vergev1.DeleteBranchRequest{
		RepoId: "repo_missing",
		Name:   "feature-x",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestDeleteBranch_ServiceReturnsBranchNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		deleteFn: func(_ context.Context, _, _ string) error {
			return domain.ErrBranchNotFound
		},
	})

	_, err := srv.DeleteBranch(context.Background(), &vergev1.DeleteBranchRequest{
		RepoId: "repo_abc123",
		Name:   "missing-branch",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestDeleteBranch_ServiceReturnsCannotDeleteDefaultBranch_ReturnsFailedPrecondition(
	t *testing.T,
) {
	srv := NewBranchServer(&mockBranchService{
		deleteFn: func(_ context.Context, _, _ string) error {
			return domain.ErrCannotDeleteDefaultBranch
		},
	})

	_, err := srv.DeleteBranch(context.Background(), &vergev1.DeleteBranchRequest{
		RepoId: "repo_abc123",
		Name:   "main",
	})

	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, testhelper.GRPCCode(err))
}

func TestDeleteBranch_ServiceReturnsUnexpectedError_ReturnsInternal(t *testing.T) {
	srv := NewBranchServer(&mockBranchService{
		deleteFn: func(_ context.Context, _, _ string) error {
			return errors.New("unexpected error")
		},
	})

	_, err := srv.DeleteBranch(context.Background(), &vergev1.DeleteBranchRequest{
		RepoId: "repo_abc123",
		Name:   "feature-x",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, testhelper.GRPCCode(err))
}

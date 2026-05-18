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

type mockMergeService struct {
	createFn func(ctx context.Context, in service.CreateMergeInput) (*domain.Commit, error)
}

func (m *mockMergeService) CreateMerge(
	ctx context.Context,
	in service.CreateMergeInput,
) (*domain.Commit, error) {
	return m.createFn(ctx, in)
}

// CreateMerge

func TestCreateMerge_ValidRequest_CallsServiceWithCorrectInput(t *testing.T) {
	var captured service.CreateMergeInput

	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, in service.CreateMergeInput) (*domain.Commit, error) {
			captured = in
			return testhelper.FixedMergeCommit(), nil
		},
	})

	resp, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_xyz789",
		ParentIds:          []string{"parent1", "parent2"},
		ExpectedTargetHead: "parent1",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/merge-path",
			Hash:     "sha256:merge123",
			Metadata: []byte(`{"key":"value"}`),
		},
		Message: "Merge branch 'feature' into main",
		Author:  "alice@example.com",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "commit_merge_abc", resp.Id)
	assert.Equal(t, "repo_xyz789", captured.RepoID)
	assert.Equal(t, []string{"parent1", "parent2"}, captured.ParentIDs)
	assert.Equal(t, "parent1", captured.ExpectedTargetHead)
	assert.Equal(t, "main", captured.TargetBranch)
	assert.Equal(t, "s3", captured.DataPointer.Type)
	assert.Equal(t, "s3://bucket/merge-path", captured.DataPointer.Location)
	assert.Equal(t, "Merge branch 'feature' into main", captured.Message)
	assert.Equal(t, "alice@example.com", captured.Author)
}

func TestCreateMerge_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		Message:            "Merge commit",
		Author:             "alice@example.com",
		ParentIds:          []string{"parent1", "parent2"},
		ExpectedTargetHead: "parent1",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestCreateMerge_MissingMessage_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_xyz789",
		Author:             "alice@example.com",
		ParentIds:          []string{"parent1", "parent2"},
		ExpectedTargetHead: "parent1",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestCreateMerge_MissingAuthor_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_xyz789",
		Message:            "Merge commit",
		ParentIds:          []string{"parent1", "parent2"},
		ExpectedTargetHead: "parent1",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestCreateMerge_MissingExpectedTargetHead_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:       "repo_xyz789",
		Message:      "Merge commit",
		Author:       "alice@example.com",
		ParentIds:    []string{"parent1", "parent2"},
		TargetBranch: "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestCreateMerge_MissingDataPointer_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_xyz789",
		Message:            "Merge commit",
		Author:             "alice@example.com",
		ParentIds:          []string{"parent1", "parent2"},
		ExpectedTargetHead: "parent1",
		TargetBranch:       "main",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestCreateMerge_DataPointerMissingType_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_xyz789",
		Message:            "Merge commit",
		Author:             "alice@example.com",
		ParentIds:          []string{"parent1", "parent2"},
		ExpectedTargetHead: "parent1",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestCreateMerge_DataPointerMissingLocation_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_xyz789",
		Message:            "Merge commit",
		Author:             "alice@example.com",
		ParentIds:          []string{"parent1", "parent2"},
		ExpectedTargetHead: "parent1",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Type: "s3",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestCreateMerge_WhitespaceOnlyFields_ReturnsInvalidArgument(t *testing.T) {
	cases := []struct {
		name    string
		request *vergev1.CreateMergeRequest
	}{
		{
			name: "whitespace_repo_id",
			request: &vergev1.CreateMergeRequest{
				RepoId:             "   ",
				Message:            "Merge commit",
				Author:             "alice@example.com",
				ParentIds:          []string{"parent1", "parent2"},
				ExpectedTargetHead: "parent1",
				TargetBranch:       "main",
				DataPointer: &vergev1.DataPointer{
					Type:     "s3",
					Location: "s3://bucket/path",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			srv := NewMergeServer(&mockMergeService{
				createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
					called = true
					return nil, nil
				},
			})

			_, err := srv.CreateMerge(context.Background(), tc.request)

			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
			assert.False(t, called)
		})
	}
}

func TestCreateMerge_ServiceReturnsRepoNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			return nil, domain.ErrRepoNotFound
		},
	})

	_, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_missing",
		Message:            "Merge commit",
		Author:             "alice@example.com",
		ParentIds:          []string{"parent1", "parent2"},
		ExpectedTargetHead: "parent1",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestCreateMerge_ServiceReturnsBranchNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			return nil, domain.ErrBranchNotFound
		},
	})

	_, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_xyz789",
		Message:            "Merge commit",
		Author:             "alice@example.com",
		ParentIds:          []string{"parent1", "parent2"},
		ExpectedTargetHead: "parent1",
		TargetBranch:       "missing-branch",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestCreateMerge_ServiceReturnsMergeBranchConflict_ReturnsAborted(t *testing.T) {
	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			return nil, &service.MergeBranchConflictError{
				BranchName:   "main",
				CurrentHead:  "commit_actual",
				ExpectedHead: "commit_old",
			}
		},
	})

	_, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_xyz789",
		Message:            "Merge commit",
		Author:             "alice@example.com",
		ParentIds:          []string{"parent1", "parent2"},
		ExpectedTargetHead: "commit_old",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.Aborted, testhelper.GRPCCode(err))
}

func TestCreateMerge_ServiceReturnsDataPointerValidationError_ReturnsInvalidArgument(t *testing.T) {
	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			return nil, &service.ValidationError{Msg: "data_pointer validation failed"}
		},
	})

	_, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_xyz789",
		Message:            "Merge commit",
		Author:             "alice@example.com",
		ParentIds:          []string{"parent1", "parent2"},
		ExpectedTargetHead: "parent1",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
}

func TestCreateMerge_ServiceReturnsParentIDsValidationError_ReturnsInvalidArgument(t *testing.T) {
	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			return nil, &service.ValidationError{Msg: "parent_ids must have exactly two elements"}
		},
	})

	_, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_xyz789",
		Message:            "Merge commit",
		Author:             "alice@example.com",
		ParentIds:          []string{"parent1", "parent2"},
		ExpectedTargetHead: "parent1",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
}

func TestCreateMerge_ServiceReturnsUnexpectedError_ReturnsInternal(t *testing.T) {
	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			return nil, errors.New("unexpected database error")
		},
	})

	_, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_xyz789",
		Message:            "Merge commit",
		Author:             "alice@example.com",
		ParentIds:          []string{"parent1", "parent2"},
		ExpectedTargetHead: "parent1",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, testhelper.GRPCCode(err))
}

func TestCreateMerge_TimestampIsISO8601(t *testing.T) {
	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			return testhelper.FixedMergeCommit(), nil
		},
	})

	resp, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_xyz789",
		Message:            "Merge commit",
		Author:             "alice@example.com",
		ParentIds:          []string{"parent1", "parent2"},
		ExpectedTargetHead: "parent1",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/merge-path",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "2024-04-05T10:00:00Z", resp.Timestamp)
}

func TestCreateMerge_DataPointerMetadataMappedCorrectly(t *testing.T) {
	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, in service.CreateMergeInput) (*domain.Commit, error) {
			assert.NotNil(t, in.DataPointer.Metadata)
			return testhelper.FixedMergeCommit(), nil
		},
	})

	metadata := []byte(`{"key":"value"}`)
	resp, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_xyz789",
		Message:            "Merge commit",
		Author:             "alice@example.com",
		ParentIds:          []string{"parent1", "parent2"},
		ExpectedTargetHead: "parent1",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/merge-path",
			Metadata: metadata,
		},
	})

	require.NoError(t, err)
	assert.Equal(t, metadata, resp.DataPointer.Metadata)
}

func TestCreateMerge_MergeCommitHasTwoParents(t *testing.T) {
	srv := NewMergeServer(&mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			return testhelper.FixedMergeCommit(), nil
		},
	})

	resp, err := srv.CreateMerge(context.Background(), &vergev1.CreateMergeRequest{
		RepoId:             "repo_xyz789",
		Message:            "Merge commit",
		Author:             "alice@example.com",
		ParentIds:          []string{"parent1", "parent2"},
		ExpectedTargetHead: "parent1",
		TargetBranch:       "main",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/merge-path",
		},
	})

	require.NoError(t, err)
	require.Len(t, resp.ParentIds, 2)
	assert.Equal(t, "commit_source", resp.ParentIds[0])
	assert.Equal(t, "commit_target", resp.ParentIds[1])
}

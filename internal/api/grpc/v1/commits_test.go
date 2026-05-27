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

type mockCommitService struct {
	createFn     func(ctx context.Context, in service.CreateCommitInput) (*service.CreateCommitResult, error)
	getFn        func(ctx context.Context, repoID, commitID string) (*domain.Commit, error)
	listFn       func(ctx context.Context, in service.ListCommitsInput) (*service.ListCommitsResult, error)
	getParentsFn func(ctx context.Context, repoID, commitID string) ([]*domain.Commit, error)
}

func (m *mockCommitService) CreateCommit(
	ctx context.Context,
	in service.CreateCommitInput,
) (*service.CreateCommitResult, error) {
	return m.createFn(ctx, in)
}

func (m *mockCommitService) GetCommit(
	ctx context.Context,
	repoID, commitID string,
) (*domain.Commit, error) {
	return m.getFn(ctx, repoID, commitID)
}

func (m *mockCommitService) ListCommits(
	ctx context.Context,
	in service.ListCommitsInput,
) (*service.ListCommitsResult, error) {
	return m.listFn(ctx, in)
}

func (m *mockCommitService) GetParents(
	ctx context.Context,
	repoID, commitID string,
) ([]*domain.Commit, error) {
	return m.getParentsFn(ctx, repoID, commitID)
}

// CreateCommit

func TestCreateCommit_ValidRequest_CallsServiceWithCorrectInput(t *testing.T) {
	var captured service.CreateCommitInput

	srv := NewCommitServer(&mockCommitService{
		createFn: func(_ context.Context, in service.CreateCommitInput) (*service.CreateCommitResult, error) {
			captured = in
			return &service.CreateCommitResult{
				Commit:   testhelper.FixedCommit(),
				Existing: false,
			}, nil
		},
	})

	resp, err := srv.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		RepoId:    "repo_xyz789",
		ParentIds: []string{"parent1"},
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
			Hash:     "sha256:abc123",
			Metadata: []byte(`{"key":"value"}`),
		},
		Message:        "test commit",
		Author:         "alice@example.com",
		IdempotencyKey: "key123",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "commit_abc123", resp.Commit.Id)
	assert.False(t, resp.Existing)
	assert.Equal(t, "repo_xyz789", captured.RepoID)
	assert.Equal(t, []string{"parent1"}, captured.ParentIDs)
	assert.Equal(t, "s3", captured.DataPointer.Type)
	assert.Equal(t, "s3://bucket/path", captured.DataPointer.Location)
	assert.Equal(t, "test commit", captured.Message)
	assert.Equal(t, "alice@example.com", captured.Author)
	assert.Equal(t, "key123", captured.IdempotencyKey)
}

func TestCreateCommit_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewCommitServer(&mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		Message: "test commit",
		Author:  "alice@example.com",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestCreateCommit_MissingMessage_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewCommitServer(&mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		RepoId: "repo_xyz789",
		Author: "alice@example.com",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestCreateCommit_MissingAuthor_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewCommitServer(&mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		RepoId:  "repo_xyz789",
		Message: "test commit",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestCreateCommit_MissingDataPointer_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewCommitServer(&mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		RepoId:  "repo_xyz789",
		Message: "test commit",
		Author:  "alice@example.com",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestCreateCommit_DataPointerMissingType_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewCommitServer(&mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		RepoId:  "repo_xyz789",
		Message: "test commit",
		Author:  "alice@example.com",
		DataPointer: &vergev1.DataPointer{
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestCreateCommit_DataPointerMissingLocation_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewCommitServer(&mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		RepoId:  "repo_xyz789",
		Message: "test commit",
		Author:  "alice@example.com",
		DataPointer: &vergev1.DataPointer{
			Type: "s3",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestCreateCommit_ServiceReturnsRepoNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			return nil, domain.ErrRepoNotFound
		},
	})

	_, err := srv.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		RepoId:  "repo_missing",
		Message: "test commit",
		Author:  "alice@example.com",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestCreateCommit_ServiceReturnsUnexpectedError_ReturnsInternal(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			return nil, errors.New("db error")
		},
	})

	_, err := srv.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		RepoId:  "repo_xyz789",
		Message: "test commit",
		Author:  "alice@example.com",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, testhelper.GRPCCode(err))
}

func TestCreateCommit_ExistingCommit_ReturnsExistingTrue(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			return &service.CreateCommitResult{
				Commit:   testhelper.FixedCommit(),
				Existing: true,
			}, nil
		},
	})

	resp, err := srv.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		RepoId:         "repo_xyz789",
		Message:        "test commit",
		Author:         "alice@example.com",
		IdempotencyKey: "existing-key",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.NoError(t, err)
	assert.True(t, resp.Existing)
}

func TestCreateCommit_TimestampIsISO8601(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			return &service.CreateCommitResult{
				Commit:   testhelper.FixedCommit(),
				Existing: false,
			}, nil
		},
	})

	resp, err := srv.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		RepoId:  "repo_xyz789",
		Message: "test commit",
		Author:  "alice@example.com",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "2024-04-05T10:00:00Z", resp.Commit.Timestamp)
}

func TestCreateCommit_DataPointerMetadataMappedCorrectly(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		createFn: func(_ context.Context, in service.CreateCommitInput) (*service.CreateCommitResult, error) {
			assert.NotNil(t, in.DataPointer.Metadata)
			return &service.CreateCommitResult{
				Commit:   testhelper.FixedCommit(),
				Existing: false,
			}, nil
		},
	})

	metadata := []byte(`{"key":"value"}`)
	resp, err := srv.CreateCommit(context.Background(), &vergev1.CreateCommitRequest{
		RepoId:  "repo_xyz789",
		Message: "test commit",
		Author:  "alice@example.com",
		DataPointer: &vergev1.DataPointer{
			Type:     "s3",
			Location: "s3://bucket/path",
			Metadata: metadata,
		},
	})

	require.NoError(t, err)
	assert.Equal(t, metadata, resp.Commit.DataPointer.Metadata)
}

// GetCommit

func TestGetCommit_CommitExists_ReturnsCorrectCommit(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		getFn: func(_ context.Context, repoID, commitID string) (*domain.Commit, error) {
			assert.Equal(t, "repo_xyz789", repoID)
			assert.Equal(t, "commit_abc123", commitID)
			return testhelper.FixedCommit(), nil
		},
	})

	resp, err := srv.GetCommit(context.Background(), &vergev1.GetCommitRequest{
		RepoId:   "repo_xyz789",
		CommitId: "commit_abc123",
	})

	require.NoError(t, err)
	assert.Equal(t, "commit_abc123", resp.Id)
	assert.Equal(t, "repo_xyz789", resp.RepoId)
	assert.Equal(t, "Add feature", resp.Message)
	assert.Equal(t, "alice@example.com", resp.Author)
}

func TestGetCommit_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewCommitServer(&mockCommitService{
		getFn: func(_ context.Context, _, _ string) (*domain.Commit, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.GetCommit(context.Background(), &vergev1.GetCommitRequest{
		CommitId: "commit_abc123",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestGetCommit_MissingCommitID_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewCommitServer(&mockCommitService{
		getFn: func(_ context.Context, _, _ string) (*domain.Commit, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.GetCommit(context.Background(), &vergev1.GetCommitRequest{
		RepoId: "repo_xyz789",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestGetCommit_ServiceReturnsRepoNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		getFn: func(_ context.Context, _, _ string) (*domain.Commit, error) {
			return nil, domain.ErrRepoNotFound
		},
	})

	_, err := srv.GetCommit(context.Background(), &vergev1.GetCommitRequest{
		RepoId:   "repo_missing",
		CommitId: "commit_abc123",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestGetCommit_ServiceReturnsCommitNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		getFn: func(_ context.Context, _, _ string) (*domain.Commit, error) {
			return nil, domain.ErrCommitNotFound
		},
	})

	_, err := srv.GetCommit(context.Background(), &vergev1.GetCommitRequest{
		RepoId:   "repo_xyz789",
		CommitId: "commit_missing",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestGetCommit_ServiceReturnsUnexpectedError_ReturnsInternal(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		getFn: func(_ context.Context, _, _ string) (*domain.Commit, error) {
			return nil, errors.New("connection lost")
		},
	})

	_, err := srv.GetCommit(context.Background(), &vergev1.GetCommitRequest{
		RepoId:   "repo_xyz789",
		CommitId: "commit_abc123",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, testhelper.GRPCCode(err))
}

// ListCommits

func TestListCommits_ValidRequest_CallsServiceWithCorrectInput(t *testing.T) {
	var captured service.ListCommitsInput

	srv := NewCommitServer(&mockCommitService{
		listFn: func(_ context.Context, in service.ListCommitsInput) (*service.ListCommitsResult, error) {
			captured = in
			return &service.ListCommitsResult{
				Commits: []*domain.Commit{testhelper.FixedCommit()},
			}, nil
		},
	})

	resp, err := srv.ListCommits(context.Background(), &vergev1.ListCommitsRequest{
		RepoId:    "repo_xyz789",
		Branch:    "main",
		Author:    "alice@example.com",
		Since:     "2024-01-01T00:00:00Z",
		Until:     "2024-12-31T23:59:59Z",
		Traversal: "bfs",
		Limit:     10,
		Cursor:    "cursor123",
	})

	require.NoError(t, err)
	require.Len(t, resp.Commits, 1)
	assert.Equal(t, "repo_xyz789", captured.RepoID)
	assert.Equal(t, "main", captured.Branch)
	assert.Equal(t, "alice@example.com", captured.Author)
	assert.Equal(t, "2024-01-01T00:00:00Z", captured.Since)
	assert.Equal(t, "2024-12-31T23:59:59Z", captured.Until)
	assert.Equal(t, "bfs", captured.Traversal)
	assert.Equal(t, 10, captured.Limit)
	assert.Equal(t, "cursor123", captured.Cursor)
}

func TestListCommits_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewCommitServer(&mockCommitService{
		listFn: func(_ context.Context, _ service.ListCommitsInput) (*service.ListCommitsResult, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.ListCommits(context.Background(), &vergev1.ListCommitsRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestListCommits_ZeroLimit_DefaultsTo20(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		listFn: func(_ context.Context, in service.ListCommitsInput) (*service.ListCommitsResult, error) {
			assert.Equal(t, 20, in.Limit)
			return &service.ListCommitsResult{}, nil
		},
	})

	_, err := srv.ListCommits(context.Background(), &vergev1.ListCommitsRequest{
		RepoId: "repo_xyz789",
	})

	require.NoError(t, err)
}

func TestListCommits_ExplicitValidLimit_PassedToService(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		listFn: func(_ context.Context, in service.ListCommitsInput) (*service.ListCommitsResult, error) {
			assert.Equal(t, 50, in.Limit)
			return &service.ListCommitsResult{}, nil
		},
	})

	_, err := srv.ListCommits(context.Background(), &vergev1.ListCommitsRequest{
		RepoId: "repo_xyz789",
		Limit:  50,
	})

	require.NoError(t, err)
}

func TestListCommits_ServiceReturnsUnexpectedError_ReturnsInternal(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		listFn: func(_ context.Context, _ service.ListCommitsInput) (*service.ListCommitsResult, error) {
			return nil, errors.New("connection timeout")
		},
	})

	_, err := srv.ListCommits(context.Background(), &vergev1.ListCommitsRequest{
		RepoId: "repo_xyz789",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, testhelper.GRPCCode(err))
}

func TestListCommits_WithNextCursor_IncludedInResponse(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		listFn: func(_ context.Context, _ service.ListCommitsInput) (*service.ListCommitsResult, error) {
			return &service.ListCommitsResult{
				Commits:    []*domain.Commit{testhelper.FixedCommit()},
				NextCursor: "next-page-abc",
			}, nil
		},
	})

	resp, err := srv.ListCommits(context.Background(), &vergev1.ListCommitsRequest{
		RepoId: "repo_xyz789",
	})

	require.NoError(t, err)
	assert.Equal(t, "next-page-abc", resp.NextCursor)
}

func TestListCommits_CommitsAreMappedCorrectly(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		listFn: func(_ context.Context, _ service.ListCommitsInput) (*service.ListCommitsResult, error) {
			return &service.ListCommitsResult{
				Commits: []*domain.Commit{testhelper.FixedCommit()},
			}, nil
		},
	})

	resp, err := srv.ListCommits(context.Background(), &vergev1.ListCommitsRequest{
		RepoId: "repo_xyz789",
	})

	require.NoError(t, err)
	require.Len(t, resp.Commits, 1)
	assert.Equal(t, "commit_abc123", resp.Commits[0].Id)
	assert.Equal(t, "repo_xyz789", resp.Commits[0].RepoId)
	assert.Equal(t, "Add feature", resp.Commits[0].Message)
	assert.Equal(t, "alice@example.com", resp.Commits[0].Author)
	assert.NotEmpty(t, resp.Commits[0].Timestamp)
}

// GetParents

func TestGetParents_CommitExists_ReturnsParents(t *testing.T) {
	parent1 := testhelper.FixedCommit()
	parent1.ID = "parent1"

	srv := NewCommitServer(&mockCommitService{
		getParentsFn: func(_ context.Context, repoID, commitID string) ([]*domain.Commit, error) {
			assert.Equal(t, "repo_xyz789", repoID)
			assert.Equal(t, "commit_abc123", commitID)
			return []*domain.Commit{parent1}, nil
		},
	})

	resp, err := srv.GetParents(context.Background(), &vergev1.GetParentsRequest{
		RepoId:   "repo_xyz789",
		CommitId: "commit_abc123",
	})

	require.NoError(t, err)
	assert.Equal(t, "commit_abc123", resp.CommitId)
	require.Len(t, resp.Parents, 1)
	assert.Equal(t, "parent1", resp.Parents[0].Id)
}

func TestGetParents_MissingRepoID_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewCommitServer(&mockCommitService{
		getParentsFn: func(_ context.Context, _, _ string) ([]*domain.Commit, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.GetParents(context.Background(), &vergev1.GetParentsRequest{
		CommitId: "commit_abc123",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestGetParents_MissingCommitID_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewCommitServer(&mockCommitService{
		getParentsFn: func(_ context.Context, _, _ string) ([]*domain.Commit, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.GetParents(context.Background(), &vergev1.GetParentsRequest{
		RepoId: "repo_xyz789",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestGetParents_ServiceReturnsRepoNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		getParentsFn: func(_ context.Context, _, _ string) ([]*domain.Commit, error) {
			return nil, domain.ErrRepoNotFound
		},
	})

	_, err := srv.GetParents(context.Background(), &vergev1.GetParentsRequest{
		RepoId:   "repo_missing",
		CommitId: "commit_abc123",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestGetParents_ServiceReturnsCommitNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		getParentsFn: func(_ context.Context, _, _ string) ([]*domain.Commit, error) {
			return nil, domain.ErrCommitNotFound
		},
	})

	_, err := srv.GetParents(context.Background(), &vergev1.GetParentsRequest{
		RepoId:   "repo_xyz789",
		CommitId: "commit_missing",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestGetParents_ServiceReturnsUnexpectedError_ReturnsInternal(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		getParentsFn: func(_ context.Context, _, _ string) ([]*domain.Commit, error) {
			return nil, errors.New("database error")
		},
	})

	_, err := srv.GetParents(context.Background(), &vergev1.GetParentsRequest{
		RepoId:   "repo_xyz789",
		CommitId: "commit_abc123",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, testhelper.GRPCCode(err))
}

func TestGetParents_NoParents_ReturnsEmptyList(t *testing.T) {
	srv := NewCommitServer(&mockCommitService{
		getParentsFn: func(_ context.Context, _, _ string) ([]*domain.Commit, error) {
			return []*domain.Commit{}, nil
		},
	})

	resp, err := srv.GetParents(context.Background(), &vergev1.GetParentsRequest{
		RepoId:   "repo_xyz789",
		CommitId: "commit_abc123",
	})

	require.NoError(t, err)
	assert.Len(t, resp.Parents, 0)
}

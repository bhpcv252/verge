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

type mockRepoService struct {
	createFn func(ctx context.Context, in service.CreateRepoInput) (*domain.Repo, error)
	getFn    func(ctx context.Context, id string) (*domain.Repo, error)
	listFn   func(ctx context.Context, in service.ListReposInput) (*service.ListReposResult, error)
}

func (m *mockRepoService) CreateRepo(
	ctx context.Context,
	in service.CreateRepoInput,
) (*domain.Repo, error) {
	return m.createFn(ctx, in)
}

func (m *mockRepoService) GetRepo(ctx context.Context, id string) (*domain.Repo, error) {
	return m.getFn(ctx, id)
}

func (m *mockRepoService) ListRepos(
	ctx context.Context,
	in service.ListReposInput,
) (*service.ListReposResult, error) {
	return m.listFn(ctx, in)
}

// CreateRepo

func TestCreateRepo_ValidRequest_CallsServiceWithCorrectInputAndReturnsRepo(t *testing.T) {
	var captured service.CreateRepoInput

	srv := NewRepoServer(&mockRepoService{
		createFn: func(_ context.Context, in service.CreateRepoInput) (*domain.Repo, error) {
			captured = in
			return testhelper.FixedRepo(), nil
		},
	})

	resp, err := srv.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
		Name:          "my-doc",
		DefaultBranch: "main",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "repo_abc123", resp.Id)
	assert.Equal(t, "my-doc", resp.Name)
	assert.Equal(t, "main", resp.DefaultBranch)
	assert.NotEmpty(t, resp.CreatedAt)
	assert.Equal(t, "my-doc", captured.Name)
	assert.Equal(t, "main", captured.DefaultBranch)
}

func TestCreateRepo_MissingName_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewRepoServer(&mockRepoService{
		createFn: func(_ context.Context, _ service.CreateRepoInput) (*domain.Repo, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
		DefaultBranch: "main",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called, "service should not be called when name is missing")
}

func TestCreateRepo_MissingDefaultBranch_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewRepoServer(&mockRepoService{
		createFn: func(_ context.Context, _ service.CreateRepoInput) (*domain.Repo, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
		Name: "my-doc",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestCreateRepo_ServiceReturnsUnexpectedError_ReturnsInternal(t *testing.T) {
	srv := NewRepoServer(&mockRepoService{
		createFn: func(_ context.Context, _ service.CreateRepoInput) (*domain.Repo, error) {
			return nil, errors.New("db exploded")
		},
	})

	_, err := srv.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
		Name:          "my-doc",
		DefaultBranch: "main",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, testhelper.GRPCCode(err))
}

func TestCreateRepo_CreatedAtIsISO8601(t *testing.T) {
	srv := NewRepoServer(&mockRepoService{
		createFn: func(_ context.Context, _ service.CreateRepoInput) (*domain.Repo, error) {
			return testhelper.FixedRepo(), nil
		},
	})

	resp, err := srv.CreateRepo(context.Background(), &vergev1.CreateRepoRequest{
		Name:          "my-doc",
		DefaultBranch: "main",
	})

	require.NoError(t, err)
	assert.Equal(t, "2024-04-05T10:00:00Z", resp.CreatedAt)
}

// GetRepo

func TestGetRepo_RepoExists_ReturnsCorrectRepo(t *testing.T) {
	srv := NewRepoServer(&mockRepoService{
		getFn: func(_ context.Context, id string) (*domain.Repo, error) {
			assert.Equal(t, "repo_abc123", id)
			return testhelper.FixedRepo(), nil
		},
	})

	resp, err := srv.GetRepo(context.Background(), &vergev1.GetRepoRequest{RepoId: "repo_abc123"})

	require.NoError(t, err)
	assert.Equal(t, "repo_abc123", resp.Id)
	assert.Equal(t, "my-doc", resp.Name)
	assert.Equal(t, "main", resp.DefaultBranch)
}

func TestGetRepo_RepoIDFromRequestPassedToService(t *testing.T) {
	srv := NewRepoServer(&mockRepoService{
		getFn: func(_ context.Context, id string) (*domain.Repo, error) {
			assert.Equal(t, "repo_xyz999", id)
			return testhelper.FixedRepo(), nil
		},
	})

	_, err := srv.GetRepo(context.Background(), &vergev1.GetRepoRequest{RepoId: "repo_xyz999"})
	require.NoError(t, err)
}

func TestGetRepo_EmptyRepoID_ReturnsInvalidArgument(t *testing.T) {
	called := false
	srv := NewRepoServer(&mockRepoService{
		getFn: func(_ context.Context, _ string) (*domain.Repo, error) {
			called = true
			return nil, nil
		},
	})

	_, err := srv.GetRepo(context.Background(), &vergev1.GetRepoRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
	assert.False(t, called)
}

func TestGetRepo_ServiceReturnsRepoNotFound_ReturnsNotFound(t *testing.T) {
	srv := NewRepoServer(&mockRepoService{
		getFn: func(_ context.Context, _ string) (*domain.Repo, error) {
			return nil, domain.ErrRepoNotFound
		},
	})

	_, err := srv.GetRepo(context.Background(), &vergev1.GetRepoRequest{RepoId: "repo_missing"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, testhelper.GRPCCode(err))
}

func TestGetRepo_ServiceReturnsUnexpectedError_ReturnsInternal(t *testing.T) {
	srv := NewRepoServer(&mockRepoService{
		getFn: func(_ context.Context, _ string) (*domain.Repo, error) {
			return nil, errors.New("connection lost")
		},
	})

	_, err := srv.GetRepo(context.Background(), &vergev1.GetRepoRequest{RepoId: "repo_abc"})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, testhelper.GRPCCode(err))
}

// ListRepos

func TestListRepos_ZeroLimit_DefaultsTo20AndPassesToService(t *testing.T) {
	srv := NewRepoServer(&mockRepoService{
		listFn: func(_ context.Context, in service.ListReposInput) (*service.ListReposResult, error) {
			assert.Equal(t, 20, in.Limit)
			assert.Equal(t, "", in.Cursor)
			return &service.ListReposResult{Repos: []*domain.Repo{testhelper.FixedRepo()}}, nil
		},
	})

	resp, err := srv.ListRepos(context.Background(), &vergev1.ListReposRequest{})

	require.NoError(t, err)
	assert.Len(t, resp.Repos, 1)
}

func TestListRepos_ExplicitValidLimit_PassedToService(t *testing.T) {
	srv := NewRepoServer(&mockRepoService{
		listFn: func(_ context.Context, in service.ListReposInput) (*service.ListReposResult, error) {
			assert.Equal(t, 50, in.Limit)
			return &service.ListReposResult{}, nil
		},
	})

	_, err := srv.ListRepos(context.Background(), &vergev1.ListReposRequest{Limit: 50})
	require.NoError(t, err)
}

func TestListRepos_LimitOutOfRange_ReturnsInvalidArgument(t *testing.T) {
	cases := []struct {
		name  string
		limit int32
	}{
		{"over_max", 101},
		{"negative", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			srv := NewRepoServer(&mockRepoService{
				listFn: func(_ context.Context, _ service.ListReposInput) (*service.ListReposResult, error) {
					called = true
					return nil, nil
				},
			})

			_, err := srv.ListRepos(
				context.Background(),
				&vergev1.ListReposRequest{Limit: tc.limit},
			)

			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, testhelper.GRPCCode(err))
			assert.False(t, called)
		})
	}
}

func TestListRepos_CursorParam_PassedToService(t *testing.T) {
	srv := NewRepoServer(&mockRepoService{
		listFn: func(_ context.Context, in service.ListReposInput) (*service.ListReposResult, error) {
			assert.Equal(t, "some-cursor", in.Cursor)
			return &service.ListReposResult{}, nil
		},
	})

	_, err := srv.ListRepos(context.Background(), &vergev1.ListReposRequest{Cursor: "some-cursor"})
	require.NoError(t, err)
}

func TestListRepos_WithNextCursor_IncludedInResponse(t *testing.T) {
	srv := NewRepoServer(&mockRepoService{
		listFn: func(_ context.Context, _ service.ListReposInput) (*service.ListReposResult, error) {
			return &service.ListReposResult{
				Repos:      []*domain.Repo{testhelper.FixedRepo()},
				NextCursor: "next-page-abc",
			}, nil
		},
	})

	resp, err := srv.ListRepos(context.Background(), &vergev1.ListReposRequest{})

	require.NoError(t, err)
	assert.Equal(t, "next-page-abc", resp.NextCursor)
}

func TestListRepos_NoNextCursor_EmptyStringInResponse(t *testing.T) {
	srv := NewRepoServer(&mockRepoService{
		listFn: func(_ context.Context, _ service.ListReposInput) (*service.ListReposResult, error) {
			return &service.ListReposResult{
				Repos:      []*domain.Repo{testhelper.FixedRepo()},
				NextCursor: "",
			}, nil
		},
	})

	resp, err := srv.ListRepos(context.Background(), &vergev1.ListReposRequest{})

	require.NoError(t, err)
	assert.Empty(t, resp.NextCursor)
}

func TestListRepos_ServiceReturnsUnexpectedError_ReturnsInternal(t *testing.T) {
	srv := NewRepoServer(&mockRepoService{
		listFn: func(_ context.Context, _ service.ListReposInput) (*service.ListReposResult, error) {
			return nil, errors.New("timeout")
		},
	})

	_, err := srv.ListRepos(context.Background(), &vergev1.ListReposRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, testhelper.GRPCCode(err))
}

func TestListRepos_ReposAreMappedCorrectly(t *testing.T) {
	srv := NewRepoServer(&mockRepoService{
		listFn: func(_ context.Context, _ service.ListReposInput) (*service.ListReposResult, error) {
			return &service.ListReposResult{
				Repos: []*domain.Repo{testhelper.FixedRepo()},
			}, nil
		},
	})

	resp, err := srv.ListRepos(context.Background(), &vergev1.ListReposRequest{})

	require.NoError(t, err)
	require.Len(t, resp.Repos, 1)
	assert.Equal(t, "repo_abc123", resp.Repos[0].Id)
	assert.Equal(t, "my-doc", resp.Repos[0].Name)
	assert.Equal(t, "main", resp.Repos[0].DefaultBranch)
	assert.NotEmpty(t, resp.Repos[0].CreatedAt)
}

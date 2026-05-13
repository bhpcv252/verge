package v1

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// helpers

func newTestRouter(svc RepoService) http.Handler {
	return NewRouter(
		NewRepoHandler(svc),
		nil, // branchHandler
		nil, // commitHandler
		nil, // mergeHandler
	)
}

// POST /v1/repos

func TestCreateRepo_ValidBody_Returns201AndCallsServiceWithCorrectInput(t *testing.T) {
	var capturedInput service.CreateRepoInput

	svc := &mockRepoService{
		createFn: func(_ context.Context, in service.CreateRepoInput) (*domain.Repo, error) {
			capturedInput = in
			return testhelper.FixedRepo(), nil
		},
	}

	w := testhelper.PostJSON(t, newTestRouter(svc), "/v1/repos", map[string]string{
		"name":           "my-doc",
		"default_branch": "main",
	})

	require.Equal(t, http.StatusCreated, w.Code)

	var got repoResponse
	testhelper.DecodeBody(t, w, &got)
	assert.Equal(t, "repo_abc123", got.ID)
	assert.Equal(t, "main", got.DefaultBranch)
	assert.False(t, got.CreatedAt.IsZero())
	assert.Equal(t, "my-doc", capturedInput.Name)
	assert.Equal(t, "main", capturedInput.DefaultBranch)
}

func TestCreateRepo_InvalidJSON_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockRepoService{
		createFn: func(_ context.Context, _ service.CreateRepoInput) (*domain.Repo, error) {
			called = true
			return nil, nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/repos", bytes.NewBufferString("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newTestRouter(svc).ServeHTTP(w, req)

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called, "service should not have been called on invalid JSON")
}

func TestCreateRepo_MissingName_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockRepoService{
		createFn: func(_ context.Context, _ service.CreateRepoInput) (*domain.Repo, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.PostJSON(t, newTestRouter(svc), "/v1/repos", map[string]string{
		"default_branch": "main",
	})

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called, "service should not have been called when name is missing")
}

func TestCreateRepo_MissingDefaultBranch_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockRepoService{
		createFn: func(_ context.Context, _ service.CreateRepoInput) (*domain.Repo, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.PostJSON(t, newTestRouter(svc), "/v1/repos", map[string]string{
		"name": "my-doc",
	})

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called, "service should not have been called when default_branch is missing")
}

func TestCreateRepo_WhitespaceOnlyName_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockRepoService{
		createFn: func(_ context.Context, _ service.CreateRepoInput) (*domain.Repo, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.PostJSON(t, newTestRouter(svc), "/v1/repos", map[string]string{
		"name":           "   ",
		"default_branch": "main",
	})

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestCreateRepo_WhitespaceOnlyDefaultBranch_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockRepoService{
		createFn: func(_ context.Context, _ service.CreateRepoInput) (*domain.Repo, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.PostJSON(t, newTestRouter(svc), "/v1/repos", map[string]string{
		"name":           "my-doc",
		"default_branch": "   ",
	})

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestCreateRepo_ServiceReturnsUnexpectedError_Returns500(t *testing.T) {
	svc := &mockRepoService{
		createFn: func(_ context.Context, _ service.CreateRepoInput) (*domain.Repo, error) {
			return nil, errors.New("something blew up")
		},
	}

	w := testhelper.PostJSON(t, newTestRouter(svc), "/v1/repos", map[string]string{
		"name":           "my-doc",
		"default_branch": "main",
	})

	testhelper.AssertErrorCode(t, w, http.StatusInternalServerError, "internal_error")
}

func TestCreateRepo_ValidBody_ResponseContentTypeIsJSON(t *testing.T) {
	svc := &mockRepoService{
		createFn: func(_ context.Context, _ service.CreateRepoInput) (*domain.Repo, error) {
			return testhelper.FixedRepo(), nil
		},
	}

	w := testhelper.PostJSON(t, newTestRouter(svc), "/v1/repos", map[string]string{
		"name":           "my-doc",
		"default_branch": "main",
	})

	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}

// GET /v1/repos/:id

func TestGetRepo_ServiceReturnsRepo_Returns200WithCorrectShape(t *testing.T) {
	repo := testhelper.FixedRepo()
	svc := &mockRepoService{
		getFn: func(_ context.Context, id string) (*domain.Repo, error) {
			assert.Equal(t, repo.ID, id)
			return repo, nil
		},
	}

	w := testhelper.GetPath(t, newTestRouter(svc), "/v1/repos/"+repo.ID)

	require.Equal(t, http.StatusOK, w.Code)
	var got repoResponse
	testhelper.DecodeBody(t, w, &got)
	assert.Equal(t, repo.ID, got.ID)
	assert.Equal(t, repo.Name, got.Name)
	assert.Equal(t, repo.DefaultBranch, got.DefaultBranch)
}

func TestGetRepo_RepoIDFromPathPassedToService(t *testing.T) {
	svc := &mockRepoService{
		getFn: func(_ context.Context, id string) (*domain.Repo, error) {
			assert.Equal(t, "repo_xyz999", id)
			return testhelper.FixedRepo(), nil
		},
	}

	w := testhelper.GetPath(t, newTestRouter(svc), "/v1/repos/repo_xyz999")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestGetRepo_ServiceReturnsRepoNotFound_Returns404WithRepoNotFoundCode(t *testing.T) {
	svc := &mockRepoService{
		getFn: func(_ context.Context, _ string) (*domain.Repo, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	w := testhelper.GetPath(t, newTestRouter(svc), "/v1/repos/repo_missing")
	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "repo_not_found")
}

func TestGetRepo_ServiceReturnsUnexpectedError_Returns500(t *testing.T) {
	svc := &mockRepoService{
		getFn: func(_ context.Context, _ string) (*domain.Repo, error) {
			return nil, errors.New("db connection lost")
		},
	}

	w := testhelper.GetPath(t, newTestRouter(svc), "/v1/repos/repo_abc")
	testhelper.AssertErrorCode(t, w, http.StatusInternalServerError, "internal_error")
}

// GET /v1/repos

func TestListRepos_NoParams_Returns200AndPassesDefaultLimitToService(t *testing.T) {
	svc := &mockRepoService{
		listFn: func(_ context.Context, in service.ListReposInput) (*service.ListReposResult, error) {
			assert.Equal(t, 20, in.Limit)
			assert.Equal(t, "", in.Cursor)
			return &service.ListReposResult{Repos: []*domain.Repo{testhelper.FixedRepo()}}, nil
		},
	}

	w := testhelper.GetPath(t, newTestRouter(svc), "/v1/repos")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestListRepos_ExplicitValidLimit_PassedToService(t *testing.T) {
	svc := &mockRepoService{
		listFn: func(_ context.Context, in service.ListReposInput) (*service.ListReposResult, error) {
			assert.Equal(t, 50, in.Limit)
			return &service.ListReposResult{}, nil
		},
	}

	w := testhelper.GetPath(t, newTestRouter(svc), fmt.Sprintf("/v1/repos?limit=%d", 50))
	require.Equal(t, http.StatusOK, w.Code)
}

func TestListRepos_InvalidLimitParam_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockRepoService{
		listFn: func(_ context.Context, _ service.ListReposInput) (*service.ListReposResult, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.GetPath(t, newTestRouter(svc), "/v1/repos?limit=abc")
	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called, "service should not have been called on invalid limit")
}

func TestListRepos_LimitOutOfRange_Returns400(t *testing.T) {
	cases := []struct{ name, limit string }{
		{"zero", "0"},
		{"over_max", "101"},
		{"negative", "-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			svc := &mockRepoService{
				listFn: func(_ context.Context, _ service.ListReposInput) (*service.ListReposResult, error) {
					called = true
					return nil, nil
				},
			}
			w := testhelper.GetPath(t, newTestRouter(svc), "/v1/repos?limit="+tc.limit)
			testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
			assert.False(t, called)
		})
	}
}

func TestListRepos_CursorParam_PassedToService(t *testing.T) {
	svc := &mockRepoService{
		listFn: func(_ context.Context, in service.ListReposInput) (*service.ListReposResult, error) {
			assert.Equal(t, "some-cursor", in.Cursor)
			return &service.ListReposResult{}, nil
		},
	}

	w := testhelper.GetPath(t, newTestRouter(svc), "/v1/repos?cursor=some-cursor")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestListRepos_WithNextCursor_IncludedInResponse(t *testing.T) {
	svc := &mockRepoService{
		listFn: func(_ context.Context, _ service.ListReposInput) (*service.ListReposResult, error) {
			return &service.ListReposResult{
				Repos:      []*domain.Repo{testhelper.FixedRepo()},
				NextCursor: "next-page-abc",
			}, nil
		},
	}

	w := testhelper.GetPath(t, newTestRouter(svc), "/v1/repos")
	require.Equal(t, http.StatusOK, w.Code)

	var got listReposResponse
	testhelper.DecodeBody(t, w, &got)
	require.NotNil(t, got.NextCursor)
	assert.Equal(t, "next-page-abc", *got.NextCursor)
}

func TestListRepos_NoNextCursor_NullInResponse(t *testing.T) {
	svc := &mockRepoService{
		listFn: func(_ context.Context, _ service.ListReposInput) (*service.ListReposResult, error) {
			return &service.ListReposResult{
				Repos:      []*domain.Repo{testhelper.FixedRepo()},
				NextCursor: "",
			}, nil
		},
	}

	w := testhelper.GetPath(t, newTestRouter(svc), "/v1/repos")
	require.Equal(t, http.StatusOK, w.Code)

	var got listReposResponse
	testhelper.DecodeBody(t, w, &got)
	assert.Nil(t, got.NextCursor)
}

func TestListRepos_ServiceReturnsUnexpectedError_Returns500(t *testing.T) {
	svc := &mockRepoService{
		listFn: func(_ context.Context, _ service.ListReposInput) (*service.ListReposResult, error) {
			return nil, errors.New("timeout")
		},
	}

	w := testhelper.GetPath(t, newTestRouter(svc), "/v1/repos")
	testhelper.AssertErrorCode(t, w, http.StatusInternalServerError, "internal_error")
}

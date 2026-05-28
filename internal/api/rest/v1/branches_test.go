package v1

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/api/core"
	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/observability"
	"github.com/bhpcv252/verge/internal/service"
	"github.com/bhpcv252/verge/testhelper"
)

// mock

type mockBranchService struct {
	createFn  func(ctx context.Context, in service.CreateBranchInput) (*domain.Branch, error)
	getFn     func(ctx context.Context, repoID, name string) (*domain.Branch, error)
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
	return m.getFn(ctx, repoID, name)
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

// helper

func newBranchTestRouter(svc core.BranchService) http.Handler {
	return NewRouter(
		observability.Noop(),
		nil, // auth disabled
		nil, // repoHandler
		NewBranchHandler(svc),
		nil, // commitHandler
		nil, // mergeHandler
	)
}

// POST /v1/repos/:repoID/branches

func TestCreateBranch_ValidBody_Returns201AndCallsServiceWithCorrectInput(t *testing.T) {
	var capturedInput service.CreateBranchInput

	svc := &mockBranchService{
		createFn: func(_ context.Context, in service.CreateBranchInput) (*domain.Branch, error) {
			capturedInput = in
			return testhelper.FixedBranch(), nil
		},
	}

	w := testhelper.PostJSON(
		t,
		newBranchTestRouter(svc),
		"/v1/repos/repo_abc123/branches",
		map[string]string{
			"name":             "main",
			"source_commit_id": "commit_xyz789",
		},
	)

	require.Equal(t, http.StatusCreated, w.Code)

	var got branchResponse
	testhelper.DecodeBody(t, w, &got)
	assert.Equal(t, "main", got.Name)
	assert.Equal(t, "repo_abc123", got.RepoID)
	assert.Equal(t, "commit_xyz789", got.CommitID)
	assert.NotEmpty(t, got.CreatedAt)

	assert.Equal(t, "repo_abc123", capturedInput.RepoID)
	assert.Equal(t, "main", capturedInput.Name)
	assert.Equal(t, "commit_xyz789", capturedInput.SourceCommitID)
}

func TestCreateBranch_InvalidJSON_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockBranchService{
		createFn: func(_ context.Context, _ service.CreateBranchInput) (*domain.Branch, error) {
			called = true
			return nil, nil
		},
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/repos/repo_abc/branches",
		bytes.NewBufferString("{bad json"),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newBranchTestRouter(svc).ServeHTTP(w, req)

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestCreateBranch_MissingName_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockBranchService{
		createFn: func(_ context.Context, _ service.CreateBranchInput) (*domain.Branch, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.PostJSON(
		t,
		newBranchTestRouter(svc),
		"/v1/repos/repo_abc/branches",
		map[string]string{
			"source_commit_id": "commit_xyz",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestCreateBranch_MissingSourceCommitID_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockBranchService{
		createFn: func(_ context.Context, _ service.CreateBranchInput) (*domain.Branch, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.PostJSON(
		t,
		newBranchTestRouter(svc),
		"/v1/repos/repo_abc/branches",
		map[string]string{
			"name": "main",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestCreateBranch_RepoNotFound_Returns404WithRepoNotFoundCode(t *testing.T) {
	svc := &mockBranchService{
		createFn: func(_ context.Context, _ service.CreateBranchInput) (*domain.Branch, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	w := testhelper.PostJSON(
		t,
		newBranchTestRouter(svc),
		"/v1/repos/repo_missing/branches",
		map[string]string{
			"name":             "main",
			"source_commit_id": "commit_xyz",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "repo_not_found")
}

func TestCreateBranch_CommitNotFound_Returns404WithCommitNotFoundCode(t *testing.T) {
	svc := &mockBranchService{
		createFn: func(_ context.Context, _ service.CreateBranchInput) (*domain.Branch, error) {
			return nil, domain.ErrCommitNotFound
		},
	}

	w := testhelper.PostJSON(
		t,
		newBranchTestRouter(svc),
		"/v1/repos/repo_abc/branches",
		map[string]string{
			"name":             "main",
			"source_commit_id": "commit_missing",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "commit_not_found")
}

func TestCreateBranch_BranchAlreadyExists_Returns409WithBranchAlreadyExistsCode(t *testing.T) {
	svc := &mockBranchService{
		createFn: func(_ context.Context, _ service.CreateBranchInput) (*domain.Branch, error) {
			return nil, domain.ErrBranchAlreadyExists
		},
	}

	w := testhelper.PostJSON(
		t,
		newBranchTestRouter(svc),
		"/v1/repos/repo_abc/branches",
		map[string]string{
			"name":             "main",
			"source_commit_id": "commit_xyz",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusConflict, "branch_already_exists")
}

// GET /v1/repos/:repoID/branches/:name

func TestGetBranch_Exists_Returns200WithCorrectFields(t *testing.T) {
	svc := &mockBranchService{
		getFn: func(_ context.Context, repoID, name string) (*domain.Branch, error) {
			assert.Equal(t, "repo_abc", repoID)
			assert.Equal(t, "main", name)
			return testhelper.FixedBranch(), nil
		},
	}

	w := testhelper.GetPath(t, newBranchTestRouter(svc), "/v1/repos/repo_abc/branches/main")

	require.Equal(t, http.StatusOK, w.Code)
	var got branchResponse
	testhelper.DecodeBody(t, w, &got)
	assert.Equal(t, "main", got.Name)
	assert.Equal(t, "repo_abc123", got.RepoID)
	assert.Equal(t, "commit_xyz789", got.CommitID)
}

func TestGetBranch_NotFound_Returns404WithBranchNotFoundCode(t *testing.T) {
	svc := &mockBranchService{
		getFn: func(_ context.Context, _, _ string) (*domain.Branch, error) {
			return nil, domain.ErrBranchNotFound
		},
	}

	w := testhelper.GetPath(t, newBranchTestRouter(svc), "/v1/repos/repo_abc/branches/missing")
	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "branch_not_found")
}

func TestCreateBranch_ServiceReturnsUnexpectedError_Returns500(t *testing.T) {
	svc := &mockBranchService{
		createFn: func(_ context.Context, _ service.CreateBranchInput) (*domain.Branch, error) {
			return nil, errors.New("db timeout")
		},
	}

	w := testhelper.PostJSON(
		t,
		newBranchTestRouter(svc),
		"/v1/repos/repo_abc/branches",
		map[string]string{
			"name":             "main",
			"source_commit_id": "commit_xyz",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusInternalServerError, "internal_error")
}

// GET /v1/repos/:repoID/branches

func TestListBranches_NoParams_Returns200AndPassesDefaultLimitToService(t *testing.T) {
	svc := &mockBranchService{
		listFn: func(_ context.Context, in service.ListBranchesInput) (*service.ListBranchesResult, error) {
			assert.Equal(t, 20, in.Limit)
			assert.Equal(t, "", in.Cursor)
			return &service.ListBranchesResult{
				Branches: []*domain.Branch{testhelper.FixedBranch()},
			}, nil
		},
	}

	w := testhelper.GetPath(t, newBranchTestRouter(svc), "/v1/repos/repo_abc/branches")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestListBranches_ExplicitValidLimit_PassedToService(t *testing.T) {
	svc := &mockBranchService{
		listFn: func(_ context.Context, in service.ListBranchesInput) (*service.ListBranchesResult, error) {
			assert.Equal(t, 50, in.Limit)
			return &service.ListBranchesResult{}, nil
		},
	}

	w := testhelper.GetPath(t, newBranchTestRouter(svc), "/v1/repos/repo_abc/branches?limit=50")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestListBranches_InvalidLimitParam_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockBranchService{
		listFn: func(_ context.Context, _ service.ListBranchesInput) (*service.ListBranchesResult, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.GetPath(t, newBranchTestRouter(svc), "/v1/repos/repo_abc/branches?limit=abc")
	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestListBranches_LimitOutOfRange_Returns400(t *testing.T) {
	cases := []struct{ name, limit string }{
		{"zero", "0"},
		{"over_max", "101"},
		{"negative", "-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			svc := &mockBranchService{
				listFn: func(_ context.Context, _ service.ListBranchesInput) (*service.ListBranchesResult, error) {
					called = true
					return nil, nil
				},
			}
			w := testhelper.GetPath(
				t,
				newBranchTestRouter(svc),
				"/v1/repos/repo_abc/branches?limit="+tc.limit,
			)
			testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
			assert.False(t, called)
		})
	}
}

func TestListBranches_WithNextCursor_IncludedInResponse(t *testing.T) {
	svc := &mockBranchService{
		listFn: func(_ context.Context, _ service.ListBranchesInput) (*service.ListBranchesResult, error) {
			return &service.ListBranchesResult{
				Branches:   []*domain.Branch{testhelper.FixedBranch()},
				NextCursor: "next-page-abc",
			}, nil
		},
	}

	w := testhelper.GetPath(t, newBranchTestRouter(svc), "/v1/repos/repo_abc/branches")
	require.Equal(t, http.StatusOK, w.Code)

	var got listBranchesResponse
	testhelper.DecodeBody(t, w, &got)
	require.NotEmpty(t, got.NextCursor)
	assert.Equal(t, "next-page-abc", got.NextCursor)
}

func TestListBranches_NoNextCursor_OmittedFromResponse(t *testing.T) {
	svc := &mockBranchService{
		listFn: func(_ context.Context, _ service.ListBranchesInput) (*service.ListBranchesResult, error) {
			return &service.ListBranchesResult{
				Branches:   []*domain.Branch{testhelper.FixedBranch()},
				NextCursor: "",
			}, nil
		},
	}

	w := testhelper.GetPath(t, newBranchTestRouter(svc), "/v1/repos/repo_abc/branches")
	require.Equal(t, http.StatusOK, w.Code)

	var got listBranchesResponse
	testhelper.DecodeBody(t, w, &got)
	assert.Empty(t, got.NextCursor)
}

func TestListBranches_RepoNotFound_Returns404(t *testing.T) {
	svc := &mockBranchService{
		listFn: func(_ context.Context, _ service.ListBranchesInput) (*service.ListBranchesResult, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	w := testhelper.GetPath(t, newBranchTestRouter(svc), "/v1/repos/repo_missing/branches")
	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "repo_not_found")
}

// PATCH /v1/repos/:repoID/branches/:name

func TestAdvanceBranch_ValidBody_Returns200AndCallsServiceWithCorrectInput(t *testing.T) {
	var capturedInput service.AdvanceBranchInput

	svc := &mockBranchService{
		advanceFn: func(_ context.Context, in service.AdvanceBranchInput) (*domain.Branch, error) {
			capturedInput = in
			return testhelper.FixedBranch(), nil
		},
	}

	w := testhelper.PatchJSON(
		t,
		newBranchTestRouter(svc),
		"/v1/repos/repo_abc/branches/main",
		map[string]string{
			"commit_id":          "commit_new",
			"expected_commit_id": "commit_old",
		},
	)

	require.Equal(t, http.StatusOK, w.Code)

	assert.Equal(t, "repo_abc", capturedInput.RepoID)
	assert.Equal(t, "main", capturedInput.Name)
	assert.Equal(t, "commit_new", capturedInput.CommitID)
	assert.Equal(t, "commit_old", capturedInput.ExpectedCommitID)
}

func TestAdvanceBranch_MissingCommitID_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockBranchService{
		advanceFn: func(_ context.Context, _ service.AdvanceBranchInput) (*domain.Branch, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.PatchJSON(
		t,
		newBranchTestRouter(svc),
		"/v1/repos/repo_abc/branches/main",
		map[string]string{
			"expected_commit_id": "commit_old",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestAdvanceBranch_MissingExpectedCommitID_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockBranchService{
		advanceFn: func(_ context.Context, _ service.AdvanceBranchInput) (*domain.Branch, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.PatchJSON(
		t,
		newBranchTestRouter(svc),
		"/v1/repos/repo_abc/branches/main",
		map[string]string{
			"commit_id": "commit_new",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestAdvanceBranch_BranchConflict_Returns409WithCurrentHead(t *testing.T) {
	svc := &mockBranchService{
		advanceFn: func(_ context.Context, _ service.AdvanceBranchInput) (*domain.Branch, error) {
			return nil, &service.BranchConflictError{
				BranchName:   "main",
				CurrentHead:  "commit_actual",
				ExpectedHead: "commit_old",
			}
		},
	}

	w := testhelper.PatchJSON(
		t,
		newBranchTestRouter(svc),
		"/v1/repos/repo_abc/branches/main",
		map[string]string{
			"commit_id":          "commit_new",
			"expected_commit_id": "commit_old",
		},
	)

	require.Equal(t, http.StatusConflict, w.Code)

	var got struct {
		Error       string  `json:"error"`
		Message     string  `json:"message"`
		CurrentHead *string `json:"current_head"`
	}
	testhelper.DecodeBody(t, w, &got)
	assert.Equal(t, "branch_conflict", got.Error)
	require.NotNil(t, got.CurrentHead)
	assert.Equal(t, "commit_actual", *got.CurrentHead)
}

func TestAdvanceBranch_BranchNotFound_Returns404(t *testing.T) {
	svc := &mockBranchService{
		advanceFn: func(_ context.Context, _ service.AdvanceBranchInput) (*domain.Branch, error) {
			return nil, domain.ErrBranchNotFound
		},
	}

	w := testhelper.PatchJSON(
		t,
		newBranchTestRouter(svc),
		"/v1/repos/repo_abc/branches/missing",
		map[string]string{
			"commit_id":          "commit_new",
			"expected_commit_id": "commit_old",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "branch_not_found")
}

func TestAdvanceBranch_CommitNotFound_Returns404(t *testing.T) {
	svc := &mockBranchService{
		advanceFn: func(_ context.Context, _ service.AdvanceBranchInput) (*domain.Branch, error) {
			return nil, domain.ErrCommitNotFound
		},
	}

	w := testhelper.PatchJSON(
		t,
		newBranchTestRouter(svc),
		"/v1/repos/repo_abc/branches/main",
		map[string]string{
			"commit_id":          "commit_missing",
			"expected_commit_id": "commit_old",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "commit_not_found")
}

// DELETE /v1/repos/:repoID/branches/:name

func TestDeleteBranch_Success_Returns204(t *testing.T) {
	svc := &mockBranchService{
		deleteFn: func(_ context.Context, _, _ string) error {
			return nil
		},
	}

	w := testhelper.DeletePath(t, newBranchTestRouter(svc), "/v1/repos/repo_abc/branches/feature-x")
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestDeleteBranch_BranchNotFound_Returns404(t *testing.T) {
	svc := &mockBranchService{
		deleteFn: func(_ context.Context, _, _ string) error {
			return domain.ErrBranchNotFound
		},
	}

	w := testhelper.DeletePath(t, newBranchTestRouter(svc), "/v1/repos/repo_abc/branches/missing")
	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "branch_not_found")
}

func TestDeleteBranch_CannotDeleteDefaultBranch_Returns409(t *testing.T) {
	svc := &mockBranchService{
		deleteFn: func(_ context.Context, _, _ string) error {
			return domain.ErrCannotDeleteDefaultBranch
		},
	}

	w := testhelper.DeletePath(t, newBranchTestRouter(svc), "/v1/repos/repo_abc/branches/main")
	testhelper.AssertErrorCode(t, w, http.StatusConflict, "cannot_delete_default_branch")
}

func TestDeleteBranch_RepoNotFound_Returns404(t *testing.T) {
	svc := &mockBranchService{
		deleteFn: func(_ context.Context, _, _ string) error {
			return domain.ErrRepoNotFound
		},
	}

	w := testhelper.DeletePath(t, newBranchTestRouter(svc), "/v1/repos/repo_missing/branches/main")
	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "repo_not_found")
}

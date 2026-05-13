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

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/service"
	"github.com/bhpcv252/verge/internal/storage/postgres"
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

// helper

func newMergeTestRouter(svc MergeService) http.Handler {
	return NewRouter(
		nil, // repoHandler
		nil, // branchHandler
		nil, // commitHandler
		NewMergeHandler(svc),
	)
}

// POST /v1/repos/:repoID/merges

func TestCreateMerge_ValidBody_Returns201AndCallsServiceWithCorrectInput(t *testing.T) {
	var capturedInput service.CreateMergeInput

	svc := &mockMergeService{
		createFn: func(_ context.Context, in service.CreateMergeInput) (*domain.Commit, error) {
			capturedInput = in
			return testhelper.FixedMergeCommit(), nil
		},
	}

	w := testhelper.PostJSON(
		t,
		newMergeTestRouter(svc),
		"/v1/repos/repo_xyz/merges",
		map[string]any{
			"parent_ids":           []string{"commit_source", "commit_target"},
			"target_branch":        "main",
			"expected_target_head": "commit_old",
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"message": "Merge feature into main",
			"author":  "alice@example.com",
		},
	)

	require.Equal(t, http.StatusCreated, w.Code)

	var got commitResponse
	testhelper.DecodeBody(t, w, &got)
	assert.Equal(t, "commit_merge_abc", got.ID)
	assert.Equal(t, "repo_xyz", got.RepoID)
	assert.Len(t, got.ParentIDs, 2)
	assert.Contains(t, got.ParentIDs, "commit_source")
	assert.Contains(t, got.ParentIDs, "commit_target")

	assert.Equal(t, "repo_xyz", capturedInput.RepoID)
	assert.Equal(t, []string{"commit_source", "commit_target"}, capturedInput.ParentIDs)
	assert.Equal(t, "main", capturedInput.TargetBranch)
	assert.Equal(t, "commit_old", capturedInput.ExpectedTargetHead)
}

func TestCreateMerge_InvalidJSON_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			called = true
			return nil, nil
		},
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/repos/repo_xyz/merges",
		bytes.NewBufferString("{bad json"),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newMergeTestRouter(svc).ServeHTTP(w, req)

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestCreateMerge_MissingParentIDs_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.PostJSON(
		t,
		newMergeTestRouter(svc),
		"/v1/repos/repo_xyz/merges",
		map[string]any{
			"target_branch":        "main",
			"expected_target_head": "commit_old",
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"message": "Merge",
			"author":  "test@example.com",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestCreateMerge_MissingTargetBranch_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.PostJSON(
		t,
		newMergeTestRouter(svc),
		"/v1/repos/repo_xyz/merges",
		map[string]any{
			"parent_ids":           []string{"commit_1", "commit_2"},
			"expected_target_head": "commit_old",
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"message": "Merge",
			"author":  "test@example.com",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestCreateMerge_MissingExpectedTargetHead_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.PostJSON(
		t,
		newMergeTestRouter(svc),
		"/v1/repos/repo_xyz/merges",
		map[string]any{
			"parent_ids":    []string{"commit_1", "commit_2"},
			"target_branch": "main",
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"message": "Merge",
			"author":  "test@example.com",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestCreateMerge_NotExactlyTwoParents_Returns400WithInvalidRequest(t *testing.T) {
	svc := &mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			return nil, errors.New("service: create merge: exactly two parent_ids required")
		},
	}

	w := testhelper.PostJSON(
		t,
		newMergeTestRouter(svc),
		"/v1/repos/repo_xyz/merges",
		map[string]any{
			"parent_ids":           []string{"commit_1"},
			"target_branch":        "main",
			"expected_target_head": "commit_old",
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"message": "Merge",
			"author":  "test@example.com",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
}

func TestCreateMerge_RepoNotFound_Returns404WithRepoNotFoundCode(t *testing.T) {
	svc := &mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	w := testhelper.PostJSON(
		t,
		newMergeTestRouter(svc),
		"/v1/repos/repo_missing/merges",
		map[string]any{
			"parent_ids":           []string{"commit_1", "commit_2"},
			"target_branch":        "main",
			"expected_target_head": "commit_old",
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"message": "Merge",
			"author":  "test@example.com",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "repo_not_found")
}

func TestCreateMerge_BranchNotFound_Returns404WithBranchNotFoundCode(t *testing.T) {
	svc := &mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			return nil, domain.ErrBranchNotFound
		},
	}

	w := testhelper.PostJSON(
		t,
		newMergeTestRouter(svc),
		"/v1/repos/repo_xyz/merges",
		map[string]any{
			"parent_ids":           []string{"commit_1", "commit_2"},
			"target_branch":        "missing",
			"expected_target_head": "commit_old",
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"message": "Merge",
			"author":  "test@example.com",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "branch_not_found")
}

func TestCreateMerge_InvalidParent_Returns422WithInvalidParentCode(t *testing.T) {
	svc := &mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			return nil, domain.ErrInvalidParent
		},
	}

	w := testhelper.PostJSON(
		t,
		newMergeTestRouter(svc),
		"/v1/repos/repo_xyz/merges",
		map[string]any{
			"parent_ids":           []string{"commit_nonexistent", "commit_2"},
			"target_branch":        "main",
			"expected_target_head": "commit_old",
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"message": "Merge",
			"author":  "test@example.com",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusUnprocessableEntity, "invalid_parent")
}

func TestCreateMerge_StaleMergeTarget_Returns409WithCurrentHead(t *testing.T) {
	svc := &mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			return nil, &postgres.MergeBranchConflictError{CurrentHead: "commit_actual"}
		},
	}

	w := testhelper.PostJSON(
		t,
		newMergeTestRouter(svc),
		"/v1/repos/repo_xyz/merges",
		map[string]any{
			"parent_ids":           []string{"commit_1", "commit_2"},
			"target_branch":        "main",
			"expected_target_head": "commit_stale",
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"message": "Merge",
			"author":  "test@example.com",
		},
	)

	require.Equal(t, http.StatusConflict, w.Code)

	var got struct {
		Error       string  `json:"error"`
		Message     string  `json:"message"`
		CurrentHead *string `json:"current_head"`
	}
	testhelper.DecodeBody(t, w, &got)
	assert.Equal(t, "stale_merge_target", got.Error)
	require.NotNil(t, got.CurrentHead)
	assert.Equal(t, "commit_actual", *got.CurrentHead)
}

func TestCreateMerge_ServiceReturnsUnexpectedError_Returns500(t *testing.T) {
	svc := &mockMergeService{
		createFn: func(_ context.Context, _ service.CreateMergeInput) (*domain.Commit, error) {
			return nil, errors.New("db timeout")
		},
	}

	w := testhelper.PostJSON(
		t,
		newMergeTestRouter(svc),
		"/v1/repos/repo_xyz/merges",
		map[string]any{
			"parent_ids":           []string{"commit_1", "commit_2"},
			"target_branch":        "main",
			"expected_target_head": "commit_old",
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"message": "Merge",
			"author":  "test@example.com",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusInternalServerError, "internal_error")
}

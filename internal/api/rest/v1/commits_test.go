package v1

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/api/core"
	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/observability"
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

// helper

func newCommitTestRouter(svc core.CommitService) http.Handler {
	return NewRouter(
		observability.Noop(),
		nil, // repoHandler
		nil, // branchHandler
		NewCommitHandler(svc),
		nil, // mergeHandler
	)
}

// POST /v1/repos/:repoID/commits

func TestCreateCommit_ValidBody_Returns201AndCallsServiceWithCorrectInput(t *testing.T) {
	var capturedInput service.CreateCommitInput

	svc := &mockCommitService{
		createFn: func(_ context.Context, in service.CreateCommitInput) (*service.CreateCommitResult, error) {
			capturedInput = in
			return &service.CreateCommitResult{
				Commit:   testhelper.FixedCommit(),
				Existing: false,
			}, nil
		},
	}

	w := testhelper.PostJSON(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits",
		map[string]any{
			"parent_ids": []string{"commit_parent"},
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"message": "Add feature",
			"author":  "alice@example.com",
		},
	)

	require.Equal(t, http.StatusCreated, w.Code)

	var got createCommitResponse
	testhelper.DecodeBody(t, w, &got)
	assert.Equal(t, "commit_abc123", got.ID)
	assert.Equal(t, "repo_xyz789", got.RepoID)
	assert.Equal(t, []string{"commit_parent"}, got.ParentIDs)
	assert.Equal(t, "Add feature", got.Message)
	assert.Equal(t, "alice@example.com", got.Author)

	assert.Equal(t, "repo_xyz", capturedInput.RepoID)
	assert.Equal(t, []string{"commit_parent"}, capturedInput.ParentIDs)
	assert.Equal(t, "db", capturedInput.DataPointer.Type)
	assert.Equal(t, "test/fixture", capturedInput.DataPointer.Location)
	assert.NotEmpty(t, got.Timestamp)
	assert.False(t, got.Existing)
}

func TestCreateCommit_IdempotencyKeyMatch_Returns200WithExistingTrue(t *testing.T) {
	svc := &mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			return &service.CreateCommitResult{
				Commit:   testhelper.FixedCommit(),
				Existing: true,
			}, nil
		},
	}

	w := testhelper.PostJSON(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits",
		map[string]any{
			"idempotency_key": "key_123",
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"message": "Test",
			"author":  "test@example.com",
		},
	)

	require.Equal(t, http.StatusOK, w.Code)

	var got createCommitResponse
	testhelper.DecodeBody(t, w, &got)
	assert.Equal(t, "commit_abc123", got.ID)
	assert.True(t, got.Existing)
}

func TestCreateCommit_InvalidJSON_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			called = true
			return nil, nil
		},
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/repos/repo_xyz/commits",
		bytes.NewBufferString("{bad json"),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newCommitTestRouter(svc).ServeHTTP(w, req)

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestCreateCommit_MissingDataPointer_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.PostJSON(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits",
		map[string]any{
			"message": "Test",
			"author":  "test@example.com",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestCreateCommit_MissingMessage_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.PostJSON(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits",
		map[string]any{
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"author": "test@example.com",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestCreateCommit_MissingAuthor_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.PostJSON(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits",
		map[string]any{
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"message": "Test",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestCreateCommit_RepoNotFound_Returns404WithRepoNotFoundCode(t *testing.T) {
	svc := &mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	w := testhelper.PostJSON(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_missing/commits",
		map[string]any{
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"message": "Test",
			"author":  "test@example.com",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "repo_not_found")
}

func TestCreateCommit_InvalidParent_Returns422WithInvalidParentCode(t *testing.T) {
	svc := &mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			return nil, domain.ErrInvalidParent
		},
	}

	w := testhelper.PostJSON(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits",
		map[string]any{
			"parent_ids": []string{"commit_nonexistent"},
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"message": "Test",
			"author":  "test@example.com",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusUnprocessableEntity, "invalid_parent")
}

func TestCreateCommit_ServiceReturnsUnexpectedError_Returns500(t *testing.T) {
	svc := &mockCommitService{
		createFn: func(_ context.Context, _ service.CreateCommitInput) (*service.CreateCommitResult, error) {
			return nil, errors.New("db timeout")
		},
	}

	w := testhelper.PostJSON(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits",
		map[string]any{
			"data_pointer": map[string]string{
				"type":     "db",
				"location": "test/fixture",
			},
			"message": "Test",
			"author":  "test@example.com",
		},
	)

	testhelper.AssertErrorCode(t, w, http.StatusInternalServerError, "internal_error")
}

// GET /v1/repos/:repoID/commits/:commitID

func TestGetCommit_Exists_Returns200WithCorrectFields(t *testing.T) {
	svc := &mockCommitService{
		getFn: func(_ context.Context, repoID, commitID string) (*domain.Commit, error) {
			assert.Equal(t, "repo_xyz", repoID)
			assert.Equal(t, "commit_abc", commitID)
			return testhelper.FixedCommit(), nil
		},
	}

	w := testhelper.GetPath(t, newCommitTestRouter(svc), "/v1/repos/repo_xyz/commits/commit_abc")

	require.Equal(t, http.StatusOK, w.Code)
	var got commitResponse
	testhelper.DecodeBody(t, w, &got)
	assert.Equal(t, "commit_abc123", got.ID)
	assert.Equal(t, "repo_xyz789", got.RepoID)
	assert.Equal(t, []string{"commit_parent"}, got.ParentIDs)
}

func TestGetCommit_NotFound_Returns404WithCommitNotFoundCode(t *testing.T) {
	svc := &mockCommitService{
		getFn: func(_ context.Context, _, _ string) (*domain.Commit, error) {
			return nil, domain.ErrCommitNotFound
		},
	}

	w := testhelper.GetPath(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits/commit_missing",
	)
	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "commit_not_found")
}

func TestGetCommit_RepoNotFound_Returns404WithRepoNotFoundCode(t *testing.T) {
	svc := &mockCommitService{
		getFn: func(_ context.Context, _, _ string) (*domain.Commit, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	w := testhelper.GetPath(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_missing/commits/commit_abc",
	)
	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "repo_not_found")
}

// GET /v1/repos/:repoID/commits

func TestListCommits_NoParams_Returns200AndPassesDefaultLimitToService(t *testing.T) {
	svc := &mockCommitService{
		listFn: func(_ context.Context, in service.ListCommitsInput) (*service.ListCommitsResult, error) {
			assert.Equal(t, 20, in.Limit)
			assert.Equal(t, "", in.Cursor)
			return &service.ListCommitsResult{
				Commits: []*domain.Commit{testhelper.FixedCommit()},
			}, nil
		},
	}

	w := testhelper.GetPath(t, newCommitTestRouter(svc), "/v1/repos/repo_xyz/commits")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestListCommits_ExplicitValidLimit_PassedToService(t *testing.T) {
	svc := &mockCommitService{
		listFn: func(_ context.Context, in service.ListCommitsInput) (*service.ListCommitsResult, error) {
			assert.Equal(t, 50, in.Limit)
			return &service.ListCommitsResult{}, nil
		},
	}

	w := testhelper.GetPath(t, newCommitTestRouter(svc), "/v1/repos/repo_xyz/commits?limit=50")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestListCommits_InvalidLimitParam_Returns400BeforeServiceIsCalled(t *testing.T) {
	called := false
	svc := &mockCommitService{
		listFn: func(_ context.Context, _ service.ListCommitsInput) (*service.ListCommitsResult, error) {
			called = true
			return nil, nil
		},
	}

	w := testhelper.GetPath(t, newCommitTestRouter(svc), "/v1/repos/repo_xyz/commits?limit=abc")
	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
	assert.False(t, called)
}

func TestListCommits_LimitOutOfRange_Returns400(t *testing.T) {
	cases := []struct{ name, limit string }{
		{"zero", "0"},
		{"over_max", "101"},
		{"negative", "-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			svc := &mockCommitService{
				listFn: func(_ context.Context, _ service.ListCommitsInput) (*service.ListCommitsResult, error) {
					called = true
					return nil, nil
				},
			}
			w := testhelper.GetPath(
				t,
				newCommitTestRouter(svc),
				"/v1/repos/repo_xyz/commits?limit="+tc.limit,
			)
			testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
			assert.False(t, called)
		})
	}
}

func TestListCommits_BranchParam_PassedToService(t *testing.T) {
	svc := &mockCommitService{
		listFn: func(_ context.Context, in service.ListCommitsInput) (*service.ListCommitsResult, error) {
			assert.Equal(t, "main", in.Branch)
			return &service.ListCommitsResult{}, nil
		},
	}

	w := testhelper.GetPath(t, newCommitTestRouter(svc), "/v1/repos/repo_xyz/commits?branch=main")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestListCommits_AuthorParam_PassedToService(t *testing.T) {
	svc := &mockCommitService{
		listFn: func(_ context.Context, in service.ListCommitsInput) (*service.ListCommitsResult, error) {
			assert.Equal(t, "alice@example.com", in.Author)
			return &service.ListCommitsResult{}, nil
		},
	}

	w := testhelper.GetPath(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits?author=alice@example.com",
	)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestListCommits_SinceParam_PassedToService(t *testing.T) {
	svc := &mockCommitService{
		listFn: func(_ context.Context, in service.ListCommitsInput) (*service.ListCommitsResult, error) {
			assert.Equal(t, "2024-01-01T00:00:00Z", in.Since)
			return &service.ListCommitsResult{}, nil
		},
	}

	w := testhelper.GetPath(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits?since=2024-01-01T00:00:00Z",
	)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestListCommits_UntilParam_PassedToService(t *testing.T) {
	svc := &mockCommitService{
		listFn: func(_ context.Context, in service.ListCommitsInput) (*service.ListCommitsResult, error) {
			assert.Equal(t, "2024-12-31T23:59:59Z", in.Until)
			return &service.ListCommitsResult{}, nil
		},
	}

	w := testhelper.GetPath(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits?until=2024-12-31T23:59:59Z",
	)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestListCommits_TraversalParam_PassedToService(t *testing.T) {
	svc := &mockCommitService{
		listFn: func(_ context.Context, in service.ListCommitsInput) (*service.ListCommitsResult, error) {
			assert.Equal(t, "dag", in.Traversal)
			return &service.ListCommitsResult{}, nil
		},
	}

	w := testhelper.GetPath(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits?traversal=dag&branch=main",
	)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestListCommits_TraversalFlat_Returns200InReverseChronological(t *testing.T) {
	svc := &mockCommitService{
		listFn: func(_ context.Context, in service.ListCommitsInput) (*service.ListCommitsResult, error) {
			assert.Equal(t, "flat", in.Traversal)
			// newest first
			return &service.ListCommitsResult{
				Commits: []*domain.Commit{
					{
						ID:        "commit_3",
						Timestamp: time.Date(2024, 4, 5, 12, 0, 0, 0, time.UTC),
					},
					{
						ID:        "commit_2",
						Timestamp: time.Date(2024, 4, 5, 11, 0, 0, 0, time.UTC),
					},
					{
						ID:        "commit_1",
						Timestamp: time.Date(2024, 4, 5, 10, 0, 0, 0, time.UTC),
					},
				},
			}, nil
		},
	}

	w := testhelper.GetPath(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits?traversal=flat",
	)
	require.Equal(t, http.StatusOK, w.Code)

	var got listCommitsResponse
	testhelper.DecodeBody(t, w, &got)
	require.Len(t, got.Commits, 3)
	// verify reverse chronological order
	assert.Equal(t, "commit_3", got.Commits[0].ID)
	assert.Equal(t, "commit_2", got.Commits[1].ID)
	assert.Equal(t, "commit_1", got.Commits[2].ID)
}

func TestListCommits_TraversalDAGWithBranch_Returns200FollowsParents(t *testing.T) {
	svc := &mockCommitService{
		listFn: func(_ context.Context, in service.ListCommitsInput) (*service.ListCommitsResult, error) {
			assert.Equal(t, "dag", in.Traversal)
			assert.Equal(t, "main", in.Branch)

			return &service.ListCommitsResult{
				Commits: []*domain.Commit{testhelper.FixedCommit()},
			}, nil
		},
	}

	w := testhelper.GetPath(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits?traversal=dag&branch=main",
	)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestListCommits_TraversalDAGWithoutBranch_Returns400(t *testing.T) {
	svc := &mockCommitService{
		listFn: func(_ context.Context, in service.ListCommitsInput) (*service.ListCommitsResult, error) {
			return nil, &service.ValidationError{Msg: "traversal=dag requires a 'branch' parameter"}
		},
	}

	w := testhelper.GetPath(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits?traversal=dag",
	)
	testhelper.AssertErrorCode(t, w, http.StatusBadRequest, "invalid_request")
}

func TestListCommits_WithNextCursor_IncludedInResponse(t *testing.T) {
	svc := &mockCommitService{
		listFn: func(_ context.Context, _ service.ListCommitsInput) (*service.ListCommitsResult, error) {
			return &service.ListCommitsResult{
				Commits:    []*domain.Commit{testhelper.FixedCommit()},
				NextCursor: "next-page-abc",
			}, nil
		},
	}

	w := testhelper.GetPath(t, newCommitTestRouter(svc), "/v1/repos/repo_xyz/commits")
	require.Equal(t, http.StatusOK, w.Code)

	var got listCommitsResponse
	testhelper.DecodeBody(t, w, &got)
	require.NotEmpty(t, got.NextCursor)
	assert.Equal(t, "next-page-abc", got.NextCursor)
}

func TestListCommits_NoNextCursor_OmittedFromResponse(t *testing.T) {
	svc := &mockCommitService{
		listFn: func(_ context.Context, _ service.ListCommitsInput) (*service.ListCommitsResult, error) {
			return &service.ListCommitsResult{
				Commits:    []*domain.Commit{testhelper.FixedCommit()},
				NextCursor: "",
			}, nil
		},
	}

	w := testhelper.GetPath(t, newCommitTestRouter(svc), "/v1/repos/repo_xyz/commits")
	require.Equal(t, http.StatusOK, w.Code)

	var got listCommitsResponse
	testhelper.DecodeBody(t, w, &got)
	assert.Empty(t, got.NextCursor)
}

func TestListCommits_RepoNotFound_Returns404(t *testing.T) {
	svc := &mockCommitService{
		listFn: func(_ context.Context, _ service.ListCommitsInput) (*service.ListCommitsResult, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	w := testhelper.GetPath(t, newCommitTestRouter(svc), "/v1/repos/repo_missing/commits")
	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "repo_not_found")
}

// GET /v1/repos/:repoID/commits/:commitID/parents

func TestGetParents_Exists_Returns200WithParents(t *testing.T) {
	parent1 := &domain.Commit{
		ID:      "commit_parent1",
		RepoID:  "repo_xyz",
		Message: "Parent 1",
		Author:  "test@example.com",
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Timestamp: time.Now().UTC(),
	}
	parent2 := &domain.Commit{
		ID:      "commit_parent2",
		RepoID:  "repo_xyz",
		Message: "Parent 2",
		Author:  "test@example.com",
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Timestamp: time.Now().UTC(),
	}

	svc := &mockCommitService{
		getParentsFn: func(_ context.Context, repoID, commitID string) ([]*domain.Commit, error) {
			assert.Equal(t, "repo_xyz", repoID)
			assert.Equal(t, "commit_merge", commitID)
			return []*domain.Commit{parent1, parent2}, nil
		},
	}

	w := testhelper.GetPath(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits/commit_merge/parents",
	)

	require.Equal(t, http.StatusOK, w.Code)
	var got getParentsResponse
	testhelper.DecodeBody(t, w, &got)
	require.Len(t, got.Parents, 2)
	assert.Equal(t, "commit_parent1", got.Parents[0].ID)
	assert.Equal(t, "commit_parent2", got.Parents[1].ID)
}

func TestGetParents_RootCommit_ReturnsEmptyArray(t *testing.T) {
	svc := &mockCommitService{
		getParentsFn: func(_ context.Context, _, _ string) ([]*domain.Commit, error) {
			return []*domain.Commit{}, nil
		},
	}

	w := testhelper.GetPath(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits/commit_root/parents",
	)

	require.Equal(t, http.StatusOK, w.Code)
	var got getParentsResponse
	testhelper.DecodeBody(t, w, &got)
	assert.Len(t, got.Parents, 0)
}

func TestGetParents_RegularCommit_ReturnsOneParent(t *testing.T) {
	parent := &domain.Commit{
		ID:      "commit_parent",
		RepoID:  "repo_xyz",
		Message: "Parent commit",
		Author:  "test@example.com",
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Timestamp: time.Now().UTC(),
	}

	svc := &mockCommitService{
		getParentsFn: func(_ context.Context, repoID, commitID string) ([]*domain.Commit, error) {
			assert.Equal(t, "repo_xyz", repoID)
			assert.Equal(t, "commit_regular", commitID)
			return []*domain.Commit{parent}, nil
		},
	}

	w := testhelper.GetPath(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits/commit_regular/parents",
	)

	require.Equal(t, http.StatusOK, w.Code)
	var got getParentsResponse
	testhelper.DecodeBody(t, w, &got)
	require.Len(t, got.Parents, 1, "regular commit should have exactly one parent")
	assert.Equal(t, "commit_parent", got.Parents[0].ID)
}

func TestGetParents_CommitNotFound_Returns404(t *testing.T) {
	svc := &mockCommitService{
		getParentsFn: func(_ context.Context, _, _ string) ([]*domain.Commit, error) {
			return nil, domain.ErrCommitNotFound
		},
	}

	w := testhelper.GetPath(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_xyz/commits/commit_missing/parents",
	)
	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "commit_not_found")
}

func TestGetParents_RepoNotFound_Returns404(t *testing.T) {
	svc := &mockCommitService{
		getParentsFn: func(_ context.Context, _, _ string) ([]*domain.Commit, error) {
			return nil, domain.ErrRepoNotFound
		},
	}

	w := testhelper.GetPath(
		t,
		newCommitTestRouter(svc),
		"/v1/repos/repo_missing/commits/commit_abc/parents",
	)
	testhelper.AssertErrorCode(t, w, http.StatusNotFound, "repo_not_found")
}

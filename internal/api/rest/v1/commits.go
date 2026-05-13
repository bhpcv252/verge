package v1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/service"
)

type CommitService interface {
	CreateCommit(
		ctx context.Context,
		in service.CreateCommitInput,
	) (*service.CreateCommitResult, error)
	GetCommit(ctx context.Context, repoID, commitID string) (*domain.Commit, error)
	ListCommits(
		ctx context.Context,
		in service.ListCommitsInput,
	) (*service.ListCommitsResult, error)
	GetParents(ctx context.Context, repoID, commitID string) ([]*domain.Commit, error)
}

type CommitHandler struct {
	svc CommitService
}

func NewCommitHandler(svc CommitService) *CommitHandler {
	return &CommitHandler{svc: svc}
}

func (h *CommitHandler) Mount(r chi.Router) {
	r.Post("/repos/{repoID}/commits", h.CreateCommit)
	r.Get("/repos/{repoID}/commits", h.ListCommits)
	r.Get("/repos/{repoID}/commits/{commitID}", h.GetCommit)
	r.Get("/repos/{repoID}/commits/{commitID}/parents", h.GetParents)
}

type createCommitRequest struct {
	ParentIDs      []string           `json:"parent_ids"`
	ExpectedHead   string             `json:"expected_head,omitempty"`
	DataPointer    domain.DataPointer `json:"data_pointer"`
	Message        string             `json:"message"`
	Author         string             `json:"author"`
	IdempotencyKey string             `json:"idempotency_key,omitempty"`
}

type commitResponse struct {
	ID          string             `json:"id"`
	RepoID      string             `json:"repo_id"`
	ParentIDs   []string           `json:"parent_ids"`
	DataPointer domain.DataPointer `json:"data_pointer"`
	Message     string             `json:"message"`
	Author      string             `json:"author"`
	Timestamp   time.Time          `json:"timestamp"`
}

type createCommitResponse struct {
	commitResponse
	Existing bool `json:"existing,omitempty"` // true if idempotency_key matched
}

type listCommitsResponse struct {
	Commits    []commitResponse `json:"commits"`
	NextCursor *string          `json:"next_cursor"` // null when there is no next page
}

type getParentsResponse struct {
	CommitID string           `json:"commit_id"`
	Parents  []commitResponse `json:"parents"`
}

func toCommitResponse(c *domain.Commit) commitResponse {
	return commitResponse{
		ID:          c.ID,
		RepoID:      c.RepoID,
		ParentIDs:   c.ParentIDs,
		DataPointer: c.DataPointer,
		Message:     c.Message,
		Author:      c.Author,
		Timestamp:   c.Timestamp,
	}
}

func (h *CommitHandler) CreateCommit(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")

	var req createCommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "Request body must be valid JSON.")
		return
	}

	req.Message = strings.TrimSpace(req.Message)
	req.Author = strings.TrimSpace(req.Author)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.ExpectedHead = strings.TrimSpace(req.ExpectedHead)

	if req.Message == "" {
		badRequest(w, "'message' is required and must not be empty.")
		return
	}
	if req.Author == "" {
		badRequest(w, "'author' is required and must not be empty.")
		return
	}
	// DataPointer presence check
	if req.DataPointer.Type == "" || req.DataPointer.Location == "" {
		badRequest(w, "'data_pointer' with 'type' and 'location' is required.")
		return
	}

	// detailed DataPointer validation happens in service layer
	result, err := h.svc.CreateCommit(r.Context(), service.CreateCommitInput{
		RepoID:         repoID,
		ParentIDs:      req.ParentIDs,
		ExpectedHead:   req.ExpectedHead,
		DataPointer:    req.DataPointer,
		Message:        req.Message,
		Author:         req.Author,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		if errors.Is(err, domain.ErrRepoNotFound) {
			notFound(w, "repo_not_found",
				fmt.Sprintf("Repository %q does not exist.", repoID))
			return
		}
		if errors.Is(err, domain.ErrInvalidParent) {
			unprocessableEntity(w, "invalid_parent",
				"One or more parent_ids do not exist in this repository.")
			return
		}
		// check for validation errors from DataPointer or parent_ids count
		errMsg := err.Error()
		if strings.Contains(errMsg, "data_pointer") || strings.Contains(errMsg, "parent_ids") ||
			strings.Contains(errMsg, "merge commits") {
			badRequest(w, err.Error())
			return
		}
		internalError(w)
		return
	}

	// if existing commit was returned due to idempotency, return 200
	if result.Existing {
		resp := createCommitResponse{
			commitResponse: toCommitResponse(result.Commit),
			Existing:       true,
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// new commit created, return 201
	resp := createCommitResponse{
		commitResponse: toCommitResponse(result.Commit),
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *CommitHandler) ListCommits(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	q := r.URL.Query()

	limit := 20
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			badRequest(w, "'limit' must be an integer between 1 and 100.")
			return
		}
		limit = n
	}

	result, err := h.svc.ListCommits(r.Context(), service.ListCommitsInput{
		RepoID:    repoID,
		Branch:    q.Get("branch"),
		Author:    q.Get("author"),
		Since:     q.Get("since"),
		Until:     q.Get("until"),
		Traversal: q.Get("traversal"),
		Limit:     limit,
		Cursor:    q.Get("cursor"),
	})
	if err != nil {
		if errors.Is(err, domain.ErrRepoNotFound) {
			notFound(w, "repo_not_found",
				fmt.Sprintf("Repository %q does not exist.", repoID))
			return
		}
		// check for validation errors
		errMsg := err.Error()
		if strings.Contains(errMsg, "timestamp") || strings.Contains(errMsg, "traversal") {
			badRequest(w, err.Error())
			return
		}
		internalError(w)
		return
	}

	resp := listCommitsResponse{
		Commits: make([]commitResponse, 0, len(result.Commits)),
	}
	for _, commit := range result.Commits {
		resp.Commits = append(resp.Commits, toCommitResponse(commit))
	}
	if result.NextCursor != "" {
		resp.NextCursor = &result.NextCursor
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *CommitHandler) GetCommit(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	commitID := chi.URLParam(r, "commitID")

	commit, err := h.svc.GetCommit(r.Context(), repoID, commitID)
	if err != nil {
		if errors.Is(err, domain.ErrRepoNotFound) {
			notFound(w, "repo_not_found",
				fmt.Sprintf("Repository %q does not exist.", repoID))
			return
		}
		if errors.Is(err, domain.ErrCommitNotFound) {
			notFound(w, "commit_not_found",
				fmt.Sprintf("Commit %q does not exist in repository %q.", commitID, repoID))
			return
		}
		internalError(w)
		return
	}

	writeJSON(w, http.StatusOK, toCommitResponse(commit))
}

func (h *CommitHandler) GetParents(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	commitID := chi.URLParam(r, "commitID")

	parents, err := h.svc.GetParents(r.Context(), repoID, commitID)
	if err != nil {
		if errors.Is(err, domain.ErrRepoNotFound) {
			notFound(w, "repo_not_found",
				fmt.Sprintf("Repository %q does not exist.", repoID))
			return
		}
		if errors.Is(err, domain.ErrCommitNotFound) {
			notFound(w, "commit_not_found",
				fmt.Sprintf("Commit %q does not exist in repository %q.", commitID, repoID))
			return
		}
		internalError(w)
		return
	}

	resp := getParentsResponse{
		CommitID: commitID,
		Parents:  make([]commitResponse, 0, len(parents)),
	}
	for _, parent := range parents {
		resp.Parents = append(resp.Parents, toCommitResponse(parent))
	}

	writeJSON(w, http.StatusOK, resp)
}

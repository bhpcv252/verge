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
	"github.com/bhpcv252/verge/internal/storage/postgres"
)

type BranchService interface {
	CreateBranch(ctx context.Context, in service.CreateBranchInput) (*domain.Branch, error)
	GetBranch(ctx context.Context, repoID, name string) (*domain.Branch, error)
	ListBranches(
		ctx context.Context,
		in service.ListBranchesInput,
	) (*service.ListBranchesResult, error)
	AdvanceBranch(ctx context.Context, in service.AdvanceBranchInput) (*domain.Branch, error)
	DeleteBranch(ctx context.Context, repoID, name string) error
}

type BranchHandler struct {
	svc BranchService
}

func NewBranchHandler(svc BranchService) *BranchHandler {
	return &BranchHandler{svc: svc}
}

func (h *BranchHandler) Mount(r chi.Router) {
	r.Post("/repos/{repoID}/branches", h.CreateBranch)
	r.Get("/repos/{repoID}/branches", h.ListBranches)
	r.Get("/repos/{repoID}/branches/{name}", h.GetBranch)
	r.Patch("/repos/{repoID}/branches/{name}", h.AdvanceBranch)
	r.Delete("/repos/{repoID}/branches/{name}", h.DeleteBranch)
}

type createBranchRequest struct {
	Name           string `json:"name"`
	SourceCommitID string `json:"source_commit_id"`
}

type advanceBranchRequest struct {
	CommitID         string `json:"commit_id"`
	ExpectedCommitID string `json:"expected_commit_id"`
}

type branchResponse struct {
	Name      string    `json:"name"`
	RepoID    string    `json:"repo_id"`
	CommitID  string    `json:"commit_id"`
	CreatedAt time.Time `json:"created_at"`
}

type listBranchesResponse struct {
	Branches   []branchResponse `json:"branches"`
	NextCursor *string          `json:"next_cursor"` // null when there is no next page
}

func toBranchResponse(b *domain.Branch) branchResponse {
	return branchResponse{
		Name:      b.Name,
		RepoID:    b.RepoID,
		CommitID:  b.CommitID,
		CreatedAt: b.CreatedAt,
	}
}

func (h *BranchHandler) CreateBranch(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")

	var req createBranchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "Request body must be valid JSON.")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.SourceCommitID = strings.TrimSpace(req.SourceCommitID)

	if req.Name == "" {
		badRequest(w, "'name' is required and must not be empty.")
		return
	}
	if req.SourceCommitID == "" {
		badRequest(w, "'source_commit_id' is required and must not be empty.")
		return
	}

	branch, err := h.svc.CreateBranch(r.Context(), service.CreateBranchInput{
		RepoID:         repoID,
		Name:           req.Name,
		SourceCommitID: req.SourceCommitID,
	})
	if err != nil {
		if errors.Is(err, domain.ErrRepoNotFound) {
			notFound(w, "repo_not_found",
				fmt.Sprintf("Repository %q does not exist.", repoID))
			return
		}
		if errors.Is(err, domain.ErrBranchAlreadyExists) {
			conflict(w, "branch_already_exists",
				fmt.Sprintf("Branch %q already exists in repository %q.", req.Name, repoID),
				nil)
			return
		}
		if errors.Is(err, domain.ErrCommitNotFound) {
			notFound(
				w,
				"commit_not_found",
				fmt.Sprintf(
					"Commit %q does not exist in repository %q.",
					req.SourceCommitID,
					repoID,
				),
			)
			return
		}
		internalError(w)
		return
	}

	writeJSON(w, http.StatusCreated, toBranchResponse(branch))
}

func (h *BranchHandler) ListBranches(w http.ResponseWriter, r *http.Request) {
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

	result, err := h.svc.ListBranches(r.Context(), service.ListBranchesInput{
		RepoID: repoID,
		Limit:  limit,
		Cursor: q.Get("cursor"),
	})
	if err != nil {
		if errors.Is(err, domain.ErrRepoNotFound) {
			notFound(w, "repo_not_found",
				fmt.Sprintf("Repository %q does not exist.", repoID))
			return
		}
		internalError(w)
		return
	}

	resp := listBranchesResponse{
		Branches: make([]branchResponse, 0, len(result.Branches)),
	}
	for _, branch := range result.Branches {
		resp.Branches = append(resp.Branches, toBranchResponse(branch))
	}
	if result.NextCursor != "" {
		resp.NextCursor = &result.NextCursor
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *BranchHandler) GetBranch(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	name := chi.URLParam(r, "name")

	branch, err := h.svc.GetBranch(r.Context(), repoID, name)
	if err != nil {
		if errors.Is(err, domain.ErrBranchNotFound) {
			notFound(w, "branch_not_found",
				fmt.Sprintf("Branch %q does not exist in repository %q.", name, repoID))
			return
		}
		if errors.Is(err, domain.ErrRepoNotFound) {
			notFound(w, "repo_not_found",
				fmt.Sprintf("Repository %q does not exist.", repoID))
			return
		}
		internalError(w)
		return
	}

	writeJSON(w, http.StatusOK, toBranchResponse(branch))
}

func (h *BranchHandler) AdvanceBranch(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	name := chi.URLParam(r, "name")

	var req advanceBranchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "Request body must be valid JSON.")
		return
	}

	req.CommitID = strings.TrimSpace(req.CommitID)
	req.ExpectedCommitID = strings.TrimSpace(req.ExpectedCommitID)

	if req.CommitID == "" {
		badRequest(w, "'commit_id' is required and must not be empty.")
		return
	}
	if req.ExpectedCommitID == "" {
		badRequest(w, "'expected_commit_id' is required and must not be empty.")
		return
	}

	branch, err := h.svc.AdvanceBranch(r.Context(), service.AdvanceBranchInput{
		RepoID:           repoID,
		Name:             name,
		CommitID:         req.CommitID,
		ExpectedCommitID: req.ExpectedCommitID,
	})
	if err != nil {
		if errors.Is(err, domain.ErrRepoNotFound) {
			notFound(w, "repo_not_found",
				fmt.Sprintf("Repository %q does not exist.", repoID))
			return
		}
		if errors.Is(err, domain.ErrBranchNotFound) {
			notFound(w, "branch_not_found",
				fmt.Sprintf("Branch %q does not exist in repository %q.", name, repoID))
			return
		}
		if errors.Is(err, domain.ErrCommitNotFound) {
			notFound(w, "commit_not_found",
				fmt.Sprintf("Commit %q does not exist in repository %q.", req.CommitID, repoID))
			return
		}
		// check for branch conflict with current head
		var conflictErr *postgres.BranchConflictError
		if errors.As(err, &conflictErr) {
			conflict(
				w,
				"branch_conflict",
				fmt.Sprintf(
					"Branch %q has advanced. Current head is %q but expected %q. Fetch latest head and retry.",
					name,
					conflictErr.CurrentHead,
					req.ExpectedCommitID,
				),
				&conflictErr.CurrentHead,
			)
			return
		}
		internalError(w)
		return
	}

	writeJSON(w, http.StatusOK, toBranchResponse(branch))
}

func (h *BranchHandler) DeleteBranch(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	name := chi.URLParam(r, "name")

	err := h.svc.DeleteBranch(r.Context(), repoID, name)
	if err != nil {
		if errors.Is(err, domain.ErrRepoNotFound) {
			notFound(w, "repo_not_found",
				fmt.Sprintf("Repository %q does not exist.", repoID))
			return
		}
		if errors.Is(err, domain.ErrBranchNotFound) {
			notFound(w, "branch_not_found",
				fmt.Sprintf("Branch %q does not exist in repository %q.", name, repoID))
			return
		}
		if errors.Is(err, domain.ErrCannotDeleteDefaultBranch) {
			conflict(
				w,
				"cannot_delete_default_branch",
				fmt.Sprintf(
					"Cannot delete the default branch %q. Set a different default branch first.",
					name,
				),
				nil,
			)
			return
		}
		internalError(w)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

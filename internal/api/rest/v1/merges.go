package v1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/service"
	"github.com/bhpcv252/verge/internal/storage/postgres"
)

type MergeService interface {
	CreateMerge(ctx context.Context, in service.CreateMergeInput) (*domain.Commit, error)
}

type MergeHandler struct {
	svc MergeService
}

func NewMergeHandler(svc MergeService) *MergeHandler {
	return &MergeHandler{svc: svc}
}

func (h *MergeHandler) Mount(r chi.Router) {
	r.Post("/repos/{repoID}/merges", h.CreateMerge)
}

type createMergeRequest struct {
	ParentIDs          []string           `json:"parent_ids"`
	ExpectedTargetHead string             `json:"expected_target_head"`
	TargetBranch       string             `json:"target_branch"`
	DataPointer        domain.DataPointer `json:"data_pointer"`
	Message            string             `json:"message"`
	Author             string             `json:"author"`
}

func (h *MergeHandler) CreateMerge(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")

	var req createMergeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "Request body must be valid JSON.")
		return
	}

	req.Message = strings.TrimSpace(req.Message)
	req.Author = strings.TrimSpace(req.Author)
	req.ExpectedTargetHead = strings.TrimSpace(req.ExpectedTargetHead)
	req.TargetBranch = strings.TrimSpace(req.TargetBranch)

	if req.Message == "" {
		badRequest(w, "'message' is required and must not be empty.")
		return
	}
	if req.Author == "" {
		badRequest(w, "'author' is required and must not be empty.")
		return
	}
	if req.ExpectedTargetHead == "" {
		badRequest(w, "'expected_target_head' is required and must not be empty.")
		return
	}
	if req.TargetBranch == "" {
		badRequest(w, "'target_branch' is required and must not be empty.")
		return
	}
	if len(req.ParentIDs) != 2 {
		badRequest(w, "Merge commits require exactly two parent_ids.")
		return
	}

	mergeCommit, err := h.svc.CreateMerge(r.Context(), service.CreateMergeInput{
		RepoID:             repoID,
		ParentIDs:          req.ParentIDs,
		ExpectedTargetHead: req.ExpectedTargetHead,
		TargetBranch:       req.TargetBranch,
		DataPointer:        req.DataPointer,
		Message:            req.Message,
		Author:             req.Author,
	})
	if err != nil {
		if errors.Is(err, domain.ErrRepoNotFound) {
			notFound(w, "repo_not_found",
				fmt.Sprintf("Repository %q does not exist.", repoID))
			return
		}
		if errors.Is(err, domain.ErrBranchNotFound) {
			notFound(w, "branch_not_found",
				fmt.Sprintf("Branch %q does not exist in repository %q.", req.TargetBranch, repoID))
			return
		}
		if errors.Is(err, domain.ErrInvalidParent) {
			unprocessableEntity(w, "invalid_parent",
				"One or more parent_ids do not exist in this repository.")
			return
		}

		// check for stale merge target
		var staleMergeErr *service.StaleMergeTargetError
		if errors.As(err, &staleMergeErr) {
			conflict(
				w,
				"stale_merge_target",
				fmt.Sprintf(
					"Branch %q has moved. Current head is %q but expected %q. Fetch latest heads and retry merge.",
					staleMergeErr.BranchName,
					staleMergeErr.CurrentHead,
					staleMergeErr.ExpectedHead,
				),
				&staleMergeErr.CurrentHead,
			)
			return
		}

		// check for merge branch conflict
		var mergeBranchConflictErr *postgres.MergeBranchConflictError
		if errors.As(err, &mergeBranchConflictErr) {
			conflict(
				w,
				"stale_merge_target",
				fmt.Sprintf(
					"Branch %q has moved during merge. Current head is %q but expected %q. Fetch latest heads and retry merge.",
					mergeBranchConflictErr.BranchName,
					mergeBranchConflictErr.CurrentHead,
					mergeBranchConflictErr.ExpectedHead,
				),
				&mergeBranchConflictErr.CurrentHead,
			)
			return
		}

		// check for validation errors from DataPointer or parent_ids count
		errMsg := err.Error()
		if strings.Contains(errMsg, "data_pointer") || strings.Contains(errMsg, "parent_ids") ||
			strings.Contains(errMsg, "exactly two") {
			badRequest(w, err.Error())
			return
		}

		internalError(w)
		return
	}

	writeJSON(w, http.StatusCreated, toCommitResponse(mergeCommit))
}

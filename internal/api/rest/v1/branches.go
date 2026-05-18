package v1

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/bhpcv252/verge/internal/api/core"
	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/service"
)

type BranchHandler struct {
	svc core.BranchService
}

func NewBranchHandler(svc core.BranchService) *BranchHandler {
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
	Name      string `json:"name"`
	RepoID    string `json:"repo_id"`
	CommitID  string `json:"commit_id"`
	CreatedAt string `json:"created_at"`
}

type listBranchesResponse struct {
	Branches   []branchResponse `json:"branches"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

func (h *BranchHandler) CreateBranch(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")

	var req createBranchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "request body must be valid JSON")
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
		writeAppError(w, core.MapDomainError(err))
		return
	}

	writeJSON(w, http.StatusCreated, toBranchResponse(branch))
}

func (h *BranchHandler) GetBranch(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	name := chi.URLParam(r, "name")

	branch, err := h.svc.GetBranch(r.Context(), repoID, name)
	if err != nil {
		writeAppError(w, core.MapDomainError(err))
		return
	}

	writeJSON(w, http.StatusOK, toBranchResponse(branch))
}

func (h *BranchHandler) ListBranches(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")

	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
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
		Cursor: r.URL.Query().Get("cursor"),
	})
	if err != nil {
		writeAppError(w, core.MapDomainError(err))
		return
	}

	resp := listBranchesResponse{
		Branches:   make([]branchResponse, 0, len(result.Branches)),
		NextCursor: result.NextCursor,
	}
	for _, b := range result.Branches {
		resp.Branches = append(resp.Branches, toBranchResponse(b))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *BranchHandler) AdvanceBranch(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	name := chi.URLParam(r, "name")

	var req advanceBranchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "request body must be valid JSON")
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
		writeAppError(w, core.MapDomainError(err))
		return
	}

	writeJSON(w, http.StatusOK, toBranchResponse(branch))
}

func (h *BranchHandler) DeleteBranch(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	name := chi.URLParam(r, "name")

	if err := h.svc.DeleteBranch(r.Context(), repoID, name); err != nil {
		writeAppError(w, core.MapDomainError(err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func toBranchResponse(b *domain.Branch) branchResponse {
	return branchResponse{
		Name:      b.Name,
		RepoID:    b.RepoID,
		CommitID:  b.CommitID,
		CreatedAt: b.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

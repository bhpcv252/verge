package v1

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/bhpcv252/verge/internal/api/core"
	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/service"
)

type MergeHandler struct {
	svc core.MergeService
}

func NewMergeHandler(svc core.MergeService) *MergeHandler {
	return &MergeHandler{svc: svc}
}

func (h *MergeHandler) Mount(r chi.Router) {
	r.Post("/repos/{repoID}/merges", h.CreateMerge)
}

type createMergeRequest struct {
	ParentIDs          []string           `json:"parent_ids"`
	ExpectedTargetHead string             `json:"expected_target_head"`
	TargetBranch       string             `json:"target_branch"`
	DataPointer        dataPointerRequest `json:"data_pointer"`
	Message            string             `json:"message"`
	Author             string             `json:"author"`
}

func (h *MergeHandler) CreateMerge(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")

	var req createMergeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "request body must be valid JSON")
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
		badRequest(w, "merge commits require exactly two parent_ids.")
		return
	}
	if req.DataPointer.Type == "" || req.DataPointer.Location == "" {
		badRequest(w, "'data_pointer' with 'type' and 'location' is required.")
		return
	}

	mergeCommit, err := h.svc.CreateMerge(r.Context(), service.CreateMergeInput{
		RepoID:             repoID,
		ParentIDs:          req.ParentIDs,
		ExpectedTargetHead: req.ExpectedTargetHead,
		TargetBranch:       req.TargetBranch,
		DataPointer: domain.DataPointer{
			Type:     req.DataPointer.Type,
			Location: req.DataPointer.Location,
			Hash:     req.DataPointer.Hash,
			Metadata: req.DataPointer.Metadata,
		},
		Message: req.Message,
		Author:  req.Author,
	})
	if err != nil {
		writeAppError(w, core.MapDomainError(err))
		return
	}

	writeJSON(w, http.StatusCreated, toCommitResponse(mergeCommit))
}

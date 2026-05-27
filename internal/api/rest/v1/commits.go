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

type CommitHandler struct {
	svc core.CommitService
}

func NewCommitHandler(svc core.CommitService) *CommitHandler {
	return &CommitHandler{svc: svc}
}

func (h *CommitHandler) Mount(r chi.Router) {
	r.Post("/repos/{repoID}/commits", h.CreateCommit)
	r.Get("/repos/{repoID}/commits", h.ListCommits)
	r.Get("/repos/{repoID}/commits/{commitID}", h.GetCommit)
	r.Get("/repos/{repoID}/commits/{commitID}/parents", h.GetParents)
}

type dataPointerRequest struct {
	Type     string          `json:"type"`
	Location string          `json:"location"`
	Hash     string          `json:"hash,omitempty"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

type createCommitRequest struct {
	ParentIDs      []string           `json:"parent_ids"`
	DataPointer    dataPointerRequest `json:"data_pointer"`
	Message        string             `json:"message"`
	Author         string             `json:"author"`
	IdempotencyKey string             `json:"idempotency_key"`
}

type commitResponse struct {
	ID          string              `json:"id"`
	RepoID      string              `json:"repo_id"`
	ParentIDs   []string            `json:"parent_ids"`
	DataPointer dataPointerResponse `json:"data_pointer"`
	Message     string              `json:"message"`
	Author      string              `json:"author"`
	Timestamp   string              `json:"timestamp"`
}

type dataPointerResponse struct {
	Type     string          `json:"type"`
	Location string          `json:"location"`
	Hash     string          `json:"hash,omitempty"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

type createCommitResponse struct {
	commitResponse
	Existing bool `json:"existing,omitempty"`
}

type listCommitsResponse struct {
	Commits    []commitResponse `json:"commits"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

type getParentsResponse struct {
	CommitID string           `json:"commit_id"`
	Parents  []commitResponse `json:"parents"`
}

func (h *CommitHandler) CreateCommit(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")

	var req createCommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "request body must be valid JSON")
		return
	}

	req.Message = strings.TrimSpace(req.Message)
	req.Author = strings.TrimSpace(req.Author)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)

	if req.Message == "" {
		badRequest(w, "'message' is required and must not be empty.")
		return
	}
	if req.Author == "" {
		badRequest(w, "'author' is required and must not be empty.")
		return
	}
	if req.DataPointer.Type == "" || req.DataPointer.Location == "" {
		badRequest(w, "'data_pointer' with 'type' and 'location' is required.")
		return
	}

	result, err := h.svc.CreateCommit(r.Context(), service.CreateCommitInput{
		RepoID:    repoID,
		ParentIDs: req.ParentIDs,
		DataPointer: domain.DataPointer{
			Type:     req.DataPointer.Type,
			Location: req.DataPointer.Location,
			Hash:     req.DataPointer.Hash,
			Metadata: req.DataPointer.Metadata,
		},
		Message:        req.Message,
		Author:         req.Author,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		writeAppError(w, core.MapDomainError(err))
		return
	}

	if result.Existing {
		writeJSON(w, http.StatusOK, createCommitResponse{
			commitResponse: toCommitResponse(result.Commit),
			Existing:       true,
		})
		return
	}

	writeJSON(w, http.StatusCreated, createCommitResponse{
		commitResponse: toCommitResponse(result.Commit),
	})
}

func (h *CommitHandler) GetCommit(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	commitID := chi.URLParam(r, "commitID")

	commit, err := h.svc.GetCommit(r.Context(), repoID, commitID)
	if err != nil {
		writeAppError(w, core.MapDomainError(err))
		return
	}

	writeJSON(w, http.StatusOK, toCommitResponse(commit))
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
		writeAppError(w, core.MapDomainError(err))
		return
	}

	resp := listCommitsResponse{
		Commits:    make([]commitResponse, 0, len(result.Commits)),
		NextCursor: result.NextCursor,
	}
	for _, c := range result.Commits {
		resp.Commits = append(resp.Commits, toCommitResponse(c))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *CommitHandler) GetParents(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")
	commitID := chi.URLParam(r, "commitID")

	parents, err := h.svc.GetParents(r.Context(), repoID, commitID)
	if err != nil {
		writeAppError(w, core.MapDomainError(err))
		return
	}

	resp := getParentsResponse{
		CommitID: commitID,
		Parents:  make([]commitResponse, 0, len(parents)),
	}
	for _, p := range parents {
		resp.Parents = append(resp.Parents, toCommitResponse(p))
	}

	writeJSON(w, http.StatusOK, resp)
}

func toCommitResponse(c *domain.Commit) commitResponse {
	return commitResponse{
		ID:        c.ID,
		RepoID:    c.RepoID,
		ParentIDs: c.ParentIDs,
		DataPointer: dataPointerResponse{
			Type:     c.DataPointer.Type,
			Location: c.DataPointer.Location,
			Hash:     c.DataPointer.Hash,
			Metadata: c.DataPointer.Metadata,
		},
		Message:   c.Message,
		Author:    c.Author,
		Timestamp: c.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

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

type RepoHandler struct {
	svc core.RepoService
}

func NewRepoHandler(svc core.RepoService) *RepoHandler {
	return &RepoHandler{svc: svc}
}

func (h *RepoHandler) Mount(r chi.Router) {
	r.Post("/repos", h.CreateRepo)
	r.Get("/repos", h.ListRepos)
	r.Get("/repos/{repoID}", h.GetRepo)
}

type createRepoRequest struct {
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch"`
}

type repoResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch"`
	CreatedAt     string `json:"created_at"`
}

type listReposResponse struct {
	Repos      []repoResponse `json:"repos"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

func (h *RepoHandler) CreateRepo(w http.ResponseWriter, r *http.Request) {
	var req createRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "request body must be valid JSON")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.DefaultBranch = strings.TrimSpace(req.DefaultBranch)

	if req.Name == "" {
		badRequest(w, "'name' is required and must not be empty.")
		return
	}
	if req.DefaultBranch == "" {
		badRequest(w, "'default_branch' is required and must not be empty.")
		return
	}

	repo, err := h.svc.CreateRepo(r.Context(), service.CreateRepoInput{
		Name:          req.Name,
		DefaultBranch: req.DefaultBranch,
	})
	if err != nil {
		writeAppError(w, core.MapDomainError(err))
		return
	}

	writeJSON(w, http.StatusCreated, toRepoResponse(repo))
}

func (h *RepoHandler) GetRepo(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")

	repo, err := h.svc.GetRepo(r.Context(), repoID)
	if err != nil {
		writeAppError(w, core.MapDomainError(err))
		return
	}

	writeJSON(w, http.StatusOK, toRepoResponse(repo))
}

func (h *RepoHandler) ListRepos(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			badRequest(w, "'limit' must be an integer between 1 and 100.")
			return
		}
		limit = n
	}

	result, err := h.svc.ListRepos(r.Context(), service.ListReposInput{
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
	})
	if err != nil {
		writeAppError(w, core.MapDomainError(err))
		return
	}

	resp := listReposResponse{
		Repos:      make([]repoResponse, 0, len(result.Repos)),
		NextCursor: result.NextCursor,
	}
	for _, repo := range result.Repos {
		resp.Repos = append(resp.Repos, toRepoResponse(repo))
	}

	writeJSON(w, http.StatusOK, resp)
}

func toRepoResponse(r *domain.Repo) repoResponse {
	return repoResponse{
		ID:            r.ID,
		Name:          r.Name,
		DefaultBranch: r.DefaultBranch,
		CreatedAt:     r.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

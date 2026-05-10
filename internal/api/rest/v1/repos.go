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

type RepoService interface {
	CreateRepo(ctx context.Context, in service.CreateRepoInput) (*domain.Repo, error)
	GetRepo(ctx context.Context, id string) (*domain.Repo, error)
	ListRepos(ctx context.Context, in service.ListReposInput) (*service.ListReposResult, error)
}

type RepoHandler struct {
	svc RepoService
}

func NewRepoHandler(svc RepoService) *RepoHandler {
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
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	DefaultBranch string    `json:"default_branch"`
	CreatedAt     time.Time `json:"created_at"`
}

type listReposResponse struct {
	Repos      []repoResponse `json:"repos"`
	NextCursor *string        `json:"next_cursor"` // null when there is no next page
}

func toRepoResponse(r *domain.Repo) repoResponse {
	return repoResponse{
		ID:            r.ID,
		Name:          r.Name,
		DefaultBranch: r.DefaultBranch,
		CreatedAt:     r.CreatedAt,
	}
}

func (h *RepoHandler) CreateRepo(w http.ResponseWriter, r *http.Request) {
	var req createRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "Request body must be valid JSON.")
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
		internalError(w)
		return
	}

	writeJSON(w, http.StatusCreated, toRepoResponse(repo))
}

func (h *RepoHandler) ListRepos(w http.ResponseWriter, r *http.Request) {
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

	result, err := h.svc.ListRepos(r.Context(), service.ListReposInput{
		Limit:  limit,
		Cursor: q.Get("cursor"),
	})
	if err != nil {
		internalError(w)
		return
	}

	resp := listReposResponse{
		Repos: make([]repoResponse, 0, len(result.Repos)),
	}
	for _, repo := range result.Repos {
		resp.Repos = append(resp.Repos, toRepoResponse(repo))
	}
	if result.NextCursor != "" {
		resp.NextCursor = &result.NextCursor
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *RepoHandler) GetRepo(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repoID")

	repo, err := h.svc.GetRepo(r.Context(), repoID)
	if err != nil {
		if errors.Is(err, domain.ErrRepoNotFound) {
			notFound(w, "repo_not_found",
				fmt.Sprintf("Repository %q does not exist.", repoID))
			return
		}
		internalError(w)
		return
	}

	writeJSON(w, http.StatusOK, toRepoResponse(repo))
}

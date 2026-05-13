package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(
	repoHandler *RepoHandler,
	branchHandler *BranchHandler,
	commitHandler *CommitHandler,
	mergeHandler *MergeHandler,
) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID) // X-Request-ID header on every response
	r.Use(middleware.RealIP)    // Reads X-Forwarded-For / X-Real-IP
	r.Use(middleware.Recoverer) // Catch panics; return 500 instead of crashing

	// Health (unauthenticated)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// v1 API
	r.Route("/v1", func(r chi.Router) {
		repoHandler.Mount(r)
		branchHandler.Mount(r)
		commitHandler.Mount(r)
		mergeHandler.Mount(r)
	})

	return r
}

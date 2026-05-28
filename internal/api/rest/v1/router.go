package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/bhpcv252/verge/internal/auth"
	"github.com/bhpcv252/verge/internal/observability"
)

func NewRouter(
	obs *observability.Provider,
	validator *auth.Validator, // nil = auth disabled
	repoHandler *RepoHandler,
	branchHandler *BranchHandler,
	commitHandler *CommitHandler,
	mergeHandler *MergeHandler,
) http.Handler {
	r := chi.NewRouter()

	// core chi middleware, order matters
	r.Use(middleware.RequestID) // must come first; HTTPMiddleware reads the ID it sets
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(observability.HTTPMiddleware(obs))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// PrometheusHandler is non-nil only when VERGE_OTEL_EXPORTER=prometheus
	if obs.PrometheusHandler != nil {
		r.Get("/metrics", obs.PrometheusHandler.ServeHTTP)
	}

	// versioned API
	r.Route("/v1", func(r chi.Router) {
		r.Use(auth.HTTPMiddleware(validator, obs))

		repoHandler.Mount(r)
		branchHandler.Mount(r)
		commitHandler.Mount(r)
		mergeHandler.Mount(r)
	})

	return r
}

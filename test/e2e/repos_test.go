//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pgmodule "github.com/testcontainers/testcontainers-go/modules/postgres"

	restv1 "github.com/bhpcv252/verge/internal/api/rest/v1"
	"github.com/bhpcv252/verge/internal/service"
	pgstore "github.com/bhpcv252/verge/internal/storage/postgres"
	"github.com/bhpcv252/verge/testhelper"
)

// startServer spins up a real Postgres container, runs all migrations via
// golang-migrate, wires the full HTTP stack, and returns the /v1 base URL.
func startServer(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	ctr, err := pgmodule.Run(ctx,
		"postgres:16",
		pgmodule.WithDatabase("verge"),
		pgmodule.WithUsername("verge"),
		pgmodule.WithPassword("changeme"),
		pgmodule.BasicWaitStrategies(),
	)
	require.NoError(t, err, "failed to start postgres container")
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	testhelper.RunMigrations(t, connStr)

	pool, err := pgstore.NewPool(ctx, connStr)
	require.NoError(t, err, "failed to connect to postgres")
	t.Cleanup(pool.Close)

	repoStore := pgstore.NewRepoStore(pool)
	repoSvc := service.NewRepoService(repoStore)
	repoHandler := restv1.NewRepoHandler(repoSvc)
	router := restv1.NewRouter(repoHandler)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to get free port")

	srv := &http.Server{
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	return fmt.Sprintf("http://%s/v1", ln.Addr().String())
}

// types

type repoResponse struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	DefaultBranch string    `json:"default_branch"`
	CreatedAt     time.Time `json:"created_at"`
}

type listReposResponse struct {
	Repos      []repoResponse `json:"repos"`
	NextCursor *string        `json:"next_cursor"`
}

type errResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// helpers

func doPost(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	require.NoError(t, err, "POST %s failed", url)
	return resp
}

func doGet(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err, "GET %s failed", url)
	return resp
}

func decodeJSON(t *testing.T, r io.Reader, v any) {
	t.Helper()
	require.NoError(t, json.NewDecoder(r).Decode(v), "failed to decode JSON response")
}

func uniqueRepoName() string { return "e2e-" + uuid.New().String() }

// POST /v1/repos

func TestRepos_Create_ValidInput_Returns201WithFields(t *testing.T) {
	base := startServer(t)

	name := uniqueRepoName()
	resp := doPost(t, base+"/repos", map[string]string{
		"name":           name,
		"default_branch": "main",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var got repoResponse
	decodeJSON(t, resp.Body, &got)
	assert.NotEmpty(t, got.ID)
	assert.Equal(t, name, got.Name)
	assert.Equal(t, "main", got.DefaultBranch)
	assert.False(t, got.CreatedAt.IsZero())
}

func TestRepos_Create_MissingName_Returns400WithInvalidRequest(t *testing.T) {
	base := startServer(t)

	resp := doPost(t, base+"/repos", map[string]string{"default_branch": "main"})
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "invalid_request", got.Error)
}

func TestRepos_Create_MissingDefaultBranch_Returns400WithInvalidRequest(t *testing.T) {
	base := startServer(t)

	resp := doPost(t, base+"/repos", map[string]string{"name": uniqueRepoName()})
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "invalid_request", got.Error)
}

// GET /v1/repos/:id

func TestRepos_GetByID_Exists_Returns200WithCorrectFields(t *testing.T) {
	base := startServer(t)

	name := uniqueRepoName()
	createResp := doPost(t, base+"/repos", map[string]string{
		"name":           name,
		"default_branch": "main",
	})
	var created repoResponse
	decodeJSON(t, createResp.Body, &created)
	createResp.Body.Close()

	resp := doGet(t, base+"/repos/"+created.ID)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got repoResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, name, got.Name)
}

func TestRepos_GetByID_NotFound_Returns404WithRepoNotFoundCode(t *testing.T) {
	base := startServer(t)

	resp := doGet(t, base+"/repos/repo_does-not-exist")
	defer resp.Body.Close()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "repo_not_found", got.Error)
}

// GET /v1/repos

func TestRepos_List_NoParams_Returns200(t *testing.T) {
	base := startServer(t)

	resp := doGet(t, base+"/repos")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRepos_List_InvalidLimit_Returns400WithInvalidRequest(t *testing.T) {
	base := startServer(t)

	resp := doGet(t, base+"/repos?limit=abc")
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var got errResponse
	decodeJSON(t, resp.Body, &got)
	assert.Equal(t, "invalid_request", got.Error)
}

func TestRepos_List_Pagination_CursorWorksCorrectly(t *testing.T) {
	base := startServer(t)

	// insert 3 repos
	for i := 0; i < 3; i++ {
		resp := doPost(t, base+"/repos", map[string]string{
			"name":           uniqueRepoName(),
			"default_branch": "main",
		})
		resp.Body.Close()
	}

	// page 1: limit=2
	resp1 := doGet(t, base+"/repos?limit=2")
	defer resp1.Body.Close()
	var page1 listReposResponse
	decodeJSON(t, resp1.Body, &page1)

	require.Len(t, page1.Repos, 2)
	require.NotNil(t, page1.NextCursor)
	require.NotEmpty(t, *page1.NextCursor)

	// page 2: use cursor from page 1
	resp2 := doGet(t, fmt.Sprintf("%s/repos?limit=2&cursor=%s", base, *page1.NextCursor))
	defer resp2.Body.Close()
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var page2 listReposResponse
	decodeJSON(t, resp2.Body, &page2)
	require.NotEmpty(t, page2.Repos)

	// no duplicates across pages
	seen := make(map[string]int)
	for _, r := range append(page1.Repos, page2.Repos...) {
		seen[r.ID]++
	}
	for id, count := range seen {
		assert.Equal(t, 1, count, "repo %s appeared more than once across pages", id)
	}
}

// GET /health

func TestHealth_Returns200(t *testing.T) {
	base := startServer(t)
	root := base[:len(base)-len("/v1")]

	resp := doGet(t, root+"/health")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	restv1 "github.com/bhpcv252/verge/internal/api/rest/v1"
	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/observability"
	"github.com/bhpcv252/verge/internal/service"
	pgstore "github.com/bhpcv252/verge/internal/storage/postgres"
	"github.com/bhpcv252/verge/testhelper"
)

// startE2EServer spins up a full HTTP server with all handlers
// and returns the base URL e.g. http://127.0.0.1:12345/v1
func startE2EServer(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	repoStore := pgstore.NewRepoStore(pool)
	branchStore := pgstore.NewBranchStore(pool)
	commitStore := pgstore.NewCommitStore(pool)

	repoSvc := service.NewRepoService(repoStore)
	branchSvc := service.NewBranchService(branchStore, repoStore, commitStore)
	commitSvc := service.NewCommitService(commitStore, repoStore)
	mergeSvc := service.NewMergeService(commitStore, repoStore, commitStore, branchStore)

	repoHandler := restv1.NewRepoHandler(repoSvc)
	branchHandler := restv1.NewBranchHandler(branchSvc)
	commitHandler := restv1.NewCommitHandler(commitSvc)
	mergeHandler := restv1.NewMergeHandler(mergeSvc)

	router := restv1.NewRouter(
		observability.Noop(),
		nil, // auth disabled
		repoHandler,
		branchHandler,
		commitHandler,
		mergeHandler,
	)

	// start HTTP server on random port
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

// HTTP client helpers

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

func doPatch(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "PATCH %s failed", url)
	return resp
}

func doDelete(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "DELETE %s failed", url)
	return resp
}

func decodeJSON(t *testing.T, r io.Reader, v any) {
	t.Helper()
	require.NoError(t, json.NewDecoder(r).Decode(v), "failed to decode JSON response")
}

type repoResponse struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	DefaultBranch string    `json:"default_branch"`
	CreatedAt     time.Time `json:"created_at"`
}

type branchResponse struct {
	Name      string    `json:"name"`
	RepoID    string    `json:"repo_id"`
	CommitID  string    `json:"commit_id"`
	CreatedAt time.Time `json:"created_at"`
}

type commitResponse struct {
	ID          string             `json:"id"`
	RepoID      string             `json:"repo_id"`
	ParentIDs   []string           `json:"parent_ids"`
	DataPointer domain.DataPointer `json:"data_pointer"`
	Message     string             `json:"message"`
	Author      string             `json:"author"`
	Timestamp   time.Time          `json:"timestamp"`
}

type errResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type listReposResponse struct {
	Repos      []repoResponse `json:"repos"`
	NextCursor *string        `json:"next_cursor"`
}

type listBranchesResponse struct {
	Branches   []branchResponse `json:"branches"`
	NextCursor *string          `json:"next_cursor"`
}

type listCommitsResponse struct {
	Commits    []commitResponse `json:"commits"`
	NextCursor *string          `json:"next_cursor"`
}

type parentsResponse struct {
	Parents []commitResponse `json:"parents"`
}

// factory helpers

func uniqueRepoName() string {
	return testhelper.UniqueName("e2e")
}

func createRepo(t *testing.T, base string) repoResponse {
	t.Helper()
	resp := doPost(t, base+"/repos", map[string]string{
		"name":           testhelper.UniqueName("e2e"),
		"default_branch": "main",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "createRepo setup failed")

	var repo repoResponse
	decodeJSON(t, resp.Body, &repo)
	return repo
}

func createCommit(t *testing.T, base, repoID string, parentIDs []string) commitResponse {
	t.Helper()
	resp := doPost(t, base+"/repos/"+repoID+"/commits", map[string]any{
		"parent_ids": parentIDs,
		"data_pointer": map[string]string{
			"type":     "db",
			"location": "test/fixture",
		},
		"message": "test commit",
		"author":  "test@example.com",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "createCommit setup failed")

	var commit commitResponse
	decodeJSON(t, resp.Body, &commit)
	return commit
}

func createBranch(t *testing.T, base, repoID, name, commitID string) branchResponse {
	t.Helper()
	resp := doPost(t, base+"/repos/"+repoID+"/branches", map[string]string{
		"name":             name,
		"source_commit_id": commitID,
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "createBranch setup failed")

	var branch branchResponse
	decodeJSON(t, resp.Body, &branch)
	return branch
}

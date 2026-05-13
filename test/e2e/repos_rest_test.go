//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/testhelper"
)

func startServer(t *testing.T) string {
	t.Helper()
	pool, cleanup := testhelper.SetupPostgres(t)
	t.Cleanup(cleanup)
	return startE2EServer(t, pool)
}

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

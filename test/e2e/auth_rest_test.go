//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	restv1 "github.com/bhpcv252/verge/internal/api/rest/v1"
	"github.com/bhpcv252/verge/internal/auth"
	"github.com/bhpcv252/verge/internal/observability"
	"github.com/bhpcv252/verge/internal/service"
	pgstore "github.com/bhpcv252/verge/internal/storage/postgres"
	"github.com/bhpcv252/verge/testhelper"
)

type authServerEnv struct {
	base    string
	testKey string
}

func startAuthServer(t *testing.T) authServerEnv {
	t.Helper()

	const testKey = "e2e-test-key-do-not-use-in-prod"

	pool, cleanup := testhelper.SetupPostgres(t)
	t.Cleanup(cleanup)

	repoStore := pgstore.NewRepoStore(pool)
	branchStore := pgstore.NewBranchStore(pool)
	commitStore := pgstore.NewCommitStore(pool)

	repoSvc := service.NewRepoService(repoStore)
	branchSvc := service.NewBranchService(branchStore, repoStore, commitStore)
	commitSvc := service.NewCommitService(commitStore, repoStore)
	mergeSvc := service.NewMergeService(commitStore, repoStore, commitStore, branchStore)

	validator, err := auth.NewValidator([]string{testKey})
	require.NoError(t, err, "build auth validator")

	router := restv1.NewRouter(
		observability.Noop(),
		validator,
		restv1.NewRepoHandler(repoSvc),
		restv1.NewBranchHandler(branchSvc),
		restv1.NewCommitHandler(commitSvc),
		restv1.NewMergeHandler(mergeSvc),
	)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "get free port for auth e2e server")

	srv := &http.Server{
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	return authServerEnv{
		base:    fmt.Sprintf("http://%s", ln.Addr().String()),
		testKey: testKey,
	}
}

func authRequest(t *testing.T, method, url, key string, body any) *http.Response {
	t.Helper()

	var reqBody *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req, err := http.NewRequest(method, url, reqBody)
	require.NoError(t, err)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// /health is always open

func TestAuth_HealthEndpoint_AlwaysAccessibleWithoutKey(t *testing.T) {
	env := startAuthServer(t)

	resp := authRequest(t, http.MethodGet, env.base+"/health", "", nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"/health must be reachable without credentials even when auth is enabled")
}

func TestAuth_HealthEndpoint_AccessibleWithKey(t *testing.T) {
	env := startAuthServer(t)

	resp := authRequest(t, http.MethodGet, env.base+"/health", env.testKey, nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// /v1 routes require a valid key

func TestAuth_NoKey_Returns401(t *testing.T) {
	env := startAuthServer(t)

	resp := authRequest(t, http.MethodGet, env.base+"/v1/repos", "", nil)
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	var body errResponse
	decodeJSON(t, resp.Body, &body)
	assert.Equal(t, "unauthorized", body.Error)
	assert.NotEmpty(t, body.Message)
}

func TestAuth_WrongKey_Returns401(t *testing.T) {
	env := startAuthServer(t)

	resp := authRequest(t, http.MethodGet, env.base+"/v1/repos", "totally-wrong-key", nil)
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	var body errResponse
	decodeJSON(t, resp.Body, &body)
	assert.Equal(t, "unauthorized", body.Error)
}

func TestAuth_WrongAndMissingKeyReturnSameError(t *testing.T) {
	env := startAuthServer(t)

	noKeyResp := authRequest(t, http.MethodGet, env.base+"/v1/repos", "", nil)
	defer noKeyResp.Body.Close()
	var noKeyBody errResponse
	decodeJSON(t, noKeyResp.Body, &noKeyBody)

	badKeyResp := authRequest(t, http.MethodGet, env.base+"/v1/repos", "bad-key", nil)
	defer badKeyResp.Body.Close()
	var badKeyBody errResponse
	decodeJSON(t, badKeyResp.Body, &badKeyBody)

	assert.Equal(t, noKeyResp.StatusCode, badKeyResp.StatusCode)
	assert.Equal(t, noKeyBody.Error, badKeyBody.Error)
	assert.Equal(t, noKeyBody.Message, badKeyBody.Message)
}

func TestAuth_ValidKey_PassesThroughToHandler(t *testing.T) {
	env := startAuthServer(t)

	resp := authRequest(t, http.MethodGet, env.base+"/v1/repos", env.testKey, nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"valid key must reach the handler (expected 200 from repo list)")
}

func TestAuth_ValidKey_AllVerbsAndRoutes(t *testing.T) {
	env := startAuthServer(t)

	createResp := authRequest(
		t,
		http.MethodPost,
		env.base+"/v1/repos",
		env.testKey,
		map[string]string{
			"name":           testhelper.UniqueName("auth-e2e"),
			"default_branch": "main",
		},
	)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var repo repoResponse
	decodeJSON(t, createResp.Body, &repo)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/repos"},
		{http.MethodGet, "/v1/repos/" + repo.ID},
		{http.MethodGet, "/v1/repos/" + repo.ID + "/branches"},
		{http.MethodGet, "/v1/repos/" + repo.ID + "/commits"},
	}

	for _, tc := range routes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			resp := authRequest(t, tc.method, env.base+tc.path, env.testKey, nil)
			defer resp.Body.Close()
			assert.NotEqual(t, http.StatusUnauthorized, resp.StatusCode,
				"%s %s must not return 401 with a valid key", tc.method, tc.path)
		})
	}
}

func TestAuth_KeyRotation_BothKeysAccepted(t *testing.T) {
	const (
		oldKey = "old-key-being-rotated-out"
		newKey = "new-key-just-added"
	)

	pool, cleanup := testhelper.SetupPostgres(t)
	t.Cleanup(cleanup)

	repoStore := pgstore.NewRepoStore(pool)
	branchStore := pgstore.NewBranchStore(pool)
	commitStore := pgstore.NewCommitStore(pool)

	repoSvc := service.NewRepoService(repoStore)
	branchSvc := service.NewBranchService(branchStore, repoStore, commitStore)
	commitSvc := service.NewCommitService(commitStore, repoStore)
	mergeSvc := service.NewMergeService(commitStore, repoStore, commitStore, branchStore)

	validator, err := auth.NewValidator([]string{oldKey, newKey})
	require.NoError(t, err)

	router := restv1.NewRouter(
		observability.Noop(),
		validator,
		restv1.NewRepoHandler(repoSvc),
		restv1.NewBranchHandler(branchSvc),
		restv1.NewCommitHandler(commitSvc),
		restv1.NewMergeHandler(mergeSvc),
	)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &http.Server{
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	base := fmt.Sprintf("http://%s", ln.Addr().String())

	for _, key := range []string{oldKey, newKey} {
		key := key
		t.Run("key="+key[:7]+"...", func(t *testing.T) {
			resp := authRequest(t, http.MethodGet, base+"/v1/repos", key, nil)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode,
				"key must be accepted during rotation window")
		})
	}

	resp := authRequest(t, http.MethodGet, base+"/v1/repos", "revoked-key", nil)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"a key not in the validator must be rejected even during rotation")
}

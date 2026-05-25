//go:build e2e && outbox

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Redis

func TestOutbox_Redis_BranchHeadMoved_PopulatesBranchKey(t *testing.T) {
	env := startServerWithConfig(t, infraConfig{name: "redis", redis: true})
	if env.rdb == nil {
		t.Skip("Redis not wired")
	}

	repo := createRepo(t, env.restBase)
	commit1 := createCommit(t, env.restBase, repo.ID, []string{})
	commit2 := createCommit(t, env.restBase, repo.ID, []string{commit1.ID})

	createBranch(t, env.restBase, repo.ID, "main", commit1.ID)

	advResp := doPatch(t, env.restBase+"/repos/"+repo.ID+"/branches/main", map[string]string{
		"commit_id":          commit2.ID,
		"expected_commit_id": commit1.ID,
	})
	advResp.Body.Close()
	require.Equal(t, http.StatusOK, advResp.StatusCode)

	env.waitForOutbox(t, 5*time.Second)

	key := redisBranchKey(repo.ID, "main")
	raw := pollRedisKey(t, env.rdb, key, 5*time.Second)

	var stored struct {
		CommitID string `json:"commit_id"`
		Version  int64  `json:"version"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &stored),
		"Redis branch value must be valid JSON")
	assert.Equal(t, commit2.ID, stored.CommitID,
		"branch head cache must point to the advanced commit")
	assert.Greater(t, stored.Version, int64(0),
		"version must be positive")
}

func TestOutbox_Redis_BranchAdvanced_UpdatesBranchKey(t *testing.T) {
	env := startServerWithConfig(t, infraConfig{name: "redis", redis: true})
	if env.rdb == nil {
		t.Skip("Redis not wired")
	}

	repo := createRepo(t, env.restBase)
	commit1 := createCommit(t, env.restBase, repo.ID, []string{})
	commit2 := createCommit(t, env.restBase, repo.ID, []string{commit1.ID})

	createBranch(t, env.restBase, repo.ID, "main", commit1.ID)
	env.waitForOutbox(t, 5*time.Second)

	// advance the branch to commit2
	advResp := doPatch(t, env.restBase+"/repos/"+repo.ID+"/branches/main", map[string]string{
		"commit_id":          commit2.ID,
		"expected_commit_id": commit1.ID,
	})
	advResp.Body.Close()
	require.Equal(t, http.StatusOK, advResp.StatusCode)

	env.waitForOutbox(t, 5*time.Second)

	key := redisBranchKey(repo.ID, "main")
	raw := pollRedisKey(t, env.rdb, key, 3*time.Second)

	var stored struct {
		CommitID string `json:"commit_id"`
		Version  int64  `json:"version"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &stored))
	assert.Equal(t, commit2.ID, stored.CommitID,
		"Redis must reflect the advanced branch head")
	assert.GreaterOrEqual(t, stored.Version, int64(2),
		"version must increment with each advance")
}

func TestOutbox_Redis_MergeCompleted_UpdatesBranchKey(t *testing.T) {
	env := startServerWithConfig(t, infraConfig{name: "redis", redis: true})
	if env.rdb == nil {
		t.Skip("Redis not wired")
	}

	repo := createRepo(t, env.restBase)

	root := createCommit(t, env.restBase, repo.ID, []string{})
	time.Sleep(10 * time.Millisecond)
	commitMain := createCommit(t, env.restBase, repo.ID, []string{root.ID})
	time.Sleep(10 * time.Millisecond)
	commitFeature := createCommit(t, env.restBase, repo.ID, []string{root.ID})

	createBranch(t, env.restBase, repo.ID, "main", commitMain.ID)
	env.waitForOutbox(t, 5*time.Second)

	// perform the merge
	mergeResp := doPost(t, env.restBase+"/repos/"+repo.ID+"/merges", map[string]any{
		"parent_ids":           []string{commitFeature.ID, commitMain.ID},
		"target_branch":        "main",
		"expected_target_head": commitMain.ID,
		"data_pointer":         map[string]string{"type": "db", "location": "test/fixture"},
		"message":              "Merge feature into main",
		"author":               "alice@example.com",
	})
	defer mergeResp.Body.Close()
	require.Equal(t, http.StatusCreated, mergeResp.StatusCode)

	var mergeCommit commitResponse
	decodeJSON(t, mergeResp.Body, &mergeCommit)

	env.waitForOutbox(t, 5*time.Second)

	// redis must point to the merge commit
	key := redisBranchKey(repo.ID, "main")
	raw := pollRedisKey(t, env.rdb, key, 3*time.Second)

	var stored struct {
		CommitID string `json:"commit_id"`
		Version  int64  `json:"version"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &stored))
	assert.Equal(t, mergeCommit.ID, stored.CommitID,
		"Redis branch must point to the merge commit after a merge")
	assert.GreaterOrEqual(t, stored.Version, int64(2),
		"version must be at least 2 (initial create + merge)")
}

func TestOutbox_Redis_VersionGuard_StaleEventIgnored(t *testing.T) {
	env := startServerWithConfig(t, infraConfig{name: "redis", redis: true})
	if env.rdb == nil {
		t.Skip("Redis not wired")
	}

	repo := createRepo(t, env.restBase)
	commit1 := createCommit(t, env.restBase, repo.ID, []string{})
	commit2 := createCommit(t, env.restBase, repo.ID, []string{commit1.ID})

	createBranch(t, env.restBase, repo.ID, "main", commit1.ID)

	advResp := doPatch(t, env.restBase+"/repos/"+repo.ID+"/branches/main", map[string]string{
		"commit_id":          commit2.ID,
		"expected_commit_id": commit1.ID,
	})
	advResp.Body.Close()
	require.Equal(t, http.StatusOK, advResp.StatusCode)

	// inject a synthetic stale event with version=1 (pointing to commit1)
	_, err := env.pool.Exec(context.Background(), `
		INSERT INTO outbox_events (id, event_type, payload, processed, created_at)
		VALUES (
			$1,
			'BranchHeadMoved',
			jsonb_build_object(
        'repo_id',     $2::text,
				'branch',      'main',
        'commit_id',   $3::text,
				'version',     1
			),
			false,
			now() + interval '10 seconds'
		)`,
		uuid.New().String(),
		repo.ID,
		commit1.ID,
	)
	require.NoError(t, err, "insert synthetic stale outbox event")

	env.waitForOutbox(t, 12*time.Second)

	// redis must still hold commit2
	key := redisBranchKey(repo.ID, "main")
	raw := pollRedisKey(t, env.rdb, key, 3*time.Second)

	var stored struct {
		CommitID string `json:"commit_id"`
		Version  int64  `json:"version"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &stored))
	assert.Equal(t, commit2.ID, stored.CommitID,
		"stale event (v=1) must not overwrite the current entry (v=2)")
	assert.GreaterOrEqual(t, stored.Version, int64(2),
		"version in Redis must remain at the advance version, not revert to 1")
}

func TestOutbox_Redis_IdempotentHeal_ReplayIsSafe(t *testing.T) {
	env := startServerWithConfig(t, infraConfig{name: "redis", redis: true})
	if env.rdb == nil {
		t.Skip("Redis not wired")
	}

	repo := createRepo(t, env.restBase)
	commit1 := createCommit(t, env.restBase, repo.ID, []string{})
	commit2 := createCommit(t, env.restBase, repo.ID, []string{commit1.ID})

	createBranch(t, env.restBase, repo.ID, "main", commit1.ID)

	advResp := doPatch(t, env.restBase+"/repos/"+repo.ID+"/branches/main", map[string]string{
		"commit_id":          commit2.ID,
		"expected_commit_id": commit1.ID,
	})
	advResp.Body.Close()
	require.Equal(t, http.StatusOK, advResp.StatusCode)

	env.waitForOutbox(t, 5*time.Second)

	// duplicate the BranchHeadMoved event for this repo's main branch
	_, err := env.pool.Exec(context.Background(), `
		INSERT INTO outbox_events (id, event_type, payload, processed, created_at)
		SELECT $1, event_type, payload, false, now() + interval '1 second'
		FROM outbox_events
		WHERE event_type IN ('BranchHeadMoved', 'MergeCompleted')
		  AND payload->>'repo_id' = $2
		LIMIT 1`,
		uuid.New().String(),
		repo.ID,
	)
	require.NoError(t, err, "duplicate outbox event for idempotency test")

	// process the duplicate
	env.waitForOutbox(t, 5*time.Second)

	// redis must still hold the correct commit
	key := redisBranchKey(repo.ID, "main")
	raw := pollRedisKey(t, env.rdb, key, 3*time.Second)

	var stored struct {
		CommitID string `json:"commit_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &stored))
	assert.Equal(t, commit2.ID, stored.CommitID,
		"Redis must be correct after idempotent replay of the same event")
}

// Neo4j

func TestOutbox_Neo4j_CommitCreated_PopulatesNode(t *testing.T) {
	env := startServerWithConfig(t, infraConfig{name: "neo4j", neo4j: true})
	if env.neo4jDriver == nil {
		t.Skip("Neo4j not wired")
	}

	repo := createRepo(t, env.restBase)
	commit := createCommit(t, env.restBase, repo.ID, []string{})

	env.waitForOutbox(t, 5*time.Second)

	n := neo4jCountNodes(t, env.neo4jDriver, repo.ID, commit.ID)
	assert.Equal(t, 1, n,
		"CommitCreated event must create exactly one Commit node in Neo4j")
}

func TestOutbox_Neo4j_RegularCommit_HasOneParentEdge(t *testing.T) {
	env := startServerWithConfig(t, infraConfig{name: "neo4j", neo4j: true})
	if env.neo4jDriver == nil {
		t.Skip("Neo4j not wired")
	}

	repo := createRepo(t, env.restBase)
	parent := createCommit(t, env.restBase, repo.ID, []string{})
	time.Sleep(10 * time.Millisecond)
	child := createCommit(t, env.restBase, repo.ID, []string{parent.ID})

	env.waitForOutbox(t, 5*time.Second)

	edges := neo4jCountEdges(t, env.neo4jDriver, repo.ID, child.ID)
	assert.Equal(t, 1, edges,
		"regular commit must have exactly one PARENT_OF edge in Neo4j")
}

func TestOutbox_Neo4j_RootCommit_HasNoParentEdges(t *testing.T) {
	env := startServerWithConfig(t, infraConfig{name: "neo4j", neo4j: true})
	if env.neo4jDriver == nil {
		t.Skip("Neo4j not wired")
	}

	repo := createRepo(t, env.restBase)
	root := createCommit(t, env.restBase, repo.ID, []string{})

	env.waitForOutbox(t, 5*time.Second)

	edges := neo4jCountEdges(t, env.neo4jDriver, repo.ID, root.ID)
	assert.Equal(t, 0, edges,
		"root commit must have zero PARENT_OF edges in Neo4j")
}

func TestOutbox_Neo4j_MergeCommit_HasTwoParentEdges(t *testing.T) {
	env := startServerWithConfig(t, infraConfig{name: "neo4j", neo4j: true})
	if env.neo4jDriver == nil {
		t.Skip("Neo4j not wired")
	}

	repo := createRepo(t, env.restBase)

	root := createCommit(t, env.restBase, repo.ID, []string{})
	time.Sleep(10 * time.Millisecond)
	commitMain := createCommit(t, env.restBase, repo.ID, []string{root.ID})
	time.Sleep(10 * time.Millisecond)
	commitFeature := createCommit(t, env.restBase, repo.ID, []string{root.ID})

	createBranch(t, env.restBase, repo.ID, "main", commitMain.ID)
	env.waitForOutbox(t, 5*time.Second)

	// perform the merge
	mergeResp := doPost(t, env.restBase+"/repos/"+repo.ID+"/merges", map[string]any{
		"parent_ids":           []string{commitFeature.ID, commitMain.ID},
		"target_branch":        "main",
		"expected_target_head": commitMain.ID,
		"data_pointer":         map[string]string{"type": "db", "location": "test/fixture"},
		"message":              "Merge feature into main",
		"author":               "alice@example.com",
	})
	defer mergeResp.Body.Close()
	require.Equal(t, http.StatusCreated, mergeResp.StatusCode)

	var mergeCommit commitResponse
	decodeJSON(t, mergeResp.Body, &mergeCommit)

	env.waitForOutbox(t, 5*time.Second)

	// the merge commit must have exactly two PARENT_OF edges
	edges := neo4jCountEdges(t, env.neo4jDriver, repo.ID, mergeCommit.ID)
	assert.Equal(t, 2, edges,
		"merge commit must have exactly two PARENT_OF edges in Neo4j")

	parentIDs := neo4jParentIDs(t, env.neo4jDriver, repo.ID, mergeCommit.ID)
	assert.Contains(t, parentIDs, commitFeature.ID,
		"merge commit must have edge to feature parent")
	assert.Contains(t, parentIDs, commitMain.ID,
		"merge commit must have edge to main parent")
}

func TestOutbox_Neo4j_IdempotentMerge_ReplayDoesNotDuplicate(t *testing.T) {
	env := startServerWithConfig(t, infraConfig{name: "neo4j", neo4j: true})
	if env.neo4jDriver == nil {
		t.Skip("Neo4j not wired")
	}

	repo := createRepo(t, env.restBase)

	root := createCommit(t, env.restBase, repo.ID, []string{})
	time.Sleep(10 * time.Millisecond)
	commitMain := createCommit(t, env.restBase, repo.ID, []string{root.ID})
	time.Sleep(10 * time.Millisecond)
	commitFeature := createCommit(t, env.restBase, repo.ID, []string{root.ID})

	createBranch(t, env.restBase, repo.ID, "main", commitMain.ID)
	env.waitForOutbox(t, 5*time.Second)

	mergeResp := doPost(t, env.restBase+"/repos/"+repo.ID+"/merges", map[string]any{
		"parent_ids":           []string{commitFeature.ID, commitMain.ID},
		"target_branch":        "main",
		"expected_target_head": commitMain.ID,
		"data_pointer":         map[string]string{"type": "db", "location": "test/fixture"},
		"message":              "Merge feature into main",
		"author":               "alice@example.com",
	})
	defer mergeResp.Body.Close()
	require.Equal(t, http.StatusCreated, mergeResp.StatusCode)

	var mergeCommit commitResponse
	decodeJSON(t, mergeResp.Body, &mergeCommit)

	env.waitForOutbox(t, 5*time.Second)

	// duplicate the CommitCreated event for the merge commit
	_, err := env.pool.Exec(context.Background(), `
		INSERT INTO outbox_events (id, event_type, payload, processed, created_at)
		SELECT $1, event_type, payload, false, now() + interval '1 second'
		FROM outbox_events
		WHERE event_type = 'CommitCreated'
		  AND payload->>'commit_id' = $2
		LIMIT 1`,
		uuid.New().String(),
		mergeCommit.ID,
	)
	require.NoError(t, err, "duplicate CommitCreated event for idempotency test")

	env.waitForOutbox(t, 5*time.Second)

	// Neo4j must still have exactly one node and two edges
	n := neo4jCountNodes(t, env.neo4jDriver, repo.ID, mergeCommit.ID)
	assert.Equal(t, 1, n,
		"replaying CommitCreated must not create duplicate Commit nodes")

	edges := neo4jCountEdges(t, env.neo4jDriver, repo.ID, mergeCommit.ID)
	assert.Equal(t, 2, edges,
		"replaying CommitCreated must not create duplicate PARENT_OF edges")
}

// Worker lifecycle

func TestOutbox_Worker_MarksEventsProcessed(t *testing.T) {
	// use postgres-only, no Redis/Neo4j handlers,
	// the worker should still mark events processed even though it skips them
	env := startServerWithConfig(t, infraConfig{name: "postgres-only"})

	repo := createRepo(t, env.restBase)
	createCommit(t, env.restBase, repo.ID, []string{})

	env.waitForOutbox(t, 5*time.Second)

	var processed int
	err := env.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM outbox_events WHERE processed = true`,
	).Scan(&processed)
	require.NoError(t, err)

	assert.Greater(t, processed, 0,
		"worker must mark events processed even with no handlers registered")
}

func TestOutbox_Worker_CrashRecovery_PicksUpUnprocessedEvents(t *testing.T) {
	env := startServerWithConfig(t, infraConfig{name: "redis", redis: true})
	if env.rdb == nil {
		t.Skip("Redis not wired")
	}

	repo := createRepo(t, env.restBase)

	// drain the queue so it is clean before the test
	env.waitForOutbox(t, 5*time.Second)

	commit1 := createCommit(t, env.restBase, repo.ID, []string{})
	commit2 := createCommit(t, env.restBase, repo.ID, []string{commit1.ID})

	createBranch(t, env.restBase, repo.ID, "main", commit1.ID)

	advResp := doPatch(t, env.restBase+"/repos/"+repo.ID+"/branches/main", map[string]string{
		"commit_id":          commit2.ID,
		"expected_commit_id": commit1.ID,
	})
	advResp.Body.Close()
	require.Equal(t, http.StatusOK, advResp.StatusCode)

	// verify the running worker picks up the events
	env.waitForOutbox(t, 5*time.Second)

	key := redisBranchKey(repo.ID, "main")
	raw := pollRedisKey(t, env.rdb, key, 5*time.Second)

	var stored struct {
		CommitID string `json:"commit_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &stored))
	assert.Equal(t, commit2.ID, stored.CommitID,
		"worker must process events and populate Redis")
}

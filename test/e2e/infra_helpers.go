//go:build e2e && outbox

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	vergev1 "github.com/bhpcv252/verge/api/proto/verge/v1"
	grpcv1 "github.com/bhpcv252/verge/internal/api/grpc/v1"
	restv1 "github.com/bhpcv252/verge/internal/api/rest/v1"
	"github.com/bhpcv252/verge/internal/observability"
	"github.com/bhpcv252/verge/internal/outbox"
	"github.com/bhpcv252/verge/internal/service"
	"github.com/bhpcv252/verge/internal/storage/composite"
	"github.com/bhpcv252/verge/internal/storage/interfaces"
	neo4jstore "github.com/bhpcv252/verge/internal/storage/neo4j"
	pgstore "github.com/bhpcv252/verge/internal/storage/postgres"
	redisstore "github.com/bhpcv252/verge/internal/storage/redis"
	"github.com/bhpcv252/verge/testhelper"
)

type infraConfig struct {
	name       string
	redis      bool
	neo4j      bool
	skipWorker bool // when true, don't start the in-process outbox worker (for Kafka tests)
}

var allTiers = []infraConfig{
	{name: "postgres-only"},
	{name: "postgres+redis", redis: true},
	{name: "postgres+neo4j", neo4j: true},
	{name: "postgres+redis+neo4j", redis: true, neo4j: true},
}

// test environment

type testEnv struct {
	restBase string

	grpc *grpcClients

	rdb *redis.Client

	neo4jDriver neo4j.DriverWithContext

	pool *pgxpool.Pool

	waitForOutbox func(t *testing.T, timeout time.Duration)
}

func startServerWithConfig(t *testing.T, cfg infraConfig) *testEnv {
	t.Helper()
	ctx := context.Background()

	pool, pgCleanup := testhelper.SetupPostgres(t)
	t.Cleanup(pgCleanup)

	pgRepoStore := pgstore.NewRepoStore(pool)
	pgBranchStore := pgstore.NewBranchStore(pool)
	pgCommitStore := pgstore.NewCommitStore(pool)

	var (
		rdb                  *redis.Client
		redisBranchHeadStore interfaces.BranchHeadStore
		effectiveBranchStore service.BranchStore = pgBranchStore
		effectiveCommitStore service.CommitStore = pgCommitStore
	)

	if cfg.redis {
		rdb = testhelper.SetupRedis(t)
		redisBranchHeadStore = redisstore.NewBranchHeadStore(rdb, 5*time.Minute)
		redisCommitCache := redisstore.NewCommitCache(rdb)

		effectiveBranchStore = composite.NewBranchRouter(
			pgBranchStore,
			redisBranchHeadStore,
			observability.Noop(),
		)
		effectiveCommitStore = composite.NewCommitRouter(
			pgCommitStore,
			redisCommitCache,
			observability.Noop(),
		)
	}

	var neo4jDriver neo4j.DriverWithContext

	if cfg.neo4j {
		neo4jDriver = testhelper.SetupNeo4j(t)
		n4jStore := neo4jstore.NewGraphStore(neo4jDriver)
		_ = composite.NewGraphRouter(n4jStore, pgstore.NewGraphStore(pool), observability.Noop())
	}

	repoSvc := service.NewRepoService(pgRepoStore)
	branchSvc := service.NewBranchService(effectiveBranchStore, pgRepoStore, effectiveCommitStore)
	commitSvc := service.NewCommitService(effectiveCommitStore, pgRepoStore)
	mergeSvc := service.NewMergeService(
		pgCommitStore,        // MergeStore - writes always go to Postgres
		pgRepoStore,          // RepoStore
		effectiveCommitStore, // CommitStore - reads can use cache
		effectiveBranchStore, // BranchStore
	)

	if !cfg.skipWorker {
		var handlers []outbox.OutboxHandler
		if cfg.neo4j {
			handlers = append(handlers, outbox.NewNeo4jHandler(neo4jDriver))
		}
		if cfg.redis {
			handlers = append(handlers, outbox.NewRedisHealHandler(redisBranchHeadStore))
		}

		workerCtx, stopWorker := context.WithCancel(ctx)
		t.Cleanup(stopWorker)

		// PollingSource owns the interval and batch size
		src := outbox.NewPollingSource(pool, 25*time.Millisecond, 100)
		worker := outbox.NewWorker(
			outbox.WithSource(src),
			outbox.WithHandlers(handlers),
		)
		go func() {
			if err := worker.Run(workerCtx); err != nil {
				t.Logf("outbox worker stopped: %v", err)
			}
		}()
	}

	// REST server
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

	restLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "get free port for REST server")

	httpSrv := &http.Server{
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	go func() { _ = httpSrv.Serve(restLn) }()
	t.Cleanup(func() { _ = httpSrv.Shutdown(context.Background()) })

	restBase := fmt.Sprintf("http://%s/v1", restLn.Addr().String())

	// gRPC
	grpcSrv := grpc.NewServer()
	vergev1.RegisterRepositoryServiceServer(grpcSrv, grpcv1.NewRepoServer(repoSvc))
	vergev1.RegisterBranchServiceServer(grpcSrv, grpcv1.NewBranchServer(branchSvc))
	vergev1.RegisterCommitServiceServer(grpcSrv, grpcv1.NewCommitServer(commitSvc))
	vergev1.RegisterMergeServiceServer(grpcSrv, grpcv1.NewMergeServer(mergeSvc))

	grpcLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "get free port for gRPC server")

	go func() { _ = grpcSrv.Serve(grpcLn) }()
	t.Cleanup(func() { grpcSrv.GracefulStop() })

	cc, err := grpc.NewClient(
		grpcLn.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err, "dial gRPC server")
	t.Cleanup(func() { _ = cc.Close() })

	clients := &grpcClients{
		repo:   vergev1.NewRepositoryServiceClient(cc),
		branch: vergev1.NewBranchServiceClient(cc),
		commit: vergev1.NewCommitServiceClient(cc),
		merge:  vergev1.NewMergeServiceClient(cc),
	}

	// waitForOutbox closure
	waitFn := makeWaitForOutbox(ctx, pool)

	return &testEnv{
		restBase:      restBase,
		grpc:          clients,
		rdb:           rdb,
		neo4jDriver:   neo4jDriver,
		pool:          pool,
		waitForOutbox: waitFn,
	}
}

func makeWaitForOutbox(ctx context.Context, pool *pgxpool.Pool) func(*testing.T, time.Duration) {
	return func(t *testing.T, timeout time.Duration) {
		t.Helper()
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			var pending int
			err := pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM outbox_events WHERE processed = false`,
			).Scan(&pending)
			if err == nil && pending == 0 {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("timed out waiting for outbox_events to be fully processed")
	}
}

func neo4jCountNodes(
	t *testing.T,
	driver neo4j.DriverWithContext,
	repoID, commitID string,
) int {
	t.Helper()
	ctx := context.Background()
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx,
		`MATCH (c:Commit {id: $id, repo_id: $repo_id}) RETURN count(c) AS n`,
		map[string]any{"id": commitID, "repo_id": repoID},
	)
	require.NoError(t, err, "neo4j count nodes query")

	record, err := result.Single(ctx)
	require.NoError(t, err, "neo4j count nodes result")

	n, _ := record.Get("n")
	count, _ := n.(int64)
	return int(count)
}

func neo4jCountEdges(
	t *testing.T,
	driver neo4j.DriverWithContext,
	repoID, childCommitID string,
) int {
	t.Helper()
	ctx := context.Background()
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx,
		`MATCH (c:Commit {id: $id, repo_id: $repo_id})-[:PARENT_OF]->(p:Commit)
		 RETURN count(p) AS n`,
		map[string]any{"id": childCommitID, "repo_id": repoID},
	)
	require.NoError(t, err, "neo4j count edges query")

	record, err := result.Single(ctx)
	require.NoError(t, err, "neo4j count edges result")

	n, _ := record.Get("n")
	count, _ := n.(int64)
	return int(count)
}

func neo4jParentIDs(
	t *testing.T,
	driver neo4j.DriverWithContext,
	repoID, childCommitID string,
) []string {
	t.Helper()
	ctx := context.Background()
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx,
		`MATCH (c:Commit {id: $id, repo_id: $repo_id})-[:PARENT_OF]->(p:Commit)
		 RETURN p.id AS parent_id`,
		map[string]any{"id": childCommitID, "repo_id": repoID},
	)
	require.NoError(t, err, "neo4j parent IDs query")

	var ids []string
	for result.Next(ctx) {
		if pid, ok := result.Record().Get("parent_id"); ok {
			ids = append(ids, pid.(string))
		}
	}
	require.NoError(t, result.Err(), "neo4j parent IDs iteration")
	return ids
}

func redisBranchKey(repoID, name string) string {
	return fmt.Sprintf("branch:%s:%s", repoID, name)
}

func redisCommitKey(repoID, commitID string) string {
	return fmt.Sprintf("commit:%s:%s", repoID, commitID)
}

func pollRedisKey(
	t *testing.T,
	rdb *redis.Client,
	key string,
	timeout time.Duration,
) string {
	t.Helper()
	var val string
	require.Eventually(t, func() bool {
		var err error
		val, err = rdb.Get(context.Background(), key).Result()
		return err == nil && val != ""
	}, timeout, 100*time.Millisecond, "timed out waiting for Redis key %q to be populated", key)
	return val
}

func assertRedisKeyEquals(
	t *testing.T,
	rdb *redis.Client,
	key string,
	expectedCommitID string,
	timeout time.Duration,
) {
	t.Helper()
	require.Eventually(t, func() bool {
		raw, err := rdb.Get(context.Background(), key).Result()
		if err != nil {
			return false
		}
		var stored struct {
			CommitID string `json:"commit_id"`
		}
		if err := json.Unmarshal([]byte(raw), &stored); err != nil {
			return false
		}
		return stored.CommitID == expectedCommitID
	}, timeout, 100*time.Millisecond, "Redis key %q did not match expected commit %q", key, expectedCommitID)
}

func assertNeo4jNodeCount(
	t *testing.T,
	driver neo4j.DriverWithContext,
	repoID, commitID string,
	expectedCount int,
	timeout time.Duration,
) {
	t.Helper()
	require.Eventually(t, func() bool {
		return neo4jCountNodes(t, driver, repoID, commitID) == expectedCount
	}, timeout, 100*time.Millisecond, "Neo4j node count for commit %q did not reach %d", commitID, expectedCount)
}

func assertNeo4jEdgeCount(
	t *testing.T,
	driver neo4j.DriverWithContext,
	repoID, commitID string,
	expectedCount int,
	timeout time.Duration,
) {
	t.Helper()
	require.Eventually(t, func() bool {
		return neo4jCountEdges(t, driver, repoID, commitID) == expectedCount
	}, timeout, 100*time.Millisecond, "Neo4j edge count for commit %q did not reach %d", commitID, expectedCount)
}

func assertNeo4jParentIDs(
	t *testing.T,
	driver neo4j.DriverWithContext,
	repoID, commitID string,
	expectedParents []string,
	timeout time.Duration,
) {
	t.Helper()
	require.Eventually(t, func() bool {
		parentIDs := neo4jParentIDs(t, driver, repoID, commitID)
		if len(parentIDs) != len(expectedParents) {
			return false
		}
		for _, expected := range expectedParents {
			found := false
			for _, actual := range parentIDs {
				if actual == expected {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}, timeout, 100*time.Millisecond, "Neo4j parent IDs for commit %q did not match expected %v", commitID, expectedParents)
}

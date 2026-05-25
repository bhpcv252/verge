//go:build e2e && outbox

package e2e

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	redpandacontainer "github.com/testcontainers/testcontainers-go/modules/redpanda"

	"github.com/bhpcv252/verge/internal/outbox"
	"github.com/bhpcv252/verge/internal/outbox/eventbus/kafka"
	redisstore "github.com/bhpcv252/verge/internal/storage/redis"
	"github.com/bhpcv252/verge/testhelper"
)

type redpandaConfig struct {
	brokers []string
	topic   string
}

func startRedpanda(t *testing.T) redpandaConfig {
	t.Helper()
	ctx := context.Background()

	container, err := redpandacontainer.Run(ctx,
		"redpandadata/redpanda:v23.3.11",
	)
	require.NoError(t, err, "start Redpanda container")
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	brokerAddr, err := container.KafkaSeedBroker(ctx)
	require.NoError(t, err, "get Kafka seed broker")

	topic := testhelper.UniqueName("verge-events")

	err = kafka.CreateTopic(ctx, brokerAddr, topic, 1, 1)
	require.NoError(t, err, "create Kafka topic")

	t.Logf("Redpanda started: broker=%s, topic=%s", brokerAddr, topic)

	return redpandaConfig{
		brokers: []string{brokerAddr},
		topic:   topic,
	}
}

type kafkaEnv struct {
	*testEnv
	kafka redpandaConfig
}

func startKafkaEnv(t *testing.T) *kafkaEnv {
	t.Helper()

	rp := startRedpanda(t)

	cfg := infraConfig{
		name:       "kafka-full-stack",
		redis:      true,
		neo4j:      true,
		skipWorker: true, // don't start in-process worker
	}

	env := startServerWithConfig(t, cfg)

	// ensure Redis and Neo4j are wired before using them
	require.NotNil(t, env.rdb, "Redis must be wired for Kafka tests")
	require.NotNil(t, env.neo4jDriver, "Neo4j must be wired for Kafka tests")

	// producer side

	producer := kafka.NewProducer(kafka.Config{
		Brokers: rp.brokers,
		Topic:   rp.topic,
	})
	t.Cleanup(func() { _ = producer.Close() })

	producerCtx, stopProducer := context.WithCancel(context.Background())
	t.Cleanup(stopProducer)

	worker := outbox.NewWorker(
		env.pool,
		outbox.WithEventBus(producer),
		outbox.WithInterval(25*time.Millisecond),
		outbox.WithBatchSize(100),
	)
	go worker.Run(producerCtx)

	// consumer side

	redisBranchHeadStore := redisstore.NewBranchHeadStore(env.rdb, 5*time.Minute)

	handlers := []outbox.OutboxHandler{
		outbox.NewNeo4jHandler(env.neo4jDriver),
		outbox.NewRedisHealHandler(redisBranchHeadStore),
	}

	consumer := kafka.NewConsumer(
		kafka.ConsumerConfig{
			Brokers: rp.brokers,
			Topic:   rp.topic,
			GroupID: testhelper.UniqueName("e2e-consumer"),
		},
		handlers,
	)
	t.Cleanup(func() { _ = consumer.Close() })

	consumerCtx, stopConsumer := context.WithCancel(context.Background())
	t.Cleanup(stopConsumer)

	go func() {
		if err := consumer.Run(consumerCtx); err != nil && consumerCtx.Err() == nil {
			t.Logf("kafka consumer stopped: %v", err)
		}
	}()

	return &kafkaEnv{
		testEnv: env,
		kafka:   rp,
	}
}

func (ke *kafkaEnv) waitForProjections(t *testing.T, timeout time.Duration) {
	t.Helper()
	ke.waitForOutbox(t, timeout)
	// Additional buffer for Kafka message propagation and consumer processing
	time.Sleep(10 * time.Second)
}

func TestKafka_CommitCreated_FlowsThroughToNeo4j(t *testing.T) {
	env := startKafkaEnv(t)

	repo := createRepo(t, env.restBase)
	commit := createCommit(t, env.restBase, repo.ID, []string{})

	env.waitForProjections(t, 30*time.Second)

	assertNeo4jNodeCount(t, env.neo4jDriver, repo.ID, commit.ID, 1, 30*time.Second)
}

func TestKafka_BranchHeadMoved_FlowsThroughToRedis(t *testing.T) {
	env := startKafkaEnv(t)

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

	env.waitForProjections(t, 30*time.Second)

	key := redisBranchKey(repo.ID, "main")
	assertRedisKeyEquals(t, env.rdb, key, commit2.ID, 30*time.Second)
}

func TestKafka_MergeCompleted_UpdatesRedisAndNeo4j(t *testing.T) {
	env := startKafkaEnv(t)

	repo := createRepo(t, env.restBase)
	root := createCommit(t, env.restBase, repo.ID, []string{})
	time.Sleep(10 * time.Millisecond)
	commitMain := createCommit(t, env.restBase, repo.ID, []string{root.ID})
	time.Sleep(10 * time.Millisecond)
	commitFeature := createCommit(t, env.restBase, repo.ID, []string{root.ID})

	createBranch(t, env.restBase, repo.ID, "main", commitMain.ID)

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

	env.waitForProjections(t, 30*time.Second)

	// redis must point to the merge commit
	key := redisBranchKey(repo.ID, "main")
	assertRedisKeyEquals(t, env.rdb, key, mergeCommit.ID, 30*time.Second)

	// neo4j must have the merge commit node with two parent edges
	assertNeo4jNodeCount(t, env.neo4jDriver, repo.ID, mergeCommit.ID, 1, 30*time.Second)
	assertNeo4jEdgeCount(t, env.neo4jDriver, repo.ID, mergeCommit.ID, 2, 30*time.Second)
	assertNeo4jParentIDs(
		t,
		env.neo4jDriver,
		repo.ID,
		mergeCommit.ID,
		[]string{commitFeature.ID, commitMain.ID},
		30*time.Second,
	)
}

func TestKafka_BrokerUnavailable_EventsProcessedAfterRecovery(t *testing.T) {
	// set up infra but skip the outbox worker entirely so we can wire our own
	pool, pgCleanup := testhelper.SetupPostgres(t)
	t.Cleanup(pgCleanup)

	rp := startRedpanda(t)

	brokenAddr := unusedLocalAddr(t)
	brokenProducer := kafka.NewProducer(kafka.Config{
		Brokers: []string{brokenAddr},
		Topic:   "will-not-reach-anywhere",
	})
	t.Cleanup(func() { _ = brokenProducer.Close() })

	brokenWorkerCtx, stopBrokenWorker := context.WithCancel(context.Background())
	t.Cleanup(stopBrokenWorker)

	brokenWorker := outbox.NewWorker(
		pool,
		outbox.WithEventBus(brokenProducer),
		outbox.WithInterval(25*time.Millisecond),
		outbox.WithBatchSize(100),
	)
	go brokenWorker.Run(brokenWorkerCtx)

	_, err := pool.Exec(context.Background(), `
		INSERT INTO outbox_events (id, event_type, payload, processed, created_at)
		VALUES ($1, 'CommitCreated', '{"id":"test-commit","repo_id":"test-repo"}', false, now())
	`, "evt_"+uuid.New().String())
	require.NoError(t, err)

	// give the broken worker a chance to try publishing
	time.Sleep(200 * time.Millisecond)

	// verify the row is still unprocessed
	var pending int
	err = pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM outbox_events WHERE processed = false`,
	).Scan(&pending)
	require.NoError(t, err)
	assert.Greater(t, pending, 0,
		"outbox rows must remain unprocessed while the broker is unavailable")

	stopBrokenWorker()

	realProducer := kafka.NewProducer(kafka.Config{
		Brokers: rp.brokers,
		Topic:   rp.topic,
	})
	t.Cleanup(func() { _ = realProducer.Close() })

	realWorkerCtx, stopRealWorker := context.WithCancel(context.Background())
	t.Cleanup(stopRealWorker)

	realWorker := outbox.NewWorker(
		pool,
		outbox.WithEventBus(realProducer),
		outbox.WithInterval(25*time.Millisecond),
		outbox.WithBatchSize(100),
	)
	go realWorker.Run(realWorkerCtx)

	// wait until all rows are processed
	require.Eventually(t, func() bool {
		var p int
		_ = pool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM outbox_events WHERE processed = false`,
		).Scan(&p)
		return p == 0
	}, 30*time.Second, 100*time.Millisecond, "all outbox rows must be processed once the broker becomes available again")
}

func TestKafka_ConcurrentBranchAdvances_ExactlyOneEventWins(t *testing.T) {
	env := startKafkaEnv(t)

	repo := createRepo(t, env.restBase)
	commit1 := createCommit(t, env.restBase, repo.ID, []string{})
	commit2a := createCommit(t, env.restBase, repo.ID, []string{commit1.ID})
	commit2b := createCommit(t, env.restBase, repo.ID, []string{commit1.ID})

	createBranch(t, env.restBase, repo.ID, "main", commit1.ID)

	// wait for the branch creation to propagate before we race
	env.waitForProjections(t, 30*time.Second)

	type result struct {
		status   int
		commitID string // the commit_id they tried to advance to
	}
	results := make([]result, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	advance := func(idx int, newCommitID string) {
		defer wg.Done()
		resp := doPatch(t, env.restBase+"/repos/"+repo.ID+"/branches/main", map[string]string{
			"commit_id":          newCommitID,
			"expected_commit_id": commit1.ID,
		})
		resp.Body.Close()
		results[idx] = result{status: resp.StatusCode, commitID: newCommitID}
	}

	go advance(0, commit2a.ID)
	go advance(1, commit2b.ID)
	wg.Wait()

	// exactly one 200 and one 409
	statuses := []int{results[0].status, results[1].status}
	assert.Contains(t, statuses, http.StatusOK, "one advance must succeed")
	assert.Contains(t, statuses, http.StatusConflict, "one advance must be rejected")

	// figure out which commit won
	var winnerCommitID string
	for _, r := range results {
		if r.status == http.StatusOK {
			winnerCommitID = r.commitID
		}
	}
	require.NotEmpty(t, winnerCommitID)

	env.waitForProjections(t, 30*time.Second)

	key := redisBranchKey(repo.ID, "main")
	assertRedisKeyEquals(t, env.rdb, key, winnerCommitID, 30*time.Second)
}

func TestKafka_DuplicatePublish_HandlersAreIdempotent(t *testing.T) {
	env := startKafkaEnv(t)

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

	env.waitForProjections(t, 30*time.Second)

	// confirm baseline state
	require.Equal(t, 1, neo4jCountNodes(t, env.neo4jDriver, repo.ID, commit2.ID))

	// insert duplicate outbox events for both CommitCreated and BranchHeadMoved
	_, err := env.pool.Exec(context.Background(), `
		INSERT INTO outbox_events (id, event_type, payload, processed, created_at)
		SELECT $1, event_type, payload, false, now() + interval '500 milliseconds'
		FROM outbox_events
		WHERE event_type IN ('CommitCreated', 'BranchHeadMoved')
		  AND payload->>'repo_id' = $2
		LIMIT 1`,
		"evt_"+uuid.New().String(),
		repo.ID,
	)
	require.NoError(t, err, "insert duplicate outbox events for idempotency test")

	// allow the producer to publish and the consumer to handle duplicates
	env.waitForProjections(t, 30*time.Second)

	// neo4j must still have exactly one node
	assertNeo4jNodeCount(t, env.neo4jDriver, repo.ID, commit2.ID, 1, 30*time.Second)

	// redis must still hold the correct commit
	key := redisBranchKey(repo.ID, "main")
	assertRedisKeyEquals(t, env.rdb, key, commit2.ID, 30*time.Second)
}

func TestKafka_LinearHistory_AllCommitsReachNeo4j(t *testing.T) {
	env := startKafkaEnv(t)

	repo := createRepo(t, env.restBase)

	root := createCommit(t, env.restBase, repo.ID, []string{})
	time.Sleep(10 * time.Millisecond)
	c1 := createCommit(t, env.restBase, repo.ID, []string{root.ID})
	time.Sleep(10 * time.Millisecond)
	c2 := createCommit(t, env.restBase, repo.ID, []string{c1.ID})
	time.Sleep(10 * time.Millisecond)
	c3 := createCommit(t, env.restBase, repo.ID, []string{c2.ID})

	env.waitForProjections(t, 30*time.Second)

	// all four commits must have Commit nodes
	for _, id := range []string{root.ID, c1.ID, c2.ID, c3.ID} {
		assertNeo4jNodeCount(t, env.neo4jDriver, repo.ID, id, 1, 30*time.Second)
	}

	// each non-root commit must have exactly one PARENT_OF edge
	assertNeo4jEdgeCount(t, env.neo4jDriver, repo.ID, root.ID, 0, 30*time.Second)
	assertNeo4jEdgeCount(t, env.neo4jDriver, repo.ID, c1.ID, 1, 30*time.Second)
	assertNeo4jEdgeCount(t, env.neo4jDriver, repo.ID, c2.ID, 1, 30*time.Second)
	assertNeo4jEdgeCount(t, env.neo4jDriver, repo.ID, c3.ID, 1, 30*time.Second)

	// verify parent linkage correctness
	assertNeo4jParentIDs(t, env.neo4jDriver, repo.ID, c1.ID, []string{root.ID}, 30*time.Second)
	assertNeo4jParentIDs(t, env.neo4jDriver, repo.ID, c2.ID, []string{c1.ID}, 30*time.Second)
	assertNeo4jParentIDs(t, env.neo4jDriver, repo.ID, c3.ID, []string{c2.ID}, 30*time.Second)
}

func unusedLocalAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

//go:build integration

package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/testhelper"
)

// test DB helpers

func setupWorkerTest(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	return testhelper.SetupPostgres(t)
}

// insertEvent writes a single unprocessed outbox_events row and returns its ID
func insertEvent(t *testing.T, pool *pgxpool.Pool, eventType string, payload any) string {
	t.Helper()

	id := "evt_" + testhelper.UniqueID("test")
	b, err := json.Marshal(payload)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(),
		`INSERT INTO outbox_events (id, event_type, payload, created_at, processed)
		 VALUES ($1, $2, $3, now(), false)`,
		id, eventType, b,
	)
	require.NoError(t, err)
	return id
}

type outboxRow struct {
	processed   bool
	processedAt *time.Time
}

// fetchEvent reads a single outbox_events row by ID
func fetchEvent(t *testing.T, pool *pgxpool.Pool, id string) outboxRow {
	t.Helper()
	var row outboxRow
	err := pool.QueryRow(context.Background(),
		`SELECT processed, processed_at FROM outbox_events WHERE id = $1`, id,
	).Scan(&row.processed, &row.processedAt)
	require.NoError(t, err)
	return row
}

// countProcessed returns how many rows in outbox_events have processed = true
func countProcessed(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM outbox_events WHERE processed = true`,
	).Scan(&n)
	require.NoError(t, err)
	return n
}

// validBranchPayload returns a minimal BranchHeadMovedPayload as a map.
func validBranchPayload() map[string]any {
	return map[string]any{
		"repo_id":   "repo-integration-test",
		"branch":    "main",
		"commit_id": "commit-abc",
		"version":   time.Now().UnixMilli(),
	}
}

// In-process mode, handler succeeds

func TestWorker_Poll_SuccessfulHandler_MarksEventProcessed(t *testing.T) {
	pool, cleanup := setupWorkerTest(t)
	defer cleanup()

	id := insertEvent(t, pool, EventTypeBranchHeadMoved, validBranchPayload())

	h := newHandler(EventTypeBranchHeadMoved)
	w := NewWorker(pool, WithHandlers([]OutboxHandler{h}))

	require.NoError(t, w.poll(context.Background()))

	row := fetchEvent(t, pool, id)
	assert.True(t, row.processed)
	assert.NotNil(t, row.processedAt)
	assert.Equal(t, 1, h.callCount())
}

func TestWorker_Poll_SuccessfulHandler_EventPayloadDeliveredIntact(t *testing.T) {
	pool, cleanup := setupWorkerTest(t)
	defer cleanup()

	payload := validBranchPayload()
	id := insertEvent(t, pool, EventTypeBranchHeadMoved, payload)

	h := newHandler(EventTypeBranchHeadMoved)
	w := NewWorker(pool, WithHandlers([]OutboxHandler{h}))

	require.NoError(t, w.poll(context.Background()))

	require.Equal(t, 1, h.callCount())
	assert.Equal(t, id, h.calls[0].ID)
	assert.Equal(t, EventTypeBranchHeadMoved, h.calls[0].EventType)

	var gotPayload map[string]any
	require.NoError(t, json.Unmarshal(h.calls[0].Payload, &gotPayload))
	assert.Equal(t, payload["commit_id"], gotPayload["commit_id"])
}

// handler fails

func TestWorker_Poll_HandlerError_EventRemainsUnprocessed(t *testing.T) {
	pool, cleanup := setupWorkerTest(t)
	defer cleanup()

	id := insertEvent(t, pool, EventTypeBranchHeadMoved, validBranchPayload())

	h := newFailingHandler(EventTypeBranchHeadMoved, errors.New("store unavailable"))
	w := NewWorker(pool, WithHandlers([]OutboxHandler{h}))

	require.NoError(t, w.poll(context.Background()))

	row := fetchEvent(t, pool, id)
	assert.False(t, row.processed, "failed event must remain unprocessed for retry")
	assert.Nil(t, row.processedAt)
}

// unknown event type

func TestWorker_Poll_UnknownEventType_MarkedProcessed(t *testing.T) {
	pool, cleanup := setupWorkerTest(t)
	defer cleanup()

	id := insertEvent(t, pool, "UnknownFutureEvent", map[string]any{"foo": "bar"})

	w := NewWorker(pool) // no handlers registered

	require.NoError(t, w.poll(context.Background()))

	row := fetchEvent(t, pool, id)
	assert.True(t, row.processed)
}

// empty outbox

func TestWorker_Poll_EmptyOutbox_ReturnsNil(t *testing.T) {
	pool, cleanup := setupWorkerTest(t)
	defer cleanup()

	w := NewWorker(pool)

	err := w.poll(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, countProcessed(t, pool))
}

// mixed batch (some succeed, some fail)

func TestWorker_Poll_MixedBatch_OnlySuccessfulEventsMarkedProcessed(t *testing.T) {
	pool, cleanup := setupWorkerTest(t)
	defer cleanup()

	goodID := insertEvent(t, pool, EventTypeBranchHeadMoved, validBranchPayload())
	failID := insertEvent(t, pool, EventTypeCommitCreated, map[string]any{"commit_id": "c1"})

	goodHandler := newHandler(EventTypeBranchHeadMoved)
	failHandler := newFailingHandler(EventTypeCommitCreated, errors.New("neo4j down"))
	w := NewWorker(pool, WithHandlers([]OutboxHandler{goodHandler, failHandler}))

	require.NoError(t, w.poll(context.Background()))

	assert.True(t, fetchEvent(t, pool, goodID).processed)
	assert.False(t, fetchEvent(t, pool, failID).processed)
}

// already processed events are skipped

func TestWorker_Poll_AlreadyProcessedEvent_NotDispatchedAgain(t *testing.T) {
	pool, cleanup := setupWorkerTest(t)
	defer cleanup()

	// insert an event and mark it processed manually
	id := insertEvent(t, pool, EventTypeBranchHeadMoved, validBranchPayload())
	_, err := pool.Exec(context.Background(),
		`UPDATE outbox_events SET processed = true, processed_at = now() WHERE id = $1`, id)
	require.NoError(t, err)

	h := newHandler(EventTypeBranchHeadMoved)
	w := NewWorker(pool, WithHandlers([]OutboxHandler{h}))

	require.NoError(t, w.poll(context.Background()))

	assert.Equal(t, 0, h.callCount(), "already-processed event must not be dispatched again")
}

// batch size respected

func TestWorker_Poll_BatchSize_LimitsEventsPerPoll(t *testing.T) {
	pool, cleanup := setupWorkerTest(t)
	defer cleanup()

	for i := 0; i < 5; i++ {
		insertEvent(t, pool, EventTypeBranchHeadMoved, validBranchPayload())
	}

	h := newHandler(EventTypeBranchHeadMoved)
	w := NewWorker(pool, WithHandlers([]OutboxHandler{h}), WithBatchSize(2))

	require.NoError(t, w.poll(context.Background()))

	assert.Equal(t, 2, h.callCount(), "worker must process at most batch-size events per poll")
	assert.Equal(t, 2, countProcessed(t, pool))
}

// publish succeeds

func TestWorker_Poll_EventBusMode_PublishSucceeds_AllMarkedProcessed(t *testing.T) {
	pool, cleanup := setupWorkerTest(t)
	defer cleanup()

	id1 := insertEvent(t, pool, EventTypeBranchHeadMoved, validBranchPayload())
	id2 := insertEvent(t, pool, EventTypeCommitCreated, map[string]any{"commit_id": "c1"})

	bus := &mockEventBus{}
	// Register handlers too - they must NOT be called in EventBus mode.
	h := newHandler(EventTypeBranchHeadMoved, EventTypeCommitCreated)
	w := NewWorker(pool, WithEventBus(bus), WithHandlers([]OutboxHandler{h}))

	require.NoError(t, w.poll(context.Background()))

	assert.True(t, fetchEvent(t, pool, id1).processed)
	assert.True(t, fetchEvent(t, pool, id2).processed)
	assert.Equal(t, 2, len(bus.published))
	assert.Equal(t, 0, h.callCount(), "in-process handlers must not be called in EventBus mode")
}

func TestWorker_Poll_EventBusMode_PublishedEventsContainCorrectIDs(t *testing.T) {
	pool, cleanup := setupWorkerTest(t)
	defer cleanup()

	id := insertEvent(t, pool, EventTypeBranchHeadMoved, validBranchPayload())

	bus := &mockEventBus{}
	w := NewWorker(pool, WithEventBus(bus))

	require.NoError(t, w.poll(context.Background()))

	require.Len(t, bus.published, 1)
	assert.Equal(t, id, bus.published[0].ID)
}

// publish fails

func TestWorker_Poll_EventBusMode_PublishFails_NothingMarkedProcessed(t *testing.T) {
	pool, cleanup := setupWorkerTest(t)
	defer cleanup()

	id := insertEvent(t, pool, EventTypeBranchHeadMoved, validBranchPayload())

	bus := &mockEventBus{err: errors.New("broker unreachable")}
	w := NewWorker(pool, WithEventBus(bus))

	err := w.poll(context.Background())
	require.Error(t, err, "poll must surface the publish error")

	row := fetchEvent(t, pool, id)
	assert.False(t, row.processed, "nothing must be marked processed when Publish fails")
}

// SKIP LOCKED ensures each event processed exactly once

func TestWorker_Poll_SkipLocked_ConcurrentWorkers_EachEventProcessedOnce(t *testing.T) {
	pool, cleanup := setupWorkerTest(t)
	defer cleanup()

	const numEvents = 10
	for i := 0; i < numEvents; i++ {
		insertEvent(t, pool, EventTypeBranchHeadMoved, validBranchPayload())
	}

	var mu sync.Mutex
	total := 0
	countingHandler := &mockHandler{
		types: []string{EventTypeBranchHeadMoved},
		handleFn: func(_ context.Context, _ OutboxEvent) error {
			mu.Lock()
			total++
			mu.Unlock()
			return nil
		},
	}

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := NewWorker(pool, WithHandlers([]OutboxHandler{countingHandler}))
			if err := w.poll(context.Background()); err != nil {
				t.Errorf("poll returned error: %v", err)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, numEvents, total,
		"each event must be dispatched exactly once across all concurrent workers")
	assert.Equal(t, numEvents, countProcessed(t, pool))
}

// events polled in created_at ASC order

func TestWorker_Poll_EventsDispatchedInCreatedAtAscOrder(t *testing.T) {
	pool, cleanup := setupWorkerTest(t)
	defer cleanup()

	base := time.Now().UTC().Truncate(time.Millisecond)
	ids := make([]string, 3)
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("ordered-evt-%d", i)
		b, _ := json.Marshal(validBranchPayload())
		_, err := pool.Exec(context.Background(),
			`INSERT INTO outbox_events (id, event_type, payload, created_at, processed)
			 VALUES ($1, $2, $3, $4, false)`,
			id, EventTypeBranchHeadMoved, b, base.Add(time.Duration(i)*10*time.Millisecond),
		)
		require.NoError(t, err)
		ids[i] = id
	}

	h := newHandler(EventTypeBranchHeadMoved)
	w := NewWorker(pool, WithHandlers([]OutboxHandler{h}))
	require.NoError(t, w.poll(context.Background()))

	require.Len(t, h.calls, 3)
	for i, call := range h.calls {
		assert.Equal(t, ids[i], call.ID,
			"event %d must be dispatched in created_at ASC order", i)
	}
}

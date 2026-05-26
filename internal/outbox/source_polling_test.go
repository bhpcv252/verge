//go:build integration

package outbox

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/testhelper"
)

// helpers

func newTestPollingSource(t *testing.T, batch int) (*PollingSource, func()) {
	t.Helper()
	pool, cleanup := testhelper.SetupPostgres(t)
	return NewPollingSource(pool, 0, batch), cleanup
}

func insertForPolling(t *testing.T, src *PollingSource, eventType string, payload any) string {
	t.Helper()

	id := "ps_" + testhelper.UniqueID("test")
	b, err := json.Marshal(payload)
	require.NoError(t, err)

	_, err = src.db.Exec(context.Background(),
		`INSERT INTO outbox_events (id, event_type, payload, created_at, processed)
		 VALUES ($1, $2, $3, now(), false)`,
		id, eventType, b,
	)
	require.NoError(t, err)
	return id
}

func isProcessed(t *testing.T, src *PollingSource, id string) bool {
	t.Helper()
	var processed bool
	err := src.db.QueryRow(context.Background(),
		`SELECT processed FROM outbox_events WHERE id = $1`, id,
	).Scan(&processed)
	require.NoError(t, err)
	return processed
}

// Start / Close / Name

func TestPollingSource_Start_ReturnsNil(t *testing.T) {
	src, cleanup := newTestPollingSource(t, 100)
	defer cleanup()

	err := src.Start(context.Background())
	require.NoError(t, err)
}

func TestPollingSource_Close_ReturnsNil(t *testing.T) {
	src, cleanup := newTestPollingSource(t, 100)
	defer cleanup()

	err := src.Close()
	require.NoError(t, err)
}

func TestPollingSource_Name_ReturnsPolling(t *testing.T) {
	src, cleanup := newTestPollingSource(t, 100)
	defer cleanup()

	assert.Equal(t, "polling", src.Name())
}

// Next

func TestPollingSource_Next_ReturnsUnprocessedEvents(t *testing.T) {
	src, cleanup := newTestPollingSource(t, 100)
	defer cleanup()

	id := insertForPolling(t, src, EventTypeBranchHeadMoved, validBranchPayload())

	events, err := src.Next(context.Background())

	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, id, events[0].ID)
	assert.Equal(t, EventTypeBranchHeadMoved, events[0].EventType)
	assert.NotNil(t, events[0].Payload)
	assert.False(t, events[0].CreatedAt.IsZero())
}

func TestPollingSource_Next_EmptyTable_ReturnsEmptySlice(t *testing.T) {
	src, cleanup := newTestPollingSource(t, 100)
	defer cleanup()

	events, err := src.Next(context.Background())

	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestPollingSource_Next_RespectsLimit(t *testing.T) {
	src, cleanup := newTestPollingSource(t, 3) // batch=3
	defer cleanup()

	for i := 0; i < 6; i++ {
		insertForPolling(t, src, EventTypeBranchHeadMoved, validBranchPayload())
	}

	events, err := src.Next(context.Background())

	require.NoError(t, err)
	assert.Len(t, events, 3, "Next must respect the batch size limit")
}

func TestPollingSource_Next_SkipsAlreadyProcessed(t *testing.T) {
	src, cleanup := newTestPollingSource(t, 100)
	defer cleanup()

	processedID := insertForPolling(t, src, EventTypeBranchHeadMoved, validBranchPayload())
	_, err := src.db.Exec(context.Background(),
		`UPDATE outbox_events SET processed = true, processed_at = now() WHERE id = $1`,
		processedID,
	)
	require.NoError(t, err)

	pendingID := insertForPolling(t, src, EventTypeBranchHeadMoved, validBranchPayload())

	events, err := src.Next(context.Background())

	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, pendingID, events[0].ID, "already-processed event must not be returned")
}

func TestPollingSource_Next_ReturnsEventsInCreatedAtAscOrder(t *testing.T) {
	src, cleanup := newTestPollingSource(t, 100)
	defer cleanup()

	base := time.Now().UTC().Truncate(time.Millisecond)
	ids := make([]string, 3)
	for i := 0; i < 3; i++ {
		id := testhelper.UniqueID("order")
		b, _ := json.Marshal(validBranchPayload())
		_, err := src.db.Exec(context.Background(),
			`INSERT INTO outbox_events (id, event_type, payload, created_at, processed)
			 VALUES ($1, $2, $3, $4, false)`,
			id, EventTypeBranchHeadMoved, b,
			base.Add(time.Duration(i)*10*time.Millisecond),
		)
		require.NoError(t, err)
		ids[i] = id
	}

	events, err := src.Next(context.Background())

	require.NoError(t, err)
	require.Len(t, events, 3)
	for i, e := range events {
		assert.Equal(t, ids[i], e.ID, "event[%d] should be %q (created_at ASC)", i, ids[i])
	}
}

func TestPollingSource_Next_PayloadDeserializesCorrectly(t *testing.T) {
	src, cleanup := newTestPollingSource(t, 100)
	defer cleanup()

	payload := map[string]any{
		"commit_id": "abc-123",
		"repo_id":   "my-repo",
	}
	insertForPolling(t, src, EventTypeCommitCreated, payload)

	events, err := src.Next(context.Background())

	require.NoError(t, err)
	require.Len(t, events, 1)

	var got map[string]any
	require.NoError(t, json.Unmarshal(events[0].Payload, &got))
	assert.Equal(t, "abc-123", got["commit_id"])
	assert.Equal(t, "my-repo", got["repo_id"])
}

// Ack

func TestPollingSource_Ack_MarksEventsAsProcessed(t *testing.T) {
	src, cleanup := newTestPollingSource(t, 100)
	defer cleanup()

	id1 := insertForPolling(t, src, EventTypeBranchHeadMoved, validBranchPayload())
	id2 := insertForPolling(t, src, EventTypeBranchHeadMoved, validBranchPayload())

	err := src.Ack(context.Background(), []string{id1, id2})
	require.NoError(t, err)

	assert.True(t, isProcessed(t, src, id1))
	assert.True(t, isProcessed(t, src, id2))
}

func TestPollingSource_Ack_SetsProcessedAt(t *testing.T) {
	src, cleanup := newTestPollingSource(t, 100)
	defer cleanup()

	id := insertForPolling(t, src, EventTypeBranchHeadMoved, validBranchPayload())

	before := time.Now().Add(-time.Second)
	require.NoError(t, src.Ack(context.Background(), []string{id}))
	after := time.Now().Add(time.Second)

	var processedAt *time.Time
	err := src.db.QueryRow(context.Background(),
		`SELECT processed_at FROM outbox_events WHERE id = $1`, id,
	).Scan(&processedAt)
	require.NoError(t, err)
	require.NotNil(t, processedAt)
	assert.True(t, processedAt.After(before), "processed_at should be after test start")
	assert.True(t, processedAt.Before(after), "processed_at should be before test end")
}

func TestPollingSource_Ack_EmptyIDs_NoError(t *testing.T) {
	src, cleanup := newTestPollingSource(t, 100)
	defer cleanup()

	err := src.Ack(context.Background(), []string{})
	require.NoError(t, err)
}

func TestPollingSource_Ack_NilIDs_NoError(t *testing.T) {
	src, cleanup := newTestPollingSource(t, 100)
	defer cleanup()

	err := src.Ack(context.Background(), nil)
	require.NoError(t, err)
}

func TestPollingSource_Ack_OnlyAckedEventsMarkedProcessed(t *testing.T) {
	src, cleanup := newTestPollingSource(t, 100)
	defer cleanup()

	ackID := insertForPolling(t, src, EventTypeBranchHeadMoved, validBranchPayload())
	skipID := insertForPolling(t, src, EventTypeBranchHeadMoved, validBranchPayload())

	require.NoError(t, src.Ack(context.Background(), []string{ackID}))

	assert.True(t, isProcessed(t, src, ackID))
	assert.False(t, isProcessed(t, src, skipID), "non-acked event must remain unprocessed")
}

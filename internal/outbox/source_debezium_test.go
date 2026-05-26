package outbox

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helpers

func newDebeziumSourceForTest() *DebeziumSource {
	return &DebeziumSource{
		batch:          100,
		pendingCommits: make([]kafka.Message, 0, 100),
	}
}

func debeziumEnvelope(op string, after map[string]any) []byte {
	env := map[string]any{
		"before": nil,
		"after":  after,
		"op":     op,
		"ts_ms":  time.Now().UnixMilli(),
	}
	b, _ := json.Marshal(env)
	return b
}

func validAfter(id, eventType string, processed bool) map[string]any {
	return map[string]any{
		"id":           id,
		"event_type":   eventType,
		"payload":      json.RawMessage(`{"key":"value"}`),
		"created_at":   time.Now().UnixMilli(),
		"processed":    processed,
		"processed_at": nil,
	}
}

func kafkaMsg(body []byte) kafka.Message {
	return kafka.Message{Value: body}
}

// Name

func TestDebeziumSource_Name(t *testing.T) {
	src := newDebeziumSourceForTest()
	assert.Equal(t, "debezium", src.Name())
}

// parseDebeziumMessage

func TestParseDebeziumMessage_CreateOp_Unprocessed_ReturnsEvent(t *testing.T) {
	src := newDebeziumSourceForTest()

	msg := kafkaMsg(debeziumEnvelope("c", validAfter("evt-1", EventTypeCommitCreated, false)))
	event, err := src.parseDebeziumMessage(msg)

	require.NoError(t, err)
	assert.Equal(t, "evt-1", event.ID)
	assert.Equal(t, EventTypeCommitCreated, event.EventType)
	assert.NotNil(t, event.Payload)
	assert.False(t, event.CreatedAt.IsZero())
}

func TestParseDebeziumMessage_ReadOp_Snapshot_ReturnsEvent(t *testing.T) {
	src := newDebeziumSourceForTest()

	msg := kafkaMsg(debeziumEnvelope("r", validAfter("evt-snap", EventTypeBranchHeadMoved, false)))
	event, err := src.parseDebeziumMessage(msg)

	require.NoError(t, err)
	assert.Equal(t, "evt-snap", event.ID)
	assert.Equal(t, EventTypeBranchHeadMoved, event.EventType)
}

func TestParseDebeziumMessage_UpdateOp_Unprocessed_ReturnsEvent(t *testing.T) {
	src := newDebeziumSourceForTest()

	msg := kafkaMsg(debeziumEnvelope("u", validAfter("evt-upd", EventTypeCommitCreated, false)))
	event, err := src.parseDebeziumMessage(msg)

	require.NoError(t, err)
	assert.Equal(t, "evt-upd", event.ID)
}

func TestParseDebeziumMessage_CreateOp_AlreadyProcessed_ReturnsError(t *testing.T) {
	src := newDebeziumSourceForTest()

	msg := kafkaMsg(debeziumEnvelope("c", validAfter("evt-1", EventTypeCommitCreated, true)))
	_, err := src.parseDebeziumMessage(msg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "skipping already processed event")
}

func TestParseDebeziumMessage_UpdateOp_AlreadyProcessed_ReturnsError(t *testing.T) {
	src := newDebeziumSourceForTest()

	msg := kafkaMsg(debeziumEnvelope("u", validAfter("evt-1", EventTypeCommitCreated, true)))
	_, err := src.parseDebeziumMessage(msg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "skipping processed event")
}

func TestParseDebeziumMessage_DeleteOp_ReturnsError(t *testing.T) {
	src := newDebeziumSourceForTest()

	envelope := map[string]any{
		"before": validAfter("evt-1", EventTypeCommitCreated, true),
		"after":  nil,
		"op":     "d",
		"ts_ms":  time.Now().UnixMilli(),
	}
	b, _ := json.Marshal(envelope)

	_, err := src.parseDebeziumMessage(kafkaMsg(b))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "skipping delete operation")
}

func TestParseDebeziumMessage_UnknownOp_ReturnsError(t *testing.T) {
	src := newDebeziumSourceForTest()

	for _, op := range []string{"x", "t", "m", ""} {
		op := op
		t.Run("op="+op, func(t *testing.T) {
			msg := kafkaMsg(
				debeziumEnvelope(op, validAfter("evt-1", EventTypeCommitCreated, false)),
			)
			_, err := src.parseDebeziumMessage(msg)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown operation")
		})
	}
}

func TestParseDebeziumMessage_InvalidJSON_ReturnsError(t *testing.T) {
	src := newDebeziumSourceForTest()

	_, err := src.parseDebeziumMessage(kafkaMsg([]byte(`not-json`)))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal debezium envelope")
}

func TestParseDebeziumMessage_EmptyBody_ReturnsError(t *testing.T) {
	src := newDebeziumSourceForTest()

	_, err := src.parseDebeziumMessage(kafkaMsg([]byte(`{}`)))

	// op="" is unknown, so we expect an error
	require.Error(t, err)
}

func TestParseDebeziumMessage_NullAfterOnCreate_ReturnsError(t *testing.T) {
	src := newDebeziumSourceForTest()

	envelope := map[string]any{
		"before": nil,
		"after":  nil, // null after on create
		"op":     "c",
		"ts_ms":  time.Now().UnixMilli(),
	}
	b, _ := json.Marshal(envelope)

	_, err := src.parseDebeziumMessage(kafkaMsg(b))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "after is null for op=c")
}

func TestParseDebeziumMessage_NullAfterOnUpdate_ReturnsError(t *testing.T) {
	src := newDebeziumSourceForTest()

	envelope := map[string]any{
		"before": validAfter("evt-1", EventTypeCommitCreated, false),
		"after":  nil,
		"op":     "u",
		"ts_ms":  time.Now().UnixMilli(),
	}
	b, _ := json.Marshal(envelope)

	_, err := src.parseDebeziumMessage(kafkaMsg(b))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "after is null for op=u")
}

func TestParseDebeziumMessage_FieldsAreMappedCorrectly(t *testing.T) {
	src := newDebeziumSourceForTest()

	const (
		wantID        = "evt-field-test"
		wantEventType = EventTypeCommitCreated
	)
	wantPayload := `{"commit_id":"abc-123","repo_id":"my-repo"}`

	after := map[string]any{
		"id":         wantID,
		"event_type": wantEventType,
		"payload":    json.RawMessage(wantPayload), // embed as JSON object
		"created_at": int64(1_700_000_000_000),     // known Unix millisecond value
		"processed":  false,
	}

	msg := kafkaMsg(debeziumEnvelope("c", after))
	event, err := src.parseDebeziumMessage(msg)

	require.NoError(t, err)
	assert.Equal(t, wantID, event.ID)
	assert.Equal(t, wantEventType, event.EventType)

	// payload should be the raw JSON string stored in the DB column
	var gotPayload map[string]any
	require.NoError(t, json.Unmarshal(event.Payload, &gotPayload))
	assert.Equal(t, "abc-123", gotPayload["commit_id"])
	assert.Equal(t, "my-repo", gotPayload["repo_id"])
}

func TestParseDebeziumMessage_CreatedAt_ConvertedFromUnixMillis(t *testing.T) {
	src := newDebeziumSourceForTest()

	// use a known timestamp: 2024-01-15 00:00:00 UTC = 1705276800000 ms
	const unixMs = int64(1_705_276_800_000)
	want := time.UnixMilli(unixMs).UTC()

	after := map[string]any{
		"id":         "evt-ts",
		"event_type": EventTypeCommitCreated,
		"payload":    `{}`,
		"created_at": unixMs,
		"processed":  false,
	}

	msg := kafkaMsg(debeziumEnvelope("c", after))
	event, err := src.parseDebeziumMessage(msg)

	require.NoError(t, err)
	assert.Equal(t, want, event.CreatedAt.UTC())
}

// Ack

func TestDebeziumSource_Ack_NoPendingCommits_NoError(t *testing.T) {
	src := newDebeziumSourceForTest()

	// no pendingCommits set, so Ack should short-circuit before touching the context
	err := src.Ack(context.Background(), []string{"evt-1"})
	require.NoError(t, err)
}

func TestDebeziumSource_Ack_ClearsPendingCommits(t *testing.T) {
	src := newDebeziumSourceForTest()

	// manually add a fake message to pendingCommits to simulate a prior Next()
	src.pendingCommits = append(src.pendingCommits, kafka.Message{})

	assert.Len(t, src.pendingCommits, 1, "precondition: one pending commit")
}

// NewDebeziumSource

func TestNewDebeziumSource_SetsConfig(t *testing.T) {
	cfg := DebeziumConfig{
		Brokers: []string{"kafka-1:9092", "kafka-2:9092"},
		Topic:   "verge.outbox.events",
		GroupID: "verge-worker",
		Batch:   500,
	}

	src := NewDebeziumSource(cfg)

	assert.Equal(t, 500, src.batch)
	assert.NotNil(t, src.reader)
	assert.Equal(t, "verge.outbox.events", src.reader.Config().Topic)
	assert.Equal(t, "verge-worker", src.reader.Config().GroupID)
	assert.Equal(t, []string{"kafka-1:9092", "kafka-2:9092"}, src.reader.Config().Brokers)
	assert.Equal(t, time.Duration(0), src.reader.Config().CommitInterval,
		"CommitInterval must be 0 for manual commits")

	// clean up reader to avoid goroutine leaks
	_ = src.reader.Close()
}

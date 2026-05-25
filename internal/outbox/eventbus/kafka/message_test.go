package kafka

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/outbox"
)

// helpers

func makeEvent(id, eventType string, payload json.RawMessage) outbox.OutboxEvent {
	return outbox.OutboxEvent{
		ID:        id,
		EventType: eventType,
		Payload:   payload,
		CreatedAt: time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
	}
}

// toKafkaMessage

func TestToKafkaMessage_AllFieldsMapped(t *testing.T) {
	ts := time.Date(2024, 6, 15, 12, 30, 0, 0, time.UTC)
	event := outbox.OutboxEvent{
		ID:        "evt-abc",
		EventType: "CommitCreated",
		Payload:   json.RawMessage(`{"commit_id":"c1"}`),
		CreatedAt: ts,
	}

	km := toKafkaMessage(event)

	assert.Equal(t, event.ID, km.ID)
	assert.Equal(t, event.EventType, km.EventType)
	assert.Equal(t, json.RawMessage(event.Payload), km.Payload)
	assert.Equal(t, ts, km.CreatedAt)
}

func TestToKafkaMessage_PayloadPreservedVerbatim(t *testing.T) {
	raw := json.RawMessage(`{"a":1,"b":"hello","c":true}`)
	km := toKafkaMessage(makeEvent("e1", "CommitCreated", raw))

	assert.Equal(t, raw, km.Payload)
}

func TestToKafkaMessage_EmptyPayload(t *testing.T) {
	km := toKafkaMessage(makeEvent("e1", "CommitCreated", json.RawMessage(`{}`)))
	assert.Equal(t, json.RawMessage(`{}`), km.Payload)
}

func TestToKafkaMessage_NilPayload(t *testing.T) {
	km := toKafkaMessage(makeEvent("e1", "CommitCreated", nil))
	assert.Nil(t, km.Payload)
}

// fromKafkaMessage

func TestFromKafkaMessage_AllFieldsMapped(t *testing.T) {
	ts := time.Date(2024, 6, 15, 12, 30, 0, 0, time.UTC)
	km := kafkaMessage{
		ID:        "evt-xyz",
		EventType: "BranchHeadMoved",
		Payload:   json.RawMessage(`{"branch":"main"}`),
		CreatedAt: ts,
	}

	event := fromKafkaMessage(km)

	assert.Equal(t, km.ID, event.ID)
	assert.Equal(t, km.EventType, event.EventType)
	assert.Equal(t, km.Payload, event.Payload)
	assert.Equal(t, ts, event.CreatedAt)
}

func TestFromKafkaMessage_PayloadPreservedVerbatim(t *testing.T) {
	raw := json.RawMessage(`{"repo_id":"r1","commit_id":"c2"}`)
	event := fromKafkaMessage(kafkaMessage{
		ID: "e1", EventType: "CommitCreated", Payload: raw,
	})

	assert.Equal(t, raw, event.Payload)
}

// round-trip

func TestRoundTrip_OutboxEventToKafkaMessageAndBack(t *testing.T) {
	original := outbox.OutboxEvent{
		ID:        "evt-roundtrip",
		EventType: "CommitCreated",
		Payload:   json.RawMessage(`{"commit_id":"abc","repo_id":"r1","parent_ids":["p1"]}`),
		CreatedAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}

	got := fromKafkaMessage(toKafkaMessage(original))

	assert.Equal(t, original.ID, got.ID)
	assert.Equal(t, original.EventType, got.EventType)
	assert.Equal(t, original.Payload, got.Payload)
	assert.Equal(t, original.CreatedAt, got.CreatedAt)
}

// JSON serialisation

func TestKafkaMessage_JSONMarshal_AllFieldsPresent(t *testing.T) {
	ts := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	km := kafkaMessage{
		ID:        "evt-1",
		EventType: "CommitCreated",
		Payload:   json.RawMessage(`{"commit_id":"c1"}`),
		CreatedAt: ts,
	}

	b, err := json.Marshal(km)
	require.NoError(t, err)

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &decoded))

	assert.Contains(t, decoded, "id")
	assert.Contains(t, decoded, "event_type")
	assert.Contains(t, decoded, "payload")
	assert.Contains(t, decoded, "created_at")
}

func TestKafkaMessage_JSONMarshal_FieldNames(t *testing.T) {
	km := kafkaMessage{
		ID:        "evt-field-names",
		EventType: "BranchHeadMoved",
		Payload:   json.RawMessage(`{}`),
		CreatedAt: time.Now(),
	}

	b, err := json.Marshal(km)
	require.NoError(t, err)

	assert.Contains(t, string(b), `"id"`)
	assert.Contains(t, string(b), `"event_type"`)
	assert.Contains(t, string(b), `"payload"`)
	assert.Contains(t, string(b), `"created_at"`)
}

func TestKafkaMessage_JSONMarshal_PayloadEmbeddedNotDoubleEncoded(t *testing.T) {
	km := kafkaMessage{
		ID:        "e1",
		EventType: "CommitCreated",
		Payload:   json.RawMessage(`{"commit_id":"c1"}`),
		CreatedAt: time.Now(),
	}

	b, err := json.Marshal(km)
	require.NoError(t, err)

	assert.NotContains(t, string(b), `"payload":"{`)
	assert.Contains(t, string(b), `"payload":{"commit_id"`)
}

func TestKafkaMessage_JSONUnmarshal_RoundTrip(t *testing.T) {
	original := kafkaMessage{
		ID:        "evt-json",
		EventType: "BranchHeadMoved",
		Payload:   json.RawMessage(`{"branch":"main","version":1234}`),
		CreatedAt: time.Date(2024, 3, 4, 5, 6, 7, 0, time.UTC),
	}

	b, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded kafkaMessage
	require.NoError(t, json.Unmarshal(b, &decoded))

	assert.Equal(t, original.ID, decoded.ID)
	assert.Equal(t, original.EventType, decoded.EventType)
	assert.Equal(t, original.Payload, decoded.Payload)
	assert.True(t, original.CreatedAt.Equal(decoded.CreatedAt))
}

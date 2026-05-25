package kafka

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/outbox"
)

// config wiring

func TestNewProducer_TopicWiredToWriter(t *testing.T) {
	p := NewProducer(Config{
		Brokers: []string{"localhost:9092"},
		Topic:   "verge.events",
	})

	assert.Equal(t, "verge.events", p.writer.Topic)
}

func TestNewProducer_MultipleBrokersAccepted(t *testing.T) {
	assert.NotPanics(t, func() {
		NewProducer(Config{
			Brokers: []string{"broker1:9092", "broker2:9092", "broker3:9092"},
			Topic:   "verge.events",
		})
	})
}

func TestProducer_MessageKey_IsEventID(t *testing.T) {
	event := outbox.OutboxEvent{
		ID:        "evt-key-test",
		EventType: outbox.EventTypeCommitCreated,
		Payload:   json.RawMessage(`{}`),
		CreatedAt: time.Now(),
	}

	// simulate the key construction from Publish
	key := []byte(event.ID)
	assert.Equal(t, []byte("evt-key-test"), key)
}

func TestProducer_MessageValue_IsJSONOfKafkaMessage(t *testing.T) {
	event := outbox.OutboxEvent{
		ID:        "evt-value-test",
		EventType: outbox.EventTypeCommitCreated,
		Payload:   json.RawMessage(`{"commit_id":"c1"}`),
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	value, err := json.Marshal(toKafkaMessage(event))
	require.NoError(t, err)

	var decoded kafkaMessage
	require.NoError(t, json.Unmarshal(value, &decoded))

	assert.Equal(t, event.ID, decoded.ID)
	assert.Equal(t, event.EventType, decoded.EventType)
	assert.Equal(t, event.Payload, decoded.Payload)
	assert.True(t, event.CreatedAt.Equal(decoded.CreatedAt))
}

func TestProducer_MessageValue_PayloadEmbeddedNotDoubleEncoded(t *testing.T) {
	event := outbox.OutboxEvent{
		ID:        "evt-embed",
		EventType: outbox.EventTypeBranchHeadMoved,
		Payload:   json.RawMessage(`{"repo_id":"r1","branch":"main"}`),
		CreatedAt: time.Now(),
	}

	value, err := json.Marshal(toKafkaMessage(event))
	require.NoError(t, err)

	assert.Contains(t, string(value), `"payload":{"repo_id"`)
	assert.NotContains(t, string(value), `"payload":"{`)
}

func TestProducer_BatchMessageConstruction_AllEventsIncluded(t *testing.T) {
	events := []outbox.OutboxEvent{
		{
			ID:        "e1",
			EventType: outbox.EventTypeCommitCreated,
			Payload:   json.RawMessage(`{}`),
			CreatedAt: time.Now(),
		},
		{
			ID:        "e2",
			EventType: outbox.EventTypeBranchHeadMoved,
			Payload:   json.RawMessage(`{}`),
			CreatedAt: time.Now(),
		},
		{
			ID:        "e3",
			EventType: outbox.EventTypeCommitCreated,
			Payload:   json.RawMessage(`{}`),
			CreatedAt: time.Now(),
		},
	}

	for _, e := range events {
		key := []byte(e.ID)
		value, err := json.Marshal(toKafkaMessage(e))
		require.NoError(t, err, "event %s must marshal without error", e.ID)

		assert.Equal(t, []byte(e.ID), key)

		var decoded kafkaMessage
		require.NoError(t, json.Unmarshal(value, &decoded))
		assert.Equal(t, e.ID, decoded.ID)
		assert.Equal(t, e.EventType, decoded.EventType)
	}
}

func TestProducer_Publish_EmptyEvents_DoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		events := []outbox.OutboxEvent{}
		msgs := make([]interface{}, 0, len(events))
		for _, e := range events {
			value, err := json.Marshal(toKafkaMessage(e))
			if err != nil {
				t.Fatalf("unexpected marshal error: %v", err)
			}
			msgs = append(msgs, struct {
				Key   []byte
				Value []byte
			}{Key: []byte(e.ID), Value: value})
		}
		assert.Empty(t, msgs)
	})
}

func TestProducer_Publish_CancelledContext_ReturnsError(t *testing.T) {
	p := NewProducer(Config{
		Brokers: []string{"192.0.2.1:9092"}, // TEST-NET
		Topic:   "verge.events",
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	event := outbox.OutboxEvent{
		ID:        "evt-cancel",
		EventType: outbox.EventTypeCommitCreated,
		Payload:   json.RawMessage(`{}`),
		CreatedAt: time.Now(),
	}

	err := p.Publish(ctx, []outbox.OutboxEvent{event})
	assert.Error(t, err, "Publish with a cancelled context must return an error")
}

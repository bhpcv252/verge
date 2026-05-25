package kafka

import (
	"encoding/json"
	"time"

	"github.com/bhpcv252/verge/internal/outbox"
)

type kafkaMessage struct {
	ID        string          `json:"id"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

func toKafkaMessage(e outbox.OutboxEvent) kafkaMessage {
	return kafkaMessage{
		ID:        e.ID,
		EventType: e.EventType,
		Payload:   e.Payload,
		CreatedAt: e.CreatedAt,
	}
}

func fromKafkaMessage(m kafkaMessage) outbox.OutboxEvent {
	return outbox.OutboxEvent{
		ID:        m.ID,
		EventType: m.EventType,
		Payload:   m.Payload,
		CreatedAt: m.CreatedAt,
	}
}

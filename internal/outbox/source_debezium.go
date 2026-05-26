package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

type DebeziumSource struct {
	reader         *kafka.Reader
	batch          int
	pendingCommits []kafka.Message
}

type DebeziumConfig struct {
	Brokers []string // kafka broker addresses
	Topic   string   // kafka topic, e.g. "verge.outbox.events"
	GroupID string   // consumer group ID
	Batch   int      // max events to fetch per batch
}

func NewDebeziumSource(cfg DebeziumConfig) *DebeziumSource {
	return &DebeziumSource{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        cfg.Brokers,
			Topic:          cfg.Topic,
			GroupID:        cfg.GroupID,
			MinBytes:       1,
			MaxBytes:       10e6,
			CommitInterval: 0, // manual commits via Ack()
			StartOffset:    kafka.LastOffset,
		}),
		batch:          cfg.Batch,
		pendingCommits: make([]kafka.Message, 0, cfg.Batch),
	}
}

func (s *DebeziumSource) Start(ctx context.Context) error {
	log.Printf("debezium source: connected to topic=%s group=%s brokers=%v",
		s.reader.Config().Topic,
		s.reader.Config().GroupID,
		s.reader.Config().Brokers)
	return nil
}

func (s *DebeziumSource) Next(ctx context.Context) ([]OutboxEvent, error) {
	var events []OutboxEvent
	s.pendingCommits = s.pendingCommits[:0] // clear previous batch

	// fetch first message
	firstMsg, err := s.reader.FetchMessage(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("fetch first message: %w", err)
	}

	// parse first message
	event, err := s.parseDebeziumMessage(firstMsg)
	if err != nil {
		log.Printf("debezium source: parse error (offset=%d partition=%d): %v",
			firstMsg.Offset, firstMsg.Partition, err)
		// commit bad message to skip it and prevent infinite loop
		if commitErr := s.reader.CommitMessages(ctx, firstMsg); commitErr != nil {
			return nil, fmt.Errorf("commit bad message: %w", commitErr)
		}
		// recurse to get next message
		return s.Next(ctx)
	}

	events = append(events, event)
	s.pendingCommits = append(s.pendingCommits, firstMsg)

	// try to fetch more messages without blocking (up to batch size)
	for len(events) < s.batch {
		// create a short timeout context for non-blocking fetch
		fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
		msg, err := s.reader.FetchMessage(fetchCtx)
		cancel()

		if err != nil {
			if fetchCtx.Err() != nil || err == context.DeadlineExceeded {
				break
			}
			if ctx.Err() != nil {
				break
			}
			return nil, fmt.Errorf("fetch message: %w", err)
		}

		// parse message
		event, err := s.parseDebeziumMessage(msg)
		if err != nil {
			log.Printf("debezium source: parse error (offset=%d partition=%d): %v",
				msg.Offset, msg.Partition, err)
			// commit bad message to skip it
			if commitErr := s.reader.CommitMessages(ctx, msg); commitErr != nil {
				log.Printf("debezium source: failed to commit bad message: %v", commitErr)
			}
			continue
		}

		events = append(events, event)
		s.pendingCommits = append(s.pendingCommits, msg)
	}

	return events, nil
}

func (s *DebeziumSource) parseDebeziumMessage(msg kafka.Message) (OutboxEvent, error) {
	var envelope struct {
		Before *json.RawMessage `json:"before"`
		After  *struct {
			ID        string          `json:"id"`
			EventType string          `json:"event_type"`
			Payload   json.RawMessage `json:"payload"`
			CreatedAt int64           `json:"created_at"` // unix milliseconds
			Processed bool            `json:"processed"`
		} `json:"after"`
		Op   string `json:"op"`
		TsMs int64  `json:"ts_ms"`
	}

	if err := json.Unmarshal(msg.Value, &envelope); err != nil {
		return OutboxEvent{}, fmt.Errorf("unmarshal debezium envelope: %w", err)
	}

	// handle different operation types
	switch envelope.Op {
	case "c", "r": // create or read (snapshot)
		if envelope.After == nil {
			return OutboxEvent{}, fmt.Errorf("after is null for op=%s", envelope.Op)
		}
		// only process unprocessed events
		if envelope.After.Processed {
			return OutboxEvent{}, fmt.Errorf("skipping already processed event")
		}

	case "u": // update
		if envelope.After == nil {
			return OutboxEvent{}, fmt.Errorf("after is null for op=%s", envelope.Op)
		}
		// skip if event was marked as processed
		if envelope.After.Processed {
			return OutboxEvent{}, fmt.Errorf("skipping processed event (op=u)")
		}

	case "d": // delete
		// we don't delete outbox events
		return OutboxEvent{}, fmt.Errorf("skipping delete operation")

	default:
		return OutboxEvent{}, fmt.Errorf("unknown operation: %s", envelope.Op)
	}

	// convert Unix milliseconds to time.Time
	createdAt := time.UnixMilli(envelope.After.CreatedAt)

	return OutboxEvent{
		ID:        envelope.After.ID,
		EventType: envelope.After.EventType,
		Payload:   envelope.After.Payload,
		CreatedAt: createdAt,
	}, nil
}

func (s *DebeziumSource) Ack(ctx context.Context, eventIDs []string) error {
	if len(s.pendingCommits) == 0 {
		return nil
	}

	// commit all messages from the last batch
	if err := s.reader.CommitMessages(ctx, s.pendingCommits...); err != nil {
		return fmt.Errorf("commit kafka messages: %w", err)
	}

	s.pendingCommits = s.pendingCommits[:0]
	return nil
}

func (s *DebeziumSource) Close() error {
	if err := s.reader.Close(); err != nil {
		return fmt.Errorf("close kafka reader: %w", err)
	}
	return nil
}

func (s *DebeziumSource) Name() string {
	return "debezium"
}

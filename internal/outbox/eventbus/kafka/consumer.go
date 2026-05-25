package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/bhpcv252/verge/internal/outbox"
)

type Consumer struct {
	reader   *kafkago.Reader
	handlers []outbox.OutboxHandler
}

type ConsumerConfig struct {
	Brokers []string
	Topic   string
	GroupID string
}

func NewConsumer(cfg ConsumerConfig, handlers []outbox.OutboxHandler) *Consumer {
	return &Consumer{
		reader: kafkago.NewReader(kafkago.ReaderConfig{
			Brokers:  cfg.Brokers,
			Topic:    cfg.Topic,
			GroupID:  cfg.GroupID,
			MinBytes: 1,
			MaxBytes: 10e6,
		}),
		handlers: handlers,
	}
}

func (c *Consumer) Run(ctx context.Context) error {
	log.Printf("kafka consumer: started (group=%s)", c.reader.Config().GroupID)
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Println("kafka consumer: stopped")
				return nil
			}
			return fmt.Errorf("kafka consumer: fetch: %w", err)
		}

		var km kafkaMessage
		if err := json.Unmarshal(msg.Value, &km); err != nil {
			log.Printf("kafka consumer: unmarshal error (skipping offset %d): %v", msg.Offset, err)
			_ = c.reader.CommitMessages(ctx, msg)
			continue
		}

		event := fromKafkaMessage(km)
		if err := c.dispatch(ctx, event); err != nil {
			log.Printf("kafka consumer: dispatch %s (%s): %v", event.ID, event.EventType, err)
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			return fmt.Errorf("kafka consumer: commit: %w", err)
		}
	}
}

func (c *Consumer) Close() error { return c.reader.Close() }

func (c *Consumer) dispatch(ctx context.Context, event outbox.OutboxEvent) error {
	for _, h := range c.handlers {
		for _, et := range h.EventTypes() {
			if et == event.EventType {
				if err := h.Handle(ctx, event); err != nil {
					return fmt.Errorf("handler %T: %w", h, err)
				}
				break
			}
		}
	}
	return nil
}

package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/bhpcv252/verge/internal/outbox"
)

type Producer struct {
	writer *kafkago.Writer
}

type Config struct {
	Brokers []string
	Topic   string
}

func NewProducer(cfg Config) *Producer {
	return &Producer{
		writer: &kafkago.Writer{
			Addr:         kafkago.TCP(cfg.Brokers...),
			Topic:        cfg.Topic,
			Balancer:     &kafkago.LeastBytes{},
			RequiredAcks: kafkago.RequireAll,
			Compression:  kafkago.Snappy,
			BatchTimeout: 5 * time.Millisecond,
		},
	}
}

func (p *Producer) Publish(ctx context.Context, events []outbox.OutboxEvent) error {
	msgs := make([]kafkago.Message, 0, len(events))
	for _, e := range events {
		value, err := json.Marshal(toKafkaMessage(e))
		if err != nil {
			return fmt.Errorf("kafka producer: marshal event %s: %w", e.ID, err)
		}
		msgs = append(msgs, kafkago.Message{
			Key:   []byte(e.ID),
			Value: value,
		})
	}
	if err := p.writer.WriteMessages(ctx, msgs...); err != nil {
		return fmt.Errorf("kafka producer: write messages: %w", err)
	}
	return nil
}

func (p *Producer) Close() error { return p.writer.Close() }

func CreateTopic(ctx context.Context, broker, topic string, partitions, replication int) error {
	conn, err := kafkago.DialContext(ctx, "tcp", broker)
	if err != nil {
		return fmt.Errorf("kafka: dial %s: %w", broker, err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("kafka: get controller: %w", err)
	}

	ctrl, err := kafkago.DialContext(ctx, "tcp",
		net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return fmt.Errorf("kafka: dial controller: %w", err)
	}
	defer ctrl.Close()

	return ctrl.CreateTopics(kafkago.TopicConfig{
		Topic:             topic,
		NumPartitions:     partitions,
		ReplicationFactor: replication,
	})
}

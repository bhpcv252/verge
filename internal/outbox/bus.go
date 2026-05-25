package outbox

import "context"

// implement EventBus to route outbox events through your own broker (Kafka, SQS, RabbitMQ,
// Pub/Sub, etc.) instead of dispatching in-process
type EventBus interface {
	Publish(ctx context.Context, events []OutboxEvent) error
}

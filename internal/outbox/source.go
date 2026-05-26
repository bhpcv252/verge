package outbox

import "context"

// EventSource abstracts where outbox events come from

type EventSource interface {
	Start(ctx context.Context) error

	// next returns the next batch of events to process
	Next(ctx context.Context) ([]OutboxEvent, error)

	// ack marks events as successfully processed
	Ack(ctx context.Context, eventIDs []string) error

	Close() error

	Name() string
}

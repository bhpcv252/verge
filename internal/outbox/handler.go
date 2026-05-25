package outbox

import "context"

type OutboxHandler interface {
	EventTypes() []string

	Handle(ctx context.Context, event OutboxEvent) error
}

package outbox

import (
	"context"
	"fmt"
	"log"
)

type Worker struct {
	source   EventSource
	handlers []OutboxHandler
	bus      EventBus // nil = in-process dispatch
}

type Option func(*Worker)

func WithSource(source EventSource) Option {
	return func(w *Worker) { w.source = source }
}

func WithHandlers(handlers []OutboxHandler) Option {
	return func(w *Worker) { w.handlers = handlers }
}

func WithEventBus(bus EventBus) Option {
	return func(w *Worker) { w.bus = bus }
}

func NewWorker(opts ...Option) *Worker {
	w := &Worker{}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (w *Worker) Run(ctx context.Context) error {
	if w.source == nil {
		return fmt.Errorf("worker: source is required (use WithSource)")
	}

	log.Printf("outbox worker: starting with source=%s", w.source.Name())

	if err := w.source.Start(ctx); err != nil {
		return fmt.Errorf("start source: %w", err)
	}
	defer func() {
		if err := w.source.Close(); err != nil {
			log.Printf("outbox worker: close source error: %v", err)
		}
	}()

	for {
		// fetch next batch of events
		events, err := w.source.Next(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Println("outbox worker: stopped")
				return nil
			}
			log.Printf("outbox worker: source error: %v", err)
			continue
		}

		if len(events) == 0 {
			continue
		}

		log.Printf("outbox worker: processing %d events", len(events))

		// process events
		processed := w.processEvents(ctx, events)

		// acknowledge successfully processed events
		if len(processed) > 0 {
			if err := w.source.Ack(ctx, processed); err != nil {
				log.Printf("outbox worker: ack error: %v", err)
				// continue even if ack fails, source will retry on next batch
			}
		}
	}
}

func (w *Worker) processEvents(ctx context.Context, events []OutboxEvent) []string {
	var processed []string

	if w.bus != nil {
		// EventBus mode: publish all events to external broker
		if err := w.bus.Publish(ctx, events); err != nil {
			log.Printf("outbox worker: eventbus publish error: %v", err)
			return processed
		}
		for _, e := range events {
			processed = append(processed, e.ID)
		}
	} else {
		// In-process mode: dispatch to registered handlers
		for _, e := range events {
			if err := w.dispatch(ctx, e); err != nil {
				log.Printf("outbox worker: dispatch %s (%s) failed: %v",
					e.ID, e.EventType, err)
				continue
			}
			processed = append(processed, e.ID)
		}
	}

	return processed
}

// dispatch calls every registered handler whose EventTypes matches this event
func (w *Worker) dispatch(ctx context.Context, event OutboxEvent) error {
	matched := false
	for _, h := range w.handlers {
		for _, et := range h.EventTypes() {
			if et == event.EventType {
				matched = true
				if err := h.Handle(ctx, event); err != nil {
					return fmt.Errorf("handler %T: %w", h, err)
				}
				break
			}
		}
	}
	if !matched {
		log.Printf(
			"outbox worker: no handler for event type %q (id=%s), skipping",
			event.EventType,
			event.ID,
		)
	}
	return nil
}

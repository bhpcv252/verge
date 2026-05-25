package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	db       *pgxpool.Pool
	handlers []OutboxHandler
	bus      EventBus // nil = in-process dispatch
	interval time.Duration
	batch    int
}

type Option func(*Worker)

func WithHandlers(handlers []OutboxHandler) Option {
	return func(w *Worker) { w.handlers = handlers }
}

func WithEventBus(bus EventBus) Option {
	return func(w *Worker) { w.bus = bus }
}

func WithInterval(d time.Duration) Option {
	return func(w *Worker) {
		if d > 0 {
			w.interval = d
		}
	}
}

func WithBatchSize(n int) Option {
	return func(w *Worker) {
		if n > 0 {
			w.batch = n
		}
	}
}

func NewWorker(db *pgxpool.Pool, opts ...Option) *Worker {
	w := &Worker{
		db:       db,
		interval: 500 * time.Millisecond,
		batch:    100,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (w *Worker) Run(ctx context.Context) {
	log.Println("outbox worker: started")
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("outbox worker: stopped")
			return
		case <-ticker.C:
			if err := w.poll(ctx); err != nil {
				log.Printf("outbox worker: poll error: %v", err)
			}
		}
	}
}

func (w *Worker) poll(ctx context.Context) error {
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id, event_type, payload, created_at
		FROM outbox_events
		WHERE processed = false
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED`,
		w.batch,
	)
	if err != nil {
		return fmt.Errorf("query outbox: %w", err)
	}

	var events []OutboxEvent
	for rows.Next() {
		var e OutboxEvent
		var payload []byte
		if err := rows.Scan(&e.ID, &e.EventType, &payload, &e.CreatedAt); err != nil {
			rows.Close()
			return fmt.Errorf("scan outbox row: %w", err)
		}
		e.Payload = json.RawMessage(payload)
		events = append(events, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("outbox rows: %w", err)
	}

	if len(events) == 0 {
		return tx.Commit(ctx)
	}

	var processed []string

	if w.bus != nil {
		if err := w.bus.Publish(ctx, events); err != nil {
			return fmt.Errorf("eventbus publish: %w", err)
		}
		for _, e := range events {
			processed = append(processed, e.ID)
		}
	} else {
		for _, e := range events {
			if err := w.dispatch(ctx, e); err != nil {
				log.Printf("outbox worker: dispatch %s (%s) failed: %v", e.ID, e.EventType, err)
				continue
			}
			processed = append(processed, e.ID)
		}
	}

	if len(processed) == 0 {
		return tx.Commit(ctx)
	}

	_, err = tx.Exec(ctx, `
		UPDATE outbox_events
		SET processed = true, processed_at = now()
		WHERE id = ANY($1)`,
		processed,
	)
	if err != nil {
		return fmt.Errorf("mark processed: %w", err)
	}

	return tx.Commit(ctx)
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

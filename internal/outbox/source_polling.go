package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PollingSource struct {
	db       *pgxpool.Pool
	interval time.Duration
	batch    int
	lastPoll time.Time
}

func NewPollingSource(db *pgxpool.Pool, interval time.Duration, batch int) *PollingSource {
	return &PollingSource{
		db:       db,
		interval: interval,
		batch:    batch,
		lastPoll: time.Now(),
	}
}

func (s *PollingSource) Start(ctx context.Context) error {
	// no initialization needed
	return nil
}

func (s *PollingSource) Next(ctx context.Context) ([]OutboxEvent, error) {
	// wait for the next poll interval
	nextPoll := s.lastPoll.Add(s.interval)
	now := time.Now()
	if now.Before(nextPoll) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(nextPoll.Sub(now)):
		}
	}
	s.lastPoll = time.Now()

	// query unprocessed events
	rows, err := s.db.Query(ctx, `
		SELECT id, event_type, payload, created_at
		FROM outbox_events
		WHERE processed = false
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED`,
		s.batch,
	)
	if err != nil {
		return nil, fmt.Errorf("query outbox: %w", err)
	}
	defer rows.Close()

	var events []OutboxEvent
	for rows.Next() {
		var e OutboxEvent
		var payload []byte
		if err := rows.Scan(&e.ID, &e.EventType, &payload, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan outbox row: %w", err)
		}
		e.Payload = json.RawMessage(payload)
		events = append(events, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox rows: %w", err)
	}

	return events, nil
}

func (s *PollingSource) Ack(ctx context.Context, eventIDs []string) error {
	if len(eventIDs) == 0 {
		return nil
	}

	_, err := s.db.Exec(ctx, `
		UPDATE outbox_events
		SET processed = true, processed_at = now()
		WHERE id = ANY($1)`,
		eventIDs,
	)
	if err != nil {
		return fmt.Errorf("mark processed: %w", err)
	}

	return nil
}

func (s *PollingSource) Close() error {
	// pool is managed externally
	return nil
}

func (s *PollingSource) Name() string {
	return "polling"
}

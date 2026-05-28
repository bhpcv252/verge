package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/bhpcv252/verge/internal/observability"
)

type LagReporter interface {
	PendingCount(ctx context.Context) (int64, error)
}

type Worker struct {
	source   EventSource
	handlers []OutboxHandler
	bus      EventBus // nil = in-process dispatch
	obs      *observability.Provider
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

func WithObservability(obs *observability.Provider) Option {
	return func(w *Worker) { w.obs = obs }
}

func NewWorker(opts ...Option) *Worker {
	w := &Worker{obs: observability.Noop()}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (w *Worker) Run(ctx context.Context) error {
	if w.source == nil {
		return fmt.Errorf("worker: source is required (use WithSource)")
	}

	workerLogger := w.obs.Logger.With(
		slog.String("component", "outbox.worker"),
		slog.String("source", w.source.Name()),
	)
	ctx = observability.WithLogger(ctx, workerLogger)

	observability.L(ctx).Info("outbox worker starting")

	if err := w.source.Start(ctx); err != nil {
		return fmt.Errorf("start source: %w", err)
	}
	defer func() {
		if err := w.source.Close(); err != nil {
			observability.L(ctx).Error("close source error",
				slog.String("error", err.Error()),
			)
		}
	}()

	for {
		events, err := w.source.Next(ctx)
		if err != nil {
			if ctx.Err() != nil {
				observability.L(ctx).Info("outbox worker stopped")
				return nil
			}
			observability.L(ctx).Error("source error",
				slog.String("error", err.Error()),
			)
			continue
		}

		if len(events) == 0 {
			continue
		}

		w.runPollCycle(ctx, events)
	}
}

func (w *Worker) runPollCycle(ctx context.Context, events []OutboxEvent) {
	pollCtx, span := w.obs.Tracer.Start(ctx, "verge.outbox.poll",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("outbox.source_type", w.source.Name()),
			attribute.Int("outbox.batch_size", len(events)),
		),
	)
	defer span.End()

	start := time.Now()

	w.obs.Metrics.OutboxBatchSize.Record(pollCtx, float64(len(events)),
		metric.WithAttributes(
			attribute.String("source_type", w.source.Name()),
		),
	)

	observability.L(pollCtx).Info("poll cycle started",
		slog.Int("batch_size", len(events)),
	)

	processed := w.processEvents(pollCtx, events)

	if len(processed) > 0 {
		if err := w.source.Ack(pollCtx, processed); err != nil {
			observability.L(pollCtx).Error("ack error",
				slog.String("error", err.Error()),
				slog.Int("attempted", len(processed)),
			)
		}
	}

	duration := time.Since(start)

	w.obs.Metrics.OutboxPollDuration.Record(pollCtx, duration.Seconds(),
		metric.WithAttributes(
			attribute.String("source_type", w.source.Name()),
		),
	)

	observability.L(pollCtx).Info("poll cycle completed",
		slog.Int("batch_size", len(events)),
		slog.Int("processed", len(processed)),
		slog.Int("failed", len(events)-len(processed)),
		slog.Int64("duration_ms", duration.Milliseconds()),
	)

	if lr, ok := w.source.(LagReporter); ok {
		if count, err := lr.PendingCount(pollCtx); err == nil {
			w.obs.Metrics.OutboxLagEvents.Record(pollCtx, count)
		} else {
			observability.L(pollCtx).Warn("could not read outbox lag",
				slog.String("error", err.Error()),
			)
		}
	}
}

func (w *Worker) processEvents(ctx context.Context, events []OutboxEvent) []string {
	var processed []string

	if w.bus != nil {
		// eventBus mode: publish the whole batch to the external broker
		if err := w.bus.Publish(ctx, events); err != nil {
			observability.L(ctx).Error("eventbus publish error",
				slog.String("error", err.Error()),
				slog.Int("batch_size", len(events)),
			)
			return processed
		}
		for _, e := range events {
			w.obs.Metrics.OutboxEventsProcessedTotal.Add(ctx, 1,
				metric.WithAttributes(
					attribute.String("event_type", e.EventType),
					attribute.String("handler", "eventbus"),
					attribute.String("status", "ok"),
				),
			)
			processed = append(processed, e.ID)
		}
	} else {
		// in-process mode: dispatch each event to its matching handler(s)
		for _, e := range events {
			if err := w.dispatch(ctx, e); err != nil {
				observability.L(ctx).Error("event dispatch failed",
					slog.String("event_id", e.ID),
					slog.String("event_type", e.EventType),
					slog.String("error", err.Error()),
				)
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
			if et != event.EventType {
				continue
			}

			matched = true
			handlerName := fmt.Sprintf("%T", h)

			if err := h.Handle(ctx, event); err != nil {
				w.obs.Metrics.OutboxEventsProcessedTotal.Add(ctx, 1,
					metric.WithAttributes(
						attribute.String("event_type", event.EventType),
						attribute.String("handler", handlerName),
						attribute.String("status", "error"),
					),
				)
				return fmt.Errorf("handler %T: %w", h, err)
			}

			w.obs.Metrics.OutboxEventsProcessedTotal.Add(ctx, 1,
				metric.WithAttributes(
					attribute.String("event_type", event.EventType),
					attribute.String("handler", handlerName),
					attribute.String("status", "ok"),
				),
			)
			break
		}
	}

	if !matched {
		observability.L(ctx).Warn("no handler for event type",
			slog.String("event_type", event.EventType),
			slog.String("event_id", event.ID),
		)
	}

	return nil
}

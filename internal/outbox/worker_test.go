package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mock

type mockHandler struct {
	types    []string
	handleFn func(ctx context.Context, event OutboxEvent) error
	calls    []OutboxEvent
}

func (m *mockHandler) EventTypes() []string { return m.types }

func (m *mockHandler) Handle(ctx context.Context, event OutboxEvent) error {
	m.calls = append(m.calls, event)
	if m.handleFn != nil {
		return m.handleFn(ctx, event)
	}
	return nil
}

func (m *mockHandler) callCount() int { return len(m.calls) }

func newHandler(types ...string) *mockHandler {
	return &mockHandler{types: types}
}

func newFailingHandler(eventType string, err error) *mockHandler {
	return &mockHandler{
		types:    []string{eventType},
		handleFn: func(_ context.Context, _ OutboxEvent) error { return err },
	}
}

func newWorkerWithHandlers(handlers ...OutboxHandler) *Worker {
	return &Worker{
		handlers: handlers,
		interval: 500 * time.Millisecond,
		batch:    100,
	}
}

func makeEvent(id, eventType string) OutboxEvent {
	return OutboxEvent{
		ID:        id,
		EventType: eventType,
		Payload:   []byte(`{}`),
		CreatedAt: time.Now(),
	}
}

// dispatch

func TestWorker_Dispatch_MatchingHandler_IsCalled(t *testing.T) {
	h := newHandler(EventTypeCommitCreated)
	w := newWorkerWithHandlers(h)

	event := makeEvent("evt-1", EventTypeCommitCreated)
	err := w.dispatch(context.Background(), event)

	require.NoError(t, err)
	assert.Equal(t, 1, h.callCount())
	assert.Equal(t, event.ID, h.calls[0].ID)
}

func TestWorker_Dispatch_NonMatchingHandler_IsNotCalled(t *testing.T) {
	h := newHandler(EventTypeCommitCreated) // registered for CommitCreated only
	w := newWorkerWithHandlers(h)

	err := w.dispatch(context.Background(), makeEvent("evt-1", EventTypeBranchHeadMoved))

	require.NoError(t, err)
	assert.Equal(t, 0, h.callCount())
}

func TestWorker_Dispatch_NoHandlersRegistered_ReturnsNil(t *testing.T) {
	w := newWorkerWithHandlers() // zero handlers

	err := w.dispatch(context.Background(), makeEvent("evt-1", EventTypeCommitCreated))

	require.NoError(t, err, "unhandled event must not be an error")
}

func TestWorker_Dispatch_UnknownEventType_ReturnsNil(t *testing.T) {
	h := newHandler(EventTypeCommitCreated)
	w := newWorkerWithHandlers(h)

	err := w.dispatch(context.Background(), makeEvent("evt-1", "SomeFutureEvent"))

	require.NoError(t, err)
	assert.Equal(t, 0, h.callCount())
}

func TestWorker_Dispatch_HandlerError_IsReturned(t *testing.T) {
	sentinelErr := errors.New("downstream unavailable")
	h := newFailingHandler(EventTypeCommitCreated, sentinelErr)
	w := newWorkerWithHandlers(h)

	err := w.dispatch(context.Background(), makeEvent("evt-1", EventTypeCommitCreated))

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinelErr)
	assert.Equal(t, 1, h.callCount(), "handler must have been called before the error")
}

func TestWorker_Dispatch_MultipleHandlers_OnlyMatchingCalled(t *testing.T) {
	commitH := newHandler(EventTypeCommitCreated)
	branchH := newHandler(EventTypeBranchHeadMoved)
	w := newWorkerWithHandlers(commitH, branchH)

	err := w.dispatch(context.Background(), makeEvent("evt-1", EventTypeCommitCreated))

	require.NoError(t, err)
	assert.Equal(t, 1, commitH.callCount())
	assert.Equal(t, 0, branchH.callCount())
}

func TestWorker_Dispatch_HandlerWithMultipleTypes_MatchesOnSecondType(t *testing.T) {
	h := newHandler(EventTypeCommitCreated, EventTypeBranchHeadMoved)
	w := newWorkerWithHandlers(h)

	err := w.dispatch(context.Background(), makeEvent("evt-1", EventTypeBranchHeadMoved))

	require.NoError(t, err)
	assert.Equal(t, 1, h.callCount())
}

func TestWorker_Dispatch_TwoHandlersBothMatchSameType_BothCalled(t *testing.T) {
	h1 := newHandler(EventTypeCommitCreated)
	h2 := newHandler(EventTypeCommitCreated)
	w := newWorkerWithHandlers(h1, h2)

	err := w.dispatch(context.Background(), makeEvent("evt-1", EventTypeCommitCreated))

	require.NoError(t, err)
	assert.Equal(t, 1, h1.callCount())
	assert.Equal(t, 1, h2.callCount())
}

func TestWorker_Dispatch_FirstHandlerFails_SecondHandlerNotCalled(t *testing.T) {
	sentinelErr := errors.New("first handler failed")
	h1 := newFailingHandler(EventTypeCommitCreated, sentinelErr)
	h2 := newHandler(EventTypeCommitCreated)
	w := newWorkerWithHandlers(h1, h2)

	err := w.dispatch(context.Background(), makeEvent("evt-1", EventTypeCommitCreated))

	require.ErrorIs(t, err, sentinelErr)
	assert.Equal(t, 1, h1.callCount())
	assert.Equal(t, 0, h2.callCount(), "second handler must not be called after first fails")
}

func TestWorker_Dispatch_EventPayloadForwardedToHandler(t *testing.T) {
	h := newHandler(EventTypeCommitCreated)
	w := newWorkerWithHandlers(h)

	event := OutboxEvent{
		ID:        "evt-payload",
		EventType: EventTypeCommitCreated,
		Payload:   []byte(`{"commit_id":"abc"}`),
		CreatedAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	require.NoError(t, w.dispatch(context.Background(), event))

	got := h.calls[0]
	assert.Equal(t, event.ID, got.ID)
	assert.Equal(t, event.Payload, got.Payload)
	assert.Equal(t, event.CreatedAt, got.CreatedAt)
}

// NewWorker

func TestNewWorker_Defaults(t *testing.T) {
	w := NewWorker(nil)

	assert.Equal(t, 500*time.Millisecond, w.interval)
	assert.Equal(t, 100, w.batch)
	assert.Nil(t, w.bus)
	assert.Empty(t, w.handlers)
}

func TestWithInterval_PositiveValue_Applied(t *testing.T) {
	w := NewWorker(nil, WithInterval(2*time.Second))
	assert.Equal(t, 2*time.Second, w.interval)
}

func TestWithInterval_Zero_DefaultKept(t *testing.T) {
	w := NewWorker(nil, WithInterval(0))
	assert.Equal(t, 500*time.Millisecond, w.interval, "zero interval must not override the default")
}

func TestWithInterval_Negative_DefaultKept(t *testing.T) {
	w := NewWorker(nil, WithInterval(-1*time.Second))
	assert.Equal(
		t,
		500*time.Millisecond,
		w.interval,
		"negative interval must not override the default",
	)
}

func TestWithBatchSize_PositiveValue_Applied(t *testing.T) {
	w := NewWorker(nil, WithBatchSize(50))
	assert.Equal(t, 50, w.batch)
}

func TestWithBatchSize_Zero_DefaultKept(t *testing.T) {
	w := NewWorker(nil, WithBatchSize(0))
	assert.Equal(t, 100, w.batch, "zero batch size must not override the default")
}

func TestWithBatchSize_Negative_DefaultKept(t *testing.T) {
	w := NewWorker(nil, WithBatchSize(-5))
	assert.Equal(t, 100, w.batch, "negative batch size must not override the default")
}

func TestWithHandlers_Registered(t *testing.T) {
	h1 := newHandler(EventTypeCommitCreated)
	h2 := newHandler(EventTypeBranchHeadMoved)
	w := NewWorker(nil, WithHandlers([]OutboxHandler{h1, h2}))

	assert.Len(t, w.handlers, 2)
}

func TestWithEventBus_Set(t *testing.T) {
	bus := &mockEventBus{}
	w := NewWorker(nil, WithEventBus(bus))
	assert.NotNil(t, w.bus)
}

type mockEventBus struct {
	published []OutboxEvent
	err       error
}

func (m *mockEventBus) Publish(_ context.Context, events []OutboxEvent) error {
	m.published = append(m.published, events...)
	return m.err
}

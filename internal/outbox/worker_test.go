package outbox

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mocks

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

type mockEventBus struct {
	published []OutboxEvent
	err       error
}

func (m *mockEventBus) Publish(_ context.Context, events []OutboxEvent) error {
	m.published = append(m.published, events...)
	return m.err
}

type mockSource struct {
	name    string
	events  []OutboxEvent // returned on the first Next() call, empty on subsequent calls
	nextErr error
	ackErr  error
	started bool
	closed  bool
	acked   []string
	drained chan struct{} // closed once events have been drained by Next()
}

func newMockSource(events ...OutboxEvent) *mockSource {
	return &mockSource{
		events:  events,
		drained: make(chan struct{}),
	}
}

func (m *mockSource) Start(_ context.Context) error {
	m.started = true
	return nil
}

func (m *mockSource) Next(ctx context.Context) ([]OutboxEvent, error) {
	if m.nextErr != nil {
		return nil, m.nextErr
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	events := m.events
	if events != nil {
		m.events = nil // drain so subsequent calls return empty
		if m.drained != nil {
			select {
			case <-m.drained: // already closed
			default:
				close(m.drained)
			}
		}
	}
	return events, nil
}

func (m *mockSource) Ack(_ context.Context, ids []string) error {
	m.acked = append(m.acked, ids...)
	return m.ackErr
}

func (m *mockSource) Close() error {
	m.closed = true
	return nil
}

func (m *mockSource) Name() string {
	if m.name != "" {
		return m.name
	}
	return "mock"
}

// helpers

func newWorkerWithHandlers(handlers ...OutboxHandler) *Worker {
	return NewWorker(
		WithSource(newMockSource()),
		WithHandlers(handlers),
	)
}

func makeEvent(id, eventType string) OutboxEvent {
	return OutboxEvent{
		ID:        id,
		EventType: eventType,
		Payload:   []byte(`{}`),
		CreatedAt: time.Now(),
	}
}

// NewWorker / options

func TestNewWorker_Defaults(t *testing.T) {
	w := NewWorker()

	assert.Nil(t, w.source)
	assert.Nil(t, w.bus)
	assert.Empty(t, w.handlers)
}

func TestWithSource_Set(t *testing.T) {
	src := newMockSource()
	src.name = "test"
	w := NewWorker(WithSource(src))

	assert.Equal(t, src, w.source)
	assert.Equal(t, "test", w.source.Name())
}

func TestWithHandlers_Registered(t *testing.T) {
	h1 := newHandler(EventTypeCommitCreated)
	h2 := newHandler(EventTypeBranchHeadMoved)
	w := NewWorker(WithHandlers([]OutboxHandler{h1, h2}))

	assert.Len(t, w.handlers, 2)
}

func TestWithEventBus_Set(t *testing.T) {
	bus := &mockEventBus{}
	w := NewWorker(WithEventBus(bus))

	assert.NotNil(t, w.bus)
}

// Run

func TestWorker_Run_NoSource_ReturnsError(t *testing.T) {
	w := NewWorker()

	err := w.Run(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "source is required")
}

func TestWorker_Run_StartsAndStopsSource(t *testing.T) {
	src := newMockSource()
	w := NewWorker(WithSource(src))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_ = w.Run(ctx)

	assert.True(t, src.started)
	assert.True(t, src.closed)
}

func TestWorker_Run_AcksSuccessfullyProcessedEvents(t *testing.T) {
	event := makeEvent("evt-1", EventTypeCommitCreated)
	h := newHandler(EventTypeCommitCreated)
	src := newMockSource(event)

	w := NewWorker(WithSource(src), WithHandlers([]OutboxHandler{h}))

	ctx, cancel := context.WithCancel(context.Background())

	// cancel as soon as the source has been drained and acked
	go func() {
		<-src.drained // Next() consumed the events
		// give the worker a moment to ack before we cancel
		for len(src.acked) == 0 {
			runtime.Gosched()
		}
		cancel()
	}()
	_ = w.Run(ctx)

	assert.Equal(t, []string{"evt-1"}, src.acked)
	assert.Equal(t, 1, h.callCount())
}

func TestWorker_Run_EventBusMode_AcksAllEvents(t *testing.T) {
	events := []OutboxEvent{
		makeEvent("evt-1", EventTypeCommitCreated),
		makeEvent("evt-2", EventTypeBranchHeadMoved),
	}
	bus := &mockEventBus{}
	src := newMockSource(events...)

	w := NewWorker(WithSource(src), WithEventBus(bus))

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-src.drained
		for len(src.acked) < 2 {
			runtime.Gosched()
		}
		cancel()
	}()
	_ = w.Run(ctx)

	assert.Len(t, bus.published, 2)
	assert.ElementsMatch(t, []string{"evt-1", "evt-2"}, src.acked)
}

func TestWorker_Run_EventBusPublishFails_NothingAcked(t *testing.T) {
	src := newMockSource(makeEvent("evt-1", EventTypeCommitCreated))
	bus := &mockEventBus{err: errors.New("broker down")}

	w := NewWorker(WithSource(src), WithEventBus(bus))

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-src.drained
		cancel()
	}()
	_ = w.Run(ctx)

	assert.Empty(t, src.acked, "nothing should be acked when Publish fails")
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

// processEvents

func TestWorker_ProcessEvents_InProcess_SuccessReturnsAllIDs(t *testing.T) {
	h := newHandler(EventTypeCommitCreated)
	w := newWorkerWithHandlers(h)

	events := []OutboxEvent{
		makeEvent("evt-1", EventTypeCommitCreated),
		makeEvent("evt-2", EventTypeCommitCreated),
	}
	processed := w.processEvents(context.Background(), events)

	assert.Equal(t, []string{"evt-1", "evt-2"}, processed)
	assert.Equal(t, 2, h.callCount())
}

func TestWorker_ProcessEvents_InProcess_HandlerFails_FailedIDNotReturned(t *testing.T) {
	goodH := newHandler(EventTypeCommitCreated)
	failH := newFailingHandler(EventTypeBranchHeadMoved, errors.New("fail"))
	w := newWorkerWithHandlers(goodH, failH)

	events := []OutboxEvent{
		makeEvent("evt-good", EventTypeCommitCreated),
		makeEvent("evt-fail", EventTypeBranchHeadMoved),
	}
	processed := w.processEvents(context.Background(), events)

	assert.Equal(t, []string{"evt-good"}, processed)
}

func TestWorker_ProcessEvents_EventBus_PublishSucceeds_ReturnsAllIDs(t *testing.T) {
	bus := &mockEventBus{}
	w := NewWorker(WithSource(newMockSource()), WithEventBus(bus))

	events := []OutboxEvent{
		makeEvent("evt-1", EventTypeCommitCreated),
		makeEvent("evt-2", EventTypeBranchHeadMoved),
	}
	processed := w.processEvents(context.Background(), events)

	assert.Equal(t, []string{"evt-1", "evt-2"}, processed)
	assert.Len(t, bus.published, 2)
}

func TestWorker_ProcessEvents_EventBus_PublishFails_ReturnsNoIDs(t *testing.T) {
	bus := &mockEventBus{err: errors.New("broker down")}
	w := NewWorker(WithSource(newMockSource()), WithEventBus(bus))

	events := []OutboxEvent{makeEvent("evt-1", EventTypeCommitCreated)}
	processed := w.processEvents(context.Background(), events)

	assert.Empty(t, processed)
}

func TestWorker_ProcessEvents_EventBus_InProcessHandlersNotCalled(t *testing.T) {
	h := newHandler(EventTypeCommitCreated)
	bus := &mockEventBus{}
	w := NewWorker(WithSource(newMockSource()), WithEventBus(bus), WithHandlers([]OutboxHandler{h}))

	w.processEvents(context.Background(), []OutboxEvent{makeEvent("evt-1", EventTypeCommitCreated)})

	assert.Equal(t, 0, h.callCount(), "in-process handlers must not be called in EventBus mode")
}

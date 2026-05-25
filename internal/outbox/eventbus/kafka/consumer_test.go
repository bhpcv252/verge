package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/outbox"
)

// mock

type mockHandler struct {
	types    []string
	handleFn func(ctx context.Context, event outbox.OutboxEvent) error
	calls    []outbox.OutboxEvent
}

func (m *mockHandler) EventTypes() []string { return m.types }

func (m *mockHandler) Handle(ctx context.Context, event outbox.OutboxEvent) error {
	m.calls = append(m.calls, event)
	if m.handleFn != nil {
		return m.handleFn(ctx, event)
	}
	return nil
}

func newHandler(types ...string) *mockHandler {
	return &mockHandler{types: types}
}

func newFailingHandler(eventType string, err error) *mockHandler {
	return &mockHandler{
		types:    []string{eventType},
		handleFn: func(_ context.Context, _ outbox.OutboxEvent) error { return err },
	}
}

func newConsumerWithHandlers(handlers ...outbox.OutboxHandler) *Consumer {
	return &Consumer{handlers: handlers}
}

func makeOutboxEvent(id, eventType string) outbox.OutboxEvent {
	return outbox.OutboxEvent{
		ID:        id,
		EventType: eventType,
		Payload:   json.RawMessage(`{}`),
		CreatedAt: time.Now(),
	}
}

// matching behaviour

func TestConsumer_Dispatch_MatchingHandler_IsCalled(t *testing.T) {
	h := newHandler(outbox.EventTypeCommitCreated)
	c := newConsumerWithHandlers(h)

	event := makeOutboxEvent("evt-1", outbox.EventTypeCommitCreated)
	err := c.dispatch(context.Background(), event)

	require.NoError(t, err)
	require.Len(t, h.calls, 1)
	assert.Equal(t, event.ID, h.calls[0].ID)
}

func TestConsumer_Dispatch_NonMatchingHandler_NotCalled(t *testing.T) {
	h := newHandler(outbox.EventTypeCommitCreated)
	c := newConsumerWithHandlers(h)

	err := c.dispatch(
		context.Background(),
		makeOutboxEvent("evt-1", outbox.EventTypeBranchHeadMoved),
	)

	require.NoError(t, err)
	assert.Empty(t, h.calls)
}

func TestConsumer_Dispatch_NoHandlersRegistered_ReturnsNil(t *testing.T) {
	c := newConsumerWithHandlers()

	err := c.dispatch(context.Background(), makeOutboxEvent("evt-1", outbox.EventTypeCommitCreated))

	require.NoError(t, err)
}

func TestConsumer_Dispatch_UnknownEventType_ReturnsNil(t *testing.T) {
	h := newHandler(outbox.EventTypeCommitCreated)
	c := newConsumerWithHandlers(h)

	err := c.dispatch(context.Background(), makeOutboxEvent("evt-1", "SomeFutureEvent"))

	require.NoError(t, err)
	assert.Empty(t, h.calls)
}

// error behaviour

func TestConsumer_Dispatch_HandlerError_IsReturned(t *testing.T) {
	sentinel := errors.New("handler failed")
	h := newFailingHandler(outbox.EventTypeCommitCreated, sentinel)
	c := newConsumerWithHandlers(h)

	err := c.dispatch(context.Background(), makeOutboxEvent("evt-1", outbox.EventTypeCommitCreated))

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.Len(t, h.calls, 1, "handler must have been called before the error")
}

// multiple handlers

func TestConsumer_Dispatch_MultipleHandlers_OnlyMatchingCalled(t *testing.T) {
	commitH := newHandler(outbox.EventTypeCommitCreated)
	branchH := newHandler(outbox.EventTypeBranchHeadMoved)
	c := newConsumerWithHandlers(commitH, branchH)

	require.NoError(t, c.dispatch(context.Background(),
		makeOutboxEvent("evt-1", outbox.EventTypeCommitCreated)))

	assert.Len(t, commitH.calls, 1)
	assert.Empty(t, branchH.calls)
}

func TestConsumer_Dispatch_TwoHandlersBothMatchSameType_BothCalled(t *testing.T) {
	h1 := newHandler(outbox.EventTypeCommitCreated)
	h2 := newHandler(outbox.EventTypeCommitCreated)
	c := newConsumerWithHandlers(h1, h2)

	require.NoError(t, c.dispatch(context.Background(),
		makeOutboxEvent("evt-1", outbox.EventTypeCommitCreated)))

	assert.Len(t, h1.calls, 1)
	assert.Len(t, h2.calls, 1)
}

func TestConsumer_Dispatch_FirstHandlerFails_SecondHandlerNotCalled(t *testing.T) {
	sentinel := errors.New("first failed")
	h1 := newFailingHandler(outbox.EventTypeCommitCreated, sentinel)
	h2 := newHandler(outbox.EventTypeCommitCreated)
	c := newConsumerWithHandlers(h1, h2)

	err := c.dispatch(context.Background(), makeOutboxEvent("evt-1", outbox.EventTypeCommitCreated))

	require.ErrorIs(t, err, sentinel)
	assert.Len(t, h1.calls, 1)
	assert.Empty(t, h2.calls, "second handler must not be called after first fails")
}

func TestConsumer_Dispatch_HandlerWithMultipleTypes_MatchesSecondType(t *testing.T) {
	h := newHandler(outbox.EventTypeCommitCreated, outbox.EventTypeBranchHeadMoved)
	c := newConsumerWithHandlers(h)

	require.NoError(t, c.dispatch(context.Background(),
		makeOutboxEvent("evt-1", outbox.EventTypeBranchHeadMoved)))

	assert.Len(t, h.calls, 1)
}

// payload forwarded intact

func TestConsumer_Dispatch_EventPayloadForwardedToHandler(t *testing.T) {
	h := newHandler(outbox.EventTypeCommitCreated)
	c := newConsumerWithHandlers(h)

	event := outbox.OutboxEvent{
		ID:        "evt-payload",
		EventType: outbox.EventTypeCommitCreated,
		Payload:   json.RawMessage(`{"commit_id":"c1","repo_id":"r1"}`),
		CreatedAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	require.NoError(t, c.dispatch(context.Background(), event))

	got := h.calls[0]
	assert.Equal(t, event.ID, got.ID)
	assert.Equal(t, event.Payload, got.Payload)
	assert.Equal(t, event.CreatedAt, got.CreatedAt)
}

// config wiring

func TestNewConsumer_HandlersRegistered(t *testing.T) {
	h1 := newHandler(outbox.EventTypeCommitCreated)
	h2 := newHandler(outbox.EventTypeBranchHeadMoved)

	c := NewConsumer(
		ConsumerConfig{
			Brokers: []string{"localhost:9092"},
			Topic:   "verge.events",
			GroupID: "test-group",
		},
		[]outbox.OutboxHandler{h1, h2},
	)

	assert.Len(t, c.handlers, 2)
}

func TestNewConsumer_ReaderConfigured(t *testing.T) {
	c := NewConsumer(
		ConsumerConfig{
			Brokers: []string{"broker1:9092", "broker2:9092"},
			Topic:   "verge.events",
			GroupID: "my-group",
		},
		nil,
	)

	cfg := c.reader.Config()
	assert.Equal(t, []string{"broker1:9092", "broker2:9092"}, cfg.Brokers)
	assert.Equal(t, "verge.events", cfg.Topic)
	assert.Equal(t, "my-group", cfg.GroupID)
}

package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mock

type mockBranchHeadStore struct {
	setHeadCalls []setHeadArgs
	setHeadErr   error
}

type setHeadArgs struct {
	repoID   string
	name     string
	commitID string
	version  int64
}

func (m *mockBranchHeadStore) GetHead(_ context.Context, _, _ string) (string, error) {
	panic("GetHead must not be called by RedisHealHandler")
}

func (m *mockBranchHeadStore) SetHead(
	_ context.Context,
	repoID, name, commitID string,
	version int64,
) error {
	m.setHeadCalls = append(m.setHeadCalls, setHeadArgs{repoID, name, commitID, version})
	return m.setHeadErr
}

// helpers

func marshalPayload(t *testing.T, p BranchHeadMovedPayload) []byte {
	t.Helper()
	b, err := json.Marshal(p)
	require.NoError(t, err)
	return b
}

func makeBranchHeadMovedEvent(t *testing.T, p BranchHeadMovedPayload) OutboxEvent {
	t.Helper()
	return OutboxEvent{
		ID:        "evt-redis-1",
		EventType: EventTypeBranchHeadMoved,
		Payload:   marshalPayload(t, p),
		CreatedAt: time.Now(),
	}
}

// EventTypes

func TestRedisHealHandler_EventTypes_ReturnsBranchHeadMoved(t *testing.T) {
	h := NewRedisHealHandler(&mockBranchHeadStore{})

	types := h.EventTypes()

	require.Len(t, types, 1)
	assert.Equal(t, EventTypeBranchHeadMoved, types[0])
}

// happy path

func TestRedisHealHandler_Handle_ValidPayload_CallsSetHead(t *testing.T) {
	store := &mockBranchHeadStore{}
	h := NewRedisHealHandler(store)

	payload := BranchHeadMovedPayload{
		RepoID:   "repo-abc",
		Branch:   "main",
		CommitID: "commit-xyz",
		Version:  1700000000000,
	}
	event := makeBranchHeadMovedEvent(t, payload)

	err := h.Handle(context.Background(), event)

	require.NoError(t, err)
	require.Len(t, store.setHeadCalls, 1)
	got := store.setHeadCalls[0]
	assert.Equal(t, payload.RepoID, got.repoID)
	assert.Equal(t, payload.Branch, got.name)
	assert.Equal(t, payload.CommitID, got.commitID)
	assert.Equal(t, payload.Version, got.version)
}

func TestRedisHealHandler_Handle_ZeroVersion_IsForwardedAsIs(t *testing.T) {
	store := &mockBranchHeadStore{}
	h := NewRedisHealHandler(store)

	payload := BranchHeadMovedPayload{
		RepoID:   "repo-abc",
		Branch:   "main",
		CommitID: "commit-xyz",
		Version:  0,
	}

	err := h.Handle(context.Background(), makeBranchHeadMovedEvent(t, payload))

	require.NoError(t, err)
	assert.Equal(t, int64(0), store.setHeadCalls[0].version)
}

func TestRedisHealHandler_Handle_SetHeadCalledExactlyOnce(t *testing.T) {
	store := &mockBranchHeadStore{}
	h := NewRedisHealHandler(store)

	err := h.Handle(context.Background(), makeBranchHeadMovedEvent(t, BranchHeadMovedPayload{
		RepoID: "repo-1", Branch: "feat", CommitID: "c1", Version: 1,
	}))

	require.NoError(t, err)
	assert.Len(t, store.setHeadCalls, 1)
}

// error paths

func TestRedisHealHandler_Handle_MalformedJSON_ReturnsError(t *testing.T) {
	store := &mockBranchHeadStore{}
	h := NewRedisHealHandler(store)

	event := OutboxEvent{
		ID:        "evt-bad",
		EventType: EventTypeBranchHeadMoved,
		Payload:   []byte(`not valid json`),
		CreatedAt: time.Now(),
	}

	err := h.Handle(context.Background(), event)

	require.Error(t, err)
	assert.Empty(t, store.setHeadCalls, "SetHead must not be called when payload is malformed")
}

func TestRedisHealHandler_Handle_EmptyPayload_ReturnsError(t *testing.T) {
	store := &mockBranchHeadStore{}
	h := NewRedisHealHandler(store)

	event := OutboxEvent{
		ID:        "evt-empty",
		EventType: EventTypeBranchHeadMoved,
		Payload:   []byte(``),
		CreatedAt: time.Now(),
	}

	err := h.Handle(context.Background(), event)

	require.Error(t, err)
	assert.Empty(t, store.setHeadCalls)
}

func TestRedisHealHandler_Handle_SetHeadError_IsPropagated(t *testing.T) {
	sentinelErr := errors.New("redis: connection refused")
	store := &mockBranchHeadStore{setHeadErr: sentinelErr}
	h := NewRedisHealHandler(store)

	event := makeBranchHeadMovedEvent(t, BranchHeadMovedPayload{
		RepoID: "repo-1", Branch: "main", CommitID: "c1", Version: 1,
	})

	err := h.Handle(context.Background(), event)

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinelErr)
}

func TestRedisHealHandler_Handle_SetHeadError_SetHeadWasStillCalled(t *testing.T) {
	store := &mockBranchHeadStore{setHeadErr: errors.New("timeout")}
	h := NewRedisHealHandler(store)

	_ = h.Handle(context.Background(), makeBranchHeadMovedEvent(t, BranchHeadMovedPayload{
		RepoID: "repo-1", Branch: "main", CommitID: "c1", Version: 1,
	}))

	assert.Len(t, store.setHeadCalls, 1)
}

// idempotency guarantee

func TestRedisHealHandler_Handle_SameEventTwice_SetHeadCalledTwice(t *testing.T) {
	store := &mockBranchHeadStore{}
	h := NewRedisHealHandler(store)

	event := makeBranchHeadMovedEvent(t, BranchHeadMovedPayload{
		RepoID: "repo-1", Branch: "main", CommitID: "c1", Version: 999,
	})

	require.NoError(t, h.Handle(context.Background(), event))
	require.NoError(t, h.Handle(context.Background(), event))

	assert.Len(t, store.setHeadCalls, 2,
		"handler always calls SetHead; idempotency is enforced by the store, not the handler")
}

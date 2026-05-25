package outbox

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mock

type mockNeo4jDriver struct {
	newSessionCalled bool
}

func (m *mockNeo4jDriver) NewSession(
	_ context.Context,
	_ neo4j.SessionConfig,
) neo4j.SessionWithContext {
	m.newSessionCalled = true
	return nil
}

func (m *mockNeo4jDriver) Target() url.URL                            { return url.URL{} }
func (m *mockNeo4jDriver) VerifyConnectivity(_ context.Context) error { return nil }
func (m *mockNeo4jDriver) VerifyAuthentication(_ context.Context, _ *neo4j.AuthToken) error {
	return nil
}
func (m *mockNeo4jDriver) Close(_ context.Context) error { return nil }
func (m *mockNeo4jDriver) IsEncrypted() bool             { return false }
func (m *mockNeo4jDriver) GetServerInfo(_ context.Context) (neo4j.ServerInfo, error) {
	return nil, nil
}
func (m *mockNeo4jDriver) ExecuteQueryBookmarkManager() neo4j.BookmarkManager { return nil }

// EventTypes

func TestNeo4jHandler_EventTypes_ReturnsCommitCreated(t *testing.T) {
	h := NewNeo4jHandler(&mockNeo4jDriver{})

	types := h.EventTypes()

	require.Len(t, types, 1)
	assert.Equal(t, EventTypeCommitCreated, types[0])
}

// JSON parsing errors

func TestNeo4jHandler_Handle_MalformedJSON_ReturnsError(t *testing.T) {
	driver := &mockNeo4jDriver{}
	h := NewNeo4jHandler(driver)

	event := OutboxEvent{
		ID:        "evt-bad",
		EventType: EventTypeCommitCreated,
		Payload:   []byte(`not valid json`),
		CreatedAt: time.Now(),
	}

	err := h.Handle(context.Background(), event)

	require.Error(t, err)
	assert.False(t, driver.newSessionCalled, "driver must not be touched when payload is malformed")
}

func TestNeo4jHandler_Handle_EmptyPayload_ReturnsError(t *testing.T) {
	driver := &mockNeo4jDriver{}
	h := NewNeo4jHandler(driver)

	event := OutboxEvent{
		ID:        "evt-empty",
		EventType: EventTypeCommitCreated,
		Payload:   []byte(``),
		CreatedAt: time.Now(),
	}

	err := h.Handle(context.Background(), event)

	require.Error(t, err)
	assert.False(t, driver.newSessionCalled)
}

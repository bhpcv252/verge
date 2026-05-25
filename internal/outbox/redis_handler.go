package outbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bhpcv252/verge/internal/storage/interfaces"
)

type RedisHealHandler struct {
	store interfaces.BranchHeadStore
}

func NewRedisHealHandler(store interfaces.BranchHeadStore) *RedisHealHandler {
	return &RedisHealHandler{store: store}
}

func (h *RedisHealHandler) EventTypes() []string {
	return []string{EventTypeBranchHeadMoved}
}

func (h *RedisHealHandler) Handle(ctx context.Context, event OutboxEvent) error {
	var payload BranchHeadMovedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("redis heal handler: unmarshal payload: %w", err)
	}

	if err := h.store.SetHead(ctx, payload.RepoID, payload.Branch, payload.CommitID, payload.Version); err != nil {
		return fmt.Errorf("redis heal handler: set head repo=%s branch=%s: %w",
			payload.RepoID, payload.Branch, err)
	}

	return nil
}

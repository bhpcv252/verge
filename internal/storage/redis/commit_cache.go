package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/storage/interfaces"
)

type commitCache struct {
	rdb *redis.Client
}

func NewCommitCache(rdb *redis.Client) interfaces.CommitCache {
	return &commitCache{rdb: rdb}
}

func commitKey(repoID, commitID string) string {
	return fmt.Sprintf("commit:%s:%s", repoID, commitID)
}

func (c *commitCache) GetCommit(
	ctx context.Context,
	repoID, commitID string,
) (*domain.Commit, error) {
	val, err := c.rdb.Get(ctx, commitKey(repoID, commitID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, interfaces.ErrCacheMiss
		}
		return nil, fmt.Errorf("redis commit: get: %w", err)
	}

	commit := &domain.Commit{}
	if err := json.Unmarshal([]byte(val), commit); err != nil {
		// corrupted entry - treat as miss
		return nil, interfaces.ErrCacheMiss
	}

	return commit, nil
}

func (c *commitCache) SetCommit(ctx context.Context, commit *domain.Commit) error {
	data, err := json.Marshal(commit)
	if err != nil {
		return fmt.Errorf("redis commit: marshal: %w", err)
	}

	// 0 TTL = no expiry
	if err := c.rdb.Set(ctx, commitKey(commit.RepoID, commit.ID), string(data), 0).Err(); err != nil {
		return fmt.Errorf("redis commit: set: %w", err)
	}

	return nil
}

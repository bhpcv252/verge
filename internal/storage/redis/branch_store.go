package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/bhpcv252/verge/internal/storage/interfaces"
)

type branchHeadStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewBranchHeadStore(rdb *redis.Client, ttl time.Duration) interfaces.BranchHeadStore {
	return &branchHeadStore{rdb: rdb, ttl: ttl}
}

type branchHeadValue struct {
	CommitID string `json:"commit_id"`
	Version  int64  `json:"version"`
}

func branchKey(repoID, name string) string {
	return fmt.Sprintf("branch:%s:%s", repoID, name)
}

func (s *branchHeadStore) GetHead(ctx context.Context, repoID, name string) (string, error) {
	val, err := s.rdb.Get(ctx, branchKey(repoID, name)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", interfaces.ErrCacheMiss
		}
		return "", fmt.Errorf("redis branch: get head: %w", err)
	}

	var head branchHeadValue
	if err := json.Unmarshal([]byte(val), &head); err != nil {
		return "", interfaces.ErrCacheMiss
	}

	return head.CommitID, nil
}

var setIfVersionGreater = redis.NewScript(`
local key    = KEYS[1]
local value  = ARGV[1]
local ver    = tonumber(ARGV[2])
local ttl    = tonumber(ARGV[3])
local cur    = redis.call('GET', key)
if cur ~= false then
    local ok, data = pcall(cjson.decode, cur)
    if ok and data['version'] and ver <= data['version'] then
        return 0
    end
end
if ttl > 0 then
    redis.call('SET', key, value, 'EX', ttl)
else
    redis.call('SET', key, value)
end
return 1
`)

// SetHead writes {commit_id, version} only if version > currently stored version
func (s *branchHeadStore) SetHead(
	ctx context.Context,
	repoID, name, commitID string,
	version int64,
) error {
	payload, err := json.Marshal(branchHeadValue{CommitID: commitID, Version: version})
	if err != nil {
		return fmt.Errorf("redis branch: marshal head value: %w", err)
	}

	ttlSeconds := int64(s.ttl.Seconds())
	err = setIfVersionGreater.Run(
		ctx,
		s.rdb,
		[]string{branchKey(repoID, name)},
		string(payload),
		version,
		ttlSeconds,
	).Err()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("redis branch: set head: %w", err)
	}

	return nil
}

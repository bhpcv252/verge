//go:build integration

package redis

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/storage/interfaces"
	"github.com/bhpcv252/verge/testhelper"
)

// helper

func setupCommitCache(t *testing.T) interfaces.CommitCache {
	t.Helper()
	rdb := testhelper.SetupRedis(t)
	return NewCommitCache(rdb)
}

func testCommit(repoID, commitID string) *domain.Commit {
	return &domain.Commit{
		ID:     commitID,
		RepoID: repoID,
		DataPointer: domain.DataPointer{
			Type:     "db",
			Location: "test/fixture",
		},
		Message:   "test commit",
		Author:    "test@example.com",
		Timestamp: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		ParentIDs: []string{"parent-1"},
	}
}

// GetCommit - cache miss

func TestCommitCache_GetCommit_KeyNotSet_ReturnsCacheMiss(t *testing.T) {
	cache := setupCommitCache(t)

	_, err := cache.GetCommit(context.Background(), "repo-1", "commit-nonexistent")

	require.ErrorIs(t, err, interfaces.ErrCacheMiss)
}

// SetCommit -> GetCommit round-trip

func TestCommitCache_SetThenGet_ReturnsCorrectCommit(t *testing.T) {
	cache := setupCommitCache(t)

	original := testCommit("repo-1", "commit-abc")
	require.NoError(t, cache.SetCommit(context.Background(), original))

	got, err := cache.GetCommit(context.Background(), "repo-1", "commit-abc")

	require.NoError(t, err)
	assert.Equal(t, original.ID, got.ID)
	assert.Equal(t, original.RepoID, got.RepoID)
}

func TestCommitCache_SetThenGet_AllFieldsPreserved(t *testing.T) {
	cache := setupCommitCache(t)

	original := testCommit("repo-1", "commit-fields")
	require.NoError(t, cache.SetCommit(context.Background(), original))

	got, err := cache.GetCommit(context.Background(), "repo-1", "commit-fields")

	require.NoError(t, err)
	assert.Equal(t, original.ID, got.ID)
	assert.Equal(t, original.RepoID, got.RepoID)
	assert.Equal(t, original.Message, got.Message)
	assert.Equal(t, original.Author, got.Author)
	assert.True(t, original.Timestamp.Equal(got.Timestamp),
		"timestamp must be preserved: want %v, got %v", original.Timestamp, got.Timestamp)
	assert.Equal(t, original.DataPointer.Type, got.DataPointer.Type)
	assert.Equal(t, original.DataPointer.Location, got.DataPointer.Location)
	assert.Equal(t, original.ParentIDs, got.ParentIDs)
}

func TestCommitCache_SetThenGet_RootCommit_EmptyParentIDs(t *testing.T) {
	cache := setupCommitCache(t)

	c := testCommit("repo-1", "commit-root")
	c.ParentIDs = []string{}
	require.NoError(t, cache.SetCommit(context.Background(), c))

	got, err := cache.GetCommit(context.Background(), "repo-1", "commit-root")

	require.NoError(t, err)
	assert.Empty(t, got.ParentIDs)
}

func TestCommitCache_SetThenGet_TwoParents_BothPreserved(t *testing.T) {
	cache := setupCommitCache(t)

	c := testCommit("repo-1", "commit-merge")
	c.ParentIDs = []string{"parent-a", "parent-b"}
	require.NoError(t, cache.SetCommit(context.Background(), c))

	got, err := cache.GetCommit(context.Background(), "repo-1", "commit-merge")

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"parent-a", "parent-b"}, got.ParentIDs)
}

func TestCommitCache_DifferentCommitsStoredIndependently(t *testing.T) {
	cache := setupCommitCache(t)

	c1 := testCommit("repo-1", "commit-1")
	c1.Message = "first"
	c2 := testCommit("repo-1", "commit-2")
	c2.Message = "second"

	require.NoError(t, cache.SetCommit(context.Background(), c1))
	require.NoError(t, cache.SetCommit(context.Background(), c2))

	got1, err := cache.GetCommit(context.Background(), "repo-1", "commit-1")
	require.NoError(t, err)
	assert.Equal(t, "first", got1.Message)

	got2, err := cache.GetCommit(context.Background(), "repo-1", "commit-2")
	require.NoError(t, err)
	assert.Equal(t, "second", got2.Message)
}

func TestCommitCache_SameCommitIDDifferentRepos_StoredIndependently(t *testing.T) {
	cache := setupCommitCache(t)

	c1 := testCommit("repo-1", "commit-x")
	c1.Message = "repo1 commit"
	c2 := testCommit("repo-2", "commit-x") // same commit ID, different repo
	c2.Message = "repo2 commit"

	require.NoError(t, cache.SetCommit(context.Background(), c1))
	require.NoError(t, cache.SetCommit(context.Background(), c2))

	got1, err := cache.GetCommit(context.Background(), "repo-1", "commit-x")
	require.NoError(t, err)
	assert.Equal(t, "repo1 commit", got1.Message)

	got2, err := cache.GetCommit(context.Background(), "repo-2", "commit-x")
	require.NoError(t, err)
	assert.Equal(t, "repo2 commit", got2.Message)
}

// SetCommit overwrites on second write

func TestCommitCache_SetCommit_Overwrite_ReturnsLatestValue(t *testing.T) {
	cache := setupCommitCache(t)

	c := testCommit("repo-1", "commit-overwrite")
	c.Message = "original"
	require.NoError(t, cache.SetCommit(context.Background(), c))

	c.Message = "updated"
	require.NoError(t, cache.SetCommit(context.Background(), c))

	got, err := cache.GetCommit(context.Background(), "repo-1", "commit-overwrite")
	require.NoError(t, err)
	assert.Equal(t, "updated", got.Message)
}

// No TTL - commits must not expire

func TestCommitCache_SetCommit_NoTTL_KeyPersists(t *testing.T) {
	rdb := testhelper.SetupRedis(t)
	cache := NewCommitCache(rdb)

	c := testCommit("repo-1", "commit-ttl")
	require.NoError(t, cache.SetCommit(context.Background(), c))

	ttl := rdb.TTL(context.Background(), commitKey("repo-1", "commit-ttl")).Val()
	assert.Equal(t, time.Duration(-1), ttl,
		"commit cache entries must have no expiry (TTL=-1 means no expiry in Redis)")
}

// Corrupted cache entry - treated as miss

func TestCommitCache_GetCommit_CorruptedEntry_ReturnsCacheMiss(t *testing.T) {
	rdb := testhelper.SetupRedis(t)
	cache := NewCommitCache(rdb)

	require.NoError(t, rdb.Set(context.Background(),
		commitKey("repo-1", "commit-corrupt"), "not-json{{{", 0).Err())

	_, err := cache.GetCommit(context.Background(), "repo-1", "commit-corrupt")

	require.ErrorIs(t, err, interfaces.ErrCacheMiss,
		"corrupted JSON must be treated as a cache miss, not propagated as an error")
}

func TestCommitCache_GetCommit_EmptyJSON_ReturnsCacheMiss(t *testing.T) {
	rdb := testhelper.SetupRedis(t)
	cache := NewCommitCache(rdb)

	require.NoError(t, rdb.Set(context.Background(),
		commitKey("repo-1", "commit-empty"), "", 0).Err())

	_, err := cache.GetCommit(context.Background(), "repo-1", "commit-empty")

	require.ErrorIs(t, err, interfaces.ErrCacheMiss)
}

//go:build integration

package redis

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bhpcv252/verge/internal/storage/interfaces"
	"github.com/bhpcv252/verge/testhelper"
)

// helper

func setupBranchHeadStore(t *testing.T, ttl time.Duration) interfaces.BranchHeadStore {
	t.Helper()
	rdb := testhelper.SetupRedis(t)
	return NewBranchHeadStore(rdb, ttl)
}

// GetHead - cache miss

func TestBranchHeadStore_GetHead_KeyNotSet_ReturnsCacheMiss(t *testing.T) {
	store := setupBranchHeadStore(t, time.Minute)

	_, err := store.GetHead(context.Background(), "repo-1", "main")

	require.ErrorIs(t, err, interfaces.ErrCacheMiss)
}

func TestBranchHeadStore_GetHead_AfterExpiry_ReturnsCacheMiss(t *testing.T) {
	store := setupBranchHeadStore(t, 1*time.Second)

	require.NoError(t, store.SetHead(context.Background(), "repo-1", "main", "commit-abc", 1))

	got, err := store.GetHead(context.Background(), "repo-1", "main")
	require.NoError(t, err)
	assert.Equal(t, "commit-abc", got)

	time.Sleep(1500 * time.Millisecond)

	_, err = store.GetHead(context.Background(), "repo-1", "main")
	require.ErrorIs(t, err, interfaces.ErrCacheMiss)
}

// SetHead → GetHead round-trip

func TestBranchHeadStore_SetThenGet_ReturnsCorrectCommitID(t *testing.T) {
	store := setupBranchHeadStore(t, time.Minute)

	require.NoError(t, store.SetHead(context.Background(), "repo-1", "main", "commit-xyz", 1000))

	got, err := store.GetHead(context.Background(), "repo-1", "main")

	require.NoError(t, err)
	assert.Equal(t, "commit-xyz", got)
}

func TestBranchHeadStore_SetThenGet_DifferentBranchesAreIndependent(t *testing.T) {
	store := setupBranchHeadStore(t, time.Minute)

	require.NoError(t, store.SetHead(context.Background(), "repo-1", "main", "commit-main", 1))
	require.NoError(t, store.SetHead(context.Background(), "repo-1", "feat", "commit-feat", 1))

	gotMain, err := store.GetHead(context.Background(), "repo-1", "main")
	require.NoError(t, err)
	assert.Equal(t, "commit-main", gotMain)

	gotFeat, err := store.GetHead(context.Background(), "repo-1", "feat")
	require.NoError(t, err)
	assert.Equal(t, "commit-feat", gotFeat)
}

func TestBranchHeadStore_SetThenGet_DifferentReposAreIndependent(t *testing.T) {
	store := setupBranchHeadStore(t, time.Minute)

	require.NoError(t, store.SetHead(context.Background(), "repo-1", "main", "commit-r1", 1))
	require.NoError(t, store.SetHead(context.Background(), "repo-2", "main", "commit-r2", 1))

	got1, err := store.GetHead(context.Background(), "repo-1", "main")
	require.NoError(t, err)
	assert.Equal(t, "commit-r1", got1)

	got2, err := store.GetHead(context.Background(), "repo-2", "main")
	require.NoError(t, err)
	assert.Equal(t, "commit-r2", got2)
}

// Version guard - Lua CAS script

func TestBranchHeadStore_SetHead_HigherVersion_Overwrites(t *testing.T) {
	store := setupBranchHeadStore(t, time.Minute)

	require.NoError(t, store.SetHead(context.Background(), "repo-1", "main", "commit-old", 100))
	require.NoError(t, store.SetHead(context.Background(), "repo-1", "main", "commit-new", 200))

	got, err := store.GetHead(context.Background(), "repo-1", "main")
	require.NoError(t, err)
	assert.Equal(t, "commit-new", got, "higher version must overwrite lower version")
}

func TestBranchHeadStore_SetHead_LowerVersion_DoesNotOverwrite(t *testing.T) {
	store := setupBranchHeadStore(t, time.Minute)

	require.NoError(t, store.SetHead(context.Background(), "repo-1", "main", "commit-new", 200))
	require.NoError(t, store.SetHead(context.Background(), "repo-1", "main", "commit-stale", 100))

	got, err := store.GetHead(context.Background(), "repo-1", "main")
	require.NoError(t, err)
	assert.Equal(t, "commit-new", got, "lower version must NOT overwrite higher version")
}

func TestBranchHeadStore_SetHead_EqualVersion_DoesNotOverwrite(t *testing.T) {
	// Lua script: ver <= data['version'] -> return 0 (no write)
	// so equal version is also rejected
	store := setupBranchHeadStore(t, time.Minute)

	require.NoError(t, store.SetHead(context.Background(), "repo-1", "main", "commit-first", 100))
	require.NoError(
		t,
		store.SetHead(context.Background(), "repo-1", "main", "commit-same-ver", 100),
	)

	got, err := store.GetHead(context.Background(), "repo-1", "main")
	require.NoError(t, err)
	assert.Equal(t, "commit-first", got, "equal version must not overwrite existing value")
}

func TestBranchHeadStore_SetHead_FirstWrite_AlwaysSucceeds(t *testing.T) {
	store := setupBranchHeadStore(t, time.Minute)

	require.NoError(t, store.SetHead(context.Background(), "repo-1", "main", "commit-first", 1))

	got, err := store.GetHead(context.Background(), "repo-1", "main")
	require.NoError(t, err)
	assert.Equal(t, "commit-first", got)
}

func TestBranchHeadStore_SetHead_VersionGuardIsPerKey(t *testing.T) {
	store := setupBranchHeadStore(t, time.Minute)

	require.NoError(
		t,
		store.SetHead(context.Background(), "repo-1", "main", "commit-main-v200", 200),
	)
	require.NoError(t, store.SetHead(context.Background(), "repo-1", "feat", "commit-feat-v50", 50))

	require.NoError(t, store.SetHead(context.Background(), "repo-1", "main", "commit-stale", 100))
	require.NoError(
		t,
		store.SetHead(context.Background(), "repo-1", "feat", "commit-feat-v100", 100),
	)

	gotMain, _ := store.GetHead(context.Background(), "repo-1", "main")
	assert.Equal(t, "commit-main-v200", gotMain, "stale write to main must be rejected")

	gotFeat, _ := store.GetHead(context.Background(), "repo-1", "feat")
	assert.Equal(t, "commit-feat-v100", gotFeat, "newer write to feat must be accepted")
}

// TTL

func TestBranchHeadStore_SetHead_ZeroTTL_KeyDoesNotExpire(t *testing.T) {
	rdb := testhelper.SetupRedis(t)
	store := NewBranchHeadStore(rdb, 0) // zero TTL

	require.NoError(t, store.SetHead(context.Background(), "repo-1", "main", "commit-abc", 1))

	ttl := rdb.TTL(context.Background(), branchKey("repo-1", "main")).Val()
	assert.Equal(t, time.Duration(-1), ttl, "TTL must be -1 (no expiry) when configured with 0")
}

func TestBranchHeadStore_SetHead_PositiveTTL_KeyHasTTL(t *testing.T) {
	rdb := testhelper.SetupRedis(t)
	store := NewBranchHeadStore(rdb, 30*time.Second)

	require.NoError(t, store.SetHead(context.Background(), "repo-1", "main", "commit-abc", 1))

	ttl := rdb.TTL(context.Background(), branchKey("repo-1", "main")).Val()
	assert.Greater(t, ttl, time.Duration(0), "TTL must be positive when configured with 30s")
	assert.LessOrEqual(t, ttl, 30*time.Second)
}

// Corrupted cache entry

func TestBranchHeadStore_GetHead_CorruptedEntry_ReturnsCacheMiss(t *testing.T) {
	rdb := testhelper.SetupRedis(t)
	store := NewBranchHeadStore(rdb, time.Minute)

	require.NoError(t, rdb.Set(context.Background(),
		branchKey("repo-1", "main"), "not-valid-json", time.Minute).Err())

	_, err := store.GetHead(context.Background(), "repo-1", "main")

	require.ErrorIs(t, err, interfaces.ErrCacheMiss,
		"corrupted JSON must be treated as a cache miss, not an error")
}

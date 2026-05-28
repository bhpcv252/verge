package observability

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// StorageSpan

func TestStorageSpan_Success_DoesNotPanic(t *testing.T) {
	obs := Noop()
	assert.NotPanics(t, func() {
		_, done := obs.StorageSpan(context.Background(), "postgres", "commit.insert")
		done(nil)
	})
}

func TestStorageSpan_Error_DoesNotPanic(t *testing.T) {
	obs := Noop()
	assert.NotPanics(t, func() {
		_, done := obs.StorageSpan(context.Background(), "redis", "branch.get")
		done(errors.New("timeout"))
	})
}

func TestStorageSpan_ReturnsNonNilContext(t *testing.T) {
	obs := Noop()
	ctx, done := obs.StorageSpan(context.Background(), "neo4j", "graph.ancestors")
	require.NotNil(t, ctx)
	require.NotNil(t, done)
	done(nil)
}

func TestStorageSpan_DoneCalledTwice_DoesNotPanic(t *testing.T) {
	obs := Noop()
	assert.NotPanics(t, func() {
		_, done := obs.StorageSpan(context.Background(), "redis", "branch.set")
		done(nil)
		done(nil)
	})
}

// dbSystem helper

func TestDbSystem_KnownBackends(t *testing.T) {
	assert.Equal(t, "postgresql", dbSystem("postgres"))
	assert.Equal(t, "redis", dbSystem("redis"))
	assert.Equal(t, "neo4j", dbSystem("neo4j"))
}

func TestDbSystem_UnknownBackend_PassesThrough(t *testing.T) {
	assert.Equal(t, "dynamo", dbSystem("dynamo"))
	assert.Equal(t, "", dbSystem(""))
}

// RecordCacheHit / RecordCacheMiss

func TestRecordCacheHit_DoesNotPanic(t *testing.T) {
	obs := Noop()
	assert.NotPanics(t, func() {
		obs.RecordCacheHit(context.Background(), "redis", "branch_head")
	})
}

func TestRecordCacheMiss_DoesNotPanic(t *testing.T) {
	obs := Noop()
	assert.NotPanics(t, func() {
		obs.RecordCacheMiss(context.Background(), "redis", "commit_cache")
	})
}

func TestRecordCacheHit_AllBackends_DoNotPanic(t *testing.T) {
	obs := Noop()
	for _, b := range []string{"redis", "postgres", "neo4j"} {
		assert.NotPanics(t, func() {
			obs.RecordCacheHit(context.Background(), b, "some-cache")
		}, "backend=%q", b)
	}
}

func TestRecordCacheMiss_AllBackends_DoNotPanic(t *testing.T) {
	obs := Noop()
	for _, b := range []string{"redis", "postgres", "neo4j"} {
		assert.NotPanics(t, func() {
			obs.RecordCacheMiss(context.Background(), b, "some-cache")
		}, "backend=%q", b)
	}
}

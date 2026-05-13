package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommit_Fields(t *testing.T) {
	now := time.Now().UTC()
	c := Commit{
		ID:        "commit_abc",
		RepoID:    "repo_xyz",
		ParentIDs: []string{"commit_parent1"},
		DataPointer: DataPointer{
			Type:     "s3",
			Location: "s3://bucket/key",
		},
		Message:        "Add feature",
		Author:         "alice@example.com",
		Timestamp:      now,
		IdempotencyKey: "idempotency_123",
	}

	assert.Equal(t, "commit_abc", c.ID)
	assert.Equal(t, "repo_xyz", c.RepoID)
	assert.Equal(t, []string{"commit_parent1"}, c.ParentIDs)
	assert.Equal(t, "s3", c.DataPointer.Type)
	assert.Equal(t, "s3://bucket/key", c.DataPointer.Location)
	assert.Equal(t, "Add feature", c.Message)
	assert.Equal(t, "alice@example.com", c.Author)
	assert.Equal(t, now, c.Timestamp)
	assert.Equal(t, "idempotency_123", c.IdempotencyKey)
}

func TestDataPointer_Fields(t *testing.T) {
	metadata := json.RawMessage(`{"key":"value"}`)
	dp := DataPointer{
		Type:     "s3",
		Location: "s3://bucket/key",
		Hash:     "sha256:abc123",
		Metadata: metadata,
	}

	assert.Equal(t, "s3", dp.Type)
	assert.Equal(t, "s3://bucket/key", dp.Location)
	assert.Equal(t, "sha256:abc123", dp.Hash)
	assert.Equal(t, metadata, dp.Metadata)
}

func TestDataPointerValidate_ValidTypes_Accepted(t *testing.T) {
	validTypes := []string{"s3", "url", "db", "custom"}

	for _, validType := range validTypes {
		t.Run(validType, func(t *testing.T) {
			dp := DataPointer{
				Type:     validType,
				Location: "some-location",
			}
			err := dp.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestDataPointerValidate_InvalidType_Rejected(t *testing.T) {
	dp := DataPointer{
		Type:     "invalid-type",
		Location: "some-location",
	}

	err := dp.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data_pointer.type must be one of")
}

func TestDataPointerValidate_EmptyLocation_Rejected(t *testing.T) {
	dp := DataPointer{
		Type:     "s3",
		Location: "",
	}

	err := dp.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data_pointer.location is required")
}

func TestDataPointerValidate_WhitespaceOnlyLocation_Rejected(t *testing.T) {
	dp := DataPointer{
		Type:     "s3",
		Location: "   ",
	}

	err := dp.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data_pointer.location is required")
}

func TestDataPointerValidate_ValidHashFormat_Accepted(t *testing.T) {
	dp := DataPointer{
		Type:     "s3",
		Location: "s3://bucket/key",
		Hash:     "sha256:abc123def456",
	}

	err := dp.Validate()
	assert.NoError(t, err)
}

func TestDataPointerValidate_EmptyHash_Accepted(t *testing.T) {
	dp := DataPointer{
		Type:     "s3",
		Location: "s3://bucket/key",
		Hash:     "",
	}

	err := dp.Validate()
	assert.NoError(t, err, "empty hash should be accepted (optional field)")
}

func TestDataPointerValidate_InvalidHashFormat_Rejected(t *testing.T) {
	cases := []struct {
		hash          string
		expectedError string
	}{
		{"abc123", "must start with 'sha256:'"},
		{"md5:abc123", "must start with 'sha256:'"},
		{"SHA256:abc123", "must start with 'sha256:'"},
		{"sha256", "must start with 'sha256:'"},
		{"sha256:", "must have content after 'sha256:' prefix"},
	}

	for _, tc := range cases {
		t.Run(tc.hash, func(t *testing.T) {
			dp := DataPointer{
				Type:     "s3",
				Location: "s3://bucket/key",
				Hash:     tc.hash,
			}

			err := dp.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedError)
		})
	}
}

func TestDataPointerValidate_MetadataOptional_Accepted(t *testing.T) {
	// without metadata
	dp1 := DataPointer{
		Type:     "s3",
		Location: "s3://bucket/key",
	}
	assert.NoError(t, dp1.Validate())

	// with metadata
	dp2 := DataPointer{
		Type:     "s3",
		Location: "s3://bucket/key",
		Metadata: json.RawMessage(`{"key":"value"}`),
	}
	assert.NoError(t, dp2.Validate())
}

func TestCommitValidate_ZeroParents_Valid(t *testing.T) {
	c := Commit{
		ID:        "commit_root",
		RepoID:    "repo_abc",
		ParentIDs: []string{}, // root commit
		DataPointer: DataPointer{
			Type:     "s3",
			Location: "s3://bucket/key",
		},
		Message:   "Initial commit",
		Author:    "alice@example.com",
		Timestamp: time.Now().UTC(),
	}

	err := c.Validate()
	assert.NoError(t, err, "root commit with zero parents should be valid")
}

func TestCommitValidate_OneParent_Valid(t *testing.T) {
	c := Commit{
		ID:        "commit_regular",
		RepoID:    "repo_abc",
		ParentIDs: []string{"commit_parent"}, // regular commit
		DataPointer: DataPointer{
			Type:     "s3",
			Location: "s3://bucket/key",
		},
		Message:   "Regular commit",
		Author:    "alice@example.com",
		Timestamp: time.Now().UTC(),
	}

	err := c.Validate()
	assert.NoError(t, err, "regular commit with one parent should be valid")
}

func TestCommitValidate_TwoParents_Rejected(t *testing.T) {
	c := Commit{
		ID:        "commit_merge",
		RepoID:    "repo_abc",
		ParentIDs: []string{"commit_parent1", "commit_parent2"}, // merge commit
		DataPointer: DataPointer{
			Type:     "s3",
			Location: "s3://bucket/key",
		},
		Message:   "Merge commit",
		Author:    "alice@example.com",
		Timestamp: time.Now().UTC(),
	}

	err := c.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "use /merges endpoint")
}

func TestCommitValidate_ThreeOrMoreParents_Rejected(t *testing.T) {
	c := Commit{
		ID:        "commit_invalid",
		RepoID:    "repo_abc",
		ParentIDs: []string{"parent1", "parent2", "parent3"}, // invalid
		DataPointer: DataPointer{
			Type:     "s3",
			Location: "s3://bucket/key",
		},
		Message:   "Invalid commit",
		Author:    "alice@example.com",
		Timestamp: time.Now().UTC(),
	}

	err := c.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "use /merges endpoint")
}

func TestErrCommitNotFound_IsNotNil(t *testing.T) {
	assert.NotNil(t, ErrCommitNotFound)
}

func TestErrCommitNotFound_HasMessage(t *testing.T) {
	assert.NotEmpty(t, ErrCommitNotFound.Error())
}

func TestErrInvalidParent_IsNotNil(t *testing.T) {
	assert.NotNil(t, ErrInvalidParent)
}

func TestErrInvalidParent_HasMessage(t *testing.T) {
	assert.NotEmpty(t, ErrInvalidParent.Error())
}

func TestErrStaleMergeTarget_IsNotNil(t *testing.T) {
	assert.NotNil(t, ErrStaleMergeTarget)
}

func TestErrStaleMergeTarget_HasMessage(t *testing.T) {
	assert.NotEmpty(t, ErrStaleMergeTarget.Error())
}

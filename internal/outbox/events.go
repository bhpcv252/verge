package outbox

import (
	"encoding/json"
	"time"
)

const (
	EventTypeCommitCreated   = "CommitCreated"
	EventTypeBranchHeadMoved = "BranchHeadMoved"
)

type OutboxEvent struct {
	ID        string
	EventType string
	Payload   json.RawMessage
	CreatedAt time.Time
}

type CommitCreatedPayload struct {
	CommitID  string   `json:"commit_id"`
	RepoID    string   `json:"repo_id"`
	ParentIDs []string `json:"parent_ids"`
	Author    string   `json:"author"`
	Message   string   `json:"message"`
	Timestamp string   `json:"timestamp"`
}

type BranchHeadMovedPayload struct {
	RepoID   string `json:"repo_id"`
	Branch   string `json:"branch"`
	CommitID string `json:"commit_id"`
	// version is the unix-millisecond timestamp of the outbox event's created_at
	Version int64 `json:"version"`
}

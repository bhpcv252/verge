package outbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type Neo4jHandler struct {
	driver neo4j.DriverWithContext
}

func NewNeo4jHandler(driver neo4j.DriverWithContext) *Neo4jHandler {
	return &Neo4jHandler{driver: driver}
}

func (h *Neo4jHandler) EventTypes() []string {
	return []string{EventTypeCommitCreated}
}

func (h *Neo4jHandler) Handle(ctx context.Context, event OutboxEvent) error {
	var payload CommitCreatedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("neo4j handler: unmarshal payload: %w", err)
	}

	session := h.driver.NewSession(ctx, neo4j.SessionConfig{
		AccessMode: neo4j.AccessModeWrite,
	})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// MERGE the commit node, SET all properties
		_, err := tx.Run(ctx, `
			MERGE (c:Commit {id: $id})
			SET c.repo_id   = $repo_id,
			    c.author    = $author,
			    c.message   = $message,
			    c.timestamp = $timestamp`,
			map[string]interface{}{
				"id":        payload.CommitID,
				"repo_id":   payload.RepoID,
				"author":    payload.Author,
				"message":   payload.Message,
				"timestamp": payload.Timestamp,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("merge commit node: %w", err)
		}

		// if the parent node doesn't exist yet, create it as a stub
		for _, parentID := range payload.ParentIDs {
			_, err := tx.Run(ctx, `
				MERGE (p:Commit {id: $parent_id})
				MERGE (c:Commit {id: $commit_id})
				MERGE (c)-[:PARENT_OF]->(p)`,
				map[string]interface{}{
					"parent_id": parentID,
					"commit_id": payload.CommitID,
				},
			)
			if err != nil {
				return nil, fmt.Errorf("merge parent edge for %s: %w", parentID, err)
			}
		}

		return nil, nil
	})
	if err != nil {
		return fmt.Errorf("neo4j handler: write transaction: %w", err)
	}

	return nil
}

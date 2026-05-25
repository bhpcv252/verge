package neo4j

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/storage/interfaces"
)

type GraphStore struct {
	driver neo4j.DriverWithContext
}

func NewGraphStore(driver neo4j.DriverWithContext) *GraphStore {
	return &GraphStore{driver: driver}
}

func (s *GraphStore) TraverseDAG(
	ctx context.Context,
	params interfaces.TraversalParams,
) ([]*domain.Commit, string, error) {
	if params.Head == "" {
		return nil, "", fmt.Errorf("neo4j graph: TraverseDAG requires a Head commit ID")
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}

	// compute skip from cursor (cursor encodes the offset for Neo4j)
	skip := 0
	if params.Cursor != "" {
		if _, err := fmt.Sscanf(params.Cursor, "%d", &skip); err != nil {
			return nil, "", fmt.Errorf("neo4j graph: invalid cursor: %w", err)
		}
	}

	cypher := `
		MATCH (start:Commit {id: $head, repo_id: $repo_id})-[:PARENT_OF*0..]->(a:Commit)
		WHERE a.repo_id = $repo_id
		  AND ($author IS NULL OR a.author = $author)
		  AND ($since  IS NULL OR a.timestamp >= $since)
		  AND ($until  IS NULL OR a.timestamp <= $until)
		RETURN a
		ORDER BY a.timestamp DESC
		SKIP $skip LIMIT $limit`

	params_ := map[string]interface{}{
		"head":    params.Head,
		"repo_id": params.RepoID,
		"author":  nullableString(params.Author),
		"since":   nullableTime(params.Since),
		"until":   nullableTime(params.Until),
		"skip":    skip,
		"limit":   limit + 1, // +1 to detect next page
	}

	commits, err := s.runCommitQuery(ctx, cypher, params_)
	if err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(commits) > limit {
		nextCursor = fmt.Sprintf("%d", skip+limit)
		commits = commits[:limit]
	}

	return commits, nextCursor, nil
}

func (s *GraphStore) GetAncestors(
	ctx context.Context,
	repoID, commitID string,
	limit int,
) ([]*domain.Commit, error) {
	if limit <= 0 {
		limit = 20
	}

	cypher := `
		MATCH (c:Commit {id: $commit_id, repo_id: $repo_id})-[:PARENT_OF*1..]->(a:Commit)
		WHERE a.repo_id = $repo_id
		RETURN a
		ORDER BY a.timestamp DESC
		LIMIT $limit`

	return s.runCommitQuery(ctx, cypher, map[string]interface{}{
		"commit_id": commitID,
		"repo_id":   repoID,
		"limit":     limit,
	})
}

func (s *GraphStore) FindMergeBase(
	ctx context.Context,
	repoID, commitA, commitB string,
) (*domain.Commit, error) {
	cypher := `
		MATCH (a:Commit {id: $commit_a})-[:PARENT_OF*0..]->(common:Commit)
		MATCH (b:Commit {id: $commit_b})-[:PARENT_OF*0..]->(common)
		WHERE common.repo_id = $repo_id
		RETURN common
		ORDER BY common.timestamp DESC
		LIMIT 1`

	commits, err := s.runCommitQuery(ctx, cypher, map[string]interface{}{
		"commit_a": commitA,
		"commit_b": commitB,
		"repo_id":  repoID,
	})
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return nil, fmt.Errorf("neo4j graph: no merge base found for %s and %s", commitA, commitB)
	}
	return commits[0], nil
}

func (s *GraphStore) runCommitQuery(
	ctx context.Context,
	cypher string,
	params map[string]interface{},
) ([]*domain.Commit, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{
		AccessMode: neo4j.AccessModeRead,
	})
	defer session.Close(ctx)

	result, err := session.Run(ctx, cypher, params)
	if err != nil {
		return nil, fmt.Errorf("neo4j graph: run query: %w", err)
	}

	var commits []*domain.Commit
	for result.Next(ctx) {
		record := result.Record()
		nodeVal, ok := record.Get("a")
		if !ok {
			nodeVal, ok = record.Get("common")
		}
		if !ok {
			continue
		}

		node, ok := nodeVal.(neo4j.Node)
		if !ok {
			continue
		}

		commit, err := nodeToCommit(node)
		if err != nil {
			return nil, fmt.Errorf("neo4j graph: map node: %w", err)
		}
		commits = append(commits, commit)
	}

	if err := result.Err(); err != nil {
		return nil, fmt.Errorf("neo4j graph: result error: %w", err)
	}

	return commits, nil
}

func nodeToCommit(node neo4j.Node) (*domain.Commit, error) {
	props := node.Props

	id, _ := props["id"].(string)
	repoID, _ := props["repo_id"].(string)
	author, _ := props["author"].(string)
	message, _ := props["message"].(string)
	tsStr, _ := props["timestamp"].(string)

	var ts time.Time
	if tsStr != "" {
		var err error
		ts, err = time.Parse(time.RFC3339, tsStr)
		if err != nil {
			return nil, fmt.Errorf("parse timestamp %q: %w", tsStr, err)
		}
	}

	return &domain.Commit{
		ID:        id,
		RepoID:    repoID,
		Author:    author,
		Message:   message,
		Timestamp: ts,
		ParentIDs: []string{}, // hydrate from postgres/redis as needed
	}, nil
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullableTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}

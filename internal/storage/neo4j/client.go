package neo4j

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func NewDriver(ctx context.Context, url string) (neo4j.DriverWithContext, error) {
	driver, err := neo4j.NewDriverWithContext(url, neo4j.NoAuth())
	if err != nil {
		return nil, fmt.Errorf("neo4j: create driver: %w", err)
	}

	if err := driver.VerifyConnectivity(ctx); err != nil {
		_ = driver.Close(ctx)
		return nil, fmt.Errorf("neo4j: verify connectivity: %w", err)
	}

	if err := createIndexes(ctx, driver); err != nil {
		_ = driver.Close(ctx)
		return nil, fmt.Errorf("neo4j: create indexes: %w", err)
	}

	return driver, nil
}

func createIndexes(ctx context.Context, driver neo4j.DriverWithContext) error {
	session := driver.NewSession(ctx, neo4j.SessionConfig{
		AccessMode: neo4j.AccessModeWrite,
	})
	defer session.Close(ctx)

	indexes := []string{
		`CREATE INDEX commit_id_index IF NOT EXISTS FOR (c:Commit) ON (c.id)`,
		`CREATE INDEX commit_repo_index IF NOT EXISTS FOR (c:Commit) ON (c.repo_id)`,
		`CREATE INDEX commit_repo_ts IF NOT EXISTS FOR (c:Commit) ON (c.repo_id, c.timestamp)`,
		`CREATE INDEX commit_repo_author IF NOT EXISTS FOR (c:Commit) ON (c.repo_id, c.author)`,
	}

	for _, idx := range indexes {
		if _, err := session.Run(ctx, idx, nil); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}

	return nil
}

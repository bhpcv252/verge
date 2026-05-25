package testhelper

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func SetupNeo4j(t *testing.T) neo4j.DriverWithContext {
	t.Helper()

	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "neo4j:5-community",
			ExposedPorts: []string{"7687/tcp"},
			Env: map[string]string{
				"NEO4J_AUTH": "none",
			},
			WaitingFor: wait.ForLog("Started.").
				WithStartupTimeout(3 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start neo4j container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("neo4j container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "7687")
	if err != nil {
		t.Fatalf("neo4j container port: %v", err)
	}

	boltURL := fmt.Sprintf("bolt://%s:%s", host, port.Port())

	// retry driver connection with backoff since container might not be immediately ready
	var driver neo4j.DriverWithContext
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		driver, err = neo4j.NewDriverWithContext(boltURL, neo4j.NoAuth())
		if err != nil {
			if i == maxRetries-1 {
				t.Fatalf("create neo4j driver after %d retries: %v", maxRetries, err)
			}
			time.Sleep(time.Second * time.Duration(i+1))
			continue
		}

		if err := driver.VerifyConnectivity(ctx); err != nil {
			_ = driver.Close(ctx)
			if i == maxRetries-1 {
				t.Fatalf("neo4j connectivity after %d retries: %v", maxRetries, err)
			}
			time.Sleep(time.Second * time.Duration(i+1))
			continue
		}
		break
	}

	t.Cleanup(func() {
		_ = driver.Close(ctx)
		_ = container.Terminate(ctx)
	})

	return driver
}

func Neo4jRunWrite(
	t *testing.T,
	driver neo4j.DriverWithContext,
	cypher string,
	params map[string]interface{},
) {
	t.Helper()
	ctx := context.Background()
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	if _, err := session.Run(ctx, cypher, params); err != nil {
		t.Fatalf("neo4j write %q: %v", cypher, err)
	}
}

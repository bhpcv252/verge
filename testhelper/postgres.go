package testhelper

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	pgmodule "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/bhpcv252/verge/migrations"
)

// SetupPostgres spins up a Postgres container, runs migrations,
// and returns a connection pool and cleanup function
func SetupPostgres(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	ctx := context.Background()

	ctr, err := pgmodule.Run(ctx,
		"postgres:16",
		pgmodule.WithDatabase("verge"),
		pgmodule.WithUsername("verge"),
		pgmodule.WithPassword("changeme"),
		pgmodule.BasicWaitStrategies(),
	)
	require.NoError(t, err, "failed to start postgres container")

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "failed to get connection string")

	RunMigrations(t, connStr)

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "failed to connect to postgres")

	cleanup := func() {
		pool.Close()
		_ = ctr.Terminate(ctx)
	}

	return pool, cleanup
}

func RunMigrations(t *testing.T, connStr string) {
	t.Helper()

	src, err := iofs.New(migrations.Files, ".")
	require.NoError(t, err)

	sqlDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	defer sqlDB.Close()

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	require.NoError(t, err)

	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	require.NoError(t, err)

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migration failed: %v", err)
	}
}

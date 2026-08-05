package integration

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// startPostgres boots a disposable Postgres+PostGIS container, applies every
// migration and returns a ready-to-use pool. Every test in this package that
// needs real persistence calls this instead of mocking pgx.
func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgis/postgis:17-3.6-alpine",
		tcpostgres.WithDatabase("llmatch_test"),
		tcpostgres.WithUsername("llmatch_test"),
		tcpostgres.WithPassword("llmatch_test_password"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	connString, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	runMigrations(t, connString)

	pool, err := pgxpool.New(ctx, connString)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func runMigrations(t *testing.T, connString string) {
	t.Helper()
	database, err := sql.Open("pgx", connString)
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	driver, err := pgxmigrate.WithInstance(database, &pgxmigrate.Config{})
	require.NoError(t, err)

	migrator, err := migrate.NewWithDatabaseInstance("file://../../migrations", "pgx5", driver)
	require.NoError(t, err)
	defer func() { _, _ = migrator.Close() }()

	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		require.NoError(t, err)
	}
}

// TestPostgresContainerAppliesMigrations is a smoke test for the shared
// container/migration setup the rest of this package's tests rely on.
func TestPostgresContainerAppliesMigrations(t *testing.T) {
	pool := startPostgres(t)
	var tableCount int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('users', 'refresh_tokens')`,
	).Scan(&tableCount)
	require.NoError(t, err)
	require.Equal(t, 2, tableCount)
}

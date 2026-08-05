package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"

	migrate "github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "llmatch-migrate: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	databaseURL := os.Getenv("MIGRATIONS_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("MIGRATIONS_DATABASE_URL is required")
	}
	sourceURL := os.Getenv("MIGRATIONS_SOURCE_URL")
	if sourceURL == "" {
		sourceURL = "file:///app/migrations"
	}

	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open migration database: %w", err)
	}
	defer func() { _ = database.Close() }()
	driver, err := pgxmigrate.WithInstance(database, &pgxmigrate.Config{})
	if err != nil {
		return fmt.Errorf("create pgx migration driver: %w", err)
	}
	migrator, err := migrate.NewWithDatabaseInstance(sourceURL, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer func() { _, _ = migrator.Close() }()
	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

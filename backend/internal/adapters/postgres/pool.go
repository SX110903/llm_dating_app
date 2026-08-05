package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, databaseURL string, minConnections, maxConnections int32) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse postgres configuration: %w", err)
	}
	config.MinConns = minConnections
	config.MaxConns = maxConnections

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}
	return pool, nil
}

type Checker struct {
	Pool *pgxpool.Pool
}

func (Checker) Name() string { return "postgres" }

func (c Checker) Check(ctx context.Context) error {
	return c.Pool.Ping(ctx)
}

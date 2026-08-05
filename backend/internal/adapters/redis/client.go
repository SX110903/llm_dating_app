package redis

import (
	"context"
	"fmt"

	redisclient "github.com/redis/go-redis/v9"
)

func NewClient(rawURL string) (*redisclient.Client, error) {
	options, err := redisclient.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis configuration: %w", err)
	}
	return redisclient.NewClient(options), nil
}

type Checker struct {
	Client *redisclient.Client
}

func (Checker) Name() string { return "redis" }

func (c Checker) Check(ctx context.Context) error {
	return c.Client.Ping(ctx).Err()
}

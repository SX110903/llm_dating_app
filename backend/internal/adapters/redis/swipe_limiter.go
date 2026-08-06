package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
)

var reserveSwipeScript = redisclient.NewScript(`
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
local persisted = tonumber(ARGV[1])
local daily_limit = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])

if current < persisted then
  current = persisted
  redis.call('SET', KEYS[1], current, 'EX', ttl)
end
if current >= daily_limit then
  return 0
end

current = redis.call('INCR', KEYS[1])
redis.call('EXPIRE', KEYS[1], ttl)
return 1
`)

var releaseSwipeScript = redisclient.NewScript(`
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
local floor = tonumber(ARGV[1])
if current > floor then
  redis.call('DECR', KEYS[1])
end
return 1
`)

type SwipeLimiter struct {
	client *redisclient.Client
}

func NewSwipeLimiter(client *redisclient.Client) *SwipeLimiter {
	return &SwipeLimiter{client: client}
}

func (l *SwipeLimiter) Reserve(
	ctx context.Context,
	userID uuid.UUID,
	dayStart time.Time,
	dailyLimit int,
	persistedCount int,
) (bool, time.Duration, error) {
	retryAfter := dayStart.Add(24 * time.Hour).Sub(time.Now().UTC())
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	ttl := int(retryAfter.Seconds()) + 3600
	result, err := reserveSwipeScript.Run(ctx, l.client, []string{swipeLimitKey(userID, dayStart)}, persistedCount, dailyLimit, ttl).Int()
	if err != nil {
		return false, 0, fmt.Errorf("reserve daily swipe: %w", err)
	}
	return result == 1, retryAfter, nil
}

func (l *SwipeLimiter) Release(ctx context.Context, userID uuid.UUID, dayStart time.Time, persistedFloor int) error {
	if err := releaseSwipeScript.Run(ctx, l.client, []string{swipeLimitKey(userID, dayStart)}, persistedFloor).Err(); err != nil {
		return fmt.Errorf("release daily swipe: %w", err)
	}
	return nil
}

func swipeLimitKey(userID uuid.UUID, dayStart time.Time) string {
	return "matching:swipes:" + dayStart.UTC().Format("2006-01-02") + ":" + userID.String()
}

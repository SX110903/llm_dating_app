package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
)

// messageRateScript is a fixed-window counter: the first send of a window sets
// the expiry, and the caller is told how long is left when the cap is hit.
var messageRateScript = redisclient.NewScript(`
local current = redis.call('INCR', KEYS[1])
if current == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
if current > tonumber(ARGV[2]) then
  local ttl = redis.call('TTL', KEYS[1])
  if ttl < 0 then
    ttl = tonumber(ARGV[1])
  end
  return -ttl
end
return current
`)

// MessageRateLimiter caps how many messages a user may send per window. It is
// distributed, so the limit holds across backend replicas.
type MessageRateLimiter struct {
	client *redisclient.Client
	window time.Duration
	limit  int
}

func NewMessageRateLimiter(client *redisclient.Client, window time.Duration, limit int) *MessageRateLimiter {
	return &MessageRateLimiter{client: client, window: window, limit: limit}
}

func (l *MessageRateLimiter) Allow(ctx context.Context, userID uuid.UUID) (bool, time.Duration, error) {
	key := fmt.Sprintf("messaging:rate:%s", userID)
	result, err := messageRateScript.Run(ctx, l.client, []string{key},
		int(l.window.Seconds()), l.limit,
	).Int64()
	if err != nil {
		return false, 0, fmt.Errorf("check message rate limit: %w", err)
	}
	if result < 0 {
		return false, time.Duration(-result) * time.Second, nil
	}
	return true, 0, nil
}

package redis

import (
	"context"
	"fmt"
	"time"

	redisclient "github.com/redis/go-redis/v9"
)

// recordFailureScript atomically increments the attempt counter for a scope
// (e.g. "login:ip:203.0.113.1" or "login:email:a@b.com") and, once the
// window threshold is exceeded, applies a lockout whose duration doubles
// with each repeated violation up to maxLockout.
var recordFailureScript = redisclient.NewScript(`
local attempts = redis.call('INCR', KEYS[1])
if attempts == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
if attempts >= tonumber(ARGV[2]) then
  local violations = redis.call('INCR', KEYS[3])
  redis.call('EXPIRE', KEYS[3], 86400)
  local lockout = tonumber(ARGV[3]) * (2 ^ (violations - 1))
  if lockout > tonumber(ARGV[4]) then
    lockout = tonumber(ARGV[4])
  end
  lockout = math.floor(lockout)
  redis.call('SET', KEYS[2], '1', 'EX', lockout)
  redis.call('DEL', KEYS[1])
  return lockout
end
return 0
`)

type RateLimiter struct {
	client      *redisclient.Client
	window      time.Duration
	maxAttempts int
	baseLockout time.Duration
	maxLockout  time.Duration
}

func NewRateLimiter(client *redisclient.Client) *RateLimiter {
	return &RateLimiter{
		client:      client,
		window:      15 * time.Minute,
		maxAttempts: 5,
		baseLockout: 5 * time.Minute,
		maxLockout:  time.Hour,
	}
}

func (l *RateLimiter) attemptsKey(scope string) string   { return "ratelimit:" + scope + ":attempts" }
func (l *RateLimiter) lockoutKey(scope string) string    { return "ratelimit:" + scope + ":lockout" }
func (l *RateLimiter) violationsKey(scope string) string { return "ratelimit:" + scope + ":violations" }

// Allowed reports whether scope is currently locked out and, if so, for how long.
func (l *RateLimiter) Allowed(ctx context.Context, scope string) (bool, time.Duration, error) {
	ttl, err := l.client.TTL(ctx, l.lockoutKey(scope)).Result()
	if err != nil {
		return false, 0, fmt.Errorf("check rate limit lockout: %w", err)
	}
	if ttl > 0 {
		return false, ttl, nil
	}
	return true, 0, nil
}

// RecordFailure registers a failed attempt for scope. It returns the lockout
// duration just applied, or zero if the scope is still under the threshold.
func (l *RateLimiter) RecordFailure(ctx context.Context, scope string) (time.Duration, error) {
	keys := []string{l.attemptsKey(scope), l.lockoutKey(scope), l.violationsKey(scope)}
	result, err := recordFailureScript.Run(ctx, l.client, keys,
		int(l.window.Seconds()), l.maxAttempts, int(l.baseLockout.Seconds()), int(l.maxLockout.Seconds()),
	).Int()
	if err != nil {
		return 0, fmt.Errorf("record rate limit failure: %w", err)
	}
	return time.Duration(result) * time.Second, nil
}

func (l *RateLimiter) Reset(ctx context.Context, scope string) error {
	if err := l.client.Del(ctx, l.attemptsKey(scope), l.lockoutKey(scope)).Err(); err != nil {
		return fmt.Errorf("reset rate limit: %w", err)
	}
	return nil
}

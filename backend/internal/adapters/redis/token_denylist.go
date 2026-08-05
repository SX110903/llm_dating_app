package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
)

const (
	denylistKeyPrefix  = "auth:denylist:"
	activeJTIKeyPrefix = "auth:active:"
)

// denylistAllActiveScript moves every still-active jti tracked for a user
// into the denylist, each with a TTL matching its own remaining lifetime,
// then drops the active set. It runs as a single Lua script so logout-all
// cannot race with a token being issued or checked concurrently.
var denylistAllActiveScript = redisclient.NewScript(`
local members = redis.call('ZRANGEBYSCORE', KEYS[1], ARGV[1], '+inf', 'WITHSCORES')
for i = 1, #members, 2 do
  local jti = members[i]
  local exp = tonumber(members[i + 1])
  local ttl = exp - tonumber(ARGV[1])
  if ttl > 0 then
    redis.call('SET', ARGV[2] .. jti, '1', 'EX', ttl)
  end
end
redis.call('DEL', KEYS[1])
return #members / 2
`)

type TokenDenylist struct {
	client *redisclient.Client
}

func NewTokenDenylist(client *redisclient.Client) *TokenDenylist {
	return &TokenDenylist{client: client}
}

func (d *TokenDenylist) IsRevoked(ctx context.Context, jti string) (bool, error) {
	count, err := d.client.Exists(ctx, denylistKeyPrefix+jti).Result()
	if err != nil {
		return false, fmt.Errorf("check jti denylist: %w", err)
	}
	return count > 0, nil
}

func (d *TokenDenylist) Denylist(ctx context.Context, jti string, expiresAt time.Time) error {
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return nil
	}
	if err := d.client.Set(ctx, denylistKeyPrefix+jti, "1", ttl).Err(); err != nil {
		return fmt.Errorf("denylist jti: %w", err)
	}
	return nil
}

// RegisterActive records jti as belonging to userID until expiresAt so a
// later logout-all can find and revoke it.
func (d *TokenDenylist) RegisterActive(ctx context.Context, userID uuid.UUID, jti string, expiresAt time.Time) error {
	key := activeJTIKeyPrefix + userID.String()
	if err := d.client.ZAdd(ctx, key, redisclient.Z{Score: float64(expiresAt.Unix()), Member: jti}).Err(); err != nil {
		return fmt.Errorf("register active jti: %w", err)
	}
	return nil
}

func (d *TokenDenylist) DenylistAllActive(ctx context.Context, userID uuid.UUID) error {
	key := activeJTIKeyPrefix + userID.String()
	now := time.Now().UTC().Unix()
	if err := denylistAllActiveScript.Run(ctx, d.client, []string{key}, now, denylistKeyPrefix).Err(); err != nil {
		return fmt.Errorf("denylist all active jti: %w", err)
	}
	return nil
}

package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"

	domainmessaging "github.com/sx110903/llmatch-v2/backend/internal/domain/messaging"
)

const wsTicketKeyPrefix = "ws:ticket:"

// consumeTicketScript reads and deletes the ticket in one atomic step, so two
// concurrent handshakes with the same ticket can never both succeed.
var consumeTicketScript = redisclient.NewScript(`
local owner = redis.call('GET', KEYS[1])
if not owner then
  return nil
end
redis.call('DEL', KEYS[1])
return owner
`)

// WSTicketStore issues and redeems the single-use WebSocket handshake tickets.
type WSTicketStore struct {
	client *redisclient.Client
}

func NewWSTicketStore(client *redisclient.Client) *WSTicketStore {
	return &WSTicketStore{client: client}
}

func (s *WSTicketStore) Issue(ctx context.Context, ticket string, userID uuid.UUID, ttl time.Duration) error {
	// SetNX guards against the (astronomically unlikely) reuse of a random
	// value that is still live.
	stored, err := s.client.SetNX(ctx, wsTicketKeyPrefix+ticket, userID.String(), ttl).Result()
	if err != nil {
		return fmt.Errorf("store websocket ticket: %w", err)
	}
	if !stored {
		return errors.New("websocket ticket collision")
	}
	return nil
}

func (s *WSTicketStore) Consume(ctx context.Context, ticket string) (uuid.UUID, error) {
	raw, err := consumeTicketScript.Run(ctx, s.client, []string{wsTicketKeyPrefix + ticket}).Result()
	if err != nil {
		if errors.Is(err, redisclient.Nil) {
			// Unknown, already used or expired: all indistinguishable on purpose.
			return uuid.Nil, domainmessaging.ErrTicketInvalid
		}
		// Redis unreachable: fail closed, never assume the ticket was valid.
		return uuid.Nil, fmt.Errorf("%w: %w", domainmessaging.ErrDependencyUnavailable, err)
	}

	owner, ok := raw.(string)
	if !ok {
		return uuid.Nil, domainmessaging.ErrTicketInvalid
	}
	userID, err := uuid.Parse(owner)
	if err != nil {
		return uuid.Nil, domainmessaging.ErrTicketInvalid
	}
	return userID, nil
}

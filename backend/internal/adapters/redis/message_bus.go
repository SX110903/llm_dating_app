package redis

import (
	"context"
	"encoding/json"
	"fmt"

	redisclient "github.com/redis/go-redis/v9"

	applicationmessaging "github.com/sx110903/llmatch-v2/backend/internal/application/messaging"
)

const messageChannel = "messaging:events"

// MessageBus fans persisted messages out across backend replicas over Redis
// Pub/Sub, so a client connected to any instance receives them.
type MessageBus struct {
	client *redisclient.Client
}

func NewMessageBus(client *redisclient.Client) *MessageBus {
	return &MessageBus{client: client}
}

func (b *MessageBus) Publish(ctx context.Context, event applicationmessaging.MessageEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode message event: %w", err)
	}
	if err := b.client.Publish(ctx, messageChannel, payload).Err(); err != nil {
		return fmt.Errorf("publish message event: %w", err)
	}
	return nil
}

// Subscribe streams events until ctx is cancelled. Malformed payloads are
// skipped rather than tearing the subscription down.
func (b *MessageBus) Subscribe(ctx context.Context, handle func(applicationmessaging.MessageEvent)) error {
	subscription := b.client.Subscribe(ctx, messageChannel)
	defer func() { _ = subscription.Close() }()

	if _, err := subscription.Receive(ctx); err != nil {
		return fmt.Errorf("subscribe to message events: %w", err)
	}

	channel := subscription.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-channel:
			if !ok {
				return nil
			}
			var event applicationmessaging.MessageEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				continue
			}
			handle(event)
		}
	}
}

package messaging

import (
	"context"
	"time"

	"github.com/google/uuid"

	domainmessaging "github.com/sx110903/llmatch-v2/backend/internal/domain/messaging"
)

// TicketStore issues and consumes the single-use handshake tickets. Redis
// backs it in production; a failure must never be interpreted as "allowed".
type TicketStore interface {
	// Issue stores a freshly minted ticket bound to userID for ttl.
	Issue(ctx context.Context, ticket string, userID uuid.UUID, ttl time.Duration) error
	// Consume atomically returns the owner and deletes the ticket. A second
	// call for the same ticket must fail.
	Consume(ctx context.Context, ticket string) (uuid.UUID, error)
}

// Publisher fans a persisted message out to the other backend replicas. It is
// only ever called after the message is durably stored.
type Publisher interface {
	Publish(ctx context.Context, event MessageEvent) error
}

// RateLimiter caps how often a single user may send. It is separate from the
// auth rate limiter so messaging limits can be tuned independently.
type RateLimiter interface {
	Allow(ctx context.Context, userID uuid.UUID) (bool, time.Duration, error)
}

// MessageEvent is the payload replicated across instances. It carries no
// tokens and no ticket, only what a participant is already allowed to read.
type MessageEvent struct {
	MatchID     uuid.UUID               `json:"match_id"`
	Recipients  []uuid.UUID             `json:"recipients"`
	Message     domainmessaging.Message `json:"-"`
	MessageJSON MessagePayload          `json:"message"`
}

type MessagePayload struct {
	ID          uuid.UUID `json:"id"`
	MatchID     uuid.UUID `json:"match_id"`
	SenderID    uuid.UUID `json:"sender_id"`
	ClientNonce uuid.UUID `json:"client_nonce"`
	Type        string    `json:"type"`
	Content     string    `json:"content,omitempty"`
	StorageKey  string    `json:"storage_key,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func NewMessagePayload(message domainmessaging.Message) MessagePayload {
	return MessagePayload{
		ID:          message.ID,
		MatchID:     message.MatchID,
		SenderID:    message.SenderID,
		ClientNonce: message.ClientNonce,
		Type:        string(message.Type),
		Content:     message.Content,
		StorageKey:  message.StorageKey,
		CreatedAt:   message.CreatedAt,
	}
}

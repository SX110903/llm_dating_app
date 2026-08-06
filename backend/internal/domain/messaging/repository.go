package messaging

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository is the persistence port for conversations. Infrastructure owns
// transactions, row conversion and the uniqueness that makes sends idempotent.
type Repository interface {
	// GetActiveParticipants returns the match only when it is still active and
	// viewerID belongs to it, with no block in either direction. Any other
	// situation is ErrMatchNotAccessible.
	GetActiveParticipants(ctx context.Context, matchID, viewerID uuid.UUID) (*Participants, error)

	// Send inserts the message, or returns the row already stored under the
	// same (sender_id, client_nonce) with Created=false.
	Send(ctx context.Context, message *Message) (*SendResult, error)

	ListHistory(ctx context.Context, params HistoryParams) ([]Message, error)

	// MarkRead flags every unread message the viewer received in the match up
	// to and including at, and reports how many rows changed. Re-running it is
	// a no-op, which makes read receipts idempotent.
	MarkRead(ctx context.Context, matchID, viewerID uuid.UUID, at time.Time) (int, error)

	ListConversations(ctx context.Context, viewerID uuid.UUID, limit int) ([]ConversationSummary, error)
}

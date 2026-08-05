package session

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository is an outbound application port implemented by infrastructure adapters.
type Repository interface {
	Create(ctx context.Context, token *RefreshToken) error
	FindByHash(ctx context.Context, tokenHash []byte) (*RefreshToken, error)

	// Rotate atomically consumes the token identified by currentID and persists next
	// as its replacement. If currentID is already revoked or already replaced, the
	// whole family is revoked with RevokeReasonReuseDetected and ErrReused is returned.
	Rotate(ctx context.Context, currentID uuid.UUID, next *RefreshToken) error

	RevokeFamily(ctx context.Context, familyID uuid.UUID, reason string, at time.Time) error
	RevokeAllForUser(ctx context.Context, userID uuid.UUID, reason string, at time.Time) error
}

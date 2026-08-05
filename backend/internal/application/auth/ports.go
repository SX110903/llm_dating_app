package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// PasswordHasher is an outbound port over the Argon2id adapter.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, encodedHash string) (bool, error)
	NeedsRehash(encodedHash string) (bool, error)
}

// AccessTokenIssuer is an outbound port over the RS256 JWT adapter.
type AccessTokenIssuer interface {
	Issue(subject string) (signed, jti string, expiresAt time.Time, err error)
}

// RefreshTokenGenerator is an outbound port over the opaque token adapter.
type RefreshTokenGenerator interface {
	New() (token string, hash []byte, err error)
	Hash(token string) ([]byte, error)
}

// Denylist is an outbound port over the Redis jti revocation adapter.
type Denylist interface {
	RegisterActive(ctx context.Context, userID uuid.UUID, jti string, expiresAt time.Time) error
	Denylist(ctx context.Context, jti string, expiresAt time.Time) error
	DenylistAllActive(ctx context.Context, userID uuid.UUID) error
}

// RateLimiter is an outbound port over the Redis rate limiting adapter.
type RateLimiter interface {
	Allowed(ctx context.Context, scope string) (bool, time.Duration, error)
	RecordFailure(ctx context.Context, scope string) (time.Duration, error)
	Reset(ctx context.Context, scope string) error
}

package matching

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"
)

// DailySwipeLimiter reserves capacity in the fast counter while PostgreSQL
// remains the source of truth supplied through persistedCount.
type DailySwipeLimiter interface {
	Reserve(
		ctx context.Context,
		userID uuid.UUID,
		dayStart time.Time,
		dailyLimit int,
		persistedCount int,
	) (allowed bool, retryAfter time.Duration, err error)
	Release(ctx context.Context, userID uuid.UUID, dayStart time.Time, persistedFloor int) error
}

// PhotoReader is the smallest storage port matching needs. Visibility is
// authorized by the repository before this port ever receives a storage key.
type PhotoReader interface {
	Get(ctx context.Context, key string) (io.ReadCloser, error)
}

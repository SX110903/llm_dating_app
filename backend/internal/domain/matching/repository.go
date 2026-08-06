package matching

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository is the persistence port for discovery, swipes, matches and
// user-safety actions. Infrastructure owns transactions and row conversion.
type Repository interface {
	EnsureDiscoveryReady(ctx context.Context, userID uuid.UUID) error
	ListCandidates(ctx context.Context, params DiscoveryParams) ([]Candidate, error)

	FindSwipe(ctx context.Context, actorID, targetID uuid.UUID) (*SwipeOutcome, error)
	RecordSwipe(ctx context.Context, swipe Swipe) (*SwipeOutcome, error)
	CountSwipesSince(ctx context.Context, actorID uuid.UUID, since time.Time) (int, error)

	ListMatches(ctx context.Context, params MatchListParams) ([]MatchSummary, error)
	Unmatch(ctx context.Context, matchID, actorID uuid.UUID, at time.Time) error
	Block(ctx context.Context, blockerID, blockedID uuid.UUID, at time.Time) error
	CreateReport(ctx context.Context, report *Report) error

	GetVisiblePhoto(ctx context.Context, viewerID, photoID uuid.UUID) (*VisiblePhoto, error)
	TouchActivity(ctx context.Context, userID uuid.UUID, at time.Time) error
}

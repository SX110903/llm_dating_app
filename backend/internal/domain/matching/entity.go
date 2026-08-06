package matching

import (
	"time"

	"github.com/google/uuid"
)

type SwipeAction string

const (
	SwipeLike      SwipeAction = "like"
	SwipeDislike   SwipeAction = "dislike"
	SwipeSuperlike SwipeAction = "superlike"
)

func (a SwipeAction) IsValid() bool {
	return a == SwipeLike || a == SwipeDislike || a == SwipeSuperlike
}

func (a SwipeAction) IsPositive() bool {
	return a == SwipeLike || a == SwipeSuperlike
}

type Swipe struct {
	ID        uuid.UUID
	ActorID   uuid.UUID
	TargetID  uuid.UUID
	Action    SwipeAction
	CreatedAt time.Time
}

type Match struct {
	ID          uuid.UUID
	UserLowID   uuid.UUID
	UserHighID  uuid.UUID
	MatchedAt   time.Time
	UnmatchedAt *time.Time
	UnmatchedBy *uuid.UUID
}

func OrderedPair(first, second uuid.UUID) (uuid.UUID, uuid.UUID) {
	if first.String() < second.String() {
		return first, second
	}
	return second, first
}

type ScoreBreakdown struct {
	Interests     float64
	Questionnaire float64
	Distance      float64
	Activity      float64
	Total         float64
}

type Candidate struct {
	UserID         uuid.UUID
	DisplayName    string
	Age            int
	Gender         string
	Bio            string
	Interests      []string
	City           string
	DistanceKM     float64
	LastActiveAt   time.Time
	PrimaryPhotoID uuid.UUID
	Score          ScoreBreakdown
}

type MatchSummary struct {
	Match           Match
	OtherUserID     uuid.UUID
	DisplayName     string
	Bio             string
	City            string
	PrimaryPhotoID  uuid.UUID
	OtherLastActive time.Time
}

type SwipeOutcome struct {
	Swipe   Swipe
	Match   *Match
	Created bool
}

type RankingWeights struct {
	Interests     float64
	Questionnaire float64
	Distance      float64
	Activity      float64
}

func (w RankingWeights) Total() float64 {
	return w.Interests + w.Questionnaire + w.Distance + w.Activity
}

type DiscoveryCursor struct {
	AsOf   time.Time
	Score  float64
	UserID uuid.UUID
}

type MatchCursor struct {
	MatchedAt time.Time
	MatchID   uuid.UUID
}

type DiscoveryParams struct {
	ViewerID uuid.UUID
	AsOf     time.Time
	Limit    int
	Cursor   *DiscoveryCursor
	Weights  RankingWeights
}

type MatchListParams struct {
	UserID uuid.UUID
	Limit  int
	Cursor *MatchCursor
}

type ReportReason string

const (
	ReportHarassment           ReportReason = "harassment"
	ReportSpam                 ReportReason = "spam"
	ReportInappropriateContent ReportReason = "inappropriate_content"
	ReportImpersonation        ReportReason = "impersonation"
	ReportUnderage             ReportReason = "underage"
	ReportOther                ReportReason = "other"
)

func (r ReportReason) IsValid() bool {
	switch r {
	case ReportHarassment, ReportSpam, ReportInappropriateContent, ReportImpersonation, ReportUnderage, ReportOther:
		return true
	default:
		return false
	}
}

type ReportStatus string

const (
	ReportPending   ReportStatus = "pending"
	ReportReviewing ReportStatus = "reviewing"
	ReportResolved  ReportStatus = "resolved"
	ReportDismissed ReportStatus = "dismissed"
)

type Report struct {
	ID          uuid.UUID
	ReporterID  uuid.UUID
	ReportedID  uuid.UUID
	Reason      ReportReason
	Description string
	Status      ReportStatus
	CreatedAt   time.Time
	ResolvedAt  *time.Time
}

type VisiblePhoto struct {
	ID         uuid.UUID
	OwnerID    uuid.UUID
	StorageKey string
	MimeType   string
}

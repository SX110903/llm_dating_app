package matching

import "errors"

var (
	ErrNotFound              = errors.New("matching resource not found")
	ErrSwipeNotFound         = errors.New("swipe not found")
	ErrMatchNotFound         = errors.New("match not found")
	ErrPhotoNotFound         = errors.New("visible photo not found")
	ErrDiscoveryNotReady     = errors.New("profile is not ready for discovery")
	ErrInvalidSwipeAction    = errors.New("invalid swipe action")
	ErrInvalidReportReason   = errors.New("invalid report reason")
	ErrReportTooLong         = errors.New("report description is too long")
	ErrSelfInteraction       = errors.New("cannot interact with oneself")
	ErrInteractionBlocked    = errors.New("interaction is blocked")
	ErrDailySwipeLimit       = errors.New("daily swipe limit reached")
	ErrDependencyUnavailable = errors.New("matching dependency unavailable")
)

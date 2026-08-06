package matching

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	domainmatching "github.com/sx110903/llmatch-v2/backend/internal/domain/matching"
)

const (
	defaultPageSize = 20
	maxPageSize     = 50
	maxReportLength = 1000
	cursorVersion   = 1
)

var (
	ErrInvalidCursor   = errors.New("invalid matching cursor")
	ErrInvalidPageSize = errors.New("invalid matching page size")
)

type DailyLimitError struct {
	RetryAfter time.Duration
}

func (e *DailyLimitError) Error() string {
	return fmt.Sprintf("daily swipe limit reached, retry after %s", e.RetryAfter)
}

func (e *DailyLimitError) Unwrap() error {
	return domainmatching.ErrDailySwipeLimit
}

type Config struct {
	DailySwipeLimit int
	DefaultPageSize int
	MaxPageSize     int
	RankingWeights  domainmatching.RankingWeights
}

type DiscoveryPage struct {
	Candidates []domainmatching.Candidate
	NextCursor string
}

type MatchPage struct {
	Matches    []domainmatching.MatchSummary
	NextCursor string
}

type SwipeInput struct {
	TargetID uuid.UUID
	Action   domainmatching.SwipeAction
}

type ReportInput struct {
	ReportedID  uuid.UUID
	Reason      domainmatching.ReportReason
	Description string
}

type Service struct {
	repository domainmatching.Repository
	limiter    DailySwipeLimiter
	photos     PhotoReader
	config     Config
	now        func() time.Time
}

func NewService(
	repository domainmatching.Repository,
	limiter DailySwipeLimiter,
	photos PhotoReader,
	config Config,
) *Service {
	config = withDefaults(config)
	return &Service{
		repository: repository,
		limiter:    limiter,
		photos:     photos,
		config:     config,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Discover(
	ctx context.Context,
	viewerID uuid.UUID,
	cursor string,
	requestedLimit int,
) (*DiscoveryPage, error) {
	limit, err := s.pageLimit(requestedLimit)
	if err != nil {
		return nil, err
	}

	params := domainmatching.DiscoveryParams{
		ViewerID: viewerID,
		AsOf:     s.now(),
		Limit:    limit + 1,
		Weights:  s.config.RankingWeights,
	}
	if cursor != "" {
		decoded, decodeErr := decodeDiscoveryCursor(cursor)
		if decodeErr != nil {
			return nil, decodeErr
		}
		params.AsOf = decoded.AsOf
		params.Cursor = decoded
	}

	if err := s.repository.EnsureDiscoveryReady(ctx, viewerID); err != nil {
		return nil, err
	}
	candidates, err := s.repository.ListCandidates(ctx, params)
	if err != nil {
		return nil, err
	}

	page := &DiscoveryPage{Candidates: candidates}
	if len(candidates) > limit {
		page.Candidates = candidates[:limit]
		last := page.Candidates[len(page.Candidates)-1]
		page.NextCursor, err = encodeCursor(discoveryCursorPayload{
			Version: cursorVersion,
			AsOf:    params.AsOf,
			Score:   last.Score.Total,
			UserID:  last.UserID,
		})
		if err != nil {
			return nil, fmt.Errorf("encode discovery cursor: %w", err)
		}
	}
	_ = s.repository.TouchActivity(ctx, viewerID, s.now())
	return page, nil
}

func (s *Service) Swipe(
	ctx context.Context,
	actorID uuid.UUID,
	input SwipeInput,
) (*domainmatching.SwipeOutcome, error) {
	if actorID == input.TargetID {
		return nil, domainmatching.ErrSelfInteraction
	}
	if !input.Action.IsValid() {
		return nil, domainmatching.ErrInvalidSwipeAction
	}

	existing, err := s.repository.FindSwipe(ctx, actorID, input.TargetID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, domainmatching.ErrSwipeNotFound) {
		return nil, err
	}

	now := s.now()
	dayStart := now.Truncate(24 * time.Hour)
	persistedCount, err := s.repository.CountSwipesSince(ctx, actorID, dayStart)
	if err != nil {
		return nil, err
	}
	if persistedCount >= s.config.DailySwipeLimit {
		return nil, newDailyLimitError(dayStart, now)
	}

	reserved, retryAfter, err := s.limiter.Reserve(
		ctx,
		actorID,
		dayStart,
		s.config.DailySwipeLimit,
		persistedCount,
	)
	if err != nil {
		return nil, errors.Join(
			domainmatching.ErrDependencyUnavailable,
			fmt.Errorf("reserve daily swipe: %w", err),
		)
	}
	if !reserved {
		return nil, &DailyLimitError{RetryAfter: retryAfter}
	}

	outcome, recordErr := s.repository.RecordSwipe(ctx, domainmatching.Swipe{
		ID:        uuid.New(),
		ActorID:   actorID,
		TargetID:  input.TargetID,
		Action:    input.Action,
		CreatedAt: now,
	})
	if recordErr != nil || outcome == nil || !outcome.Created {
		releaseErr := s.limiter.Release(ctx, actorID, dayStart, persistedCount)
		if recordErr != nil {
			if releaseErr != nil {
				return nil, errors.Join(recordErr, fmt.Errorf("release daily swipe reservation: %w", releaseErr))
			}
			return nil, recordErr
		}
		if outcome == nil {
			return nil, errors.New("record swipe returned no outcome")
		}
		if releaseErr != nil {
			return nil, errors.Join(
				domainmatching.ErrDependencyUnavailable,
				fmt.Errorf("release duplicate swipe reservation: %w", releaseErr),
			)
		}
	}
	_ = s.repository.TouchActivity(ctx, actorID, now)
	return outcome, nil
}

func (s *Service) ListMatches(
	ctx context.Context,
	userID uuid.UUID,
	cursor string,
	requestedLimit int,
) (*MatchPage, error) {
	limit, err := s.pageLimit(requestedLimit)
	if err != nil {
		return nil, err
	}
	params := domainmatching.MatchListParams{UserID: userID, Limit: limit + 1}
	if cursor != "" {
		params.Cursor, err = decodeMatchCursor(cursor)
		if err != nil {
			return nil, err
		}
	}

	matches, err := s.repository.ListMatches(ctx, params)
	if err != nil {
		return nil, err
	}
	page := &MatchPage{Matches: matches}
	if len(matches) > limit {
		page.Matches = matches[:limit]
		last := page.Matches[len(page.Matches)-1]
		page.NextCursor, err = encodeCursor(matchCursorPayload{
			Version:   cursorVersion,
			MatchedAt: last.Match.MatchedAt,
			MatchID:   last.Match.ID,
		})
		if err != nil {
			return nil, fmt.Errorf("encode match cursor: %w", err)
		}
	}
	_ = s.repository.TouchActivity(ctx, userID, s.now())
	return page, nil
}

func (s *Service) Unmatch(ctx context.Context, userID, matchID uuid.UUID) error {
	return s.repository.Unmatch(ctx, matchID, userID, s.now())
}

func (s *Service) Block(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	if blockerID == blockedID {
		return domainmatching.ErrSelfInteraction
	}
	return s.repository.Block(ctx, blockerID, blockedID, s.now())
}

func (s *Service) Report(
	ctx context.Context,
	reporterID uuid.UUID,
	input ReportInput,
) (*domainmatching.Report, error) {
	if reporterID == input.ReportedID {
		return nil, domainmatching.ErrSelfInteraction
	}
	if !input.Reason.IsValid() {
		return nil, domainmatching.ErrInvalidReportReason
	}
	description := strings.TrimSpace(input.Description)
	if utf8.RuneCountInString(description) > maxReportLength {
		return nil, domainmatching.ErrReportTooLong
	}

	report := &domainmatching.Report{
		ID:          uuid.New(),
		ReporterID:  reporterID,
		ReportedID:  input.ReportedID,
		Reason:      input.Reason,
		Description: description,
		Status:      domainmatching.ReportPending,
		CreatedAt:   s.now(),
	}
	if err := s.repository.CreateReport(ctx, report); err != nil {
		return nil, err
	}
	return report, nil
}

func (s *Service) GetPhotoContent(
	ctx context.Context,
	viewerID, photoID uuid.UUID,
) (io.ReadCloser, string, error) {
	photo, err := s.repository.GetVisiblePhoto(ctx, viewerID, photoID)
	if err != nil {
		return nil, "", err
	}
	reader, err := s.photos.Get(ctx, photo.StorageKey)
	if err != nil {
		return nil, "", fmt.Errorf("read matching photo: %w", err)
	}
	return reader, photo.MimeType, nil
}

func (s *Service) pageLimit(requested int) (int, error) {
	if requested < 0 || requested > s.config.MaxPageSize {
		return 0, ErrInvalidPageSize
	}
	if requested == 0 {
		return s.config.DefaultPageSize, nil
	}
	return requested, nil
}

func withDefaults(config Config) Config {
	if config.DailySwipeLimit <= 0 {
		config.DailySwipeLimit = 100
	}
	if config.MaxPageSize <= 0 || config.MaxPageSize > maxPageSize {
		config.MaxPageSize = maxPageSize
	}
	if config.DefaultPageSize <= 0 || config.DefaultPageSize > config.MaxPageSize {
		config.DefaultPageSize = defaultPageSize
	}
	if !validWeights(config.RankingWeights) {
		config.RankingWeights = domainmatching.RankingWeights{
			Interests: 0.35, Questionnaire: 0.30, Distance: 0.20, Activity: 0.15,
		}
	}
	return config
}

func validWeights(weights domainmatching.RankingWeights) bool {
	values := []float64{weights.Interests, weights.Questionnaire, weights.Distance, weights.Activity}
	for _, value := range values {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return weights.Total() > 0
}

func newDailyLimitError(dayStart, now time.Time) *DailyLimitError {
	retryAfter := dayStart.Add(24 * time.Hour).Sub(now)
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return &DailyLimitError{RetryAfter: retryAfter}
}

type discoveryCursorPayload struct {
	Version int       `json:"v"`
	AsOf    time.Time `json:"as_of"`
	Score   float64   `json:"score"`
	UserID  uuid.UUID `json:"user_id"`
}

type matchCursorPayload struct {
	Version   int       `json:"v"`
	MatchedAt time.Time `json:"matched_at"`
	MatchID   uuid.UUID `json:"match_id"`
}

func encodeCursor(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeDiscoveryCursor(encoded string) (*domainmatching.DiscoveryCursor, error) {
	var payload discoveryCursorPayload
	if err := decodeCursor(encoded, &payload); err != nil ||
		payload.Version != cursorVersion || payload.AsOf.IsZero() || payload.UserID == uuid.Nil ||
		math.IsNaN(payload.Score) || math.IsInf(payload.Score, 0) {
		return nil, ErrInvalidCursor
	}
	return &domainmatching.DiscoveryCursor{
		AsOf: payload.AsOf.UTC(), Score: payload.Score, UserID: payload.UserID,
	}, nil
}

func decodeMatchCursor(encoded string) (*domainmatching.MatchCursor, error) {
	var payload matchCursorPayload
	if err := decodeCursor(encoded, &payload); err != nil ||
		payload.Version != cursorVersion || payload.MatchedAt.IsZero() || payload.MatchID == uuid.Nil {
		return nil, ErrInvalidCursor
	}
	return &domainmatching.MatchCursor{MatchedAt: payload.MatchedAt.UTC(), MatchID: payload.MatchID}, nil
}

func decodeCursor(encoded string, target any) error {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrInvalidCursor
	}
	return nil
}

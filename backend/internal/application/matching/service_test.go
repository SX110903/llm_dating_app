package matching_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	applicationmatching "github.com/sx110903/llmatch-v2/backend/internal/application/matching"
	domainmatching "github.com/sx110903/llmatch-v2/backend/internal/domain/matching"
)

type swipePair struct {
	actor  uuid.UUID
	target uuid.UUID
}

type fakeRepository struct {
	readyErr error

	candidates          []domainmatching.Candidate
	discoveryParams     domainmatching.DiscoveryParams
	discoveryCalls      int
	discoveryReadyCalls int

	swipes        map[swipePair]*domainmatching.SwipeOutcome
	findErr       error
	findCalls     int
	dailyCount    int
	dailyCountErr error
	countCalls    int
	recordedSwipe domainmatching.Swipe
	recordOutcome *domainmatching.SwipeOutcome
	recordErr     error
	recordCalls   int

	matches         []domainmatching.MatchSummary
	matchListParams domainmatching.MatchListParams
	unmatchErr      error
	blockedPair     swipePair
	report          *domainmatching.Report

	visiblePhoto *domainmatching.VisiblePhoto
	photoErr     error
	touchCalls   int
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{swipes: make(map[swipePair]*domainmatching.SwipeOutcome)}
}

func (r *fakeRepository) EnsureDiscoveryReady(context.Context, uuid.UUID) error {
	r.discoveryReadyCalls++
	return r.readyErr
}

func (r *fakeRepository) ListCandidates(
	_ context.Context,
	params domainmatching.DiscoveryParams,
) ([]domainmatching.Candidate, error) {
	r.discoveryCalls++
	r.discoveryParams = params
	return r.candidates, nil
}

func (r *fakeRepository) FindSwipe(
	_ context.Context,
	actorID, targetID uuid.UUID,
) (*domainmatching.SwipeOutcome, error) {
	r.findCalls++
	if r.findErr != nil {
		return nil, r.findErr
	}
	if found, ok := r.swipes[swipePair{actor: actorID, target: targetID}]; ok {
		return found, nil
	}
	return nil, domainmatching.ErrSwipeNotFound
}

func (r *fakeRepository) RecordSwipe(
	_ context.Context,
	swipe domainmatching.Swipe,
) (*domainmatching.SwipeOutcome, error) {
	r.recordCalls++
	r.recordedSwipe = swipe
	if r.recordErr != nil {
		return nil, r.recordErr
	}
	if r.recordOutcome != nil {
		return r.recordOutcome, nil
	}
	return &domainmatching.SwipeOutcome{Swipe: swipe, Created: true}, nil
}

func (r *fakeRepository) CountSwipesSince(context.Context, uuid.UUID, time.Time) (int, error) {
	r.countCalls++
	return r.dailyCount, r.dailyCountErr
}

func (r *fakeRepository) ListMatches(
	_ context.Context,
	params domainmatching.MatchListParams,
) ([]domainmatching.MatchSummary, error) {
	r.matchListParams = params
	return r.matches, nil
}

func (r *fakeRepository) Unmatch(context.Context, uuid.UUID, uuid.UUID, time.Time) error {
	return r.unmatchErr
}

func (r *fakeRepository) Block(_ context.Context, blockerID, blockedID uuid.UUID, _ time.Time) error {
	r.blockedPair = swipePair{actor: blockerID, target: blockedID}
	return nil
}

func (r *fakeRepository) CreateReport(_ context.Context, report *domainmatching.Report) error {
	copied := *report
	r.report = &copied
	return nil
}

func (r *fakeRepository) GetVisiblePhoto(
	context.Context,
	uuid.UUID,
	uuid.UUID,
) (*domainmatching.VisiblePhoto, error) {
	return r.visiblePhoto, r.photoErr
}

func (r *fakeRepository) TouchActivity(context.Context, uuid.UUID, time.Time) error {
	r.touchCalls++
	return nil
}

type fakeLimiter struct {
	allowed      bool
	retryAfter   time.Duration
	reserveErr   error
	releaseErr   error
	reserveCalls int
	releaseCalls int
	persisted    int
}

func (l *fakeLimiter) Reserve(
	_ context.Context,
	_ uuid.UUID,
	_ time.Time,
	_ int,
	persistedCount int,
) (bool, time.Duration, error) {
	l.reserveCalls++
	l.persisted = persistedCount
	return l.allowed, l.retryAfter, l.reserveErr
}

func (l *fakeLimiter) Release(context.Context, uuid.UUID, time.Time, int) error {
	l.releaseCalls++
	return l.releaseErr
}

type fakePhotoReader struct {
	data     []byte
	getErr   error
	lastKey  string
	getCalls int
}

func (r *fakePhotoReader) Get(_ context.Context, key string) (io.ReadCloser, error) {
	r.getCalls++
	r.lastKey = key
	if r.getErr != nil {
		return nil, r.getErr
	}
	return io.NopCloser(bytes.NewReader(r.data)), nil
}

func newService(
	repository *fakeRepository,
	limiter *fakeLimiter,
	photos *fakePhotoReader,
) *applicationmatching.Service {
	return applicationmatching.NewService(repository, limiter, photos, applicationmatching.Config{
		DailySwipeLimit: 100,
		RankingWeights: domainmatching.RankingWeights{
			Interests: 0.35, Questionnaire: 0.30, Distance: 0.20, Activity: 0.15,
		},
	})
}

func TestDiscoverBuildsOpaqueStableCursor(t *testing.T) {
	repository := newFakeRepository()
	repository.candidates = []domainmatching.Candidate{
		{UserID: uuid.New(), Score: domainmatching.ScoreBreakdown{Total: 0.9}},
		{UserID: uuid.New(), Score: domainmatching.ScoreBreakdown{Total: 0.8}},
		{UserID: uuid.New(), Score: domainmatching.ScoreBreakdown{Total: 0.7}},
	}
	service := newService(repository, &fakeLimiter{}, &fakePhotoReader{})

	first, err := service.Discover(context.Background(), uuid.New(), "", 2)
	require.NoError(t, err)
	require.Len(t, first.Candidates, 2)
	require.NotEmpty(t, first.NextCursor)
	firstAsOf := repository.discoveryParams.AsOf
	require.Equal(t, 3, repository.discoveryParams.Limit)
	require.InDelta(t, 1, repository.discoveryParams.Weights.Total(), 0.0001)

	repository.candidates = nil
	_, err = service.Discover(context.Background(), uuid.New(), first.NextCursor, 2)
	require.NoError(t, err)
	require.NotNil(t, repository.discoveryParams.Cursor)
	require.Equal(t, firstAsOf, repository.discoveryParams.AsOf)
	require.Equal(t, first.Candidates[1].UserID, repository.discoveryParams.Cursor.UserID)
	require.InDelta(t, first.Candidates[1].Score.Total, repository.discoveryParams.Cursor.Score, 0.0001)
}

func TestDiscoverRejectsMalformedCursorBeforeQuerying(t *testing.T) {
	repository := newFakeRepository()
	service := newService(repository, &fakeLimiter{}, &fakePhotoReader{})

	_, err := service.Discover(context.Background(), uuid.New(), "not-a-cursor", 20)
	require.ErrorIs(t, err, applicationmatching.ErrInvalidCursor)
	require.Zero(t, repository.discoveryReadyCalls)
	require.Zero(t, repository.discoveryCalls)
}

func TestSwipeIsIdempotentWithoutConsumingAnotherReservation(t *testing.T) {
	repository := newFakeRepository()
	actorID, targetID := uuid.New(), uuid.New()
	existing := &domainmatching.SwipeOutcome{
		Swipe: domainmatching.Swipe{ActorID: actorID, TargetID: targetID, Action: domainmatching.SwipeLike},
	}
	repository.swipes[swipePair{actor: actorID, target: targetID}] = existing
	limiter := &fakeLimiter{}
	service := newService(repository, limiter, &fakePhotoReader{})

	result, err := service.Swipe(context.Background(), actorID, applicationmatching.SwipeInput{
		TargetID: targetID, Action: domainmatching.SwipeLike,
	})
	require.NoError(t, err)
	require.Same(t, existing, result)
	require.Zero(t, repository.countCalls)
	require.Zero(t, repository.recordCalls)
	require.Zero(t, limiter.reserveCalls)
}

func TestSwipeStopsAtDatabaseConfirmedDailyLimit(t *testing.T) {
	repository := newFakeRepository()
	repository.dailyCount = 100
	limiter := &fakeLimiter{allowed: true}
	service := newService(repository, limiter, &fakePhotoReader{})

	_, err := service.Swipe(context.Background(), uuid.New(), applicationmatching.SwipeInput{
		TargetID: uuid.New(), Action: domainmatching.SwipeDislike,
	})
	require.ErrorIs(t, err, domainmatching.ErrDailySwipeLimit)
	var dailyErr *applicationmatching.DailyLimitError
	require.ErrorAs(t, err, &dailyErr)
	require.Positive(t, dailyErr.RetryAfter)
	require.Zero(t, limiter.reserveCalls)
	require.Zero(t, repository.recordCalls)
}

func TestSwipeFailsClosedWhenRedisIsUnavailable(t *testing.T) {
	repository := newFakeRepository()
	limiter := &fakeLimiter{reserveErr: errors.New("redis unavailable")}
	service := newService(repository, limiter, &fakePhotoReader{})

	_, err := service.Swipe(context.Background(), uuid.New(), applicationmatching.SwipeInput{
		TargetID: uuid.New(), Action: domainmatching.SwipeLike,
	})
	require.ErrorIs(t, err, domainmatching.ErrDependencyUnavailable)
	require.Zero(t, repository.recordCalls, "a swipe must not persist without a Redis reservation")
}

func TestSwipeReleasesReservationWhenPersistenceFails(t *testing.T) {
	repository := newFakeRepository()
	repository.dailyCount = 41
	repository.recordErr = errors.New("database write failed")
	limiter := &fakeLimiter{allowed: true}
	service := newService(repository, limiter, &fakePhotoReader{})

	_, err := service.Swipe(context.Background(), uuid.New(), applicationmatching.SwipeInput{
		TargetID: uuid.New(), Action: domainmatching.SwipeSuperlike,
	})
	require.Error(t, err)
	require.Equal(t, 1, limiter.reserveCalls)
	require.Equal(t, 41, limiter.persisted)
	require.Equal(t, 1, limiter.releaseCalls)
}

func TestSwipeReleasesDuplicateRaceReservation(t *testing.T) {
	repository := newFakeRepository()
	repository.recordOutcome = &domainmatching.SwipeOutcome{Created: false}
	limiter := &fakeLimiter{allowed: true}
	service := newService(repository, limiter, &fakePhotoReader{})

	result, err := service.Swipe(context.Background(), uuid.New(), applicationmatching.SwipeInput{
		TargetID: uuid.New(), Action: domainmatching.SwipeLike,
	})
	require.NoError(t, err)
	require.False(t, result.Created)
	require.Equal(t, 1, limiter.releaseCalls)
}

func TestSwipeValidatesSelfAndActionBeforeDependencies(t *testing.T) {
	repository := newFakeRepository()
	service := newService(repository, &fakeLimiter{}, &fakePhotoReader{})
	userID := uuid.New()

	_, err := service.Swipe(context.Background(), userID, applicationmatching.SwipeInput{
		TargetID: userID, Action: domainmatching.SwipeLike,
	})
	require.ErrorIs(t, err, domainmatching.ErrSelfInteraction)

	_, err = service.Swipe(context.Background(), userID, applicationmatching.SwipeInput{
		TargetID: uuid.New(), Action: domainmatching.SwipeAction("unknown"),
	})
	require.ErrorIs(t, err, domainmatching.ErrInvalidSwipeAction)
	require.Zero(t, repository.findCalls)
}

func TestListMatchesUsesOpaqueCursor(t *testing.T) {
	repository := newFakeRepository()
	repository.matches = []domainmatching.MatchSummary{
		{Match: domainmatching.Match{ID: uuid.New(), MatchedAt: time.Now().Add(-time.Hour)}},
		{Match: domainmatching.Match{ID: uuid.New(), MatchedAt: time.Now().Add(-2 * time.Hour)}},
	}
	service := newService(repository, &fakeLimiter{}, &fakePhotoReader{})

	first, err := service.ListMatches(context.Background(), uuid.New(), "", 1)
	require.NoError(t, err)
	require.Len(t, first.Matches, 1)
	require.NotEmpty(t, first.NextCursor)

	repository.matches = nil
	_, err = service.ListMatches(context.Background(), uuid.New(), first.NextCursor, 1)
	require.NoError(t, err)
	require.NotNil(t, repository.matchListParams.Cursor)
	require.Equal(t, first.Matches[0].Match.ID, repository.matchListParams.Cursor.MatchID)
}

func TestReportTrimsDescriptionAndEnforcesUnicodeLength(t *testing.T) {
	repository := newFakeRepository()
	service := newService(repository, &fakeLimiter{}, &fakePhotoReader{})
	reporterID := uuid.New()

	report, err := service.Report(context.Background(), reporterID, applicationmatching.ReportInput{
		ReportedID: uuid.New(), Reason: domainmatching.ReportSpam, Description: "  repeated messages  ",
	})
	require.NoError(t, err)
	require.Equal(t, "repeated messages", report.Description)
	require.Equal(t, domainmatching.ReportPending, report.Status)
	require.Equal(t, repository.report.ID, report.ID)

	_, err = service.Report(context.Background(), reporterID, applicationmatching.ReportInput{
		ReportedID: uuid.New(), Reason: domainmatching.ReportOther, Description: strings.Repeat("Ã±", 1001),
	})
	require.ErrorIs(t, err, domainmatching.ErrReportTooLong)
}

func TestGetPhotoContentAuthorizesBeforeReadingStorage(t *testing.T) {
	repository := newFakeRepository()
	photos := &fakePhotoReader{data: []byte("photo")}
	service := newService(repository, &fakeLimiter{}, photos)

	repository.photoErr = domainmatching.ErrPhotoNotFound
	_, _, err := service.GetPhotoContent(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, domainmatching.ErrPhotoNotFound)
	require.Zero(t, photos.getCalls)

	repository.photoErr = nil
	repository.visiblePhoto = &domainmatching.VisiblePhoto{StorageKey: "photos/key", MimeType: "image/png"}
	reader, mimeType, err := service.GetPhotoContent(context.Background(), uuid.New(), uuid.New())
	require.NoError(t, err)
	defer func() { _ = reader.Close() }()
	raw, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, []byte("photo"), raw)
	require.Equal(t, "image/png", mimeType)
	require.Equal(t, "photos/key", photos.lastKey)
}

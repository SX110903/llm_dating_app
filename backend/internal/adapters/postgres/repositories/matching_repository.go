package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/sx110903/llmatch-v2/backend/internal/adapters/postgres/sqlc"
	domainmatching "github.com/sx110903/llmatch-v2/backend/internal/domain/matching"
)

type MatchingRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewMatchingRepository(pool *pgxpool.Pool) *MatchingRepository {
	return &MatchingRepository{pool: pool, queries: db.New(pool)}
}

func (r *MatchingRepository) EnsureDiscoveryReady(ctx context.Context, userID uuid.UUID) error {
	ready, err := r.queries.EnsureDiscoveryReady(ctx, toPgUUID(userID))
	if err != nil {
		return fmt.Errorf("check discovery readiness: %w", err)
	}
	if !ready {
		return domainmatching.ErrDiscoveryNotReady
	}
	return nil
}

func (r *MatchingRepository) ListCandidates(
	ctx context.Context,
	params domainmatching.DiscoveryParams,
) ([]domainmatching.Candidate, error) {
	queryParams := db.ListDiscoveryCandidatesParams{
		PageLimit:           int32(params.Limit), // #nosec G115 -- application restricts page size to 50.
		ViewerID:            toPgUUID(params.ViewerID),
		AsOf:                toPgTimestamptz(params.AsOf),
		ActivityWindowHours: 30 * 24,
		InterestsWeight:     params.Weights.Interests,
		QuestionnaireWeight: params.Weights.Questionnaire,
		DistanceWeight:      params.Weights.Distance,
		ActivityWeight:      params.Weights.Activity,
	}
	if params.Cursor != nil {
		queryParams.CursorScore = pgtype.Float8{Float64: params.Cursor.Score, Valid: true}
		queryParams.CursorUserID = toPgUUID(params.Cursor.UserID)
	}

	rows, err := r.queries.ListDiscoveryCandidates(ctx, queryParams)
	if err != nil {
		return nil, fmt.Errorf("list discovery candidates: %w", err)
	}
	candidates := make([]domainmatching.Candidate, 0, len(rows))
	for _, row := range rows {
		candidates = append(candidates, domainmatching.Candidate{
			UserID:         fromPgUUID(row.UserID),
			DisplayName:    fromPgText(row.DisplayName),
			Age:            int(row.Age),
			Gender:         fromPgText(row.Gender),
			Bio:            fromPgText(row.Bio),
			Interests:      row.Interests,
			City:           fromPgText(row.City),
			DistanceKM:     row.DistanceKm,
			LastActiveAt:   fromPgTimestamptz(row.LastActiveAt),
			PrimaryPhotoID: fromPgUUID(row.PrimaryPhotoID),
			Score: domainmatching.ScoreBreakdown{
				Interests:     row.InterestsScore,
				Questionnaire: row.QuestionnaireScore,
				Distance:      row.DistanceScore,
				Activity:      row.ActivityScore,
				Total:         row.TotalScore,
			},
		})
	}
	return candidates, nil
}

func (r *MatchingRepository) FindSwipe(
	ctx context.Context,
	actorID, targetID uuid.UUID,
) (*domainmatching.SwipeOutcome, error) {
	return findSwipeOutcome(ctx, r.queries, actorID, targetID)
}

func (r *MatchingRepository) RecordSwipe(
	ctx context.Context,
	swipe domainmatching.Swipe,
) (*domainmatching.SwipeOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin swipe transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := r.queries.WithTx(tx)

	pair := db.LockInteractionPairParams{FirstUserID: toPgUUID(swipe.ActorID), SecondUserID: toPgUUID(swipe.TargetID)}
	if err := qtx.LockInteractionPair(ctx, pair); err != nil {
		return nil, fmt.Errorf("lock interaction pair: %w", err)
	}

	blocked, err := qtx.InteractionBlocked(ctx, db.InteractionBlockedParams(pair))
	if err != nil {
		return nil, fmt.Errorf("check interaction block: %w", err)
	}
	if blocked {
		return nil, domainmatching.ErrInteractionBlocked
	}

	existing, err := findSwipeOutcome(ctx, qtx, swipe.ActorID, swipe.TargetID)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit idempotent swipe transaction: %w", err)
		}
		return existing, nil
	}
	if !errors.Is(err, domainmatching.ErrSwipeNotFound) {
		return nil, err
	}

	allowed, err := qtx.CanSwipeTarget(ctx, db.CanSwipeTargetParams{
		ActorID: toPgUUID(swipe.ActorID), TargetID: toPgUUID(swipe.TargetID), AsOf: toPgTimestamptz(swipe.CreatedAt),
	})
	if err != nil {
		return nil, fmt.Errorf("authorize swipe target: %w", err)
	}
	if !allowed {
		return nil, domainmatching.ErrNotFound
	}

	inserted, err := qtx.InsertSwipe(ctx, db.InsertSwipeParams{
		ID:        toPgUUID(swipe.ID),
		ActorID:   toPgUUID(swipe.ActorID),
		TargetID:  toPgUUID(swipe.TargetID),
		Action:    string(swipe.Action),
		CreatedAt: toPgTimestamptz(swipe.CreatedAt),
	})
	if err != nil {
		return nil, fmt.Errorf("insert swipe: %w", err)
	}

	outcome := &domainmatching.SwipeOutcome{Swipe: swipeFromRow(inserted), Created: true}
	if swipe.Action.IsPositive() {
		mutual, mutualErr := qtx.HasPositiveReverseSwipe(ctx, db.HasPositiveReverseSwipeParams{
			ActorID: toPgUUID(swipe.ActorID), TargetID: toPgUUID(swipe.TargetID),
		})
		if mutualErr != nil {
			return nil, fmt.Errorf("check mutual swipe: %w", mutualErr)
		}
		if mutual {
			lowID, highID := domainmatching.OrderedPair(swipe.ActorID, swipe.TargetID)
			createdMatch, matchErr := qtx.InsertMatch(ctx, db.InsertMatchParams{
				ID: toPgUUID(uuid.New()), UserLowID: toPgUUID(lowID), UserHighID: toPgUUID(highID), MatchedAt: toPgTimestamptz(swipe.CreatedAt),
			})
			if matchErr != nil {
				return nil, fmt.Errorf("insert mutual match: %w", matchErr)
			}
			mapped := matchFromRow(createdMatch)
			outcome.Match = &mapped
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit swipe transaction: %w", err)
	}
	return outcome, nil
}

func (r *MatchingRepository) CountSwipesSince(ctx context.Context, actorID uuid.UUID, since time.Time) (int, error) {
	count, err := r.queries.CountSwipesSince(ctx, db.CountSwipesSinceParams{
		ActorID: toPgUUID(actorID), CreatedAt: toPgTimestamptz(since),
	})
	if err != nil {
		return 0, fmt.Errorf("count daily swipes: %w", err)
	}
	return int(count), nil // #nosec G115 -- a daily row count cannot approach the platform integer limit.
}

func (r *MatchingRepository) ListMatches(
	ctx context.Context,
	params domainmatching.MatchListParams,
) ([]domainmatching.MatchSummary, error) {
	queryParams := db.ListActiveMatchesParams{
		UserID: toPgUUID(params.UserID), PageLimit: int32(params.Limit), // #nosec G115 -- application restricts page size to 50.
	}
	if params.Cursor != nil {
		queryParams.CursorMatchedAt = toPgTimestamptz(params.Cursor.MatchedAt)
		queryParams.CursorMatchID = toPgUUID(params.Cursor.MatchID)
	}
	rows, err := r.queries.ListActiveMatches(ctx, queryParams)
	if err != nil {
		return nil, fmt.Errorf("list active matches: %w", err)
	}
	result := make([]domainmatching.MatchSummary, 0, len(rows))
	for _, row := range rows {
		result = append(result, domainmatching.MatchSummary{
			Match: domainmatching.Match{
				ID:          fromPgUUID(row.ID),
				UserLowID:   fromPgUUID(row.UserLowID),
				UserHighID:  fromPgUUID(row.UserHighID),
				MatchedAt:   fromPgTimestamptz(row.MatchedAt),
				UnmatchedAt: fromPgTimestamptzPtr(row.UnmatchedAt),
				UnmatchedBy: fromPgUUIDPtr(row.UnmatchedBy),
			},
			OtherUserID:     fromPgUUID(row.OtherUserID),
			DisplayName:     fromPgText(row.DisplayName),
			Bio:             fromPgText(row.Bio),
			City:            fromPgText(row.City),
			PrimaryPhotoID:  fromPgUUID(row.PrimaryPhotoID),
			OtherLastActive: fromPgTimestamptz(row.OtherLastActiveAt),
		})
	}
	return result, nil
}

func (r *MatchingRepository) Unmatch(ctx context.Context, matchID, actorID uuid.UUID, at time.Time) error {
	affected, err := r.queries.Unmatch(ctx, db.UnmatchParams{
		MatchID: toPgUUID(matchID), ActorID: toPgUUID(actorID), UnmatchedAt: toPgTimestamptz(at),
	})
	if err != nil {
		return fmt.Errorf("unmatch: %w", err)
	}
	if affected > 0 {
		return nil
	}
	exists, err := r.queries.MatchParticipantExists(ctx, db.MatchParticipantExistsParams{
		MatchID: toPgUUID(matchID), ActorID: toPgUUID(actorID),
	})
	if err != nil {
		return fmt.Errorf("authorize unmatch: %w", err)
	}
	if !exists {
		return domainmatching.ErrMatchNotFound
	}
	return nil
}

func (r *MatchingRepository) Block(ctx context.Context, blockerID, blockedID uuid.UUID, at time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin block transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := r.queries.WithTx(tx)

	if err := qtx.LockInteractionPair(ctx, db.LockInteractionPairParams{
		FirstUserID: toPgUUID(blockerID), SecondUserID: toPgUUID(blockedID),
	}); err != nil {
		return fmt.Errorf("lock block pair: %w", err)
	}
	if err := qtx.InsertBlock(ctx, db.InsertBlockParams{
		BlockerID: toPgUUID(blockerID), BlockedID: toPgUUID(blockedID), CreatedAt: toPgTimestamptz(at),
	}); err != nil {
		return fmt.Errorf("insert block: %w", err)
	}
	if err := qtx.UnmatchPairForBlock(ctx, db.UnmatchPairForBlockParams{
		BlockerID: toPgUUID(blockerID), BlockedID: toPgUUID(blockedID), UnmatchedAt: toPgTimestamptz(at),
	}); err != nil {
		return fmt.Errorf("unmatch blocked pair: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit block transaction: %w", err)
	}
	return nil
}

func (r *MatchingRepository) CreateReport(ctx context.Context, report *domainmatching.Report) error {
	row, err := r.queries.InsertReport(ctx, db.InsertReportParams{
		ID:          toPgUUID(report.ID),
		ReporterID:  toPgUUID(report.ReporterID),
		ReportedID:  toPgUUID(report.ReportedID),
		Reason:      string(report.Reason),
		Description: report.Description,
		Status:      string(report.Status),
		CreatedAt:   toPgTimestamptz(report.CreatedAt),
	})
	if err != nil {
		return fmt.Errorf("insert report: %w", err)
	}
	*report = reportFromRow(row)
	return nil
}

func (r *MatchingRepository) GetVisiblePhoto(
	ctx context.Context,
	viewerID, photoID uuid.UUID,
) (*domainmatching.VisiblePhoto, error) {
	row, err := r.queries.GetVisibleMatchingPhoto(ctx, db.GetVisibleMatchingPhotoParams{
		ViewerID: toPgUUID(viewerID), PhotoID: toPgUUID(photoID), AsOf: toPgTimestamptz(time.Now().UTC()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainmatching.ErrPhotoNotFound
		}
		return nil, fmt.Errorf("get visible matching photo: %w", err)
	}
	return &domainmatching.VisiblePhoto{
		ID: fromPgUUID(row.ID), OwnerID: fromPgUUID(row.UserID), StorageKey: row.StorageKey, MimeType: row.MimeType,
	}, nil
}

func (r *MatchingRepository) TouchActivity(ctx context.Context, userID uuid.UUID, at time.Time) error {
	if err := r.queries.TouchUserActivity(ctx, db.TouchUserActivityParams{
		UserID: toPgUUID(userID), ActiveAt: toPgTimestamptz(at),
	}); err != nil {
		return fmt.Errorf("touch user activity: %w", err)
	}
	return nil
}

func findSwipeOutcome(
	ctx context.Context,
	queries *db.Queries,
	actorID, targetID uuid.UUID,
) (*domainmatching.SwipeOutcome, error) {
	row, err := queries.GetSwipe(ctx, db.GetSwipeParams{ActorID: toPgUUID(actorID), TargetID: toPgUUID(targetID)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainmatching.ErrSwipeNotFound
		}
		return nil, fmt.Errorf("get swipe: %w", err)
	}
	outcome := &domainmatching.SwipeOutcome{Swipe: swipeFromRow(row), Created: false}
	lowID, highID := domainmatching.OrderedPair(actorID, targetID)
	matchRow, matchErr := queries.GetMatchByPair(ctx, db.GetMatchByPairParams{
		UserLowID: toPgUUID(lowID), UserHighID: toPgUUID(highID),
	})
	if matchErr == nil && !matchRow.UnmatchedAt.Valid {
		mapped := matchFromRow(matchRow)
		outcome.Match = &mapped
	} else if matchErr != nil && !errors.Is(matchErr, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get swipe match: %w", matchErr)
	}
	return outcome, nil
}

func swipeFromRow(row db.Swipe) domainmatching.Swipe {
	return domainmatching.Swipe{
		ID: fromPgUUID(row.ID), ActorID: fromPgUUID(row.ActorID), TargetID: fromPgUUID(row.TargetID),
		Action: domainmatching.SwipeAction(row.Action), CreatedAt: fromPgTimestamptz(row.CreatedAt),
	}
}

func matchFromRow(row db.Match) domainmatching.Match {
	return domainmatching.Match{
		ID: fromPgUUID(row.ID), UserLowID: fromPgUUID(row.UserLowID), UserHighID: fromPgUUID(row.UserHighID),
		MatchedAt: fromPgTimestamptz(row.MatchedAt), UnmatchedAt: fromPgTimestamptzPtr(row.UnmatchedAt),
		UnmatchedBy: fromPgUUIDPtr(row.UnmatchedBy),
	}
}

func reportFromRow(row db.Report) domainmatching.Report {
	return domainmatching.Report{
		ID: fromPgUUID(row.ID), ReporterID: fromPgUUID(row.ReporterID), ReportedID: fromPgUUID(row.ReportedID),
		Reason: domainmatching.ReportReason(row.Reason), Description: row.Description,
		Status: domainmatching.ReportStatus(row.Status), CreatedAt: fromPgTimestamptz(row.CreatedAt),
		ResolvedAt: fromPgTimestamptzPtr(row.ResolvedAt),
	}
}

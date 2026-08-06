package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/sx110903/llmatch-v2/backend/internal/adapters/postgres/sqlc"
	domainaccount "github.com/sx110903/llmatch-v2/backend/internal/domain/account"
)

type ConsentRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewConsentRepository(pool *pgxpool.Pool) *ConsentRepository {
	return &ConsentRepository{pool: pool, queries: db.New(pool)}
}

func (r *ConsentRepository) Grant(ctx context.Context, c *domainaccount.Consent) error {
	row, err := r.queries.GrantConsent(ctx, db.GrantConsentParams{
		UserID:        toPgUUID(c.UserID),
		Purpose:       c.Purpose,
		PolicyVersion: c.PolicyVersion,
		Source:        c.Source,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return domainaccount.ErrConsentActive
		}
		return fmt.Errorf("grant consent: %w", err)
	}
	*c = consentFromRow(row)
	return nil
}

func (r *ConsentRepository) FindActive(ctx context.Context, userID uuid.UUID, purpose string) (*domainaccount.Consent, error) {
	row, err := r.queries.FindActiveConsent(ctx, db.FindActiveConsentParams{UserID: toPgUUID(userID), Purpose: purpose})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainaccount.ErrConsentNotFound
		}
		return nil, fmt.Errorf("find active consent: %w", err)
	}
	found := consentFromRow(row)
	return &found, nil
}

func (r *ConsentRepository) WithdrawGenderPreferenceConsent(ctx context.Context, userID uuid.UUID, at time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin withdraw consent transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := r.queries.WithTx(tx)

	if err := qtx.WithdrawActiveConsent(ctx, db.WithdrawActiveConsentParams{
		UserID:      toPgUUID(userID),
		Purpose:     domainaccount.PurposeGenderPreferences,
		WithdrawnAt: toPgTimestamptz(at),
	}); err != nil {
		return fmt.Errorf("withdraw consent: %w", err)
	}
	if err := qtx.ClearGenders(ctx, toPgUUID(userID)); err != nil {
		return fmt.Errorf("clear genders: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit withdraw consent transaction: %w", err)
	}
	return nil
}

func consentFromRow(row db.PrivacyConsent) domainaccount.Consent {
	return domainaccount.Consent{
		ID:            fromPgUUID(row.ID),
		UserID:        fromPgUUID(row.UserID),
		Purpose:       row.Purpose,
		PolicyVersion: row.PolicyVersion,
		GrantedAt:     fromPgTimestamptz(row.GrantedAt),
		WithdrawnAt:   fromPgTimestamptzPtr(row.WithdrawnAt),
		Source:        row.Source,
		CreatedAt:     fromPgTimestamptz(row.CreatedAt),
	}
}

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
	domainsession "github.com/sx110903/llmatch-v2/backend/internal/domain/session"
)

type SessionRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool, queries: db.New(pool)}
}

func (r *SessionRepository) Create(ctx context.Context, token *domainsession.RefreshToken) error {
	return r.queries.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		ID:          toPgUUID(token.ID),
		UserID:      toPgUUID(token.UserID),
		FamilyID:    toPgUUID(token.FamilyID),
		TokenHash:   token.TokenHash,
		DeviceLabel: toPgTextPtr(token.DeviceLabel),
		UserAgent:   toPgTextPtr(token.UserAgent),
		Ip:          token.IP,
		ExpiresAt:   toPgTimestamptz(token.ExpiresAt),
	})
}

func (r *SessionRepository) FindByHash(ctx context.Context, tokenHash []byte) (*domainsession.RefreshToken, error) {
	row, err := r.queries.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainsession.ErrNotFound
		}
		return nil, fmt.Errorf("get refresh token by hash: %w", err)
	}
	found := refreshTokenFromRow(row)
	return &found, nil
}

// Rotate consumes currentID inside a single transaction with a row lock so
// concurrent refresh attempts on the same token cannot both succeed. Reusing
// a token that was already revoked or already replaced revokes its entire
// family and reports ErrReused instead of applying the rotation.
func (r *SessionRepository) Rotate(ctx context.Context, currentID uuid.UUID, next *domainsession.RefreshToken) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin refresh token rotation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.queries.WithTx(tx)
	current, err := qtx.GetRefreshTokenForUpdate(ctx, toPgUUID(currentID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domainsession.ErrNotFound
		}
		return fmt.Errorf("lock refresh token: %w", err)
	}

	now := time.Now().UTC()
	if current.RevokedAt.Valid || current.ReplacedBy.Valid {
		if err := qtx.RevokeRefreshTokenFamily(ctx, db.RevokeRefreshTokenFamilyParams{
			FamilyID:     current.FamilyID,
			RevokedAt:    toPgTimestamptz(now),
			RevokeReason: toPgText(domainsession.RevokeReasonReuseDetected),
		}); err != nil {
			return fmt.Errorf("revoke reused refresh token family: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit reused family revocation: %w", err)
		}
		return domainsession.ErrReused
	}

	if err := qtx.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		ID:          toPgUUID(next.ID),
		UserID:      toPgUUID(next.UserID),
		FamilyID:    toPgUUID(next.FamilyID),
		TokenHash:   next.TokenHash,
		DeviceLabel: toPgTextPtr(next.DeviceLabel),
		UserAgent:   toPgTextPtr(next.UserAgent),
		Ip:          next.IP,
		ExpiresAt:   toPgTimestamptz(next.ExpiresAt),
	}); err != nil {
		return fmt.Errorf("insert rotated refresh token: %w", err)
	}

	if err := qtx.ReplaceRefreshToken(ctx, db.ReplaceRefreshTokenParams{
		ID:           toPgUUID(currentID),
		ReplacedBy:   toPgUUID(next.ID),
		RevokedAt:    toPgTimestamptz(now),
		RevokeReason: toPgText(domainsession.RevokeReasonRotated),
	}); err != nil {
		return fmt.Errorf("mark refresh token as rotated: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit refresh token rotation: %w", err)
	}
	return nil
}

func (r *SessionRepository) RevokeFamily(ctx context.Context, familyID uuid.UUID, reason string, at time.Time) error {
	return r.queries.RevokeRefreshTokenFamily(ctx, db.RevokeRefreshTokenFamilyParams{
		FamilyID:     toPgUUID(familyID),
		RevokedAt:    toPgTimestamptz(at),
		RevokeReason: toPgText(reason),
	})
}

func (r *SessionRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID, reason string, at time.Time) error {
	return r.queries.RevokeAllRefreshTokensForUser(ctx, db.RevokeAllRefreshTokensForUserParams{
		UserID:       toPgUUID(userID),
		RevokedAt:    toPgTimestamptz(at),
		RevokeReason: toPgText(reason),
	})
}

func refreshTokenFromRow(row db.RefreshToken) domainsession.RefreshToken {
	return domainsession.RefreshToken{
		ID:           fromPgUUID(row.ID),
		UserID:       fromPgUUID(row.UserID),
		FamilyID:     fromPgUUID(row.FamilyID),
		TokenHash:    row.TokenHash,
		ReplacedBy:   fromPgUUIDPtr(row.ReplacedBy),
		DeviceLabel:  fromPgTextPtr(row.DeviceLabel),
		UserAgent:    fromPgTextPtr(row.UserAgent),
		IP:           row.Ip,
		ExpiresAt:    fromPgTimestamptz(row.ExpiresAt),
		LastUsedAt:   fromPgTimestamptzPtr(row.LastUsedAt),
		RevokedAt:    fromPgTimestamptzPtr(row.RevokedAt),
		RevokeReason: fromPgTextPtr(row.RevokeReason),
		CreatedAt:    fromPgTimestamptz(row.CreatedAt),
	}
}

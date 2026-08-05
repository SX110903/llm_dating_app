package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/sx110903/llmatch-v2/backend/internal/adapters/postgres/sqlc"
	domainuser "github.com/sx110903/llmatch-v2/backend/internal/domain/user"
)

const uniqueViolationCode = "23505"

type UserRepository struct {
	queries *db.Queries
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{queries: db.New(pool)}
}

func (r *UserRepository) Create(ctx context.Context, u *domainuser.User) error {
	row, err := r.queries.CreateUser(ctx, db.CreateUserParams{
		ID:           toPgUUID(u.ID),
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		DisplayName:  toPgText(u.DisplayName),
		BirthDate:    toPgDate(u.BirthDate),
		Gender:       toPgText(u.Gender),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return domainuser.ErrEmailTaken
		}
		return fmt.Errorf("insert user: %w", err)
	}
	*u = userFromRow(row)
	return nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domainuser.User, error) {
	row, err := r.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainuser.ErrNotFound
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	found := userFromRow(row)
	return &found, nil
}

func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*domainuser.User, error) {
	row, err := r.queries.GetUserByID(ctx, toPgUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainuser.ErrNotFound
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	found := userFromRow(row)
	return &found, nil
}

func (r *UserRepository) UpdatePasswordHash(ctx context.Context, id uuid.UUID, passwordHash string, changedAt time.Time) error {
	return r.queries.UpdateUserPasswordHash(ctx, db.UpdateUserPasswordHashParams{
		ID:                toPgUUID(id),
		PasswordHash:      passwordHash,
		PasswordChangedAt: toPgTimestamptz(changedAt),
	})
}

func userFromRow(row db.User) domainuser.User {
	return domainuser.User{
		ID:                fromPgUUID(row.ID),
		Email:             row.Email,
		PasswordHash:      row.PasswordHash,
		DisplayName:       fromPgText(row.DisplayName),
		BirthDate:         fromPgDate(row.BirthDate),
		Gender:            fromPgText(row.Gender),
		Status:            domainuser.Status(row.Status),
		EmailVerifiedAt:   fromPgTimestamptzPtr(row.EmailVerifiedAt),
		PasswordChangedAt: fromPgTimestamptz(row.PasswordChangedAt),
		CreatedAt:         fromPgTimestamptz(row.CreatedAt),
		UpdatedAt:         fromPgTimestamptz(row.UpdatedAt),
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode
}

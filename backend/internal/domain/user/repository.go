package user

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository is an outbound application port implemented by infrastructure adapters.
type Repository interface {
	Create(ctx context.Context, u *User) error
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*User, error)
	UpdatePasswordHash(ctx context.Context, id uuid.UUID, passwordHash string, changedAt time.Time) error
}

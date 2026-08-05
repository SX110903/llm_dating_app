package profile

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository is an outbound application port implemented by infrastructure adapters.
type Repository interface {
	GetProfile(ctx context.Context, userID uuid.UUID) (*Profile, error)
	UpsertProfile(ctx context.Context, p *Profile) error

	GetPreferences(ctx context.Context, userID uuid.UUID) (*Preferences, error)
	// UpsertPreferences persists min/max age and max distance only; it never
	// touches genders, which is gated behind explicit consent and only ever
	// changed via UpdateGenders.
	UpsertPreferences(ctx context.Context, p *Preferences) error
	UpdateGenders(ctx context.Context, userID uuid.UUID, genders []string) error

	CountActivePhotos(ctx context.Context, userID uuid.UUID) (int, error)
	ListPhotos(ctx context.Context, userID uuid.UUID) ([]Photo, error)
	GetPhoto(ctx context.Context, id uuid.UUID) (*Photo, error)
	CreatePhoto(ctx context.Context, photo *Photo) error
	ReorderPhotos(ctx context.Context, userID uuid.UUID, orderedPhotoIDs []uuid.UUID) error
	SetPrimaryPhoto(ctx context.Context, userID, photoID uuid.UUID) error
	SoftDeletePhoto(ctx context.Context, id uuid.UUID, at time.Time) error
}

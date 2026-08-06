package profile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	domainprofile "github.com/sx110903/llmatch-v2/backend/internal/domain/profile"
)

var (
	ErrOnboardingIncomplete = errors.New("profile is missing required fields to complete onboarding")
	ErrInvalidPhotoOrder    = errors.New("photo order must contain exactly the user's current photos")
)

type UpdateProfileInput struct {
	Bio                 string
	Interests           []string
	City                string
	Latitude            *float64
	Longitude           *float64
	Questionnaire       map[string]any
	OnboardingCompleted bool
}

type UpdatePreferencesInput struct {
	MinAge        int
	MaxAge        int
	MaxDistanceKM int
	// Genders is nil when the caller does not want to change it. An empty,
	// non-nil slice clears it (equivalent to withdrawing the preference
	// without withdrawing consent).
	Genders []string
}

type NewPhotoInput struct {
	MimeType string
	Width    int
	Height   int
	ByteSize int64
	Data     []byte
}

type Service struct {
	profiles domainprofile.Repository
	storage  domainprofile.Storage
	consents ConsentChecker
	now      func() time.Time
}

func NewService(profiles domainprofile.Repository, storage domainprofile.Storage, consents ConsentChecker) *Service {
	return &Service{
		profiles: profiles,
		storage:  storage,
		consents: consents,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) GetProfile(ctx context.Context, userID uuid.UUID) (*domainprofile.Profile, error) {
	return s.profiles.GetProfile(ctx, userID)
}

func (s *Service) UpdateProfile(ctx context.Context, userID uuid.UUID, in UpdateProfileInput) (*domainprofile.Profile, error) {
	if in.OnboardingCompleted {
		photoCount, err := s.profiles.CountActivePhotos(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("count active photos: %w", err)
		}
		if strings.TrimSpace(in.Bio) == "" || photoCount == 0 {
			return nil, ErrOnboardingIncomplete
		}
	}

	p := &domainprofile.Profile{
		UserID:              userID,
		Bio:                 in.Bio,
		Interests:           in.Interests,
		City:                in.City,
		Questionnaire:       in.Questionnaire,
		OnboardingCompleted: in.OnboardingCompleted,
	}
	if in.Latitude != nil && in.Longitude != nil {
		p.Location = &domainprofile.Coordinates{Latitude: *in.Latitude, Longitude: *in.Longitude}
	}
	if err := s.profiles.UpsertProfile(ctx, p); err != nil {
		return nil, fmt.Errorf("upsert profile: %w", err)
	}
	return s.profiles.GetProfile(ctx, userID)
}

func (s *Service) GetPreferences(ctx context.Context, userID uuid.UUID) (*domainprofile.Preferences, error) {
	return s.profiles.GetPreferences(ctx, userID)
}

func (s *Service) UpdatePreferences(ctx context.Context, userID uuid.UUID, in UpdatePreferencesInput) (*domainprofile.Preferences, error) {
	if in.MinAge < 18 || in.MinAge > in.MaxAge || in.MaxAge > 100 {
		return nil, domainprofile.ErrInvalidAgeRange
	}
	if in.MaxDistanceKM < 1 || in.MaxDistanceKM > 500 {
		return nil, domainprofile.ErrInvalidDistance
	}

	if err := s.profiles.UpsertPreferences(ctx, &domainprofile.Preferences{
		UserID:        userID,
		MinAge:        in.MinAge,
		MaxAge:        in.MaxAge,
		MaxDistanceKM: in.MaxDistanceKM,
	}); err != nil {
		return nil, fmt.Errorf("upsert preferences: %w", err)
	}

	if in.Genders != nil {
		hasConsent, err := s.consents.HasActiveGenderPreferenceConsent(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("check gender preference consent: %w", err)
		}
		if !hasConsent {
			return nil, domainprofile.ErrConsentRequired
		}
		if err := s.profiles.UpdateGenders(ctx, userID, in.Genders); err != nil {
			return nil, fmt.Errorf("update genders: %w", err)
		}
	}

	return s.profiles.GetPreferences(ctx, userID)
}

func (s *Service) ListPhotos(ctx context.Context, userID uuid.UUID) ([]domainprofile.Photo, error) {
	return s.profiles.ListPhotos(ctx, userID)
}

// GetPhotoContent returns the raw bytes of a photo the caller owns, along
// with its MIME type, so the HTTP adapter can stream it back.
func (s *Service) GetPhotoContent(ctx context.Context, userID, photoID uuid.UUID) (io.ReadCloser, string, error) {
	found, err := s.profiles.GetPhoto(ctx, photoID)
	if err != nil {
		return nil, "", err
	}
	if found.UserID != userID {
		return nil, "", domainprofile.ErrPhotoNotFound
	}
	reader, err := s.storage.Get(ctx, found.StorageKey)
	if err != nil {
		return nil, "", fmt.Errorf("read photo content: %w", err)
	}
	return reader, found.MimeType, nil
}

// CreatePhoto assumes the caller (the HTTP adapter) has already sniffed the
// real content, decoded it as an image and validated its size and format;
// this service only enforces the photo-count business rule and orchestrates
// storage plus persistence.
func (s *Service) CreatePhoto(ctx context.Context, userID uuid.UUID, in NewPhotoInput) (*domainprofile.Photo, error) {
	count, err := s.profiles.CountActivePhotos(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("count active photos: %w", err)
	}
	if count >= domainprofile.MaxPhotos {
		return nil, domainprofile.ErrPhotoLimitReached
	}

	photoID := uuid.New()
	key := fmt.Sprintf("photos/%s/%s", userID, photoID)
	if err := s.storage.Put(ctx, key, in.MimeType, in.Data); err != nil {
		return nil, fmt.Errorf("store photo: %w", err)
	}

	photo := &domainprofile.Photo{
		ID:         photoID,
		UserID:     userID,
		StorageKey: key,
		MimeType:   in.MimeType,
		ByteSize:   in.ByteSize,
		Width:      in.Width,
		Height:     in.Height,
		Position:   count,
		IsPrimary:  count == 0,
	}
	if err := s.profiles.CreatePhoto(ctx, photo); err != nil {
		// Compensate for the orphaned blob: the DB row that would reference
		// it was never created, so nothing else will ever clean it up.
		if deleteErr := s.storage.Delete(ctx, key); deleteErr != nil {
			return nil, fmt.Errorf("create photo record: %w (storage compensation also failed: %w)", err, deleteErr)
		}
		return nil, fmt.Errorf("create photo record: %w", err)
	}
	return photo, nil
}

func (s *Service) ReorderPhotos(ctx context.Context, userID uuid.UUID, orderedPhotoIDs []uuid.UUID) error {
	existing, err := s.profiles.ListPhotos(ctx, userID)
	if err != nil {
		return fmt.Errorf("list photos: %w", err)
	}
	if len(orderedPhotoIDs) != len(existing) {
		return ErrInvalidPhotoOrder
	}
	existingSet := make(map[uuid.UUID]bool, len(existing))
	for _, photo := range existing {
		existingSet[photo.ID] = true
	}
	seen := make(map[uuid.UUID]bool, len(orderedPhotoIDs))
	for _, id := range orderedPhotoIDs {
		if !existingSet[id] || seen[id] {
			return ErrInvalidPhotoOrder
		}
		seen[id] = true
	}
	return s.profiles.ReorderPhotos(ctx, userID, orderedPhotoIDs)
}

func (s *Service) SetPrimaryPhoto(ctx context.Context, userID, photoID uuid.UUID) error {
	found, err := s.profiles.GetPhoto(ctx, photoID)
	if err != nil {
		return err
	}
	if found.UserID != userID {
		return domainprofile.ErrPhotoNotFound
	}
	return s.profiles.SetPrimaryPhoto(ctx, userID, photoID)
}

// DeletePhoto performs the logical delete synchronously (the guarantee that
// matters), promotes a new primary photo if needed, and best-effort deletes
// the underlying blob asynchronously so the request never waits on storage.
func (s *Service) DeletePhoto(ctx context.Context, userID, photoID uuid.UUID) error {
	found, err := s.profiles.GetPhoto(ctx, photoID)
	if err != nil {
		return err
	}
	if found.UserID != userID {
		return domainprofile.ErrPhotoNotFound
	}

	if err := s.profiles.SoftDeletePhoto(ctx, photoID, s.now()); err != nil {
		return fmt.Errorf("soft delete photo: %w", err)
	}

	if found.IsPrimary {
		if remaining, err := s.profiles.ListPhotos(ctx, userID); err == nil && len(remaining) > 0 {
			_ = s.profiles.SetPrimaryPhoto(ctx, userID, remaining[0].ID)
		}
	}

	// #nosec G118 -- intentionally detached from the request context: the
	// blob cleanup must outlive the HTTP response that triggered it.
	go func(key string) {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = s.storage.Delete(cleanupCtx, key)
	}(found.StorageKey)

	return nil
}

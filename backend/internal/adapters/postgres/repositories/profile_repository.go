package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/sx110903/llmatch-v2/backend/internal/adapters/postgres/sqlc"
	domainprofile "github.com/sx110903/llmatch-v2/backend/internal/domain/profile"
)

type ProfileRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewProfileRepository(pool *pgxpool.Pool) *ProfileRepository {
	return &ProfileRepository{pool: pool, queries: db.New(pool)}
}

func (r *ProfileRepository) GetProfile(ctx context.Context, userID uuid.UUID) (*domainprofile.Profile, error) {
	row, err := r.queries.GetProfile(ctx, toPgUUID(userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainprofile.ErrNotFound
		}
		return nil, fmt.Errorf("get profile: %w", err)
	}

	questionnaire, err := decodeQuestionnaire(row.Questionnaire)
	if err != nil {
		return nil, err
	}
	hasLocation, _ := row.HasLocation.(bool)

	return &domainprofile.Profile{
		UserID:              fromPgUUID(row.UserID),
		Bio:                 fromPgText(row.Bio),
		Interests:           row.Interests,
		City:                fromPgText(row.City),
		HasLocation:         hasLocation,
		Questionnaire:       questionnaire,
		OnboardingCompleted: row.OnboardingCompleted,
		CreatedAt:           fromPgTimestamptz(row.CreatedAt),
		UpdatedAt:           fromPgTimestamptz(row.UpdatedAt),
	}, nil
}

func (r *ProfileRepository) UpsertProfile(ctx context.Context, p *domainprofile.Profile) error {
	questionnaire, err := json.Marshal(p.Questionnaire)
	if err != nil {
		return fmt.Errorf("marshal questionnaire: %w", err)
	}
	var longitude, latitude pgtype.Float8
	if p.Location != nil {
		longitude = pgtype.Float8{Float64: p.Location.Longitude, Valid: true}
		latitude = pgtype.Float8{Float64: p.Location.Latitude, Valid: true}
	}

	return r.queries.UpsertProfile(ctx, db.UpsertProfileParams{
		UserID:              toPgUUID(p.UserID),
		Bio:                 toPgText(p.Bio),
		Interests:           p.Interests,
		City:                toPgText(p.City),
		Longitude:           longitude,
		Latitude:            latitude,
		Questionnaire:       questionnaire,
		OnboardingCompleted: p.OnboardingCompleted,
	})
}

func (r *ProfileRepository) GetPreferences(ctx context.Context, userID uuid.UUID) (*domainprofile.Preferences, error) {
	row, err := r.queries.GetPreferences(ctx, toPgUUID(userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainprofile.ErrPreferencesNotFound
		}
		return nil, fmt.Errorf("get preferences: %w", err)
	}
	return preferencesFromRow(row), nil
}

func (r *ProfileRepository) UpsertPreferences(ctx context.Context, p *domainprofile.Preferences) error {
	// The application layer already validated 18<=MinAge<=MaxAge<=100 and
	// 1<=MaxDistanceKM<=500, all well within int16.
	return r.queries.UpsertPreferences(ctx, db.UpsertPreferencesParams{
		UserID:        toPgUUID(p.UserID),
		MinAge:        int16(p.MinAge),        // #nosec G115
		MaxAge:        int16(p.MaxAge),        // #nosec G115
		MaxDistanceKm: int16(p.MaxDistanceKM), // #nosec G115
	})
}

func (r *ProfileRepository) UpdateGenders(ctx context.Context, userID uuid.UUID, genders []string) error {
	return r.queries.UpdateGenders(ctx, db.UpdateGendersParams{UserID: toPgUUID(userID), Genders: genders})
}

func (r *ProfileRepository) CountActivePhotos(ctx context.Context, userID uuid.UUID) (int, error) {
	count, err := r.queries.CountActivePhotos(ctx, toPgUUID(userID))
	if err != nil {
		return 0, fmt.Errorf("count active photos: %w", err)
	}
	return int(count), nil
}

func (r *ProfileRepository) ListPhotos(ctx context.Context, userID uuid.UUID) ([]domainprofile.Photo, error) {
	rows, err := r.queries.ListPhotos(ctx, toPgUUID(userID))
	if err != nil {
		return nil, fmt.Errorf("list photos: %w", err)
	}
	photos := make([]domainprofile.Photo, 0, len(rows))
	for _, row := range rows {
		photos = append(photos, photoFromRow(row))
	}
	return photos, nil
}

func (r *ProfileRepository) GetPhoto(ctx context.Context, id uuid.UUID) (*domainprofile.Photo, error) {
	row, err := r.queries.GetPhoto(ctx, toPgUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainprofile.ErrPhotoNotFound
		}
		return nil, fmt.Errorf("get photo: %w", err)
	}
	found := photoFromRow(row)
	return &found, nil
}

func (r *ProfileRepository) CreatePhoto(ctx context.Context, photo *domainprofile.Photo) error {
	row, err := r.queries.CreatePhoto(ctx, db.CreatePhotoParams{
		ID:         toPgUUID(photo.ID),
		UserID:     toPgUUID(photo.UserID),
		StorageKey: photo.StorageKey,
		MimeType:   photo.MimeType,
		ByteSize:   photo.ByteSize,
		// Width/Height come from image.DecodeConfig header fields (already
		// int32-range) and Position is 0..MaxPhotos-1.
		Width:     int32(photo.Width),    // #nosec G115
		Height:    int32(photo.Height),   // #nosec G115
		Position:  int16(photo.Position), // #nosec G115
		IsPrimary: photo.IsPrimary,
	})
	if err != nil {
		return fmt.Errorf("insert photo: %w", err)
	}
	*photo = photoFromRow(row)
	return nil
}

// ReorderPhotos assigns positions 0..n-1 following orderedPhotoIDs. Every
// photo is first moved to a negative placeholder position so the
// intermediate state never collides with photos_user_position_uq
// (user_id, position) WHERE deleted_at IS NULL.
func (r *ProfileRepository) ReorderPhotos(ctx context.Context, userID uuid.UUID, orderedPhotoIDs []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin reorder transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := r.queries.WithTx(tx)

	for i, photoID := range orderedPhotoIDs {
		if err := qtx.SetPhotoPosition(ctx, db.SetPhotoPositionParams{
			ID: toPgUUID(photoID), UserID: toPgUUID(userID), Position: int16(-(i + 1)),
		}); err != nil {
			return fmt.Errorf("stage photo position: %w", err)
		}
	}
	for i, photoID := range orderedPhotoIDs {
		if err := qtx.SetPhotoPosition(ctx, db.SetPhotoPositionParams{
			ID: toPgUUID(photoID), UserID: toPgUUID(userID), Position: int16(i),
		}); err != nil {
			return fmt.Errorf("apply photo position: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit reorder transaction: %w", err)
	}
	return nil
}

func (r *ProfileRepository) SetPrimaryPhoto(ctx context.Context, userID, photoID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin set primary photo transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := r.queries.WithTx(tx)

	if err := qtx.ClearPrimaryPhoto(ctx, toPgUUID(userID)); err != nil {
		return fmt.Errorf("clear primary photo: %w", err)
	}
	if err := qtx.SetPrimaryPhoto(ctx, db.SetPrimaryPhotoParams{ID: toPgUUID(photoID), UserID: toPgUUID(userID)}); err != nil {
		return fmt.Errorf("set primary photo: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit set primary photo transaction: %w", err)
	}
	return nil
}

func (r *ProfileRepository) SoftDeletePhoto(ctx context.Context, id uuid.UUID, at time.Time) error {
	return r.queries.SoftDeletePhoto(ctx, db.SoftDeletePhotoParams{ID: toPgUUID(id), DeletedAt: toPgTimestamptz(at)})
}

func decodeQuestionnaire(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode questionnaire: %w", err)
	}
	return value, nil
}

func preferencesFromRow(row db.UserPreference) *domainprofile.Preferences {
	return &domainprofile.Preferences{
		UserID:        fromPgUUID(row.UserID),
		MinAge:        int(row.MinAge),
		MaxAge:        int(row.MaxAge),
		MaxDistanceKM: int(row.MaxDistanceKm),
		Genders:       row.Genders,
		CreatedAt:     fromPgTimestamptz(row.CreatedAt),
		UpdatedAt:     fromPgTimestamptz(row.UpdatedAt),
	}
}

func photoFromRow(row db.Photo) domainprofile.Photo {
	return domainprofile.Photo{
		ID:         fromPgUUID(row.ID),
		UserID:     fromPgUUID(row.UserID),
		StorageKey: row.StorageKey,
		MimeType:   row.MimeType,
		ByteSize:   row.ByteSize,
		Width:      int(row.Width),
		Height:     int(row.Height),
		Position:   int(row.Position),
		IsPrimary:  row.IsPrimary,
		CreatedAt:  fromPgTimestamptz(row.CreatedAt),
		DeletedAt:  fromPgTimestamptzPtr(row.DeletedAt),
	}
}

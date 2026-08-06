package profile_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	applicationprofile "github.com/sx110903/llmatch-v2/backend/internal/application/profile"
	domainprofile "github.com/sx110903/llmatch-v2/backend/internal/domain/profile"
)

// --- fakes -----------------------------------------------------------------

type fakeProfileRepo struct {
	profiles            map[uuid.UUID]*domainprofile.Profile
	preferences         map[uuid.UUID]*domainprofile.Preferences
	photos              map[uuid.UUID]*domainprofile.Photo
	forceCreatePhotoErr error
}

func newFakeProfileRepo() *fakeProfileRepo {
	return &fakeProfileRepo{
		profiles:    map[uuid.UUID]*domainprofile.Profile{},
		preferences: map[uuid.UUID]*domainprofile.Preferences{},
		photos:      map[uuid.UUID]*domainprofile.Photo{},
	}
}

func (r *fakeProfileRepo) GetProfile(_ context.Context, userID uuid.UUID) (*domainprofile.Profile, error) {
	p, ok := r.profiles[userID]
	if !ok {
		return nil, domainprofile.ErrNotFound
	}
	copied := *p
	return &copied, nil
}

// UpsertProfile mirrors the location semantics of the real SQL: an explicit
// clear drops the coordinates, new coordinates replace them, and a write that
// carries neither leaves whatever was stored untouched.
func (r *fakeProfileRepo) UpsertProfile(_ context.Context, p *domainprofile.Profile) error {
	copied := *p
	switch {
	case p.ClearLocation:
		copied.HasLocation = false
	case p.Location != nil:
		copied.HasLocation = true
	default:
		if existing, ok := r.profiles[p.UserID]; ok {
			copied.HasLocation = existing.HasLocation
		}
	}
	r.profiles[p.UserID] = &copied
	return nil
}

func (r *fakeProfileRepo) GetPreferences(_ context.Context, userID uuid.UUID) (*domainprofile.Preferences, error) {
	p, ok := r.preferences[userID]
	if !ok {
		return nil, domainprofile.ErrPreferencesNotFound
	}
	copied := *p
	return &copied, nil
}

func (r *fakeProfileRepo) UpsertPreferences(_ context.Context, p *domainprofile.Preferences) error {
	genders := []string{}
	if existing, ok := r.preferences[p.UserID]; ok {
		genders = existing.Genders
	}
	copied := *p
	copied.Genders = genders
	r.preferences[p.UserID] = &copied
	return nil
}

func (r *fakeProfileRepo) UpdateGenders(_ context.Context, userID uuid.UUID, genders []string) error {
	existing, ok := r.preferences[userID]
	if !ok {
		return domainprofile.ErrPreferencesNotFound
	}
	existing.Genders = genders
	return nil
}

func (r *fakeProfileRepo) CountActivePhotos(_ context.Context, userID uuid.UUID) (int, error) {
	count := 0
	for _, p := range r.photos {
		if p.UserID == userID && !p.IsDeleted() {
			count++
		}
	}
	return count, nil
}

func (r *fakeProfileRepo) ListPhotos(_ context.Context, userID uuid.UUID) ([]domainprofile.Photo, error) {
	result := []domainprofile.Photo{}
	for _, p := range r.photos {
		if p.UserID == userID && !p.IsDeleted() {
			result = append(result, *p)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Position < result[j].Position })
	return result, nil
}

func (r *fakeProfileRepo) GetPhoto(_ context.Context, id uuid.UUID) (*domainprofile.Photo, error) {
	p, ok := r.photos[id]
	if !ok || p.IsDeleted() {
		return nil, domainprofile.ErrPhotoNotFound
	}
	copied := *p
	return &copied, nil
}

func (r *fakeProfileRepo) CreatePhoto(_ context.Context, photo *domainprofile.Photo) error {
	if r.forceCreatePhotoErr != nil {
		return r.forceCreatePhotoErr
	}
	copied := *photo
	r.photos[photo.ID] = &copied
	return nil
}

func (r *fakeProfileRepo) ReorderPhotos(_ context.Context, _ uuid.UUID, orderedPhotoIDs []uuid.UUID) error {
	for i, id := range orderedPhotoIDs {
		p, ok := r.photos[id]
		if !ok {
			return domainprofile.ErrPhotoNotFound
		}
		p.Position = i
	}
	return nil
}

func (r *fakeProfileRepo) SetPrimaryPhoto(_ context.Context, userID, photoID uuid.UUID) error {
	target, ok := r.photos[photoID]
	if !ok {
		return domainprofile.ErrPhotoNotFound
	}
	for _, p := range r.photos {
		if p.UserID == userID {
			p.IsPrimary = false
		}
	}
	target.IsPrimary = true
	return nil
}

func (r *fakeProfileRepo) SoftDeletePhoto(_ context.Context, id uuid.UUID, at time.Time) error {
	p, ok := r.photos[id]
	if !ok {
		return domainprofile.ErrPhotoNotFound
	}
	deletedAt := at
	p.DeletedAt = &deletedAt
	return nil
}

// fakeStorage is used concurrently: DeletePhoto's cleanup runs in its own
// goroutine while tests poll it via require.Eventually, so every access
// needs to go through mu.
type fakeStorage struct {
	mu          sync.Mutex
	objects     map[string][]byte
	putErr      error
	deleteErr   error
	deletedKeys []string
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{objects: map[string][]byte{}}
}

func (s *fakeStorage) Put(_ context.Context, key, _ string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.putErr != nil {
		return s.putErr
	}
	s.objects[key] = data
	return nil
}

func (s *fakeStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.objects[key]
	if !ok {
		return nil, errors.New("object not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *fakeStorage) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.objects, key)
	s.deletedKeys = append(s.deletedKeys, key)
	return nil
}

func (s *fakeStorage) hasObject(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.objects[key]
	return ok
}

func (s *fakeStorage) objectCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.objects)
}

func (s *fakeStorage) deletedKeyCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.deletedKeys)
}

type fakeConsentChecker struct {
	active bool
	err    error
}

func (f fakeConsentChecker) HasActiveGenderPreferenceConsent(context.Context, uuid.UUID) (bool, error) {
	return f.active, f.err
}

// --- tests -------------------------------------------------------------

func newService(repo *fakeProfileRepo, store *fakeStorage, consents applicationprofile.ConsentChecker) *applicationprofile.Service {
	return applicationprofile.NewService(repo, store, consents)
}

func TestUpdatePreferencesRejectsGendersWithoutConsent(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := newService(repo, newFakeStorage(), fakeConsentChecker{active: false})
	userID := uuid.New()

	_, err := svc.UpdatePreferences(context.Background(), userID, applicationprofile.UpdatePreferencesInput{
		MinAge: 20, MaxAge: 40, MaxDistanceKM: 50, Genders: []string{"woman"},
	})
	require.ErrorIs(t, err, domainprofile.ErrConsentRequired)
}

func TestUpdatePreferencesSavesGendersWithActiveConsent(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := newService(repo, newFakeStorage(), fakeConsentChecker{active: true})
	userID := uuid.New()

	updated, err := svc.UpdatePreferences(context.Background(), userID, applicationprofile.UpdatePreferencesInput{
		MinAge: 20, MaxAge: 40, MaxDistanceKM: 50, Genders: []string{"woman", "man"},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"woman", "man"}, updated.Genders)
}

func TestUpdatePreferencesSavesNonSensitiveFieldsWithoutTouchingConsent(t *testing.T) {
	repo := newFakeProfileRepo()
	consents := fakeConsentChecker{err: errors.New("should not be called")}
	svc := newService(repo, newFakeStorage(), consents)
	userID := uuid.New()

	updated, err := svc.UpdatePreferences(context.Background(), userID, applicationprofile.UpdatePreferencesInput{
		MinAge: 21, MaxAge: 35, MaxDistanceKM: 25,
	})
	require.NoError(t, err)
	require.Equal(t, 21, updated.MinAge)
	require.Empty(t, updated.Genders)
}

func TestUpdatePreferencesRejectsInvalidAgeRange(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := newService(repo, newFakeStorage(), fakeConsentChecker{})
	_, err := svc.UpdatePreferences(context.Background(), uuid.New(), applicationprofile.UpdatePreferencesInput{
		MinAge: 40, MaxAge: 30, MaxDistanceKM: 10,
	})
	require.ErrorIs(t, err, domainprofile.ErrInvalidAgeRange)
}

func TestUpdatePreferencesRejectsInvalidDistance(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := newService(repo, newFakeStorage(), fakeConsentChecker{})
	_, err := svc.UpdatePreferences(context.Background(), uuid.New(), applicationprofile.UpdatePreferencesInput{
		MinAge: 20, MaxAge: 30, MaxDistanceKM: 999,
	})
	require.ErrorIs(t, err, domainprofile.ErrInvalidDistance)
}

func TestUpdateProfileRequiresBioAndPhotoToCompleteOnboarding(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := newService(repo, newFakeStorage(), fakeConsentChecker{})
	userID := uuid.New()

	_, err := svc.UpdateProfile(context.Background(), userID, applicationprofile.UpdateProfileInput{
		Bio: "", OnboardingCompleted: true,
	})
	require.ErrorIs(t, err, applicationprofile.ErrOnboardingIncomplete)
}

// --- location intent ------------------------------------------------------

func coord(value float64) *float64 { return &value }

// TestUpdateProfileWithoutCoordinatesPreservesStoredLocation is the
// regression test for the bug that made every profile undiscoverable: saving
// the profile from the UI carried no coordinates and wiped the stored ones.
func TestUpdateProfileWithoutCoordinatesPreservesStoredLocation(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := newService(repo, newFakeStorage(), fakeConsentChecker{})
	userID := uuid.New()

	stored, err := svc.UpdateProfile(context.Background(), userID, applicationprofile.UpdateProfileInput{
		Bio: "con ubicación", Latitude: coord(40.4168), Longitude: coord(-3.7038),
	})
	require.NoError(t, err)
	require.True(t, stored.HasLocation)

	// A later save that only edits the bio must not erase the location.
	updated, err := svc.UpdateProfile(context.Background(), userID, applicationprofile.UpdateProfileInput{
		Bio: "bio editada sin tocar la ubicación",
	})
	require.NoError(t, err)
	require.True(t, updated.HasLocation, "omitting coordinates must preserve the stored location")
}

func TestUpdateProfileClearLocationRemovesIt(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := newService(repo, newFakeStorage(), fakeConsentChecker{})
	userID := uuid.New()

	_, err := svc.UpdateProfile(context.Background(), userID, applicationprofile.UpdateProfileInput{
		Bio: "con ubicación", Latitude: coord(40.4168), Longitude: coord(-3.7038),
	})
	require.NoError(t, err)

	cleared, err := svc.UpdateProfile(context.Background(), userID, applicationprofile.UpdateProfileInput{
		Bio: "sin ubicación", ClearLocation: true,
	})
	require.NoError(t, err)
	require.False(t, cleared.HasLocation)
}

func TestUpdateProfileRejectsHalfCoordinates(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := newService(repo, newFakeStorage(), fakeConsentChecker{})

	_, err := svc.UpdateProfile(context.Background(), uuid.New(), applicationprofile.UpdateProfileInput{
		Bio: "solo latitud", Latitude: coord(40.4168),
	})
	require.ErrorIs(t, err, domainprofile.ErrIncompleteCoordinates)

	_, err = svc.UpdateProfile(context.Background(), uuid.New(), applicationprofile.UpdateProfileInput{
		Bio: "solo longitud", Longitude: coord(-3.7038),
	})
	require.ErrorIs(t, err, domainprofile.ErrIncompleteCoordinates)
}

func TestUpdateProfileRejectsOutOfRangeCoordinates(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := newService(repo, newFakeStorage(), fakeConsentChecker{})

	_, err := svc.UpdateProfile(context.Background(), uuid.New(), applicationprofile.UpdateProfileInput{
		Bio: "latitud imposible", Latitude: coord(91), Longitude: coord(0),
	})
	require.ErrorIs(t, err, domainprofile.ErrIncompleteCoordinates)
}

func TestUpdateProfileRejectsCoordinatesCombinedWithClear(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := newService(repo, newFakeStorage(), fakeConsentChecker{})

	_, err := svc.UpdateProfile(context.Background(), uuid.New(), applicationprofile.UpdateProfileInput{
		Bio: "contradictorio", Latitude: coord(40.4168), Longitude: coord(-3.7038), ClearLocation: true,
	})
	require.ErrorIs(t, err, domainprofile.ErrConflictingLocation)
}

// TestUpdateProfileCompletesOnboardingWithoutLocation pins the deliberate
// decision that a location is not required to finish onboarding.
func TestUpdateProfileCompletesOnboardingWithoutLocation(t *testing.T) {
	repo := newFakeProfileRepo()
	store := newFakeStorage()
	svc := newService(repo, store, fakeConsentChecker{})
	userID := uuid.New()

	_, err := svc.CreatePhoto(context.Background(), userID, applicationprofile.NewPhotoInput{
		MimeType: "image/png", Width: 10, Height: 10, ByteSize: 1, Data: []byte("x"),
	})
	require.NoError(t, err)

	updated, err := svc.UpdateProfile(context.Background(), userID, applicationprofile.UpdateProfileInput{
		Bio: "sin ubicación pero completo", OnboardingCompleted: true,
	})
	require.NoError(t, err)
	require.True(t, updated.OnboardingCompleted)
	require.False(t, updated.HasLocation)
}

func TestCreatePhotoAssignsFirstPhotoAsPrimary(t *testing.T) {
	repo := newFakeProfileRepo()
	store := newFakeStorage()
	svc := newService(repo, store, fakeConsentChecker{})
	userID := uuid.New()

	photo, err := svc.CreatePhoto(context.Background(), userID, applicationprofile.NewPhotoInput{
		MimeType: "image/png", Width: 100, Height: 100, ByteSize: 4, Data: []byte("data"),
	})
	require.NoError(t, err)
	require.True(t, photo.IsPrimary)
	require.Equal(t, 0, photo.Position)
	require.True(t, store.hasObject(photo.StorageKey))
}

func TestCreatePhotoRejectsWhenLimitReached(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := newService(repo, newFakeStorage(), fakeConsentChecker{})
	userID := uuid.New()

	for i := 0; i < domainprofile.MaxPhotos; i++ {
		_, err := svc.CreatePhoto(context.Background(), userID, applicationprofile.NewPhotoInput{
			MimeType: "image/png", Width: 10, Height: 10, ByteSize: 1, Data: []byte("x"),
		})
		require.NoError(t, err)
	}

	_, err := svc.CreatePhoto(context.Background(), userID, applicationprofile.NewPhotoInput{
		MimeType: "image/png", Width: 10, Height: 10, ByteSize: 1, Data: []byte("x"),
	})
	require.ErrorIs(t, err, domainprofile.ErrPhotoLimitReached)
}

// TestCreatePhotoCompensatesStorageWhenDatabaseFails is the mandatory
// storage/DB compensation test: if the DB insert fails after the blob was
// already written, the orphaned blob must be deleted.
func TestCreatePhotoCompensatesStorageWhenDatabaseFails(t *testing.T) {
	repo := newFakeProfileRepo()
	repo.forceCreatePhotoErr = errors.New("database is down")
	store := newFakeStorage()
	svc := newService(repo, store, fakeConsentChecker{})
	userID := uuid.New()

	_, err := svc.CreatePhoto(context.Background(), userID, applicationprofile.NewPhotoInput{
		MimeType: "image/png", Width: 10, Height: 10, ByteSize: 1, Data: []byte("x"),
	})
	require.Error(t, err)
	require.Equal(t, 0, store.objectCount(), "the orphaned blob must have been deleted")
	require.Equal(t, 1, store.deletedKeyCount())
}

func TestGetPhotoContentRejectsForeignPhoto(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := newService(repo, newFakeStorage(), fakeConsentChecker{})
	owner := uuid.New()
	attacker := uuid.New()

	photo, err := svc.CreatePhoto(context.Background(), owner, applicationprofile.NewPhotoInput{MimeType: "image/png", Width: 1, Height: 1, ByteSize: 1, Data: []byte("x")})
	require.NoError(t, err)

	_, _, err = svc.GetPhotoContent(context.Background(), attacker, photo.ID)
	require.ErrorIs(t, err, domainprofile.ErrPhotoNotFound)
}

func TestGetPhotoContentReturnsStoredBytes(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := newService(repo, newFakeStorage(), fakeConsentChecker{})
	userID := uuid.New()

	photo, err := svc.CreatePhoto(context.Background(), userID, applicationprofile.NewPhotoInput{MimeType: "image/png", Width: 1, Height: 1, ByteSize: 5, Data: []byte("hello")})
	require.NoError(t, err)

	reader, mimeType, err := svc.GetPhotoContent(context.Background(), userID, photo.ID)
	require.NoError(t, err)
	defer func() { _ = reader.Close() }()
	require.Equal(t, "image/png", mimeType)

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), data)
}

func TestReorderPhotosRejectsMismatchedSet(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := newService(repo, newFakeStorage(), fakeConsentChecker{})
	userID := uuid.New()

	first, err := svc.CreatePhoto(context.Background(), userID, applicationprofile.NewPhotoInput{MimeType: "image/png", Width: 1, Height: 1, ByteSize: 1, Data: []byte("x")})
	require.NoError(t, err)
	_, err = svc.CreatePhoto(context.Background(), userID, applicationprofile.NewPhotoInput{MimeType: "image/png", Width: 1, Height: 1, ByteSize: 1, Data: []byte("x")})
	require.NoError(t, err)

	// Only one of the two current photos: must be rejected.
	err = svc.ReorderPhotos(context.Background(), userID, []uuid.UUID{first.ID})
	require.ErrorIs(t, err, applicationprofile.ErrInvalidPhotoOrder)

	// A foreign photo id: must be rejected.
	err = svc.ReorderPhotos(context.Background(), userID, []uuid.UUID{first.ID, uuid.New()})
	require.ErrorIs(t, err, applicationprofile.ErrInvalidPhotoOrder)
}

func TestSetPrimaryPhotoRejectsForeignPhoto(t *testing.T) {
	repo := newFakeProfileRepo()
	svc := newService(repo, newFakeStorage(), fakeConsentChecker{})
	owner := uuid.New()
	attacker := uuid.New()

	photo, err := svc.CreatePhoto(context.Background(), owner, applicationprofile.NewPhotoInput{MimeType: "image/png", Width: 1, Height: 1, ByteSize: 1, Data: []byte("x")})
	require.NoError(t, err)

	err = svc.SetPrimaryPhoto(context.Background(), attacker, photo.ID)
	require.ErrorIs(t, err, domainprofile.ErrPhotoNotFound)
}

// TestDeletePhotoPromotesNewPrimaryAndCleansUpStorage covers the primary
// photo consistency rule and the asynchronous storage cleanup.
func TestDeletePhotoPromotesNewPrimaryAndCleansUpStorage(t *testing.T) {
	repo := newFakeProfileRepo()
	store := newFakeStorage()
	svc := newService(repo, store, fakeConsentChecker{})
	userID := uuid.New()

	first, err := svc.CreatePhoto(context.Background(), userID, applicationprofile.NewPhotoInput{MimeType: "image/png", Width: 1, Height: 1, ByteSize: 1, Data: []byte("x")})
	require.NoError(t, err)
	second, err := svc.CreatePhoto(context.Background(), userID, applicationprofile.NewPhotoInput{MimeType: "image/png", Width: 1, Height: 1, ByteSize: 1, Data: []byte("y")})
	require.NoError(t, err)
	require.True(t, first.IsPrimary)

	require.NoError(t, svc.DeletePhoto(context.Background(), userID, first.ID))

	remaining, err := svc.ListPhotos(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	require.Equal(t, second.ID, remaining[0].ID)
	require.True(t, remaining[0].IsPrimary, "a remaining photo must be promoted to primary")

	require.Eventually(t, func() bool {
		return !store.hasObject(first.StorageKey)
	}, time.Second, 5*time.Millisecond, "the deleted photo's blob must be cleaned up asynchronously")
}

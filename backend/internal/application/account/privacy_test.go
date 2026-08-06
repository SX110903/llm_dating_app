package account_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	applicationaccount "github.com/sx110903/llmatch-v2/backend/internal/application/account"
	domainaccount "github.com/sx110903/llmatch-v2/backend/internal/domain/account"
)

type fakeConsentRepo struct {
	consents map[uuid.UUID]*domainaccount.Consent
	grantErr error
}

func newFakeConsentRepo() *fakeConsentRepo {
	return &fakeConsentRepo{consents: map[uuid.UUID]*domainaccount.Consent{}}
}

func (r *fakeConsentRepo) Grant(_ context.Context, c *domainaccount.Consent) error {
	if r.grantErr != nil {
		return r.grantErr
	}
	if existing, ok := r.consents[c.UserID]; ok && existing.IsActive() {
		return domainaccount.ErrConsentActive
	}
	c.ID = uuid.New()
	c.GrantedAt = time.Now().UTC()
	c.CreatedAt = c.GrantedAt
	copied := *c
	r.consents[c.UserID] = &copied
	return nil
}

func (r *fakeConsentRepo) FindActive(_ context.Context, userID uuid.UUID, purpose string) (*domainaccount.Consent, error) {
	c, ok := r.consents[userID]
	if !ok || !c.IsActive() || c.Purpose != purpose {
		return nil, domainaccount.ErrConsentNotFound
	}
	copied := *c
	return &copied, nil
}

func (r *fakeConsentRepo) WithdrawGenderPreferenceConsent(_ context.Context, userID uuid.UUID, at time.Time) error {
	c, ok := r.consents[userID]
	if !ok {
		return nil
	}
	withdrawnAt := at
	c.WithdrawnAt = &withdrawnAt
	return nil
}

func TestGrantGenderPreferenceConsent(t *testing.T) {
	repo := newFakeConsentRepo()
	svc := applicationaccount.NewPrivacyService(repo)
	userID := uuid.New()

	consent, err := svc.GrantGenderPreferenceConsent(context.Background(), userID, "onboarding")
	require.NoError(t, err)
	require.Equal(t, domainaccount.PurposeGenderPreferences, consent.Purpose)
	require.True(t, consent.IsActive())
}

func TestHasActiveGenderPreferenceConsentWithoutGrant(t *testing.T) {
	repo := newFakeConsentRepo()
	svc := applicationaccount.NewPrivacyService(repo)

	active, err := svc.HasActiveGenderPreferenceConsent(context.Background(), uuid.New())
	require.NoError(t, err)
	require.False(t, active)
}

func TestHasActiveGenderPreferenceConsentAfterGrant(t *testing.T) {
	repo := newFakeConsentRepo()
	svc := applicationaccount.NewPrivacyService(repo)
	userID := uuid.New()

	_, err := svc.GrantGenderPreferenceConsent(context.Background(), userID, "onboarding")
	require.NoError(t, err)

	active, err := svc.HasActiveGenderPreferenceConsent(context.Background(), userID)
	require.NoError(t, err)
	require.True(t, active)
}

func TestGetActiveGenderPreferenceConsentReturnsNotFoundWithoutGrant(t *testing.T) {
	repo := newFakeConsentRepo()
	svc := applicationaccount.NewPrivacyService(repo)

	_, err := svc.GetActiveGenderPreferenceConsent(context.Background(), uuid.New())
	require.ErrorIs(t, err, domainaccount.ErrConsentNotFound)
}

func TestGetActiveGenderPreferenceConsentReturnsConsentAfterGrant(t *testing.T) {
	repo := newFakeConsentRepo()
	svc := applicationaccount.NewPrivacyService(repo)
	userID := uuid.New()

	_, err := svc.GrantGenderPreferenceConsent(context.Background(), userID, "onboarding")
	require.NoError(t, err)

	consent, err := svc.GetActiveGenderPreferenceConsent(context.Background(), userID)
	require.NoError(t, err)
	require.Equal(t, domainaccount.PurposeGenderPreferences, consent.Purpose)
}

// TestWithdrawGenderPreferenceConsentDisablesIt is the mandatory withdrawal
// test: after withdrawing, the consent must no longer be considered active.
func TestWithdrawGenderPreferenceConsentDisablesIt(t *testing.T) {
	repo := newFakeConsentRepo()
	svc := applicationaccount.NewPrivacyService(repo)
	userID := uuid.New()

	_, err := svc.GrantGenderPreferenceConsent(context.Background(), userID, "onboarding")
	require.NoError(t, err)

	require.NoError(t, svc.WithdrawGenderPreferenceConsent(context.Background(), userID))

	active, err := svc.HasActiveGenderPreferenceConsent(context.Background(), userID)
	require.NoError(t, err)
	require.False(t, active)
}

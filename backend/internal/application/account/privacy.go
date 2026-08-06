package account

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	domainaccount "github.com/sx110903/llmatch-v2/backend/internal/domain/account"
)

// currentPrivacyPolicyVersion is recorded on every granted consent so a
// future audit can tell which policy text the user actually agreed to.
const currentPrivacyPolicyVersion = "2026-08-05"

type PrivacyService struct {
	consents domainaccount.Repository
	now      func() time.Time
}

func NewPrivacyService(consents domainaccount.Repository) *PrivacyService {
	return &PrivacyService{consents: consents, now: func() time.Time { return time.Now().UTC() }}
}

func (s *PrivacyService) GrantGenderPreferenceConsent(ctx context.Context, userID uuid.UUID, source string) (*domainaccount.Consent, error) {
	consent := &domainaccount.Consent{
		UserID:        userID,
		Purpose:       domainaccount.PurposeGenderPreferences,
		PolicyVersion: currentPrivacyPolicyVersion,
		Source:        source,
	}
	if err := s.consents.Grant(ctx, consent); err != nil {
		return nil, err
	}
	return consent, nil
}

func (s *PrivacyService) WithdrawGenderPreferenceConsent(ctx context.Context, userID uuid.UUID) error {
	return s.consents.WithdrawGenderPreferenceConsent(ctx, userID, s.now())
}

func (s *PrivacyService) GetActiveGenderPreferenceConsent(ctx context.Context, userID uuid.UUID) (*domainaccount.Consent, error) {
	return s.consents.FindActive(ctx, userID, domainaccount.PurposeGenderPreferences)
}

func (s *PrivacyService) HasActiveGenderPreferenceConsent(ctx context.Context, userID uuid.UUID) (bool, error) {
	_, err := s.consents.FindActive(ctx, userID, domainaccount.PurposeGenderPreferences)
	if err != nil {
		if errors.Is(err, domainaccount.ErrConsentNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("check active consent: %w", err)
	}
	return true, nil
}

package account

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository is an outbound application port implemented by infrastructure adapters.
type Repository interface {
	Grant(ctx context.Context, c *Consent) error
	FindActive(ctx context.Context, userID uuid.UUID, purpose string) (*Consent, error)

	// WithdrawGenderPreferenceConsent atomically withdraws the active
	// PurposeGenderPreferences consent and clears user_preferences.genders
	// in the same transaction, so discovery can never observe a withdrawn
	// consent alongside a still-populated sensitive value.
	WithdrawGenderPreferenceConsent(ctx context.Context, userID uuid.UUID, at time.Time) error
}

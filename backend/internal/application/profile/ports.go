package profile

import (
	"context"

	"github.com/google/uuid"
)

// ConsentChecker is satisfied by application/account.PrivacyService. Kept as
// a narrow interface here so this package never imports application/account.
type ConsentChecker interface {
	HasActiveGenderPreferenceConsent(ctx context.Context, userID uuid.UUID) (bool, error)
}

package account

import (
	"time"

	"github.com/google/uuid"
)

// PurposeGenderPreferences is the only consent purpose defined so far: the
// user_preferences.genders column reveals de facto sexual orientation and is
// treated as an RGPD article 9 special category of data.
const PurposeGenderPreferences = "matching_gender_preferences"

type Consent struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	Purpose       string
	PolicyVersion string
	GrantedAt     time.Time
	WithdrawnAt   *time.Time
	Source        string
	CreatedAt     time.Time
}

func (c Consent) IsActive() bool {
	return c.WithdrawnAt == nil
}

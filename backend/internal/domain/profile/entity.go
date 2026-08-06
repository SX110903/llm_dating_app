package profile

import (
	"time"

	"github.com/google/uuid"
)

const MaxPhotos = 6

type Coordinates struct {
	Latitude  float64
	Longitude float64
}

type Profile struct {
	UserID    uuid.UUID
	Bio       string
	Interests []string
	City      string
	// Location and ClearLocation express the write intent for the stored
	// coordinates, which has three states: Location set replaces them,
	// ClearLocation drops them, and neither leaves them untouched. A profile
	// update that simply omits coordinates must never erase them.
	Location      *Coordinates
	ClearLocation bool
	// HasLocation is read-only: reads never resolve Location back, because a
	// lossy PostGIS NULL-geography round trip can't be scanned safely.
	HasLocation         bool
	Questionnaire       map[string]any
	OnboardingCompleted bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Preferences struct {
	UserID        uuid.UUID
	MinAge        int
	MaxAge        int
	MaxDistanceKM int
	Genders       []string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Photo struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	StorageKey string
	MimeType   string
	ByteSize   int64
	Width      int
	Height     int
	Position   int
	IsPrimary  bool
	CreatedAt  time.Time
	DeletedAt  *time.Time
}

func (p Photo) IsDeleted() bool {
	return p.DeletedAt != nil
}

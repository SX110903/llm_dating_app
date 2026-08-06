package profile

type UpdateProfileRequest struct {
	Bio       string   `json:"bio" validate:"max=500"`
	Interests []string `json:"interests" validate:"max=20,dive,max=40"`
	City      string   `json:"city" validate:"max=120"`
	// Omitting both coordinates preserves the stored location. ClearLocation
	// is the only way to remove it.
	Latitude            *float64       `json:"latitude" validate:"omitempty,min=-90,max=90"`
	Longitude           *float64       `json:"longitude" validate:"omitempty,min=-180,max=180"`
	ClearLocation       bool           `json:"clear_location"`
	Questionnaire       map[string]any `json:"questionnaire"`
	OnboardingCompleted bool           `json:"onboarding_completed"`
}

type ProfileResponse struct {
	UserID              string         `json:"user_id"`
	Bio                 string         `json:"bio"`
	Interests           []string       `json:"interests"`
	City                string         `json:"city"`
	HasLocation         bool           `json:"has_location"`
	Questionnaire       map[string]any `json:"questionnaire"`
	OnboardingCompleted bool           `json:"onboarding_completed"`
	CreatedAt           string         `json:"created_at"`
	UpdatedAt           string         `json:"updated_at"`
}

type UpdatePreferencesRequest struct {
	MinAge        int `json:"min_age" validate:"required,min=18,max=100"`
	MaxAge        int `json:"max_age" validate:"required,min=18,max=100"`
	MaxDistanceKM int `json:"max_distance_km" validate:"required,min=1,max=500"`
	// Genders is a pointer so an omitted field (nil) can be distinguished
	// from an explicit empty array, matching the application layer's
	// "nil means leave untouched" contract.
	Genders *[]string `json:"genders" validate:"omitempty,max=10,dive,max=50"`
}

type PreferencesResponse struct {
	MinAge        int      `json:"min_age"`
	MaxAge        int      `json:"max_age"`
	MaxDistanceKM int      `json:"max_distance_km"`
	Genders       []string `json:"genders"`
}

type PhotoResponse struct {
	ID        string `json:"id"`
	MimeType  string `json:"mime_type"`
	ByteSize  int64  `json:"byte_size"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Position  int    `json:"position"`
	IsPrimary bool   `json:"is_primary"`
	CreatedAt string `json:"created_at"`
}

type ReorderPhotosRequest struct {
	PhotoIDs []string `json:"photo_ids" validate:"required,min=1,max=6,dive,uuid4"`
}

type ErrorResponse struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	RequestID string            `json:"request_id"`
	Details   map[string]string `json:"details,omitempty"`
}

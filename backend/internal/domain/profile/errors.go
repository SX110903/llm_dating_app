package profile

import "errors"

var (
	ErrNotFound            = errors.New("profile not found")
	ErrPreferencesNotFound = errors.New("preferences not found")
	ErrPhotoNotFound       = errors.New("photo not found")
	ErrPhotoLimitReached   = errors.New("maximum number of photos reached")
	ErrInvalidPosition     = errors.New("invalid photo position")
	ErrUnsupportedMimeType = errors.New("unsupported photo mime type")
	ErrPhotoTooLarge       = errors.New("photo exceeds the maximum allowed size")
	ErrConsentRequired     = errors.New("explicit consent is required before saving this preference")
	ErrInvalidAgeRange     = errors.New("invalid age range preference")
	ErrInvalidDistance     = errors.New("invalid max distance preference")
)

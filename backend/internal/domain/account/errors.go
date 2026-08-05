package account

import "errors"

var (
	ErrConsentNotFound = errors.New("consent not found")
	ErrConsentActive   = errors.New("an active consent for this purpose already exists")
)

package session

import "errors"

var (
	ErrNotFound = errors.New("refresh token not found")
	ErrReused   = errors.New("refresh token reuse detected")
	ErrRevoked  = errors.New("refresh token revoked")
	ErrExpired  = errors.New("refresh token expired")
)

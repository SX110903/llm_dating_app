package user

import "errors"

var (
	ErrNotFound   = errors.New("user not found")
	ErrEmailTaken = errors.New("email already registered")
	ErrNotActive  = errors.New("user is not active")
	ErrUnderage   = errors.New("user does not meet the minimum age")
)

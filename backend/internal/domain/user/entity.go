package user

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
	StatusDeleted   Status = "deleted"
)

type User struct {
	ID                uuid.UUID
	Email             string
	PasswordHash      string
	DisplayName       string
	BirthDate         time.Time
	Gender            string
	Status            Status
	EmailVerifiedAt   *time.Time
	PasswordChangedAt time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (u User) IsActive() bool {
	return u.Status == StatusActive
}

func (u User) Age(at time.Time) int {
	years := at.Year() - u.BirthDate.Year()
	anniversary := time.Date(at.Year(), u.BirthDate.Month(), u.BirthDate.Day(), 0, 0, 0, 0, time.UTC)
	if at.Before(anniversary) {
		years--
	}
	return years
}

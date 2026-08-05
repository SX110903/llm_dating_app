package session

import (
	"net/netip"
	"time"

	"github.com/google/uuid"
)

// RefreshToken represents one issued refresh token within a rotation family.
// Only its SHA-256 hash is ever persisted; the opaque token value itself
// never reaches storage.
type RefreshToken struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	FamilyID     uuid.UUID
	TokenHash    []byte
	ReplacedBy   *uuid.UUID
	DeviceLabel  *string
	UserAgent    *string
	IP           *netip.Addr
	ExpiresAt    time.Time
	LastUsedAt   *time.Time
	RevokedAt    *time.Time
	RevokeReason *string
	CreatedAt    time.Time
}

func (r RefreshToken) IsRevoked() bool {
	return r.RevokedAt != nil
}

func (r RefreshToken) IsExpired(at time.Time) bool {
	return at.After(r.ExpiresAt)
}

func (r RefreshToken) IsUsable(at time.Time) bool {
	return !r.IsRevoked() && !r.IsExpired(at) && r.ReplacedBy == nil
}

const (
	RevokeReasonRotated       = "rotated"
	RevokeReasonReuseDetected = "reuse_detected"
	RevokeReasonLogout        = "logout"
	RevokeReasonLogoutAll     = "logout_all"
)

package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const opaqueTokenBytes = 32 // 256 bits

// NewOpaqueToken returns a random 256-bit token encoded for transport plus
// the SHA-256 hash that must be persisted instead of the token itself.
func NewOpaqueToken() (token string, hash []byte, err error) {
	raw := make([]byte, opaqueTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("generate opaque token: %w", err)
	}
	sum := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(raw), sum[:], nil
}

// HashOpaqueToken recomputes the lookup hash for a token received from a client.
func HashOpaqueToken(token string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("decode opaque token: %w", err)
	}
	sum := sha256.Sum256(raw)
	return sum[:], nil
}

// OpaqueTokenGenerator adapts the free functions above to the
// application-layer refresh token generator port.
type OpaqueTokenGenerator struct{}

func (OpaqueTokenGenerator) New() (token string, hash []byte, err error) {
	return NewOpaqueToken()
}

func (OpaqueTokenGenerator) Hash(token string) ([]byte, error) {
	return HashOpaqueToken(token)
}

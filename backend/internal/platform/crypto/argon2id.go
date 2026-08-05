package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2Params follows the OWASP baseline for Argon2id (19 MiB, t=2, p=1).
// Persisted inside the PHC-formatted hash itself, so raising these values
// later only requires a rehash of existing users after their next login.
type Argon2Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

var DefaultArgon2Params = Argon2Params{
	Memory:      19 * 1024,
	Iterations:  2,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

func HashPassword(password string) (string, error) {
	return hashPasswordWithParams(password, DefaultArgon2Params)
}

func hashPasswordWithParams(password string, params Argon2Params) (string, error) {
	salt := make([]byte, params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate argon2id salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, params.Iterations, params.Memory, params.Parallelism, params.KeyLength)
	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, params.Memory, params.Iterations, params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

func VerifyPassword(password, encodedHash string) (bool, error) {
	params, salt, hash, err := decodeHash(encodedHash)
	if err != nil {
		return false, err
	}
	candidate := argon2.IDKey([]byte(password), salt, params.Iterations, params.Memory, params.Parallelism, params.KeyLength)
	return subtle.ConstantTimeCompare(hash, candidate) == 1, nil
}

// NeedsRehash reports whether a stored hash was produced with parameters
// older than DefaultArgon2Params, so callers can rehash transparently
// after a successful login.
func NeedsRehash(encodedHash string) (bool, error) {
	params, _, _, err := decodeHash(encodedHash)
	if err != nil {
		return false, err
	}
	return params != DefaultArgon2Params, nil
}

// Argon2idHasher adapts the free functions above to the application-layer
// password hasher port.
type Argon2idHasher struct{}

func (Argon2idHasher) Hash(password string) (string, error) {
	return HashPassword(password)
}

func (Argon2idHasher) Verify(password, encodedHash string) (bool, error) {
	return VerifyPassword(password, encodedHash)
}

func (Argon2idHasher) NeedsRehash(encodedHash string) (bool, error) {
	return NeedsRehash(encodedHash)
}

func decodeHash(encoded string) (Argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return Argon2Params{}, nil, nil, errors.New("invalid argon2id hash format")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("parse argon2id version: %w", err)
	}
	if version != argon2.Version {
		return Argon2Params{}, nil, nil, errors.New("unsupported argon2id version")
	}

	var params Argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.Memory, &params.Iterations, &params.Parallelism); err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("parse argon2id params: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("decode argon2id salt: %w", err)
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("decode argon2id hash: %w", err)
	}
	params.SaltLength = uint32(len(salt)) // #nosec G115 -- decoded PHC salt/hash are always a few dozen bytes
	params.KeyLength = uint32(len(hash))  // #nosec G115 -- decoded PHC salt/hash are always a few dozen bytes
	return params, salt, hash, nil
}

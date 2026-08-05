package crypto_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sx110903/llmatch-v2/backend/internal/platform/crypto"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := crypto.HashPassword("correct horse battery staple")
	require.NoError(t, err)
	require.NotEmpty(t, hash)

	match, err := crypto.VerifyPassword("correct horse battery staple", hash)
	require.NoError(t, err)
	require.True(t, match)
}

func TestVerifyPasswordRejectsWrongPassword(t *testing.T) {
	hash, err := crypto.HashPassword("correct horse battery staple")
	require.NoError(t, err)

	match, err := crypto.VerifyPassword("wrong password", hash)
	require.NoError(t, err)
	require.False(t, match)
}

func TestHashPasswordProducesUniqueSalts(t *testing.T) {
	first, err := crypto.HashPassword("same password")
	require.NoError(t, err)
	second, err := crypto.HashPassword("same password")
	require.NoError(t, err)

	require.NotEqual(t, first, second)
}

func TestNeedsRehashDetectsOlderParameters(t *testing.T) {
	current, err := crypto.HashPassword("current params")
	require.NoError(t, err)
	needsRehash, err := crypto.NeedsRehash(current)
	require.NoError(t, err)
	require.False(t, needsRehash)

	stale := "$argon2id$v=19$m=4096,t=1,p=1$c29tZXNhbHQxMjM0NQ$c29tZWhhc2gxMjM0NTY3ODkwMTIzNDU2"
	needsRehash, err = crypto.NeedsRehash(stale)
	require.NoError(t, err)
	require.True(t, needsRehash)
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	_, err := crypto.VerifyPassword("password", "not-a-valid-hash")
	require.Error(t, err)
}

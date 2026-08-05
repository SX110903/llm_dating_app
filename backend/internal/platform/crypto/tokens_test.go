package crypto_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sx110903/llmatch-v2/backend/internal/platform/crypto"
)

func TestNewOpaqueTokenHashRoundTrip(t *testing.T) {
	token, hash, err := crypto.NewOpaqueToken()
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.Len(t, hash, 32)

	recomputed, err := crypto.HashOpaqueToken(token)
	require.NoError(t, err)
	require.Equal(t, hash, recomputed)
}

func TestNewOpaqueTokenIsUnique(t *testing.T) {
	first, _, err := crypto.NewOpaqueToken()
	require.NoError(t, err)
	second, _, err := crypto.NewOpaqueToken()
	require.NoError(t, err)
	require.NotEqual(t, first, second)
}

func TestHashOpaqueTokenRejectsInvalidEncoding(t *testing.T) {
	_, err := crypto.HashOpaqueToken("not base64url!!")
	require.Error(t, err)
}

package crypto_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/sx110903/llmatch-v2/backend/internal/platform/crypto"
)

func generateKeyPair(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key, &key.PublicKey
}

func TestTokenIssuerIssueAndParse(t *testing.T) {
	priv, pub := generateKeyPair(t)
	issuer := crypto.NewTokenIssuer(priv, pub, "llmatch-v2", "llmatch-v2-clients", 15*time.Minute)

	subject := "11111111-1111-1111-1111-111111111111"
	signed, jti, expiresAt, err := issuer.Issue(subject)
	require.NoError(t, err)
	require.NotEmpty(t, signed)
	require.NotEmpty(t, jti)
	require.WithinDuration(t, time.Now().Add(15*time.Minute), expiresAt, 2*time.Second)

	claims, err := issuer.Parse(signed)
	require.NoError(t, err)
	require.Equal(t, subject, claims.Subject)
	require.Equal(t, jti, claims.ID)
}

func TestTokenIssuerRejectsWrongAudience(t *testing.T) {
	priv, pub := generateKeyPair(t)
	issuer := crypto.NewTokenIssuer(priv, pub, "llmatch-v2", "llmatch-v2-clients", 15*time.Minute)
	otherAudienceIssuer := crypto.NewTokenIssuer(priv, pub, "llmatch-v2", "other-audience", 15*time.Minute)

	signed, _, _, err := otherAudienceIssuer.Issue("subject")
	require.NoError(t, err)

	_, err = issuer.Parse(signed)
	require.Error(t, err)
}

func TestTokenIssuerRejectsExpiredToken(t *testing.T) {
	priv, pub := generateKeyPair(t)
	issuer := crypto.NewTokenIssuer(priv, pub, "llmatch-v2", "llmatch-v2-clients", -time.Minute)

	signed, _, _, err := issuer.Issue("subject")
	require.NoError(t, err)

	_, err = issuer.Parse(signed)
	require.Error(t, err)
}

func TestTokenIssuerRejectsWrongSigningKey(t *testing.T) {
	priv1, pub1 := generateKeyPair(t)
	priv2, pub2 := generateKeyPair(t)

	issuedWithOtherKey := crypto.NewTokenIssuer(priv2, pub2, "llmatch-v2", "llmatch-v2-clients", 15*time.Minute)
	signed, _, _, err := issuedWithOtherKey.Issue("subject")
	require.NoError(t, err)

	issuer := crypto.NewTokenIssuer(priv1, pub1, "llmatch-v2", "llmatch-v2-clients", 15*time.Minute)
	_, err = issuer.Parse(signed)
	require.Error(t, err)
}

// TestTokenIssuerRejectsWrongAlgorithm guards against an algorithm-confusion
// attack where an attacker forges an HS256 token instead of the expected RS256.
func TestTokenIssuerRejectsWrongAlgorithm(t *testing.T) {
	priv, pub := generateKeyPair(t)
	issuer := crypto.NewTokenIssuer(priv, pub, "llmatch-v2", "llmatch-v2-clients", 15*time.Minute)

	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   "attacker",
		ID:        "forged-jti",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		Issuer:    "llmatch-v2",
		Audience:  jwt.ClaimStrings{"llmatch-v2-clients"},
	})
	signed, err := forged.SignedString([]byte("guessed-secret"))
	require.NoError(t, err)

	_, err = issuer.Parse(signed)
	require.Error(t, err)
}

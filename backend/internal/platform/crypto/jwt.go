package crypto

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AccessClaims are the registered claims carried by an access token: sub,
// jti, iat, exp, iss and aud, as required by the plan. No custom claims are
// added so the token cannot leak profile data.
type AccessClaims struct {
	jwt.RegisteredClaims
}

type TokenIssuer struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	issuer     string
	audience   string
	ttl        time.Duration
}

func NewTokenIssuer(privateKey *rsa.PrivateKey, publicKey *rsa.PublicKey, issuer, audience string, ttl time.Duration) *TokenIssuer {
	return &TokenIssuer{privateKey: privateKey, publicKey: publicKey, issuer: issuer, audience: audience, ttl: ttl}
}

// Issue signs a new access token for subject and returns it alongside its
// jti and expiry so the caller can register the jti in Redis.
func (i *TokenIssuer) Issue(subject string) (signed, jti string, expiresAt time.Time, err error) {
	now := time.Now().UTC()
	jti = uuid.NewString()
	expiresAt = now.Add(i.ttl)

	claims := AccessClaims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   subject,
		ID:        jti,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		Issuer:    i.issuer,
		Audience:  jwt.ClaimStrings{i.audience},
	}}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err = token.SignedString(i.privateKey)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, jti, expiresAt, nil
}

// Parse validates signature, algorithm, issuer, audience and expiry. It does
// not consult the revocation denylist; callers are responsible for that.
func (i *TokenIssuer) Parse(tokenString string) (*AccessClaims, error) {
	claims := &AccessClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		return i.publicKey, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(i.issuer),
		jwt.WithAudience(i.audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("parse access token: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("invalid access token")
	}
	if claims.ID == "" || claims.Subject == "" {
		return nil, errors.New("access token missing required claims")
	}
	return claims, nil
}

func LoadRSAPrivateKey(path string) (*rsa.PrivateKey, error) {
	// #nosec G304 -- key path comes from trusted deployment configuration.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read RSA private key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("RSA private key PEM block not found")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("expected an RSA PKCS#8 or PKCS#1 private key")
}

func LoadRSAPublicKey(path string) (*rsa.PublicKey, error) {
	// #nosec G304 -- key path comes from trusted deployment configuration.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read RSA public key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("RSA public key PEM block not found")
	}
	if key, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PublicKey); ok {
			return rsaKey, nil
		}
	}
	if key, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("expected an RSA PKIX or PKCS#1 public key")
}

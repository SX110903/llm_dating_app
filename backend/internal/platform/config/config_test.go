package config_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sx110903/llmatch-v2/backend/internal/platform/config"
)

func TestLoadRejectsMissingEnvironment(t *testing.T) {
	setBaseEnvironment(t)
	t.Setenv("APP_ENV", "")

	_, err := config.Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "APP_ENV")
}

func TestLoadRejectsWildcardCORS(t *testing.T) {
	setBaseEnvironment(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")

	_, err := config.Load()

	require.EqualError(t, err, "wildcard CORS origins are not allowed")
}

func TestLoadRejectsProductionWithoutVerifiedTransport(t *testing.T) {
	setBaseEnvironment(t)
	t.Setenv("APP_ENV", "production")

	_, err := config.Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sslmode=verify-full")
}

func TestLoadAcceptsCompleteDevelopmentConfiguration(t *testing.T) {
	setBaseEnvironment(t)

	loaded, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, config.Development, loaded.Environment)
	assert.Equal(t, []string{"http://localhost:8080"}, loaded.AllowedOrigins)
}

func setBaseEnvironment(t *testing.T) {
	t.Helper()
	privatePath, publicPath := writeTestKeyPair(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://llmatch_app:development-password@postgres:5432/llmatch?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://:development-password@redis:6379/0")
	t.Setenv("JWT_PRIVATE_KEY_FILE", privatePath)
	t.Setenv("JWT_PUBLIC_KEY_FILE", publicPath)
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:8080")
}

func writeTestKeyPair(t *testing.T) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	directory := t.TempDir()
	privatePath := filepath.Join(directory, "private.pem")
	publicPath := filepath.Join(directory, "public.pem")
	privateBlock, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	publicBlock, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(privatePath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateBlock}), 0o600))
	require.NoError(t, os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicBlock}), 0o600))
	return privatePath, publicPath
}

package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/sx110903/llmatch-v2/backend/internal/platform/crypto"
	"github.com/sx110903/llmatch-v2/backend/internal/platform/middleware"
)

type fakeParser struct {
	claims *crypto.AccessClaims
	err    error
}

func (f fakeParser) Parse(string) (*crypto.AccessClaims, error) {
	return f.claims, f.err
}

type fakeDenylist struct {
	revoked bool
	err     error
}

func (f fakeDenylist) IsRevoked(context.Context, string) (bool, error) {
	return f.revoked, f.err
}

func validClaims(subject, jti string) *crypto.AccessClaims {
	return &crypto.AccessClaims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   subject,
		ID:        jti,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}}
}

func TestAuthMiddlewareMissingBearerToken(t *testing.T) {
	handlerCalled := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { handlerCalled = true })
	mw := middleware.Auth(fakeParser{}, fakeDenylist{}, time.Second)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	require.False(t, handlerCalled)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddlewareInvalidToken(t *testing.T) {
	handlerCalled := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { handlerCalled = true })
	mw := middleware.Auth(fakeParser{err: errors.New("bad token")}, fakeDenylist{}, time.Second)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer bad")
	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	require.False(t, handlerCalled)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestAuthMiddlewareRedisDownReturns503AndNeverCallsHandler is the contract
// test required by the plan: when the jti revocation check cannot be
// confirmed against Redis, the response is 503 AUTH_DEPENDENCY_UNAVAILABLE
// and the protected handler never executes.
func TestAuthMiddlewareRedisDownReturns503AndNeverCallsHandler(t *testing.T) {
	userID := uuid.New()
	handlerCalled := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { handlerCalled = true })
	mw := middleware.Auth(
		fakeParser{claims: validClaims(userID.String(), "jti-1")},
		fakeDenylist{err: errors.New("redis unavailable")},
		time.Second,
	)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer good")
	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	require.False(t, handlerCalled, "protected handler must never run when revocation status cannot be confirmed")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "AUTH_DEPENDENCY_UNAVAILABLE")
}

func TestAuthMiddlewareRevokedTokenIsRejected(t *testing.T) {
	userID := uuid.New()
	handlerCalled := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { handlerCalled = true })
	mw := middleware.Auth(
		fakeParser{claims: validClaims(userID.String(), "jti-1")},
		fakeDenylist{revoked: true},
		time.Second,
	)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer good")
	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	require.False(t, handlerCalled)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddlewareValidTokenAttachesIdentity(t *testing.T) {
	userID := uuid.New()
	var gotIdentity middleware.Identity
	var gotOK bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdentity, gotOK = middleware.IdentityFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	mw := middleware.Auth(
		fakeParser{claims: validClaims(userID.String(), "jti-1")},
		fakeDenylist{},
		time.Second,
	)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer good")
	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, gotOK)
	require.Equal(t, userID, gotIdentity.UserID)
	require.Equal(t, "jti-1", gotIdentity.JTI)
}

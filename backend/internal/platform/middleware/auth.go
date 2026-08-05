package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/sx110903/llmatch-v2/backend/internal/platform/crypto"
)

// TokenParser validates signature, algorithm, issuer, audience and expiry.
type TokenParser interface {
	Parse(tokenString string) (*crypto.AccessClaims, error)
}

// Denylist checks whether an access token's jti has been revoked.
type Denylist interface {
	IsRevoked(ctx context.Context, jti string) (bool, error)
}

type identityKey struct{}

type Identity struct {
	UserID    uuid.UUID
	JTI       string
	ExpiresAt time.Time
}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityKey{}).(Identity)
	return identity, ok
}

// Auth enforces the fail-closed authentication contract: a JWT that fails
// signature/claims validation is 401, and a jti whose revocation status
// cannot be confirmed against Redis (timeout, error or disconnection) is
// 503 AUTH_DEPENDENCY_UNAVAILABLE. The protected handler never runs unless
// both checks succeed.
func Auth(parser TokenParser, denylist Denylist, checkTimeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				writeAuthError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token")
				return
			}

			claims, err := parser.Parse(token)
			if err != nil {
				writeAuthError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid access token")
				return
			}

			checkCtx, cancel := context.WithTimeout(r.Context(), checkTimeout)
			revoked, err := denylist.IsRevoked(checkCtx, claims.ID)
			cancel()
			if err != nil {
				writeAuthError(w, r, http.StatusServiceUnavailable, "AUTH_DEPENDENCY_UNAVAILABLE", "authentication dependency unavailable")
				return
			}
			if revoked {
				writeAuthError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "token has been revoked")
				return
			}

			userID, err := uuid.Parse(claims.Subject)
			if err != nil {
				writeAuthError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid access token subject")
				return
			}

			identity := Identity{UserID: userID, JTI: claims.ID, ExpiresAt: claims.ExpiresAt.Time}
			ctx := context.WithValue(r.Context(), identityKey{}, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return "", false
	}
	return token, true
}

func writeAuthError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":       code,
		"message":    message,
		"request_id": RequestIDFromContext(r.Context()),
	})
}

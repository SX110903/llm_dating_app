package auth

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"

	domainsession "github.com/sx110903/llmatch-v2/backend/internal/domain/session"
	domainuser "github.com/sx110903/llmatch-v2/backend/internal/domain/user"
)

var (
	ErrInvalidCredentials    = errors.New("invalid email or password")
	ErrInvalidRefreshToken   = errors.New("invalid or expired refresh token")
	ErrWeakPassword          = errors.New("password does not meet the minimum policy")
	ErrDependencyUnavailable = errors.New("auth dependency unavailable")
)

const (
	minPasswordLength = 12
	maxPasswordLength = 128
	minimumAge        = 18
)

// RateLimitedError signals that a scope (IP or email) is temporarily locked out.
type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("rate limit exceeded, retry after %s", e.RetryAfter)
}

type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
	BirthDate   time.Time
	Gender      string
	IP          string
}

type LoginInput struct {
	Email       string
	Password    string
	IP          string
	UserAgent   string
	DeviceLabel string
}

type RefreshInput struct {
	RefreshToken string
	IP           string
	UserAgent    string
}

// AuthResult carries a freshly issued access/refresh token pair.
type AuthResult struct {
	User                  domainuser.User
	AccessToken           string
	AccessTokenExpiresAt  time.Time
	RefreshToken          string
	RefreshTokenExpiresAt time.Time
}

type Config struct {
	RefreshTokenTTL time.Duration
}

type Service struct {
	users         domainuser.Repository
	sessions      domainsession.Repository
	hasher        PasswordHasher
	tokens        AccessTokenIssuer
	refreshTokens RefreshTokenGenerator
	denylist      Denylist
	rateLimiter   RateLimiter
	refreshTTL    time.Duration
	now           func() time.Time
}

func NewService(
	users domainuser.Repository,
	sessions domainsession.Repository,
	hasher PasswordHasher,
	tokens AccessTokenIssuer,
	refreshTokens RefreshTokenGenerator,
	denylist Denylist,
	rateLimiter RateLimiter,
	cfg Config,
) *Service {
	return &Service{
		users:         users,
		sessions:      sessions,
		hasher:        hasher,
		tokens:        tokens,
		refreshTokens: refreshTokens,
		denylist:      denylist,
		rateLimiter:   rateLimiter,
		refreshTTL:    cfg.RefreshTokenTTL,
		now:           func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (*domainuser.User, error) {
	email := normalizeEmail(in.Email)
	ipScope := "register:ip:" + in.IP

	if err := s.checkRateLimit(ctx, ipScope); err != nil {
		return nil, err
	}

	if err := validatePassword(in.Password, email); err != nil {
		if recordErr := s.recordFailure(ctx, ipScope); recordErr != nil {
			return nil, recordErr
		}
		return nil, err
	}

	now := s.now()
	candidate := domainuser.User{BirthDate: in.BirthDate}
	if candidate.Age(now) < minimumAge {
		if recordErr := s.recordFailure(ctx, ipScope); recordErr != nil {
			return nil, recordErr
		}
		return nil, domainuser.ErrUnderage
	}

	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	newUser := &domainuser.User{
		ID:                uuid.New(),
		Email:             email,
		PasswordHash:      hash,
		DisplayName:       in.DisplayName,
		BirthDate:         in.BirthDate,
		Gender:            in.Gender,
		Status:            domainuser.StatusActive,
		PasswordChangedAt: now,
	}
	if err := s.users.Create(ctx, newUser); err != nil {
		if errors.Is(err, domainuser.ErrEmailTaken) {
			if recordErr := s.recordFailure(ctx, ipScope); recordErr != nil {
				return nil, recordErr
			}
		}
		return nil, err
	}

	if err := s.rateLimiter.Reset(ctx, ipScope); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDependencyUnavailable, err)
	}
	return newUser, nil
}

func (s *Service) Login(ctx context.Context, in LoginInput) (*AuthResult, error) {
	email := normalizeEmail(in.Email)
	ipScope := "login:ip:" + in.IP
	emailScope := "login:email:" + email

	if err := s.checkRateLimit(ctx, ipScope, emailScope); err != nil {
		return nil, err
	}

	foundUser, err := s.users.FindByEmail(ctx, email)
	valid := err == nil && foundUser.IsActive()
	if valid {
		valid, err = s.hasher.Verify(in.Password, foundUser.PasswordHash)
		if err != nil {
			return nil, fmt.Errorf("verify password: %w", err)
		}
	}
	if !valid {
		if recordErr := s.recordFailure(ctx, ipScope, emailScope); recordErr != nil {
			return nil, recordErr
		}
		return nil, ErrInvalidCredentials
	}

	if resetErr := s.rateLimiter.Reset(ctx, ipScope); resetErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrDependencyUnavailable, resetErr)
	}
	if resetErr := s.rateLimiter.Reset(ctx, emailScope); resetErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrDependencyUnavailable, resetErr)
	}

	if needsRehash, err := s.hasher.NeedsRehash(foundUser.PasswordHash); err == nil && needsRehash {
		if newHash, err := s.hasher.Hash(in.Password); err == nil {
			_ = s.users.UpdatePasswordHash(ctx, foundUser.ID, newHash, s.now())
		}
	}

	return s.issueSession(ctx, foundUser, uuid.New(), in.IP, in.UserAgent, in.DeviceLabel)
}

func (s *Service) Refresh(ctx context.Context, in RefreshInput) (*AuthResult, error) {
	hash, err := s.refreshTokens.Hash(in.RefreshToken)
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}

	current, err := s.sessions.FindByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, domainsession.ErrNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, fmt.Errorf("find refresh token: %w", err)
	}

	now := s.now()
	if current.IsExpired(now) {
		return nil, ErrInvalidRefreshToken
	}

	foundUser, err := s.users.FindByID(ctx, current.UserID)
	if err != nil {
		if errors.Is(err, domainuser.ErrNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, fmt.Errorf("find user for refresh: %w", err)
	}
	if !foundUser.IsActive() {
		return nil, ErrInvalidRefreshToken
	}

	nextToken, nextHash, err := s.refreshTokens.New()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	next := &domainsession.RefreshToken{
		ID:          uuid.New(),
		UserID:      current.UserID,
		FamilyID:    current.FamilyID,
		TokenHash:   nextHash,
		DeviceLabel: current.DeviceLabel,
		UserAgent:   optionalString(in.UserAgent),
		IP:          parseIP(in.IP),
		ExpiresAt:   now.Add(s.refreshTTL),
	}

	if err := s.sessions.Rotate(ctx, current.ID, next); err != nil {
		if errors.Is(err, domainsession.ErrReused) {
			if denylistErr := s.denylist.DenylistAllActive(ctx, current.UserID); denylistErr != nil {
				return nil, fmt.Errorf("%w: %w", ErrDependencyUnavailable, denylistErr)
			}
			return nil, ErrInvalidRefreshToken
		}
		if errors.Is(err, domainsession.ErrNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, fmt.Errorf("rotate refresh token: %w", err)
	}

	accessToken, jti, accessExpiresAt, err := s.tokens.Issue(foundUser.ID.String())
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}
	if err := s.denylist.RegisterActive(ctx, foundUser.ID, jti, accessExpiresAt); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDependencyUnavailable, err)
	}

	return &AuthResult{
		User:                  *foundUser,
		AccessToken:           accessToken,
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshToken:          nextToken,
		RefreshTokenExpiresAt: next.ExpiresAt,
	}, nil
}

// Logout denylists the caller's current access token jti and, when a refresh
// cookie was presented, revokes that session's whole rotation family.
func (s *Service) Logout(ctx context.Context, userID uuid.UUID, jti string, accessExpiresAt time.Time, refreshToken string) error {
	if err := s.denylist.Denylist(ctx, jti, accessExpiresAt); err != nil {
		return fmt.Errorf("%w: %w", ErrDependencyUnavailable, err)
	}
	if refreshToken == "" {
		return nil
	}

	hash, hashErr := s.refreshTokens.Hash(refreshToken)
	if hashErr != nil {
		return nil //nolint:nilerr // malformed refresh cookie does not block logout once the jti is denylisted
	}
	current, findErr := s.sessions.FindByHash(ctx, hash)
	if findErr != nil || current.UserID != userID {
		return nil //nolint:nilerr // unknown or foreign refresh cookie does not block logout once the jti is denylisted
	}
	if err := s.sessions.RevokeFamily(ctx, current.FamilyID, domainsession.RevokeReasonLogout, s.now()); err != nil {
		return fmt.Errorf("revoke refresh token family: %w", err)
	}
	return nil
}

func (s *Service) LogoutAll(ctx context.Context, userID uuid.UUID) error {
	if err := s.denylist.DenylistAllActive(ctx, userID); err != nil {
		return fmt.Errorf("%w: %w", ErrDependencyUnavailable, err)
	}
	if err := s.sessions.RevokeAllForUser(ctx, userID, domainsession.RevokeReasonLogoutAll, s.now()); err != nil {
		return fmt.Errorf("revoke all refresh tokens: %w", err)
	}
	return nil
}

func (s *Service) Me(ctx context.Context, userID uuid.UUID) (*domainuser.User, error) {
	return s.users.FindByID(ctx, userID)
}

func (s *Service) issueSession(ctx context.Context, u *domainuser.User, familyID uuid.UUID, ip, userAgent, deviceLabel string) (*AuthResult, error) {
	accessToken, jti, accessExpiresAt, err := s.tokens.Issue(u.ID.String())
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}
	if err := s.denylist.RegisterActive(ctx, u.ID, jti, accessExpiresAt); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDependencyUnavailable, err)
	}

	refreshToken, hash, err := s.refreshTokens.New()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	refreshExpiresAt := s.now().Add(s.refreshTTL)
	rt := &domainsession.RefreshToken{
		ID:          uuid.New(),
		UserID:      u.ID,
		FamilyID:    familyID,
		TokenHash:   hash,
		DeviceLabel: optionalString(deviceLabel),
		UserAgent:   optionalString(userAgent),
		IP:          parseIP(ip),
		ExpiresAt:   refreshExpiresAt,
	}
	if err := s.sessions.Create(ctx, rt); err != nil {
		return nil, fmt.Errorf("persist refresh token: %w", err)
	}

	return &AuthResult{
		User:                  *u,
		AccessToken:           accessToken,
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshToken:          refreshToken,
		RefreshTokenExpiresAt: refreshExpiresAt,
	}, nil
}

func (s *Service) checkRateLimit(ctx context.Context, scopes ...string) error {
	for _, scope := range scopes {
		allowed, retryAfter, err := s.rateLimiter.Allowed(ctx, scope)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrDependencyUnavailable, err)
		}
		if !allowed {
			return &RateLimitedError{RetryAfter: retryAfter}
		}
	}
	return nil
}

func (s *Service) recordFailure(ctx context.Context, scopes ...string) error {
	for _, scope := range scopes {
		if _, err := s.rateLimiter.RecordFailure(ctx, scope); err != nil {
			return fmt.Errorf("%w: %w", ErrDependencyUnavailable, err)
		}
	}
	return nil
}

func validatePassword(password, email string) error {
	if len(password) < minPasswordLength || len(password) > maxPasswordLength {
		return ErrWeakPassword
	}
	if strings.EqualFold(password, email) {
		return ErrWeakPassword
	}
	return nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func parseIP(value string) *netip.Addr {
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return nil
	}
	return &addr
}

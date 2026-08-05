package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	applicationauth "github.com/sx110903/llmatch-v2/backend/internal/application/auth"
	domainsession "github.com/sx110903/llmatch-v2/backend/internal/domain/session"
	domainuser "github.com/sx110903/llmatch-v2/backend/internal/domain/user"
)

// --- fake adapters -------------------------------------------------------

type fakeUserRepo struct {
	byID                     map[uuid.UUID]*domainuser.User
	byEmail                  map[string]uuid.UUID
	updatePasswordHashCalled bool
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byID: map[uuid.UUID]*domainuser.User{}, byEmail: map[string]uuid.UUID{}}
}

func (r *fakeUserRepo) Create(_ context.Context, u *domainuser.User) error {
	if _, exists := r.byEmail[u.Email]; exists {
		return domainuser.ErrEmailTaken
	}
	copied := *u
	r.byID[u.ID] = &copied
	r.byEmail[u.Email] = u.ID
	return nil
}

func (r *fakeUserRepo) FindByEmail(_ context.Context, email string) (*domainuser.User, error) {
	id, ok := r.byEmail[email]
	if !ok {
		return nil, domainuser.ErrNotFound
	}
	copied := *r.byID[id]
	return &copied, nil
}

func (r *fakeUserRepo) FindByID(_ context.Context, id uuid.UUID) (*domainuser.User, error) {
	found, ok := r.byID[id]
	if !ok {
		return nil, domainuser.ErrNotFound
	}
	copied := *found
	return &copied, nil
}

func (r *fakeUserRepo) UpdatePasswordHash(_ context.Context, id uuid.UUID, passwordHash string, changedAt time.Time) error {
	found, ok := r.byID[id]
	if !ok {
		return domainuser.ErrNotFound
	}
	found.PasswordHash = passwordHash
	found.PasswordChangedAt = changedAt
	r.updatePasswordHashCalled = true
	return nil
}

type fakeSessionRepo struct {
	byID                 map[uuid.UUID]*domainsession.RefreshToken
	byHash               map[string]uuid.UUID
	forceRevokeFamilyErr error
	forceRevokeAllErr    error
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{byID: map[uuid.UUID]*domainsession.RefreshToken{}, byHash: map[string]uuid.UUID{}}
}

func (r *fakeSessionRepo) Create(_ context.Context, token *domainsession.RefreshToken) error {
	copied := *token
	r.byID[token.ID] = &copied
	r.byHash[string(token.TokenHash)] = token.ID
	return nil
}

func (r *fakeSessionRepo) FindByHash(_ context.Context, tokenHash []byte) (*domainsession.RefreshToken, error) {
	id, ok := r.byHash[string(tokenHash)]
	if !ok {
		return nil, domainsession.ErrNotFound
	}
	copied := *r.byID[id]
	return &copied, nil
}

func (r *fakeSessionRepo) Rotate(_ context.Context, currentID uuid.UUID, next *domainsession.RefreshToken) error {
	current, ok := r.byID[currentID]
	if !ok {
		return domainsession.ErrNotFound
	}
	now := time.Now().UTC()
	if current.RevokedAt != nil || current.ReplacedBy != nil {
		for _, token := range r.byID {
			if token.FamilyID == current.FamilyID && token.RevokedAt == nil {
				revokedAt := now
				reason := domainsession.RevokeReasonReuseDetected
				token.RevokedAt = &revokedAt
				token.RevokeReason = &reason
			}
		}
		return domainsession.ErrReused
	}

	copied := *next
	r.byID[next.ID] = &copied
	r.byHash[string(next.TokenHash)] = next.ID

	replacedBy := next.ID
	revokedAt := now
	reason := domainsession.RevokeReasonRotated
	current.ReplacedBy = &replacedBy
	current.RevokedAt = &revokedAt
	current.RevokeReason = &reason
	current.LastUsedAt = &revokedAt
	return nil
}

func (r *fakeSessionRepo) RevokeFamily(_ context.Context, familyID uuid.UUID, reason string, at time.Time) error {
	if r.forceRevokeFamilyErr != nil {
		return r.forceRevokeFamilyErr
	}
	for _, token := range r.byID {
		if token.FamilyID == familyID && token.RevokedAt == nil {
			revokedAt := at
			token.RevokedAt = &revokedAt
			r := reason
			token.RevokeReason = &r
		}
	}
	return nil
}

func (r *fakeSessionRepo) RevokeAllForUser(_ context.Context, userID uuid.UUID, reason string, at time.Time) error {
	if r.forceRevokeAllErr != nil {
		return r.forceRevokeAllErr
	}
	for _, token := range r.byID {
		if token.UserID == userID && token.RevokedAt == nil {
			revokedAt := at
			token.RevokedAt = &revokedAt
			r := reason
			token.RevokeReason = &r
		}
	}
	return nil
}

type fakeHasher struct{ staleHashes map[string]bool }

func (fakeHasher) Hash(password string) (string, error) { return "hashed:" + password, nil }

func (fakeHasher) Verify(password, encodedHash string) (bool, error) {
	return "hashed:"+password == encodedHash, nil
}

func (h fakeHasher) NeedsRehash(encodedHash string) (bool, error) {
	return h.staleHashes[encodedHash], nil
}

type fakeIssuer struct{ counter int }

func (f *fakeIssuer) Issue(subject string) (string, string, time.Time, error) {
	f.counter++
	jti := uuid.NewString()
	return "access-" + subject, jti, time.Now().UTC().Add(15 * time.Minute), nil
}

type fakeRefreshGenerator struct{ counter int }

func (f *fakeRefreshGenerator) New() (string, []byte, error) {
	f.counter++
	token := uuid.NewString()
	return token, []byte(token), nil
}

func (fakeRefreshGenerator) Hash(token string) ([]byte, error) { return []byte(token), nil }

type fakeDenylist struct {
	active     map[uuid.UUID]map[string]time.Time
	denylisted map[string]bool
	forceErr   error
}

func newFakeDenylist() *fakeDenylist {
	return &fakeDenylist{active: map[uuid.UUID]map[string]time.Time{}, denylisted: map[string]bool{}}
}

func (d *fakeDenylist) RegisterActive(_ context.Context, userID uuid.UUID, jti string, expiresAt time.Time) error {
	if d.forceErr != nil {
		return d.forceErr
	}
	if d.active[userID] == nil {
		d.active[userID] = map[string]time.Time{}
	}
	d.active[userID][jti] = expiresAt
	return nil
}

func (d *fakeDenylist) Denylist(_ context.Context, jti string, _ time.Time) error {
	if d.forceErr != nil {
		return d.forceErr
	}
	d.denylisted[jti] = true
	return nil
}

func (d *fakeDenylist) DenylistAllActive(_ context.Context, userID uuid.UUID) error {
	if d.forceErr != nil {
		return d.forceErr
	}
	for jti := range d.active[userID] {
		d.denylisted[jti] = true
	}
	delete(d.active, userID)
	return nil
}

type fakeRateLimiter struct {
	locked          map[string]time.Duration
	failures        map[string]int
	forceErr        error
	forceErrOnReset error
}

func newFakeRateLimiter() *fakeRateLimiter {
	return &fakeRateLimiter{locked: map[string]time.Duration{}, failures: map[string]int{}}
}

func (l *fakeRateLimiter) Allowed(_ context.Context, scope string) (bool, time.Duration, error) {
	if l.forceErr != nil {
		return false, 0, l.forceErr
	}
	if retryAfter, locked := l.locked[scope]; locked {
		return false, retryAfter, nil
	}
	return true, 0, nil
}

func (l *fakeRateLimiter) RecordFailure(_ context.Context, scope string) (time.Duration, error) {
	if l.forceErr != nil {
		return 0, l.forceErr
	}
	l.failures[scope]++
	if l.failures[scope] >= 5 {
		l.locked[scope] = 5 * time.Minute
		return 5 * time.Minute, nil
	}
	return 0, nil
}

func (l *fakeRateLimiter) Reset(_ context.Context, scope string) error {
	if l.forceErr != nil {
		return l.forceErr
	}
	if l.forceErrOnReset != nil {
		return l.forceErrOnReset
	}
	delete(l.failures, scope)
	delete(l.locked, scope)
	return nil
}

// --- test harness ---------------------------------------------------------

type harness struct {
	service     *applicationauth.Service
	users       *fakeUserRepo
	sessions    *fakeSessionRepo
	hasher      fakeHasher
	issuer      *fakeIssuer
	refreshGen  *fakeRefreshGenerator
	denylist    *fakeDenylist
	rateLimiter *fakeRateLimiter
}

func newHarness() *harness {
	h := &harness{
		users:       newFakeUserRepo(),
		sessions:    newFakeSessionRepo(),
		hasher:      fakeHasher{staleHashes: map[string]bool{}},
		issuer:      &fakeIssuer{},
		refreshGen:  &fakeRefreshGenerator{},
		denylist:    newFakeDenylist(),
		rateLimiter: newFakeRateLimiter(),
	}
	h.service = applicationauth.NewService(
		h.users, h.sessions, h.hasher, h.issuer, h.refreshGen, h.denylist, h.rateLimiter,
		applicationauth.Config{RefreshTokenTTL: 30 * 24 * time.Hour},
	)
	return h
}

func validRegisterInput() applicationauth.RegisterInput {
	return applicationauth.RegisterInput{
		Email:       "person@example.com",
		Password:    "correct horse battery",
		DisplayName: "Person",
		BirthDate:   time.Now().AddDate(-25, 0, 0),
		Gender:      "woman",
		IP:          "203.0.113.10",
	}
}

// --- Register ---------------------------------------------------------

func TestRegisterSuccess(t *testing.T) {
	h := newHarness()
	created, err := h.service.Register(context.Background(), validRegisterInput())
	require.NoError(t, err)
	require.Equal(t, "person@example.com", created.Email)
	require.Equal(t, domainuser.StatusActive, created.Status)
	require.Nil(t, created.EmailVerifiedAt)
}

func TestRegisterRejectsWeakPassword(t *testing.T) {
	h := newHarness()
	in := validRegisterInput()
	in.Password = "short"
	_, err := h.service.Register(context.Background(), in)
	require.ErrorIs(t, err, applicationauth.ErrWeakPassword)
}

func TestRegisterRejectsUnderage(t *testing.T) {
	h := newHarness()
	in := validRegisterInput()
	in.BirthDate = time.Now().AddDate(-17, 0, 0)
	_, err := h.service.Register(context.Background(), in)
	require.ErrorIs(t, err, domainuser.ErrUnderage)
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	_, err := h.service.Register(ctx, validRegisterInput())
	require.NoError(t, err)

	_, err = h.service.Register(ctx, validRegisterInput())
	require.ErrorIs(t, err, domainuser.ErrEmailTaken)
}

func TestRegisterFailsClosedWhenRateLimiterUnavailable(t *testing.T) {
	h := newHarness()
	h.rateLimiter.forceErr = errors.New("redis down")
	_, err := h.service.Register(context.Background(), validRegisterInput())
	require.ErrorIs(t, err, applicationauth.ErrDependencyUnavailable)
}

// --- Login --------------------------------------------------------------

func registerUser(t *testing.T, h *harness) applicationauth.RegisterInput {
	t.Helper()
	in := validRegisterInput()
	_, err := h.service.Register(context.Background(), in)
	require.NoError(t, err)
	return in
}

func TestLoginSuccess(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)

	result, err := h.service.Login(context.Background(), applicationauth.LoginInput{
		Email: in.Email, Password: in.Password, IP: "203.0.113.10", UserAgent: "test-agent",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.AccessToken)
	require.NotEmpty(t, result.RefreshToken)
}

func TestLoginRejectsUnknownEmail(t *testing.T) {
	h := newHarness()
	_, err := h.service.Login(context.Background(), applicationauth.LoginInput{
		Email: "nobody@example.com", Password: "irrelevant password", IP: "203.0.113.10",
	})
	require.ErrorIs(t, err, applicationauth.ErrInvalidCredentials)
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	_, err := h.service.Login(context.Background(), applicationauth.LoginInput{
		Email: in.Email, Password: "wrong password entirely", IP: "203.0.113.10",
	})
	require.ErrorIs(t, err, applicationauth.ErrInvalidCredentials)
}

func TestLoginLocksOutAfterRepeatedFailures(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)

	var lastErr error
	for i := 0; i < 5; i++ {
		_, lastErr = h.service.Login(context.Background(), applicationauth.LoginInput{
			Email: in.Email, Password: "wrong password entirely", IP: "203.0.113.10",
		})
	}
	require.ErrorIs(t, lastErr, applicationauth.ErrInvalidCredentials)

	_, err := h.service.Login(context.Background(), applicationauth.LoginInput{
		Email: in.Email, Password: in.Password, IP: "203.0.113.10",
	})
	var rateLimited *applicationauth.RateLimitedError
	require.ErrorAs(t, err, &rateLimited)
	require.Contains(t, rateLimited.Error(), "rate limit exceeded")
}

func TestLoginFailsClosedWhenDenylistUnavailable(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	h.denylist.forceErr = errors.New("redis down")

	_, err := h.service.Login(context.Background(), applicationauth.LoginInput{
		Email: in.Email, Password: in.Password, IP: "203.0.113.10",
	})
	require.ErrorIs(t, err, applicationauth.ErrDependencyUnavailable)
}

func TestLoginSucceedsWithoutClientIP(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)

	result, err := h.service.Login(context.Background(), applicationauth.LoginInput{
		Email: in.Email, Password: in.Password, IP: "not-an-ip",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.AccessToken)
}

func TestLoginRehashesStalePasswordHash(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	h.hasher.staleHashes["hashed:"+in.Password] = true

	_, err := h.service.Login(context.Background(), applicationauth.LoginInput{
		Email: in.Email, Password: in.Password, IP: "203.0.113.10",
	})
	require.NoError(t, err)

	require.True(t, h.users.updatePasswordHashCalled)
}

func TestLoginFailsClosedWhenRateLimiterResetUnavailable(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	h.rateLimiter.forceErrOnReset = errors.New("redis down")

	_, err := h.service.Login(context.Background(), applicationauth.LoginInput{
		Email: in.Email, Password: in.Password, IP: "203.0.113.10",
	})
	require.ErrorIs(t, err, applicationauth.ErrDependencyUnavailable)
}

func TestLoginFailsClosedWhenRateLimiterUnavailable(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	h.rateLimiter.forceErr = errors.New("redis down")

	_, err := h.service.Login(context.Background(), applicationauth.LoginInput{
		Email: in.Email, Password: in.Password, IP: "203.0.113.10",
	})
	require.ErrorIs(t, err, applicationauth.ErrDependencyUnavailable)
}

// --- Refresh --------------------------------------------------------------

func loginUser(t *testing.T, h *harness, in applicationauth.RegisterInput) *applicationauth.AuthResult {
	t.Helper()
	result, err := h.service.Login(context.Background(), applicationauth.LoginInput{
		Email: in.Email, Password: in.Password, IP: "203.0.113.10", UserAgent: "test-agent",
	})
	require.NoError(t, err)
	return result
}

func TestRefreshRotatesToken(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	session := loginUser(t, h, in)

	rotated, err := h.service.Refresh(context.Background(), applicationauth.RefreshInput{
		RefreshToken: session.RefreshToken, IP: "203.0.113.10", UserAgent: "test-agent",
	})
	require.NoError(t, err)
	require.NotEqual(t, session.RefreshToken, rotated.RefreshToken)
	require.NotEmpty(t, rotated.AccessToken)
}

// TestRefreshReuseDetectionRevokesFamily is the mandatory rotation-reuse
// test: replaying an already-rotated refresh token must revoke the whole
// family, so even the most recently issued token in that family stops working.
func TestRefreshReuseDetectionRevokesFamily(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	session := loginUser(t, h, in)

	rotated, err := h.service.Refresh(context.Background(), applicationauth.RefreshInput{
		RefreshToken: session.RefreshToken, IP: "203.0.113.10", UserAgent: "test-agent",
	})
	require.NoError(t, err)

	// Replay the already-rotated (now stale) refresh token.
	_, err = h.service.Refresh(context.Background(), applicationauth.RefreshInput{
		RefreshToken: session.RefreshToken, IP: "198.51.100.1", UserAgent: "attacker-agent",
	})
	require.ErrorIs(t, err, applicationauth.ErrInvalidRefreshToken)

	// The legitimately rotated token must now be revoked too.
	_, err = h.service.Refresh(context.Background(), applicationauth.RefreshInput{
		RefreshToken: rotated.RefreshToken, IP: "203.0.113.10", UserAgent: "test-agent",
	})
	require.ErrorIs(t, err, applicationauth.ErrInvalidRefreshToken)
}

func TestRefreshRejectsExpiredToken(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	session := loginUser(t, h, in)

	hash, err := h.refreshGen.Hash(session.RefreshToken)
	require.NoError(t, err)
	stored, err := h.sessions.FindByHash(context.Background(), hash)
	require.NoError(t, err)
	expired := time.Now().Add(-time.Hour)
	h.sessions.byID[stored.ID].ExpiresAt = expired

	_, err = h.service.Refresh(context.Background(), applicationauth.RefreshInput{
		RefreshToken: session.RefreshToken, IP: "203.0.113.10",
	})
	require.ErrorIs(t, err, applicationauth.ErrInvalidRefreshToken)
}

func TestRefreshRejectsUnknownToken(t *testing.T) {
	h := newHarness()
	_, err := h.service.Refresh(context.Background(), applicationauth.RefreshInput{
		RefreshToken: "does-not-exist", IP: "203.0.113.10",
	})
	require.ErrorIs(t, err, applicationauth.ErrInvalidRefreshToken)
}

func TestRefreshRejectsWhenUserIsNoLongerActive(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	session := loginUser(t, h, in)
	h.users.byID[session.User.ID].Status = domainuser.StatusSuspended

	_, err := h.service.Refresh(context.Background(), applicationauth.RefreshInput{
		RefreshToken: session.RefreshToken, IP: "203.0.113.10",
	})
	require.ErrorIs(t, err, applicationauth.ErrInvalidRefreshToken)
}

func TestRefreshFailsClosedWhenDenylistUnavailable(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	session := loginUser(t, h, in)
	h.denylist.forceErr = errors.New("redis down")

	_, err := h.service.Refresh(context.Background(), applicationauth.RefreshInput{
		RefreshToken: session.RefreshToken, IP: "203.0.113.10",
	})
	require.ErrorIs(t, err, applicationauth.ErrDependencyUnavailable)
}

// --- Logout / LogoutAll ----------------------------------------------------

func TestLogoutDenylistsJTIAndRevokesFamily(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	session := loginUser(t, h, in)

	hash, err := h.refreshGen.Hash(session.RefreshToken)
	require.NoError(t, err)
	stored, err := h.sessions.FindByHash(context.Background(), hash)
	require.NoError(t, err)

	var jti string
	for id := range h.denylist.active[stored.UserID] {
		jti = id
	}
	require.NotEmpty(t, jti)

	err = h.service.Logout(context.Background(), stored.UserID, jti, time.Now().Add(15*time.Minute), session.RefreshToken)
	require.NoError(t, err)

	require.True(t, h.denylist.denylisted[jti])
	require.NotNil(t, h.sessions.byID[stored.ID].RevokedAt)

	_, err = h.service.Refresh(context.Background(), applicationauth.RefreshInput{
		RefreshToken: session.RefreshToken, IP: "203.0.113.10",
	})
	require.ErrorIs(t, err, applicationauth.ErrInvalidRefreshToken)
}

func TestLogoutFailsClosedWhenDenylistUnavailable(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	session := loginUser(t, h, in)
	h.denylist.forceErr = errors.New("redis down")

	err := h.service.Logout(context.Background(), uuid.New(), "jti", time.Now().Add(time.Minute), session.RefreshToken)
	require.ErrorIs(t, err, applicationauth.ErrDependencyUnavailable)
}

func TestLogoutAllRevokesEverySessionAndActiveJTI(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	first := loginUser(t, h, in)
	second, err := h.service.Login(context.Background(), applicationauth.LoginInput{
		Email: in.Email, Password: in.Password, IP: "198.51.100.2", UserAgent: "device-2",
	})
	require.NoError(t, err)

	err = h.service.LogoutAll(context.Background(), first.User.ID)
	require.NoError(t, err)

	_, err = h.service.Refresh(context.Background(), applicationauth.RefreshInput{RefreshToken: first.RefreshToken, IP: "203.0.113.10"})
	require.ErrorIs(t, err, applicationauth.ErrInvalidRefreshToken)
	_, err = h.service.Refresh(context.Background(), applicationauth.RefreshInput{RefreshToken: second.RefreshToken, IP: "198.51.100.2"})
	require.ErrorIs(t, err, applicationauth.ErrInvalidRefreshToken)
}

func TestLogoutAllFailsClosedWhenDenylistUnavailable(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	session := loginUser(t, h, in)
	h.denylist.forceErr = errors.New("redis down")

	err := h.service.LogoutAll(context.Background(), session.User.ID)
	require.ErrorIs(t, err, applicationauth.ErrDependencyUnavailable)
}

func TestLogoutAllPropagatesSessionRevocationFailure(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	session := loginUser(t, h, in)
	h.sessions.forceRevokeAllErr = errors.New("database down")

	err := h.service.LogoutAll(context.Background(), session.User.ID)
	require.Error(t, err)
	require.NotErrorIs(t, err, applicationauth.ErrDependencyUnavailable)
}

func TestLogoutPropagatesSessionRevocationFailure(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	session := loginUser(t, h, in)
	h.sessions.forceRevokeFamilyErr = errors.New("database down")

	var jti string
	for id := range h.denylist.active[session.User.ID] {
		jti = id
	}
	require.NotEmpty(t, jti)

	err := h.service.Logout(context.Background(), session.User.ID, jti, time.Now().Add(time.Minute), session.RefreshToken)
	require.Error(t, err)
}

// --- Me ---------------------------------------------------------------

func TestMeReturnsUser(t *testing.T) {
	h := newHarness()
	in := registerUser(t, h)
	session := loginUser(t, h, in)

	found, err := h.service.Me(context.Background(), session.User.ID)
	require.NoError(t, err)
	require.Equal(t, in.Email, found.Email)
}

func TestMeReturnsNotFoundForUnknownUser(t *testing.T) {
	h := newHarness()
	_, err := h.service.Me(context.Background(), uuid.New())
	require.ErrorIs(t, err, domainuser.ErrNotFound)
}

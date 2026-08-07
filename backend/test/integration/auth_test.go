package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	redisclient "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	httpadapter "github.com/sx110903/llmatch-v2/backend/internal/adapters/http"
	httpaccount "github.com/sx110903/llmatch-v2/backend/internal/adapters/http/account"
	httpauth "github.com/sx110903/llmatch-v2/backend/internal/adapters/http/auth"
	httphealth "github.com/sx110903/llmatch-v2/backend/internal/adapters/http/health"
	httpmatching "github.com/sx110903/llmatch-v2/backend/internal/adapters/http/matching"
	httpmessaging "github.com/sx110903/llmatch-v2/backend/internal/adapters/http/messaging"
	httpprofile "github.com/sx110903/llmatch-v2/backend/internal/adapters/http/profile"
	"github.com/sx110903/llmatch-v2/backend/internal/adapters/postgres"
	"github.com/sx110903/llmatch-v2/backend/internal/adapters/postgres/repositories"
	redisadapter "github.com/sx110903/llmatch-v2/backend/internal/adapters/redis"
	"github.com/sx110903/llmatch-v2/backend/internal/adapters/storage"
	websocketadapter "github.com/sx110903/llmatch-v2/backend/internal/adapters/websocket"
	applicationaccount "github.com/sx110903/llmatch-v2/backend/internal/application/account"
	applicationauth "github.com/sx110903/llmatch-v2/backend/internal/application/auth"
	applicationhealth "github.com/sx110903/llmatch-v2/backend/internal/application/health"
	applicationmatching "github.com/sx110903/llmatch-v2/backend/internal/application/matching"
	applicationmessaging "github.com/sx110903/llmatch-v2/backend/internal/application/messaging"
	applicationprofile "github.com/sx110903/llmatch-v2/backend/internal/application/profile"
	domainmatching "github.com/sx110903/llmatch-v2/backend/internal/domain/matching"
	platformcrypto "github.com/sx110903/llmatch-v2/backend/internal/platform/crypto"
	platformmiddleware "github.com/sx110903/llmatch-v2/backend/internal/platform/middleware"
)

const testPassword = "correct horse battery staple"

var emailCounter int64

func uniqueEmail() string {
	return fmt.Sprintf("user-%d-%d@example.com", time.Now().UnixNano(), atomic.AddInt64(&emailCounter, 1))
}

func startRedis(t *testing.T) *redisclient.Client {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:8.4.5-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	connString, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	client, err := redisadapter.NewClient(connString)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func testKeyPair(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key, &key.PublicKey
}

// newTestServer wires the same components as cmd/api/main.go against real
// Postgres and Redis so integration tests exercise the actual adapters, not
// doubles.
func newTestServer(t *testing.T, pool *pgxpool.Pool, redisClient *redisclient.Client, production bool, allowedOrigins []string) *httptest.Server {
	server, _ := newTestServerWithMessaging(t, pool, redisClient, production, allowedOrigins, 100)
	return server
}

func newTestServerWithMatchingLimit(
	t *testing.T,
	pool *pgxpool.Pool,
	redisClient *redisclient.Client,
	production bool,
	allowedOrigins []string,
	dailySwipeLimit int,
) *httptest.Server {
	server, _ := newTestServerWithMessaging(t, pool, redisClient, production, allowedOrigins, dailySwipeLimit)
	return server
}

// newTestServerWithMessaging also returns the websocket handler so a test can
// drive cross-instance fan-out without opening a real socket.
func newTestServerWithMessaging(
	t *testing.T,
	pool *pgxpool.Pool,
	redisClient *redisclient.Client,
	production bool,
	allowedOrigins []string,
	dailySwipeLimit int,
) (*httptest.Server, *websocketadapter.Handler) {
	t.Helper()
	privateKey, publicKey := testKeyPair(t)
	tokenIssuer := platformcrypto.NewTokenIssuer(privateKey, publicKey, "llmatch-v2-test", "llmatch-v2-test-clients", 15*time.Minute)
	denylist := redisadapter.NewTokenDenylist(redisClient)

	authService := applicationauth.NewService(
		repositories.NewUserRepository(pool),
		repositories.NewSessionRepository(pool),
		platformcrypto.Argon2idHasher{},
		tokenIssuer,
		platformcrypto.OpaqueTokenGenerator{},
		denylist,
		redisadapter.NewRateLimiter(redisClient),
		applicationauth.Config{RefreshTokenTTL: 30 * 24 * time.Hour},
	)

	authHandler := httpauth.NewHandler(authService, production, "/api/v1/auth", allowedOrigins)
	authMiddleware := platformmiddleware.Auth(tokenIssuer, denylist, 300*time.Millisecond)
	healthService := applicationhealth.NewService(2*time.Second, postgres.Checker{Pool: pool}, redisadapter.Checker{Client: redisClient})

	photoStorage := storage.NewLocalStorage(t.TempDir())
	consentRepository := repositories.NewConsentRepository(pool)
	privacyService := applicationaccount.NewPrivacyService(consentRepository)
	profileService := applicationprofile.NewService(repositories.NewProfileRepository(pool), photoStorage, privacyService)
	matchingService := applicationmatching.NewService(
		repositories.NewMatchingRepository(pool),
		redisadapter.NewSwipeLimiter(redisClient),
		photoStorage,
		applicationmatching.Config{
			DailySwipeLimit: dailySwipeLimit,
			RankingWeights: domainmatching.RankingWeights{
				Interests: 0.35, Questionnaire: 0.30, Distance: 0.20, Activity: 0.15,
			},
		},
	)

	messageBus := redisadapter.NewMessageBus(redisClient)
	messagingService := applicationmessaging.NewService(
		repositories.NewMessagingRepository(pool),
		redisadapter.NewWSTicketStore(redisClient),
		messageBus,
		redisadapter.NewMessageRateLimiter(redisClient, time.Minute, 10_000),
		applicationmessaging.Config{TicketTTL: 30 * time.Second},
	)
	websocketHandler := websocketadapter.NewHandler(
		messagingService, websocketadapter.NewHub(), zerolog.Nop(), allowedOrigins,
		websocketadapter.Config{
			ReadLimitBytes: 32 * 1024, QueueSize: 64, WriteTimeout: 5 * time.Second,
			PingInterval: 25 * time.Second, ReadTimeout: time.Minute, ClientEventsPerMinute: 120,
		},
	)

	router := httpadapter.NewRouter(httpadapter.RouterConfig{
		Logger:         zerolog.Nop(),
		AllowedOrigins: allowedOrigins,
		Production:     production,
		Health:         httphealth.NewHandler(healthService),
		Auth:           authHandler,
		Profile:        httpprofile.NewHandler(profileService),
		Account:        httpaccount.NewHandler(privacyService),
		Matching:       httpmatching.NewHandler(matchingService),
		Messaging:      httpmessaging.NewHandler(messagingService),
		WebSocket:      websocketHandler,
		AuthMiddleware: authMiddleware,
	})

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server, websocketHandler
}

func registerUser(t *testing.T, server *httptest.Server, email, password string) {
	registerUserWithDetails(t, server, email, password, "Test User", "woman")
}

func registerUserWithDetails(
	t *testing.T,
	server *httptest.Server,
	email, password, displayName, gender string,
) {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"email":        email,
		"password":     password,
		"display_name": displayName,
		"birth_date":   "1995-06-15",
		"gender":       gender,
	})
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/api/v1/auth/register", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
}

func doLogin(t *testing.T, server *httptest.Server, email, password string) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]string{"email": email, "password": password})
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/api/v1/auth/login", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	return resp
}

func loginAndGetRefreshCookie(t *testing.T, server *httptest.Server, email, password string) *http.Cookie {
	t.Helper()
	resp := doLogin(t, server, email, password)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "refresh_token" {
			return cookie
		}
	}
	t.Fatal("login response did not set a refresh_token cookie")
	return nil
}

// TestLoginSetsSecureRefreshCookie proves the refresh cookie carries every
// attribute the plan mandates: HttpOnly, Secure in production, SameSite=Strict
// and scoped to the auth path only.
func TestLoginSetsSecureRefreshCookie(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, true, []string{"https://app.example.com"})

	email := uniqueEmail()
	registerUser(t, server, email, testPassword)
	cookie := loginAndGetRefreshCookie(t, server, email, testPassword)

	require.True(t, cookie.HttpOnly)
	require.True(t, cookie.Secure)
	require.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
	require.Equal(t, "/api/v1/auth", cookie.Path)
	require.NotEmpty(t, cookie.Value)
}

// TestRefreshTokenRotationRaceHasExactlyOneWinner fires many concurrent
// refresh attempts at the same token to prove the Postgres row lock in
// SessionRepository.Rotate serializes them: only the first to grab the lock
// rotates successfully, every other concurrent attempt is rejected.
func TestRefreshTokenRotationRaceHasExactlyOneWinner(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})

	email := uniqueEmail()
	registerUser(t, server, email, testPassword)
	refreshCookie := loginAndGetRefreshCookie(t, server, email, testPassword)

	const attempts = 8
	statusCodes := make([]int, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/api/v1/auth/refresh", nil)
			if err != nil {
				return
			}
			req.AddCookie(refreshCookie)
			resp, err := server.Client().Do(req)
			if err != nil {
				return
			}
			defer func() { _ = resp.Body.Close() }()
			statusCodes[index] = resp.StatusCode
		}(i)
	}
	wg.Wait()

	successCount := 0
	for _, code := range statusCodes {
		if code == http.StatusOK {
			successCount++
		}
	}
	require.Equal(t, 1, successCount, "exactly one concurrent refresh attempt must win the rotation race")
}

// TestLoginRateLimitIsSharedAcrossServerInstances proves rate limiting state
// lives in Redis, not in a single process: a lockout triggered through one
// server instance is enforced by a second instance sharing the same Redis.
func TestLoginRateLimitIsSharedAcrossServerInstances(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	instanceA := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})
	instanceB := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})

	email := uniqueEmail()
	registerUser(t, instanceA, email, testPassword)

	for i := 0; i < 5; i++ {
		resp := doLogin(t, instanceA, email, "wrong password entirely")
		_ = resp.Body.Close()
	}

	resp := doLogin(t, instanceB, email, testPassword)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	require.NotEmpty(t, resp.Header.Get("Retry-After"))
}

// TestAuthenticatedFlowEndToEnd exercises register, login, an authenticated
// me call, logout and confirms the access token is rejected afterwards.
func TestAuthenticatedFlowEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	redisClient := startRedis(t)
	server := newTestServer(t, pool, redisClient, false, []string{"http://localhost:5173"})

	email := uniqueEmail()
	registerUser(t, server, email, testPassword)

	loginResp := doLogin(t, server, email, testPassword)
	defer func() { _ = loginResp.Body.Close() }()
	require.Equal(t, http.StatusOK, loginResp.StatusCode)

	var loginBody struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(loginResp.Body).Decode(&loginBody))
	require.NotEmpty(t, loginBody.AccessToken)

	meReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/v1/auth/me", nil)
	require.NoError(t, err)
	meReq.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
	meResp, err := server.Client().Do(meReq)
	require.NoError(t, err)
	defer func() { _ = meResp.Body.Close() }()
	require.Equal(t, http.StatusOK, meResp.StatusCode)

	logoutReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/api/v1/auth/logout", nil)
	require.NoError(t, err)
	logoutReq.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
	logoutResp, err := server.Client().Do(logoutReq)
	require.NoError(t, err)
	defer func() { _ = logoutResp.Body.Close() }()
	require.Equal(t, http.StatusNoContent, logoutResp.StatusCode)

	meAgainReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/v1/auth/me", nil)
	require.NoError(t, err)
	meAgainReq.Header.Set("Authorization", "Bearer "+loginBody.AccessToken)
	meAgainResp, err := server.Client().Do(meAgainReq)
	require.NoError(t, err)
	defer func() { _ = meAgainResp.Body.Close() }()
	require.Equal(t, http.StatusUnauthorized, meAgainResp.StatusCode)
}

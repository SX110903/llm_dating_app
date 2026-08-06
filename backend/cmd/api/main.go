package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpadapter "github.com/sx110903/llmatch-v2/backend/internal/adapters/http"
	httpaccount "github.com/sx110903/llmatch-v2/backend/internal/adapters/http/account"
	httpauth "github.com/sx110903/llmatch-v2/backend/internal/adapters/http/auth"
	httphealth "github.com/sx110903/llmatch-v2/backend/internal/adapters/http/health"
	httpmatching "github.com/sx110903/llmatch-v2/backend/internal/adapters/http/matching"
	httpprofile "github.com/sx110903/llmatch-v2/backend/internal/adapters/http/profile"
	"github.com/sx110903/llmatch-v2/backend/internal/adapters/postgres"
	"github.com/sx110903/llmatch-v2/backend/internal/adapters/postgres/repositories"
	redisadapter "github.com/sx110903/llmatch-v2/backend/internal/adapters/redis"
	"github.com/sx110903/llmatch-v2/backend/internal/adapters/storage"
	applicationaccount "github.com/sx110903/llmatch-v2/backend/internal/application/account"
	applicationauth "github.com/sx110903/llmatch-v2/backend/internal/application/auth"
	applicationhealth "github.com/sx110903/llmatch-v2/backend/internal/application/health"
	applicationmatching "github.com/sx110903/llmatch-v2/backend/internal/application/matching"
	applicationprofile "github.com/sx110903/llmatch-v2/backend/internal/application/profile"
	domainmatching "github.com/sx110903/llmatch-v2/backend/internal/domain/matching"
	"github.com/sx110903/llmatch-v2/backend/internal/platform/config"
	platformcrypto "github.com/sx110903/llmatch-v2/backend/internal/platform/crypto"
	platformlogger "github.com/sx110903/llmatch-v2/backend/internal/platform/logger"
	platformmiddleware "github.com/sx110903/llmatch-v2/backend/internal/platform/middleware"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "llmatch-api: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load secure configuration: %w", err)
	}
	logger, err := platformlogger.New(os.Stdout, cfg.LogLevel)
	if err != nil {
		return err
	}

	startupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	postgresPool, err := postgres.NewPool(startupContext, cfg.DatabaseURL, cfg.DatabaseMinConns, cfg.DatabaseMaxConns)
	if err != nil {
		return err
	}
	defer postgresPool.Close()

	redisClient, err := redisadapter.NewClient(cfg.RedisURL)
	if err != nil {
		return err
	}
	defer func() { _ = redisClient.Close() }()

	privateKey, err := platformcrypto.LoadRSAPrivateKey(cfg.JWTPrivateKey)
	if err != nil {
		return err
	}
	publicKey, err := platformcrypto.LoadRSAPublicKey(cfg.JWTPublicKey)
	if err != nil {
		return err
	}
	tokenIssuer := platformcrypto.NewTokenIssuer(privateKey, publicKey, cfg.JWTIssuer, cfg.JWTAudience, cfg.AccessTokenTTL)
	denylist := redisadapter.NewTokenDenylist(redisClient)

	authService := applicationauth.NewService(
		repositories.NewUserRepository(postgresPool),
		repositories.NewSessionRepository(postgresPool),
		platformcrypto.Argon2idHasher{},
		tokenIssuer,
		platformcrypto.OpaqueTokenGenerator{},
		denylist,
		redisadapter.NewRateLimiter(redisClient),
		applicationauth.Config{RefreshTokenTTL: cfg.RefreshTokenTTL},
	)

	const authCookiePath = "/api/v1/auth"
	authHandler := httpauth.NewHandler(authService, cfg.IsProduction(), authCookiePath, cfg.AllowedOrigins)
	authMiddleware := platformmiddleware.Auth(tokenIssuer, denylist, cfg.AuthCheckTimeout)

	photoStorage, err := storage.New(startupContext, storage.Config{
		Driver:       storage.Driver(cfg.PhotoStorageDriver),
		LocalBaseDir: cfg.PhotoStorageLocalDir,
		S3Bucket:     cfg.PhotoStorageS3Bucket,
		S3Region:     cfg.PhotoStorageS3Region,
		S3Endpoint:   cfg.PhotoStorageS3Endpoint,
		S3PathStyle:  cfg.PhotoStorageS3PathStyle,
	})
	if err != nil {
		return err
	}
	consentRepository := repositories.NewConsentRepository(postgresPool)
	privacyService := applicationaccount.NewPrivacyService(consentRepository)
	profileService := applicationprofile.NewService(
		repositories.NewProfileRepository(postgresPool),
		photoStorage,
		privacyService,
	)
	matchingService := applicationmatching.NewService(
		repositories.NewMatchingRepository(postgresPool),
		redisadapter.NewSwipeLimiter(redisClient),
		photoStorage,
		applicationmatching.Config{
			DailySwipeLimit: cfg.MatchingDailySwipeLimit,
			RankingWeights: domainmatching.RankingWeights{
				Interests:     cfg.MatchingWeightInterests,
				Questionnaire: cfg.MatchingWeightQuestionnaire,
				Distance:      cfg.MatchingWeightDistance,
				Activity:      cfg.MatchingWeightActivity,
			},
		},
	)

	healthService := applicationhealth.NewService(cfg.ReadinessTimeout,
		postgres.Checker{Pool: postgresPool},
		redisadapter.Checker{Client: redisClient},
	)
	router := httpadapter.NewRouter(httpadapter.RouterConfig{
		Logger:         logger,
		AllowedOrigins: cfg.AllowedOrigins,
		Production:     cfg.IsProduction(),
		Health:         httphealth.NewHandler(healthService),
		Auth:           authHandler,
		Profile:        httpprofile.NewHandler(profileService),
		Account:        httpaccount.NewHandler(privacyService),
		Matching:       httpmatching.NewHandler(matchingService),
		AuthMiddleware: authMiddleware,
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info().Str("address", cfg.HTTPAddress).Msg("HTTP server starting")
		serverErrors <- server.ListenAndServe()
	}()

	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-signalContext.Done():
		logger.Info().Msg("shutdown signal received")
	case serverError := <-serverErrors:
		if !errors.Is(serverError, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", serverError)
		}
	}

	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info().Msg("HTTP server stopped")
	return nil
}

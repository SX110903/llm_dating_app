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
	httphealth "github.com/sx110903/llmatch-v2/backend/internal/adapters/http/health"
	"github.com/sx110903/llmatch-v2/backend/internal/adapters/postgres"
	redisadapter "github.com/sx110903/llmatch-v2/backend/internal/adapters/redis"
	applicationhealth "github.com/sx110903/llmatch-v2/backend/internal/application/health"
	"github.com/sx110903/llmatch-v2/backend/internal/platform/config"
	platformlogger "github.com/sx110903/llmatch-v2/backend/internal/platform/logger"
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

	healthService := applicationhealth.NewService(cfg.ReadinessTimeout,
		postgres.Checker{Pool: postgresPool},
		redisadapter.Checker{Client: redisClient},
	)
	router := httpadapter.NewRouter(httpadapter.RouterConfig{
		Logger:         logger,
		AllowedOrigins: cfg.AllowedOrigins,
		Production:     cfg.IsProduction(),
		Health:         httphealth.NewHandler(healthService),
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

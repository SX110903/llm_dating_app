package config

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
)

type Environment string

const (
	Development Environment = "development"
	Test        Environment = "test"
	Production  Environment = "production"
)

type Config struct {
	Environment      Environment
	HTTPAddress      string
	LogLevel         string
	DatabaseURL      string
	DatabaseMinConns int32
	DatabaseMaxConns int32
	RedisURL         string
	JWTPrivateKey    string
	JWTPublicKey     string
	JWTIssuer        string
	JWTAudience      string
	AccessTokenTTL   time.Duration
	RefreshTokenTTL  time.Duration
	AuthCheckTimeout time.Duration
	AllowedOrigins   []string
	ReadinessTimeout time.Duration
	ShutdownTimeout  time.Duration

	PhotoStorageDriver      string
	PhotoStorageLocalDir    string
	PhotoStorageS3Bucket    string
	PhotoStorageS3Region    string
	PhotoStorageS3Endpoint  string
	PhotoStorageS3PathStyle bool

	MatchingDailySwipeLimit     int
	MatchingWeightInterests     float64
	MatchingWeightQuestionnaire float64
	MatchingWeightDistance      float64
	MatchingWeightActivity      float64

	MessagingTicketTTL             time.Duration
	MessagingRateWindow            time.Duration
	MessagingRateLimit             int
	MessagingSocketQueueSize       int
	MessagingSocketReadLimitBytes  int64
	MessagingSocketWriteTimeout    time.Duration
	MessagingSocketPingInterval    time.Duration
	MessagingSocketReadTimeout     time.Duration
	MessagingClientEventsPerMinute int
}

type rawConfig struct {
	Environment    string `validate:"required,oneof=development test production"`
	HTTPAddress    string `validate:"required"`
	LogLevel       string `validate:"required,oneof=trace debug info warn error"`
	DatabaseURL    string `validate:"required,url"`
	RedisURL       string `validate:"required,url"`
	JWTPrivateKey  string `validate:"required,file"`
	JWTPublicKey   string `validate:"required,file"`
	AllowedOrigins string `validate:"required"`
}

func Load() (Config, error) {
	raw := rawConfig{
		Environment:    strings.TrimSpace(os.Getenv("APP_ENV")),
		HTTPAddress:    getEnv("HTTP_ADDR", ":8080"),
		LogLevel:       getEnv("LOG_LEVEL", "info"),
		DatabaseURL:    strings.TrimSpace(os.Getenv("DATABASE_URL")),
		RedisURL:       strings.TrimSpace(os.Getenv("REDIS_URL")),
		JWTPrivateKey:  strings.TrimSpace(os.Getenv("JWT_PRIVATE_KEY_FILE")),
		JWTPublicKey:   strings.TrimSpace(os.Getenv("JWT_PUBLIC_KEY_FILE")),
		AllowedOrigins: strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS")),
	}
	if missing := missingRequiredVariables(raw); len(missing) > 0 {
		return Config{}, fmt.Errorf("required environment variables are missing: %s", strings.Join(missing, ", "))
	}

	if err := validator.New(validator.WithRequiredStructEnabled()).Struct(raw); err != nil {
		return Config{}, fmt.Errorf("invalid environment configuration: %w", err)
	}

	minConnections, err := int32Env("DB_MIN_CONNS", 2, 0, 100)
	if err != nil {
		return Config{}, err
	}
	maxConnections, err := int32Env("DB_MAX_CONNS", 10, 1, 200)
	if err != nil {
		return Config{}, err
	}
	if minConnections > maxConnections {
		return Config{}, errors.New("DB_MIN_CONNS must not exceed DB_MAX_CONNS")
	}

	readinessTimeout, err := durationEnv("READINESS_TIMEOUT", 2*time.Second)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := durationEnv("SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	accessTokenTTL, err := durationEnv("ACCESS_TOKEN_TTL", 15*time.Minute)
	if err != nil {
		return Config{}, err
	}
	refreshTokenTTL, err := durationEnv("REFRESH_TOKEN_TTL", 30*24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	authCheckTimeout, err := durationEnv("AUTH_CHECK_TIMEOUT", 300*time.Millisecond)
	if err != nil {
		return Config{}, err
	}
	photoStoragePathStyle, err := boolEnv("PHOTO_STORAGE_S3_PATH_STYLE", false)
	if err != nil {
		return Config{}, err
	}
	dailySwipeLimit, err := int32Env("MATCHING_DAILY_SWIPE_LIMIT", 100, 1, 10000)
	if err != nil {
		return Config{}, err
	}
	matchingWeightInterests, err := float64Env("MATCHING_WEIGHT_INTERESTS", 0.35, 0, 1)
	if err != nil {
		return Config{}, err
	}
	matchingWeightQuestionnaire, err := float64Env("MATCHING_WEIGHT_QUESTIONNAIRE", 0.30, 0, 1)
	if err != nil {
		return Config{}, err
	}
	matchingWeightDistance, err := float64Env("MATCHING_WEIGHT_DISTANCE", 0.20, 0, 1)
	if err != nil {
		return Config{}, err
	}
	matchingWeightActivity, err := float64Env("MATCHING_WEIGHT_ACTIVITY", 0.15, 0, 1)
	if err != nil {
		return Config{}, err
	}
	if matchingWeightInterests+matchingWeightQuestionnaire+matchingWeightDistance+matchingWeightActivity <= 0 {
		return Config{}, errors.New("at least one MATCHING_WEIGHT_* value must be greater than zero")
	}

	// The ticket TTL is deliberately short: it only has to survive the round
	// trip between issuing it and opening the socket.
	messagingTicketTTL, err := durationEnv("MESSAGING_TICKET_TTL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	messagingRateWindow, err := durationEnv("MESSAGING_RATE_WINDOW", time.Minute)
	if err != nil {
		return Config{}, err
	}
	messagingRateLimit, err := int32Env("MESSAGING_RATE_LIMIT", 60, 1, 10_000)
	if err != nil {
		return Config{}, err
	}
	messagingQueueSize, err := int32Env("MESSAGING_SOCKET_QUEUE_SIZE", 64, 1, 10_000)
	if err != nil {
		return Config{}, err
	}
	messagingReadLimit, err := int32Env("MESSAGING_SOCKET_READ_LIMIT_BYTES", 32*1024, 1024, 1024*1024)
	if err != nil {
		return Config{}, err
	}
	messagingWriteTimeout, err := durationEnv("MESSAGING_SOCKET_WRITE_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	messagingPingInterval, err := durationEnv("MESSAGING_SOCKET_PING_INTERVAL", 25*time.Second)
	if err != nil {
		return Config{}, err
	}
	messagingReadTimeout, err := durationEnv("MESSAGING_SOCKET_READ_TIMEOUT", 5*time.Minute)
	if err != nil {
		return Config{}, err
	}
	messagingClientEvents, err := int32Env("MESSAGING_CLIENT_EVENTS_PER_MINUTE", 120, 1, 10_000)
	if err != nil {
		return Config{}, err
	}
	if messagingPingInterval >= messagingReadTimeout {
		return Config{}, errors.New("MESSAGING_SOCKET_PING_INTERVAL must be shorter than MESSAGING_SOCKET_READ_TIMEOUT")
	}

	config := Config{
		Environment:      Environment(raw.Environment),
		HTTPAddress:      raw.HTTPAddress,
		LogLevel:         raw.LogLevel,
		DatabaseURL:      raw.DatabaseURL,
		DatabaseMinConns: minConnections,
		DatabaseMaxConns: maxConnections,
		RedisURL:         raw.RedisURL,
		JWTPrivateKey:    raw.JWTPrivateKey,
		JWTPublicKey:     raw.JWTPublicKey,
		JWTIssuer:        getEnv("JWT_ISSUER", "llmatch-v2"),
		JWTAudience:      getEnv("JWT_AUDIENCE", "llmatch-v2-clients"),
		AccessTokenTTL:   accessTokenTTL,
		RefreshTokenTTL:  refreshTokenTTL,
		AuthCheckTimeout: authCheckTimeout,
		AllowedOrigins:   splitAndTrim(raw.AllowedOrigins),
		ReadinessTimeout: readinessTimeout,
		ShutdownTimeout:  shutdownTimeout,

		PhotoStorageDriver:      getEnv("PHOTO_STORAGE_DRIVER", "local"),
		PhotoStorageLocalDir:    getEnv("PHOTO_STORAGE_LOCAL_DIR", "./data/photos"),
		PhotoStorageS3Bucket:    strings.TrimSpace(os.Getenv("PHOTO_STORAGE_S3_BUCKET")),
		PhotoStorageS3Region:    strings.TrimSpace(os.Getenv("PHOTO_STORAGE_S3_REGION")),
		PhotoStorageS3Endpoint:  strings.TrimSpace(os.Getenv("PHOTO_STORAGE_S3_ENDPOINT")),
		PhotoStorageS3PathStyle: photoStoragePathStyle,

		MatchingDailySwipeLimit:     int(dailySwipeLimit),
		MatchingWeightInterests:     matchingWeightInterests,
		MatchingWeightQuestionnaire: matchingWeightQuestionnaire,
		MatchingWeightDistance:      matchingWeightDistance,
		MatchingWeightActivity:      matchingWeightActivity,

		MessagingTicketTTL:             messagingTicketTTL,
		MessagingRateWindow:            messagingRateWindow,
		MessagingRateLimit:             int(messagingRateLimit),
		MessagingSocketQueueSize:       int(messagingQueueSize),
		MessagingSocketReadLimitBytes:  int64(messagingReadLimit),
		MessagingSocketWriteTimeout:    messagingWriteTimeout,
		MessagingSocketPingInterval:    messagingPingInterval,
		MessagingSocketReadTimeout:     messagingReadTimeout,
		MessagingClientEventsPerMinute: int(messagingClientEvents),
	}

	if err := validateURLs(config); err != nil {
		return Config{}, err
	}
	if err := validateOrigins(config); err != nil {
		return Config{}, err
	}
	if err := validateRSAKeyPair(config.JWTPrivateKey, config.JWTPublicKey, config.Environment == Production); err != nil {
		return Config{}, err
	}
	if err := validatePhotoStorage(config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func missingRequiredVariables(raw rawConfig) []string {
	variables := []struct {
		name  string
		value string
	}{
		{name: "APP_ENV", value: raw.Environment},
		{name: "DATABASE_URL", value: raw.DatabaseURL},
		{name: "REDIS_URL", value: raw.RedisURL},
		{name: "JWT_PRIVATE_KEY_FILE", value: raw.JWTPrivateKey},
		{name: "JWT_PUBLIC_KEY_FILE", value: raw.JWTPublicKey},
		{name: "CORS_ALLOWED_ORIGINS", value: raw.AllowedOrigins},
	}
	missing := make([]string, 0)
	for _, variable := range variables {
		if variable.value == "" {
			missing = append(missing, variable.name)
		}
	}
	return missing
}

func (c Config) IsProduction() bool { return c.Environment == Production }

func validateURLs(config Config) error {
	databaseURL, err := url.Parse(config.DatabaseURL)
	if err != nil || (databaseURL.Scheme != "postgres" && databaseURL.Scheme != "postgresql") {
		return errors.New("DATABASE_URL must be a valid postgres URL")
	}
	if databaseURL.User == nil {
		return errors.New("DATABASE_URL must include a least-privilege user")
	}
	username := strings.ToLower(databaseURL.User.Username())
	if username == "postgres" || strings.Contains(username, "admin") || strings.Contains(username, "migrator") {
		return errors.New("DATABASE_URL must not use a superuser or migration role")
	}
	password, hasPassword := databaseURL.User.Password()
	if !hasPassword || password == "" {
		return errors.New("DATABASE_URL must include a password")
	}

	redisURL, err := url.Parse(config.RedisURL)
	if err != nil || (redisURL.Scheme != "redis" && redisURL.Scheme != "rediss") {
		return errors.New("REDIS_URL must be a valid redis or rediss URL")
	}
	redisPassword := ""
	if redisURL.User != nil {
		redisPassword, _ = redisURL.User.Password()
	}
	if redisPassword == "" {
		return errors.New("REDIS_URL must include a password")
	}

	if config.IsProduction() {
		if len(password) < 16 || len(redisPassword) < 16 {
			return errors.New("production database and redis passwords must contain at least 16 characters")
		}
		if databaseURL.Query().Get("sslmode") != "verify-full" {
			return errors.New("production DATABASE_URL must use sslmode=verify-full")
		}
		if redisURL.Scheme != "rediss" {
			return errors.New("production REDIS_URL must use TLS (rediss)")
		}
	}
	return nil
}

func validateOrigins(config Config) error {
	if len(config.AllowedOrigins) == 0 {
		return errors.New("CORS_ALLOWED_ORIGINS must contain at least one origin")
	}
	for _, origin := range config.AllowedOrigins {
		if origin == "*" {
			return errors.New("wildcard CORS origins are not allowed")
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("invalid CORS origin %q", origin)
		}
		if config.IsProduction() && parsed.Scheme != "https" {
			return errors.New("production CORS origins must use HTTPS")
		}
	}
	return nil
}

func validatePhotoStorage(config Config) error {
	switch config.PhotoStorageDriver {
	case "local":
		if config.IsProduction() {
			return errors.New("production must use PHOTO_STORAGE_DRIVER=s3, not local")
		}
	case "s3":
		if config.PhotoStorageS3Bucket == "" || config.PhotoStorageS3Region == "" {
			return errors.New("PHOTO_STORAGE_S3_BUCKET and PHOTO_STORAGE_S3_REGION are required when PHOTO_STORAGE_DRIVER=s3")
		}
	default:
		return fmt.Errorf("PHOTO_STORAGE_DRIVER must be 'local' or 's3', got %q", config.PhotoStorageDriver)
	}
	return nil
}

func validateRSAKeyPair(privatePath, publicPath string, production bool) error {
	// #nosec G304,G703 -- key paths are trusted, required deployment configuration.
	privateData, err := os.ReadFile(privatePath)
	if err != nil {
		return fmt.Errorf("read JWT private key: %w", err)
	}
	// #nosec G304,G703 -- key paths are trusted, required deployment configuration.
	publicData, err := os.ReadFile(publicPath)
	if err != nil {
		return fmt.Errorf("read JWT public key: %w", err)
	}

	privateKey, err := parsePrivateKey(privateData)
	if err != nil {
		return fmt.Errorf("parse JWT private key: %w", err)
	}
	publicKey, err := parsePublicKey(publicData)
	if err != nil {
		return fmt.Errorf("parse JWT public key: %w", err)
	}
	minimumBits := 2048
	if production {
		minimumBits = 3072
	}
	if privateKey.N.BitLen() < minimumBits {
		return fmt.Errorf("JWT RSA key must contain at least %d bits", minimumBits)
	}
	if privateKey.E != publicKey.E || privateKey.N.Cmp(publicKey.N) != 0 {
		return errors.New("JWT private and public keys do not match")
	}
	return nil
}

func parsePrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("PEM block not found")
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

func parsePublicKey(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("PEM block not found")
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

func splitAndTrim(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return duration, nil
}

func boolEnv(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return parsed, nil
}

func int32Env(key string, fallback, minimum, maximum int32) (int32, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < int64(minimum) || parsed > int64(maximum) {
		return 0, fmt.Errorf("%s must be between %d and %d", key, minimum, maximum)
	}
	return int32(parsed), nil
}

func float64Env(key string, fallback, minimum, maximum float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s must be between %g and %g", key, minimum, maximum)
	}
	return parsed, nil
}

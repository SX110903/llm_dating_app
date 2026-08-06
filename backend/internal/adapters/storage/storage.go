package storage

import (
	"context"
	"fmt"

	domainprofile "github.com/sx110903/llmatch-v2/backend/internal/domain/profile"
)

type Driver string

const (
	DriverLocal Driver = "local"
	DriverS3    Driver = "s3"
)

type Config struct {
	Driver       Driver
	LocalBaseDir string
	S3Bucket     string
	S3Region     string
	S3Endpoint   string
	S3PathStyle  bool
}

// New builds the photo storage adapter selected by cfg.Driver: local
// filesystem for development, S3 (or an S3-compatible endpoint) for
// production.
func New(ctx context.Context, cfg Config) (domainprofile.Storage, error) {
	switch cfg.Driver {
	case DriverLocal:
		return NewLocalStorage(cfg.LocalBaseDir), nil
	case DriverS3:
		client, err := NewS3Client(ctx, cfg.S3Region, cfg.S3Endpoint, cfg.S3PathStyle)
		if err != nil {
			return nil, err
		}
		return NewS3Storage(client, cfg.S3Bucket), nil
	default:
		return nil, fmt.Errorf("unsupported photo storage driver: %q", cfg.Driver)
	}
}

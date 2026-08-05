package profile

import (
	"context"
	"io"
)

// Storage is an outbound port over the photo blob storage adapter (local
// filesystem in development, S3 in production). Keys are opaque UUID-based
// identifiers chosen by the application layer; the caller's original
// filename never becomes part of the key.
type Storage interface {
	Put(ctx context.Context, key, contentType string, data []byte) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

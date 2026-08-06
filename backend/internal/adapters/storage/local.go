package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalStorage is the development adapter for domain/profile.Storage: photos
// are written to a directory tree on disk.
type LocalStorage struct {
	baseDir string
}

func NewLocalStorage(baseDir string) *LocalStorage {
	return &LocalStorage{baseDir: baseDir}
}

func (s *LocalStorage) Put(_ context.Context, key, _ string, data []byte) error {
	path, err := s.resolvePath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create photo directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write photo file: %w", err)
	}
	return nil
}

func (s *LocalStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	path, err := s.resolvePath(key)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path) // #nosec G304 -- resolvePath already confines path to baseDir
	if err != nil {
		return nil, fmt.Errorf("open photo file: %w", err)
	}
	return file, nil
}

func (s *LocalStorage) Delete(_ context.Context, key string) error {
	path, err := s.resolvePath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove photo file: %w", err)
	}
	return nil
}

// resolvePath rejects any key that would resolve outside baseDir. Keys are
// always application-generated UUIDs, never user input, but this guard is
// kept as defense in depth against path traversal.
func (s *LocalStorage) resolvePath(key string) (string, error) {
	if key == "" {
		return "", errors.New("photo key must not be empty")
	}
	baseAbs, err := filepath.Abs(s.baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve storage base dir: %w", err)
	}
	// filepath.Join lets ".." climb past baseAbs exactly like the
	// underlying OS path resolution would; the prefix check below is what
	// actually rejects a key that tries to escape the storage root.
	pathAbs, err := filepath.Abs(filepath.Join(baseAbs, key))
	if err != nil {
		return "", fmt.Errorf("resolve photo path: %w", err)
	}
	if pathAbs != baseAbs && !strings.HasPrefix(pathAbs, baseAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("photo key escapes storage root: %q", key)
	}
	return pathAbs, nil
}

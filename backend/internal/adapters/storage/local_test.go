package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sx110903/llmatch-v2/backend/internal/adapters/storage"
)

func TestLocalStoragePutAndDelete(t *testing.T) {
	dir := t.TempDir()
	s := storage.NewLocalStorage(dir)
	key := "photos/user-1/photo-1"

	require.NoError(t, s.Put(context.Background(), key, "image/png", []byte("data")))

	data, err := os.ReadFile(filepath.Join(dir, key)) // #nosec G304 -- test-controlled path
	require.NoError(t, err)
	require.Equal(t, []byte("data"), data)

	require.NoError(t, s.Delete(context.Background(), key))
	_, err = os.Stat(filepath.Join(dir, key))
	require.True(t, os.IsNotExist(err))
}

// TestLocalStorageRejectsPathTraversal is the mandatory path-traversal test:
// a key trying to escape the storage root must be rejected instead of
// writing outside dir.
func TestLocalStorageRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	s := storage.NewLocalStorage(dir)

	maliciousKeys := []string{
		"../../etc/passwd",
		"../outside",
		"a/../../outside",
	}
	for _, key := range maliciousKeys {
		err := s.Put(context.Background(), key, "image/png", []byte("data"))
		require.Error(t, err, "key %q should have been rejected", key)
	}

	parent := filepath.Dir(dir)
	entries, err := os.ReadDir(parent)
	require.NoError(t, err)
	for _, entry := range entries {
		require.NotEqual(t, "outside", entry.Name())
	}
}

func TestLocalStorageDeleteMissingFileIsNotAnError(t *testing.T) {
	s := storage.NewLocalStorage(t.TempDir())
	require.NoError(t, s.Delete(context.Background(), "photos/does/not/exist"))
}

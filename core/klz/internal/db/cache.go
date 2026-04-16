package db

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
)

// CacheSchemaVersion is bumped whenever the SQLite schema changes.
// Cache entries tagged with a different version are discarded and
// rebuilt on next access per RFC 0001 §Schema evolution.
const CacheSchemaVersion = "1"

// Cache is the interface Phase 4 implements to provide random-access
// query acceleration over a .klz archive. When the `klzcache` build
// tag is OFF, every method returns ErrNotImplemented; when ON, the
// SQLite-backed implementation in impl.go wires everything up.
type Cache interface {
	// BlockByID returns the block's source-runs JSON payload or a
	// nil-ok pair when no block with that id is in the archive.
	BlockByID(ctx context.Context, id string) ([]byte, error)

	// SimilarSources returns up to `limit` block ids whose source
	// text is most similar to `query`, ranked by the FTS5 index.
	SimilarSources(ctx context.Context, query, locale string, limit int) ([]string, error)

	// Close releases any cache resources.
	Close() error
}

// ErrNotImplemented is returned when a cache method is called in a
// build that doesn't have the klzcache tag set.
type ErrNotImplemented struct{ Op string }

func (e *ErrNotImplemented) Error() string {
	return "klz/internal/db: " + e.Op + " is unavailable (rebuild with -tags klzcache)"
}

// CacheRoot returns the per-user cache directory neokapi uses to
// store .klz runtime cache entries. Derived from XDG_CACHE_HOME on
// Linux, ~/Library/Caches on macOS, and %LOCALAPPDATA% on Windows
// per RFC 0001 §Where the cache lives. Respects NEOKAPI_KLZ_CACHE
// for test overrides.
func CacheRoot() string {
	if env := os.Getenv("NEOKAPI_KLZ_CACHE"); env != "" {
		return env
	}
	switch runtime.GOOS {
	case "windows":
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, "neokapi", "klz")
		}
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Caches", "neokapi", "klz")
		}
	default:
		if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
			return filepath.Join(dir, "neokapi", "klz")
		}
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".cache", "neokapi", "klz")
		}
	}
	return filepath.Join(os.TempDir(), "neokapi", "klz")
}

// EntryDir returns the cache directory for a given manifest hash,
// sharded by the first two hex characters per RFC 0001 §Where the
// cache lives.
func EntryDir(manifestHash string) string {
	if len(manifestHash) < 2 {
		return filepath.Join(CacheRoot(), manifestHash)
	}
	return filepath.Join(CacheRoot(), manifestHash[:2], manifestHash[2:])
}

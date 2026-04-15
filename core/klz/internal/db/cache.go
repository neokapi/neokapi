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

// Cache is the interface Phase 4 will implement to provide
// random-access query acceleration over a .klz archive. Every
// method returns ErrNotImplemented in Phase 1 to keep the public
// API shape stable before the SQLite layer lands.
type Cache interface {
	// BlockByID returns the Block with the given id or a nil-ok
	// pair if not found. Implementation will route through the
	// `blocks` table; Phase-1 stub returns ErrNotImplemented.
	BlockByID(ctx context.Context, id string) ([]byte, error)

	// SimilarSources returns up to `limit` block ids whose source
	// text is most similar to `query`, via the FTS5 index.
	SimilarSources(ctx context.Context, query, locale string, limit int) ([]string, error)

	// Close releases any cache resources.
	Close() error
}

// ErrNotImplemented is returned by every Cache method in Phase 1.
// Phase 4 replaces this with a working SQLite implementation.
type ErrNotImplemented struct{ Op string }

func (e *ErrNotImplemented) Error() string {
	return "klz/internal/db: " + e.Op + " is deferred to phase 4 (see RFC 0001)"
}

// CacheRoot returns the per-user cache directory neokapi uses to
// store .klz runtime cache entries. Derived from XDG_CACHE_HOME on
// Linux, ~/Library/Caches on macOS, and %LOCALAPPDATA% on Windows
// per RFC 0001 §Where the cache lives.
//
// Phase 1 exposes the path so administrative CLI subcommands (kapi
// cache info / cache path / cache clear) can be implemented without
// waiting for the query engine.
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
	// Fallback: temp dir. No harm; a cache entry here is just
	// shorter-lived.
	return filepath.Join(os.TempDir(), "neokapi", "klz")
}

// EntryDir returns the cache directory for a given content hash,
// sharded by the first two hex characters per RFC 0001 §Where the
// cache lives.
func EntryDir(manifestHash string) string {
	if len(manifestHash) < 2 {
		return filepath.Join(CacheRoot(), manifestHash)
	}
	return filepath.Join(CacheRoot(), manifestHash[:2], manifestHash[2:])
}

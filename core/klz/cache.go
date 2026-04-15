package klz

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/klz/internal/db"
)

// CacheRoot returns the per-user cache directory neokapi uses for
// .klz runtime cache entries. This is the public re-export of
// internal/db.CacheRoot; administrative tooling (the kapi cache
// subcommands) imports from here so Go's internal/ visibility rule
// stays honored.
func CacheRoot() string { return db.CacheRoot() }

// CacheEntryDir returns the cache directory for a given manifest
// hash, sharded by the first two hex characters per RFC 0001.
func CacheEntryDir(manifestHash string) string { return db.EntryDir(manifestHash) }

// WarmCache eagerly builds the runtime cache entry for the given
// .klz archive. Phase 1 returns a friendly deferred error because
// the SQLite layer lands in Phase 4.
func (r *Reader) WarmCache(ctx context.Context) error {
	return db.Build(ctx, r.ManifestHash())
}

// CacheErrNotBuilt is returned when query helpers are called
// before the Phase-4 cache layer is present.
var CacheErrNotBuilt = fmt.Errorf("klz: runtime cache is unavailable in this build (phase 4 feature)")

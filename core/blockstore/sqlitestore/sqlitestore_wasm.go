//go:build wasm

package sqlitestore

import (
	"sync"

	"github.com/neokapi/neokapi/core/blockstore"
)

// On wasm there is no SQLite driver (see core/storage/driver_wasm.go), so the
// on-disk cache store is unavailable. New instead returns a process-lifetime,
// path-keyed in-memory store: re-opening the same path returns the same store,
// so a project's `.kapi/cache` or a `.klz` workspace cache "persists" across
// commands within one wasm session (e.g. the docs lab running
// extract → transform → merge). Behaviour is identical to the native cache for
// everything the engine observes — only durability across a process restart is
// lost, which a single browser session doesn't need.
//
// This keeps the SessionTool overlay-caching path (Capabilities.Persistent)
// active in wasm, so cached resume and the .klz workspace work in the lab.
var (
	wasmCacheMu     sync.Mutex
	wasmCacheStores = map[string]blockstore.Store{}
)

// New returns the session-persistent in-memory store for path, creating it on
// first use. The error return matches the native signature.
func New(path string) (blockstore.Store, error) {
	wasmCacheMu.Lock()
	defer wasmCacheMu.Unlock()
	if s, ok := wasmCacheStores[path]; ok {
		return s, nil
	}
	s := blockstore.NewPersistentMemoryStore()
	wasmCacheStores[path] = s
	return s, nil
}

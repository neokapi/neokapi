// Package extregistry holds the process-wide extension-decoder registry state
// for core/project. It is factored out of the production package so the public
// test-support package (core/project/projecttest) can reset the registry
// between tests without core/project exporting a test hook on its production
// surface.
package extregistry

import (
	"sync"

	"gopkg.in/yaml.v3"
)

// Entry is one registered extension binding: the owning group and the decode
// hook (nil for a marker-only registration).
type Entry struct {
	Group  string
	Decode func(node yaml.Node) error
}

type key struct {
	scope int
	name  string
}

var (
	mu      sync.RWMutex
	entries = map[key]Entry{}
	groups  = map[string]struct{}{}
)

// Register stores an entry for (scope, name). It reports false — without
// storing — when the key is already registered; the caller decides how loudly
// to fail.
func Register(scope int, name string, e Entry) bool {
	mu.Lock()
	defer mu.Unlock()
	k := key{scope, name}
	if _, exists := entries[k]; exists {
		return false
	}
	entries[k] = e
	if e.Group != "" {
		groups[e.Group] = struct{}{}
	}
	return true
}

// Lookup returns the entry registered for (scope, name).
func Lookup(scope int, name string) (Entry, bool) {
	mu.RLock()
	defer mu.RUnlock()
	e, ok := entries[key{scope, name}]
	return e, ok
}

// HasGroup reports whether at least one entry with this group is registered.
func HasGroup(group string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := groups[group]
	return ok
}

// Reset clears the registry. Test support only — production code registers
// extensions once, at init, and never unregisters.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	entries = map[key]Entry{}
	groups = map[string]struct{}{}
}

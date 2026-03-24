package server

import (
	"strings"
	"sync"
	"time"
)

// pulseCacheTTLs defines per-endpoint cache durations.
var pulseCacheTTLs = map[string]time.Duration{
	"overview":    5 * time.Minute,
	"projects":    2 * time.Minute,
	"project":     2 * time.Minute,
	"locale":      2 * time.Minute,
	"activity":    1 * time.Minute,
	"leaderboard": 10 * time.Minute,
	"terms":       15 * time.Minute,
	"term":        15 * time.Minute,
}

// pulseCacheEntry holds a cached response with expiry.
type pulseCacheEntry struct {
	data      any
	expiresAt time.Time
}

// pulseCache is a TTL-based in-memory cache for Pulse endpoints.
type pulseCache struct {
	mu      sync.RWMutex
	entries map[string]*pulseCacheEntry
}

// newPulseCache creates a new Pulse cache.
func newPulseCache() *pulseCache {
	return &pulseCache{
		entries: make(map[string]*pulseCacheEntry),
	}
}

// pulseCacheKey builds a cache key from workspace, endpoint, and query params.
func pulseCacheKey(workspaceID, endpoint, queryParams string) string {
	return workspaceID + ":" + endpoint + ":" + queryParams
}

// Get returns the cached value if present and not expired.
func (c *pulseCache) Get(key string) (any, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}
	return entry.data, true
}

// Set stores a value with the TTL for the given endpoint type.
func (c *pulseCache) Set(key, endpointType string, data any) {
	ttl := pulseCacheTTLs[endpointType]
	if ttl == 0 {
		ttl = 2 * time.Minute
	}
	c.mu.Lock()
	c.entries[key] = &pulseCacheEntry{
		data:      data,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()
}

// InvalidateWorkspace removes all cache entries for a workspace.
func (c *pulseCache) InvalidateWorkspace(workspaceID string) {
	prefix := workspaceID + ":"
	c.mu.Lock()
	for key := range c.entries {
		if strings.HasPrefix(key, prefix) {
			delete(c.entries, key)
		}
	}
	c.mu.Unlock()
}

// InvalidateProject removes cache entries related to a specific project.
func (c *pulseCache) InvalidateProject(workspaceID, projectID string) {
	c.mu.Lock()
	for key := range c.entries {
		if strings.HasPrefix(key, workspaceID+":") && strings.Contains(key, projectID) {
			delete(c.entries, key)
		}
	}
	// Also invalidate overview and leaderboard since they aggregate across projects.
	delete(c.entries, pulseCacheKey(workspaceID, "overview", ""))
	delete(c.entries, pulseCacheKey(workspaceID, "leaderboard", ""))
	c.mu.Unlock()
}

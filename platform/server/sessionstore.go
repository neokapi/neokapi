package server

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// ErrSessionNotFound is returned when a session state key does not exist or has expired.
var ErrSessionNotFound = errors.New("session state not found")

// SessionStateStore abstracts ephemeral auth state storage (device codes,
// OIDC states, desktop auth states). Implementations must handle key expiry.
//
// Keys are opaque strings. Values are JSON-encoded structs. All operations
// are safe for concurrent use.
type SessionStateStore interface {
	// Set stores a value with the given TTL. If the key already exists, it is overwritten.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Get retrieves a value by key. Returns ErrSessionNotFound if the key
	// does not exist or has expired.
	Get(ctx context.Context, key string) ([]byte, error)

	// Delete removes a key. Returns nil if the key does not exist.
	Delete(ctx context.Context, key string) error
}

// memoryEntry is a value with an expiry time in the in-memory store.
type memoryEntry struct {
	Value     []byte
	ExpiresAt time.Time
}

// MemorySessionStore is an in-memory SessionStateStore with lazy expiry
// and periodic background cleanup. Suitable for single-instance deployments.
type MemorySessionStore struct {
	mu      sync.Mutex
	entries map[string]*memoryEntry
	done    chan struct{}
}

// NewMemorySessionStore creates an in-memory session store with a background
// cleanup goroutine that removes expired entries every minute.
func NewMemorySessionStore() *MemorySessionStore {
	s := &MemorySessionStore{
		entries: make(map[string]*memoryEntry),
		done:    make(chan struct{}),
	}
	go s.cleanupLoop()
	return s
}

func (s *MemorySessionStore) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	s.mu.Lock()
	s.entries[key] = &memoryEntry{
		Value:     value,
		ExpiresAt: time.Now().Add(ttl),
	}
	s.mu.Unlock()
	return nil
}

func (s *MemorySessionStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	e, ok := s.entries[key]
	if !ok {
		s.mu.Unlock()
		return nil, ErrSessionNotFound
	}
	if time.Now().After(e.ExpiresAt) {
		delete(s.entries, key)
		s.mu.Unlock()
		return nil, ErrSessionNotFound
	}
	val := e.Value
	s.mu.Unlock()
	return val, nil
}

func (s *MemorySessionStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	delete(s.entries, key)
	s.mu.Unlock()
	return nil
}

// Close stops the background cleanup goroutine.
func (s *MemorySessionStore) Close() {
	close(s.done)
}

func (s *MemorySessionStore) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.done:
			return
		}
	}
}

func (s *MemorySessionStore) cleanup() {
	now := time.Now()
	s.mu.Lock()
	for k, v := range s.entries {
		if now.After(v.ExpiresAt) {
			delete(s.entries, k)
		}
	}
	s.mu.Unlock()
}

// Key prefixes for the four auth state stores.
const (
	prefixDeviceCode   = "device:"
	prefixWebAuth      = "webauth:"
	prefixDesktopAuth  = "desktop:"
	prefixDeviceVerify = "deviceverify:"
	prefixUserCode     = "usercode:" // secondary index: userCode → deviceCode
)

// Typed helper functions for storing/retrieving auth states via the SessionStateStore.
// These handle JSON serialization and key prefixing.

func sessionSet[T any](ctx context.Context, store SessionStateStore, prefix, key string, val *T, ttl time.Duration) error {
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return store.Set(ctx, prefix+key, data, ttl)
}

func sessionGet[T any](ctx context.Context, store SessionStateStore, prefix, key string) (*T, error) {
	data, err := store.Get(ctx, prefix+key)
	if err != nil {
		return nil, err
	}
	var val T
	if err := json.Unmarshal(data, &val); err != nil {
		return nil, err
	}
	return &val, nil
}

func sessionDelete(ctx context.Context, store SessionStateStore, prefix, key string) error {
	return store.Delete(ctx, prefix+key)
}

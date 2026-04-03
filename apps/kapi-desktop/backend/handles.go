package backend

import (
	"io"
	"sync"

	"github.com/neokapi/neokapi/core/id"
)

// handleStore is a thread-safe store for open resources keyed by handle ID.
// T must implement io.Closer so resources can be cleaned up.
type handleStore[T io.Closer] struct {
	mu      sync.RWMutex
	handles map[string]T
}

func newHandleStore[T io.Closer]() *handleStore[T] {
	return &handleStore[T]{handles: make(map[string]T)}
}

// Open adds a resource and returns its handle ID.
func (s *handleStore[T]) Open(val T) string {
	h := id.New()
	s.mu.Lock()
	s.handles[h] = val
	s.mu.Unlock()
	return h
}

// Get retrieves a resource by handle ID.
func (s *handleStore[T]) Get(h string) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.handles[h]
	return val, ok
}

// Close closes and removes a single resource.
func (s *handleStore[T]) Close(h string) error {
	s.mu.Lock()
	val, ok := s.handles[h]
	if ok {
		delete(s.handles, h)
	}
	s.mu.Unlock()
	if ok {
		return val.Close()
	}
	return nil
}

// CloseAll closes all open resources.
func (s *handleStore[T]) CloseAll() {
	s.mu.Lock()
	handles := s.handles
	s.handles = make(map[string]T)
	s.mu.Unlock()

	for _, val := range handles {
		_ = val.Close()
	}
}

// Count returns the number of open handles.
func (s *handleStore[T]) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.handles)
}

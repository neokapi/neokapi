package backend

import (
	"io"
	"sync"

	"github.com/neokapi/neokapi/core/id"
)

// handleStore is a thread-safe store for open resources keyed by handle ID.
// T must implement io.Closer so resources can be cleaned up.
//
// A handle is either owned or borrowed. Owned is the ordinary case: the store
// opened the resource and Close closes it. Borrowed exists because a project's
// content memory and terms are two schemas inside its one `.kapi/store.db`,
// handed out by a handle that does not own the pool — closing either would take
// the block cache and the working set with it. Both kinds are addressed
// identically by the frontend; only teardown differs.
type handleStore[T io.Closer] struct {
	mu       sync.RWMutex
	handles  map[string]T
	borrowed map[string]bool
}

func newHandleStore[T io.Closer]() *handleStore[T] {
	return &handleStore[T]{handles: make(map[string]T), borrowed: map[string]bool{}}
}

// Open adds an owned resource and returns its handle ID. Closing the handle
// closes the resource.
func (s *handleStore[T]) Open(val T) string {
	return s.add(val, false)
}

// Adopt registers a resource this store does not own and returns its handle ID.
// Closing the handle forgets it without closing it — the owner (the project
// store) decides when the underlying pool goes away.
func (s *handleStore[T]) Adopt(val T) string {
	return s.add(val, true)
}

func (s *handleStore[T]) add(val T, borrowed bool) string {
	h := id.New()
	s.mu.Lock()
	s.handles[h] = val
	if borrowed {
		s.borrowed[h] = true
	}
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

// Close removes a single resource, closing it unless it was adopted.
func (s *handleStore[T]) Close(h string) error {
	s.mu.Lock()
	val, ok := s.handles[h]
	borrowed := s.borrowed[h]
	delete(s.handles, h)
	delete(s.borrowed, h)
	s.mu.Unlock()
	if ok && !borrowed {
		return val.Close()
	}
	return nil
}

// CloseAll removes every resource, closing the owned ones.
func (s *handleStore[T]) CloseAll() {
	s.mu.Lock()
	handles, borrowed := s.handles, s.borrowed
	s.handles = make(map[string]T)
	s.borrowed = map[string]bool{}
	s.mu.Unlock()

	for h, val := range handles {
		if !borrowed[h] {
			_ = val.Close()
		}
	}
}

// Count returns the number of open handles.
func (s *handleStore[T]) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.handles)
}

package blockstore

import (
	"context"
	"errors"
	"iter"
	"sync"
	"time"
)

// NewMemoryStore returns a Store that keeps everything in Go maps.
// Always RandomAccess + Writable; not Concurrent (a single Session
// at a time) and not Remote.
//
// Intended for: streaming flows (current default), tests, one-shot
// CLI invocations. Equivalent to "no persistence" from the user's
// point of view — when the Store is Closed, the state is gone.
func NewMemoryStore() Store {
	return &memoryStore{
		blocks:   make(map[string]memBlock),
		sidecars: make(map[string]Sidecar),
	}
}

type memBlock struct {
	collection string
	block      Block
}

type memoryStore struct {
	mu       sync.Mutex
	blocks   map[string]memBlock // key: block hash
	sidecars map[string]Sidecar  // key: kind+"\x00"+blockHash
	active   bool                // a Session is in progress
	closed   bool
}

func (m *memoryStore) Capabilities() Capabilities {
	return Capabilities{RandomAccess: true, Writable: true}
}

func (m *memoryStore) Begin(ctx context.Context) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, errors.New("blockstore: memory store closed")
	}
	if m.active {
		// Keep semantics simple: one session at a time. Tests that
		// want "concurrent reads" use NewCacheStore.
		return nil, errors.New("blockstore: single-session cap hit")
	}
	m.active = true
	// Snapshot-on-begin: Sessions work against a fresh copy and only
	// merge back on Commit. Gives us clean Rollback semantics without
	// a write-ahead log.
	s := &memorySession{
		store:    m,
		blocks:   copyBlocks(m.blocks),
		sidecars: copySidecars(m.sidecars),
	}
	return s, nil
}

func (m *memoryStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

type memorySession struct {
	store    *memoryStore
	blocks   map[string]memBlock
	sidecars map[string]Sidecar
	done     bool // committed, rolled back, or closed
}

func (s *memorySession) Capabilities() Capabilities { return s.store.Capabilities() }

func (s *memorySession) Blocks(filter BlockFilter) iter.Seq2[*Block, error] {
	return func(yield func(*Block, error) bool) {
		if s.done {
			yield(nil, ErrClosed)
			return
		}
		count := 0
		for _, mb := range s.blocks {
			if filter.Collection != "" && mb.collection != filter.Collection {
				continue
			}
			if filter.Translatable != nil && mb.block.Translatable != *filter.Translatable {
				continue
			}
			// Yield a copy so iteration can't mutate store state.
			b := mb.block
			if !yield(&b, nil) {
				return
			}
			count++
			if filter.Limit > 0 && count >= filter.Limit {
				return
			}
		}
	}
}

func (s *memorySession) GetBlock(hash string) (*Block, error) {
	if s.done {
		return nil, ErrClosed
	}
	mb, ok := s.blocks[hash]
	if !ok {
		return nil, ErrNotFound
	}
	b := mb.block
	return &b, nil
}

func (s *memorySession) PutBlock(collection string, b *Block) error {
	if s.done {
		return ErrClosed
	}
	if b == nil || b.Hash == "" {
		return errors.New("blockstore: block must have a non-empty Hash")
	}
	s.blocks[b.Hash] = memBlock{collection: collection, block: *b}
	return nil
}

func (s *memorySession) GetSidecar(kind, blockHash string) (Sidecar, error) {
	if s.done {
		return Sidecar{}, ErrClosed
	}
	sc, ok := s.sidecars[sidecarKey(kind, blockHash)]
	if !ok {
		return Sidecar{}, ErrNotFound
	}
	return sc, nil
}

func (s *memorySession) PutSidecar(sc Sidecar) error {
	if s.done {
		return ErrClosed
	}
	if sc.Kind == "" || sc.BlockHash == "" {
		return errors.New("blockstore: sidecar needs both Kind and BlockHash")
	}
	if sc.UpdatedAt == 0 {
		sc.UpdatedAt = time.Now().Unix()
	}
	s.sidecars[sidecarKey(sc.Kind, sc.BlockHash)] = sc
	return nil
}

func (s *memorySession) ListSidecars(kind string) iter.Seq2[Sidecar, error] {
	return func(yield func(Sidecar, error) bool) {
		if s.done {
			yield(Sidecar{}, ErrClosed)
			return
		}
		prefix := kind + "\x00"
		for k, v := range s.sidecars {
			if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
				if !yield(v, nil) {
					return
				}
			}
		}
	}
}

func (s *memorySession) Commit() error {
	if s.done {
		return ErrClosed
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	s.store.blocks = s.blocks
	s.store.sidecars = s.sidecars
	s.store.active = false
	s.done = true
	return nil
}

func (s *memorySession) Rollback() error {
	if s.done {
		return nil
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	s.store.active = false
	s.done = true
	return nil
}

func (s *memorySession) Close() error {
	if !s.done {
		return s.Rollback()
	}
	return nil
}

// ─── helpers ────────────────────────────────────────────────────

func sidecarKey(kind, blockHash string) string {
	return kind + "\x00" + blockHash
}

func copyBlocks(in map[string]memBlock) map[string]memBlock {
	out := make(map[string]memBlock, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copySidecars(in map[string]Sidecar) map[string]Sidecar {
	out := make(map[string]Sidecar, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

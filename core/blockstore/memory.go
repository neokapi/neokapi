package blockstore

import (
	"context"
	"errors"
	"iter"
	"sort"
	"sync"
	"time"
)

// NewMemoryStore returns a Store that keeps everything in Go maps.
// RandomAccess + Writable; Concurrent (multiple sessions supported
// simultaneously, each working against a snapshot taken at Begin;
// commit replaces the store's state with the session's state —
// last-writer-wins). Not Remote.
//
// Intended for: streaming flows (the executor default when no
// persistent store is declared), tests, one-shot CLI invocations.
// Equivalent to "no persistence" from the user's point of view —
// when the Store is Closed, the state is gone.
func NewMemoryStore() Store {
	return &memoryStore{
		blocks:   make(map[string]memBlock),
		overlays: make(map[string]Overlay),
	}
}

// NewPersistentMemoryStore returns a memory store that advertises
// Persistent=true and whose Close is a no-op — so it survives Begin/Commit
// cycles and repeated open/close across commands. It is the substrate for
// the wasm build's NewCacheStore (where SQLite is unavailable): a single
// process-lifetime store stands in for the on-disk cache. Not for native
// use (the SQLite cache is the real persistent store there).
func NewPersistentMemoryStore() Store {
	return &memoryStore{
		blocks:     make(map[string]memBlock),
		overlays:   make(map[string]Overlay),
		persistent: true,
	}
}

type memBlock struct {
	collection string
	block      Block
}

type memoryStore struct {
	mu         sync.Mutex
	blocks     map[string]memBlock // key: block hash
	overlays   map[string]Overlay  // key: kind+"\x00"+blockHash
	closed     bool
	persistent bool // advertise Persistent; Close is a no-op
}

func (m *memoryStore) Capabilities() Capabilities {
	return Capabilities{RandomAccess: true, Concurrent: true, Writable: true, Persistent: m.persistent}
}

func (m *memoryStore) Begin(ctx context.Context) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, errors.New("blockstore: memory store closed")
	}
	// Snapshot-on-begin: each Session works against its own copy.
	// On Commit the session's state replaces the store's state
	// (last-writer-wins across concurrent sessions). Rollback
	// discards the session's copy without touching the store.
	s := &memorySession{
		store:    m,
		blocks:   copyBlocks(m.blocks),
		overlays: copyOverlays(m.overlays),
	}
	return s, nil
}

func (m *memoryStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.persistent {
		// A persistent (wasm cache) store outlives individual open/close
		// cycles, so Close keeps it usable for the next command.
		return nil
	}
	m.closed = true
	return nil
}

type memorySession struct {
	store    *memoryStore
	blocks   map[string]memBlock
	overlays map[string]Overlay
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

func (s *memorySession) GetOverlay(kind, blockHash string) (Overlay, error) {
	if s.done {
		return Overlay{}, ErrClosed
	}
	sc, ok := s.overlays[overlayKey(kind, blockHash)]
	if !ok {
		return Overlay{}, ErrNotFound
	}
	return sc, nil
}

func (s *memorySession) PutOverlay(sc Overlay) error {
	if s.done {
		return ErrClosed
	}
	if sc.Kind == "" || sc.BlockHash == "" {
		return errors.New("blockstore: overlay needs both Kind and BlockHash")
	}
	if sc.UpdatedAt == 0 {
		sc.UpdatedAt = time.Now().Unix()
	}
	s.overlays[overlayKey(sc.Kind, sc.BlockHash)] = sc
	return nil
}

func (s *memorySession) ListOverlays(kind string) iter.Seq2[Overlay, error] {
	return func(yield func(Overlay, error) bool) {
		if s.done {
			yield(Overlay{}, ErrClosed)
			return
		}
		prefix := kind + "\x00"
		for k, v := range s.overlays {
			if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
				if !yield(v, nil) {
					return
				}
			}
		}
	}
}

func (s *memorySession) AllOverlays() iter.Seq2[Overlay, error] {
	return func(yield func(Overlay, error) bool) {
		if s.done {
			yield(Overlay{}, ErrClosed)
			return
		}
		keys := make([]string, 0, len(s.overlays))
		for k := range s.overlays {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if !yield(s.overlays[k], nil) {
				return
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
	s.store.overlays = s.overlays
	s.done = true
	return nil
}

func (s *memorySession) Rollback() error {
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

func overlayKey(kind, blockHash string) string {
	return kind + "\x00" + blockHash
}

func copyBlocks(in map[string]memBlock) map[string]memBlock {
	out := make(map[string]memBlock, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyOverlays(in map[string]Overlay) map[string]Overlay {
	out := make(map[string]Overlay, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

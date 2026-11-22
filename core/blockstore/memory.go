package blockstore

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"iter"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
)

// NewMemoryStore returns a Store that keeps everything in Go maps.
// RandomAccess + Writable; Concurrent (multiple sessions supported
// simultaneously, each working against a snapshot taken at Begin;
// commit replaces the store's state with the session's state,
// last-writer-wins). Not Remote.
//
// Intended for: streaming flows (the executor default when no
// persistent store is declared), tests, one-shot CLI invocations.
// Equivalent to "no persistence" from the user's point of view,
// when the Store is Closed, the state is gone.
func NewMemoryStore() Store {
	return &memoryStore{
		blocks:   make(map[string]memBlock),
		overlays: make(map[overlayKey]Overlay),
	}
}

type memBlock struct {
	collection string
	block      Block
}

type memoryStore struct {
	mu       sync.Mutex
	blocks   map[string]memBlock // key: block hash
	overlays map[overlayKey]Overlay
	closed   bool
}

func (m *memoryStore) Capabilities() Capabilities {
	return Capabilities{RandomAccess: true, Concurrent: true, Writable: true}
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
	m.closed = true
	return nil
}

type memorySession struct {
	store    *memoryStore
	blocks   map[string]memBlock
	overlays map[overlayKey]Overlay
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
	sc, ok := s.overlays[overlayKey{CanonicalOverlayKind(kind), blockHash}]
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
	sc.Kind = CanonicalOverlayKind(sc.Kind)
	key := overlayKey{sc.Kind, sc.BlockHash}
	if held, ok := s.overlays[key]; ok && bytes.Equal(held.Payload, sc.Payload) {
		return nil
	}
	if sc.UpdatedAt == 0 {
		sc.UpdatedAt = time.Now().Unix()
	}
	s.overlays[key] = sc
	return nil
}

func (s *memorySession) ListOverlays(kind string) iter.Seq2[Overlay, error] {
	kind = CanonicalOverlayKind(kind)
	return func(yield func(Overlay, error) bool) {
		if s.done {
			yield(Overlay{}, ErrClosed)
			return
		}
		for k, v := range s.overlays {
			if k.kind == kind {
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
		keys := make([]overlayKey, 0, len(s.overlays))
		for k := range s.overlays {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(a, b overlayKey) int {
			return cmp.Or(strings.Compare(a.kind, b.kind), strings.Compare(a.blockHash, b.blockHash))
		})
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

// overlayKey is what the memory store files an overlay under: its kind and
// the hash of the block it overlays.
type overlayKey struct{ kind, blockHash string }

func copyBlocks(in map[string]memBlock) map[string]memBlock {
	out := make(map[string]memBlock, len(in))
	maps.Copy(out, in)
	return out
}

func copyOverlays(in map[overlayKey]Overlay) map[overlayKey]Overlay {
	out := make(map[overlayKey]Overlay, len(in))
	maps.Copy(out, in)
	return out
}

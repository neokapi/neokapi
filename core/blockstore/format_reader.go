package blockstore

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/model"
)

// FormatReaderFactory returns a fresh reader + raw document pair for
// each materialisation. The factory shape lets consumers pass
// parameters (format config, encoding) at the point of use without
// the blockstore depending on any specific reader type.
type FormatReaderFactory func() (format.DataFormatReader, *model.RawDocument, error)

// NewFormatReaderStore wraps a DataFormatReader as a read-only Store.
// The reader is drained once on the first Begin(); subsequent
// sessions share the same in-memory snapshot.
//
// Intended use: ad-hoc CLI flows that take a single file as input
// (`kapi translate -i file.xliff`). Construct the store from the
// file's reader, run the flow, emit output via a format writer.
//
// Capabilities: RandomAccess=true (the snapshot is a map, GetBlock
// is O(1)), Writable=false (any Put* returns ErrReadOnly).
func NewFormatReaderStore(factory FormatReaderFactory) Store {
	if factory == nil {
		// Nil factory is a programming error; fail fast at Begin.
		factory = func() (format.DataFormatReader, *model.RawDocument, error) {
			return nil, nil, errors.New("blockstore: format reader factory is nil")
		}
	}
	return &formatReaderStore{factory: factory}
}

type formatReaderStore struct {
	factory FormatReaderFactory
	once    sync.Once
	loadErr error
	blocks  map[string]Block
	ordered []string // iteration order matches reader emission
}

func (s *formatReaderStore) Capabilities() Capabilities {
	return Capabilities{RandomAccess: true, Writable: false}
}

func (s *formatReaderStore) Begin(ctx context.Context) (Session, error) {
	s.once.Do(func() {
		s.load(ctx)
	})
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return &formatReaderSession{store: s}, nil
}

func (s *formatReaderStore) Close() error { return nil }

func (s *formatReaderStore) load(ctx context.Context) {
	reader, doc, err := s.factory()
	if err != nil {
		s.loadErr = fmt.Errorf("blockstore: format reader factory: %w", err)
		return
	}
	if doc == nil {
		s.loadErr = errors.New("blockstore: format reader factory returned nil RawDocument")
		return
	}
	if err := reader.Open(ctx, doc); err != nil {
		s.loadErr = fmt.Errorf("blockstore: format reader open: %w", err)
		return
	}
	defer func() { _ = reader.Close() }()

	s.blocks = make(map[string]Block)
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			s.loadErr = fmt.Errorf("blockstore: format reader stream: %w", res.Error)
			return
		}
		if res.Part == nil || res.Part.Type != model.PartBlock {
			continue
		}
		modelBlock, ok := res.Part.Resource.(*model.Block)
		if !ok || modelBlock == nil {
			continue
		}
		key := blockKey(modelBlock)
		if _, exists := s.blocks[key]; exists {
			continue // first-wins on dup hashes
		}
		s.blocks[key] = liftBlock(modelBlock)
		s.ordered = append(s.ordered, key)
	}
}

// blockKey returns the model.Block ID as the addressing key. Format
// readers populate ID with whatever deterministic identifier their
// wire format carries (klf.Document.Blocks[].id, xliff trans-unit
// id, etc.). When no ID is present the block is skipped.
func blockKey(b *model.Block) string { return b.ID }

// liftBlock converts a model.Block into the klf.Block shape the
// BlockStore exposes. Only the fields the session's readers care
// about are copied; the rest of model.Block's rich state stays on
// the original reader output. ID doubles as Hash — callers address
// blocks by the same key the reader handed them.
func liftBlock(b *model.Block) Block {
	return Block{
		ID:           b.ID,
		Hash:         b.ID,
		Translatable: b.Translatable,
	}
}

// ─── Session ────────────────────────────────────────────────────

type formatReaderSession struct {
	store *formatReaderStore
	done  bool
}

func (s *formatReaderSession) Capabilities() Capabilities { return s.store.Capabilities() }

func (s *formatReaderSession) Blocks(filter BlockFilter) iter.Seq2[*Block, error] {
	return func(yield func(*Block, error) bool) {
		if s.done {
			yield(nil, ErrClosed)
			return
		}
		count := 0
		for _, key := range s.store.ordered {
			b := s.store.blocks[key]
			if filter.Collection != "" {
				// Format readers don't carry collection metadata;
				// the filter is effectively a no-op here (no match).
				continue
			}
			if filter.Translatable != nil && b.Translatable != *filter.Translatable {
				continue
			}
			blockCopy := b
			if !yield(&blockCopy, nil) {
				return
			}
			count++
			if filter.Limit > 0 && count >= filter.Limit {
				return
			}
		}
	}
}

func (s *formatReaderSession) GetBlock(hash string) (*Block, error) {
	if s.done {
		return nil, ErrClosed
	}
	b, ok := s.store.blocks[hash]
	if !ok {
		return nil, ErrNotFound
	}
	return &b, nil
}

func (s *formatReaderSession) PutBlock(_ string, _ *Block) error {
	if s.done {
		return ErrClosed
	}
	return ErrReadOnly
}

func (s *formatReaderSession) GetOverlay(_, _ string) (Overlay, error) {
	if s.done {
		return Overlay{}, ErrClosed
	}
	// Format readers don't emit overlays; every lookup is "not found".
	return Overlay{}, ErrNotFound
}

func (s *formatReaderSession) PutOverlay(_ Overlay) error {
	if s.done {
		return ErrClosed
	}
	return ErrReadOnly
}

func (s *formatReaderSession) ListOverlays(_ string) iter.Seq2[Overlay, error] {
	return func(yield func(Overlay, error) bool) {
		if s.done {
			yield(Overlay{}, ErrClosed)
			return
		}
		// Empty iterator — nothing to yield.
	}
}

func (s *formatReaderSession) Commit() error {
	if s.done {
		return ErrClosed
	}
	s.done = true
	return nil
}

func (s *formatReaderSession) Rollback() error {
	s.done = true
	return nil
}

func (s *formatReaderSession) Close() error {
	s.done = true
	return nil
}

// Silence unused-import when klf is only transitively used via Block.
var _ = klf.SchemaVersion

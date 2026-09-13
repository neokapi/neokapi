package format

import (
	"context"
	"errors"
	"io"

	"github.com/neokapi/neokapi/core/model"
)

// RefRenderer renders the output bytes for a skeleton SkeletonRef given the
// block it refers to. block is nil when no block with that id arrived (the
// buffered skeleton path emits nothing in that case, so a renderer should
// return nil, nil to match). Returning an error aborts the write.
type RefRenderer func(block *model.Block) ([]byte, error)

// LangRenderer renders the output bytes for a skeleton SkeletonLang entry given
// the stored source-locale value. It may be nil for writers that never emit
// SkeletonLang (only the HTML reader does today).
type LangRenderer func(value string) ([]byte, error)

// writeSkeletonEntry writes the bytes one skeleton entry contributes, resolving
// a SkeletonRef through blockFor. StreamSkeletonWrite and BufferedSkeletonWrite
// share this body, so the two paths cannot drift apart: they differ only in
// where a ref's block comes from.
//
// SkeletonInserted bytes are written as text is. Entry types this switch does
// not name (SkeletonOriginal, SkeletonTrimmed) contribute nothing. A format
// that acts on them replays its own skeleton (see the html, odf and openxml
// writers).
func writeSkeletonEntry(out io.Writer, entry SkeletonEntry, blockFor func(id string) *model.Block, renderRef RefRenderer, renderLang LangRenderer) error {
	switch entry.Type {
	case SkeletonText, SkeletonInserted:
		_, err := out.Write(entry.Data)
		return err
	case SkeletonRef:
		data, err := renderRef(blockFor(string(entry.Data)))
		if err != nil {
			return err
		}
		return writeNonEmpty(out, data)
	case SkeletonLang:
		if renderLang == nil {
			_, err := out.Write(entry.Data)
			return err
		}
		data, err := renderLang(string(entry.Data))
		if err != nil {
			return err
		}
		return writeNonEmpty(out, data)
	}
	return nil
}

// writeNonEmpty writes data, skipping the call for an empty render. A renderer
// says "this ref contributes nothing" by returning no bytes, and a writer that
// counts or delimits its Write calls should not see one for that.
func writeNonEmpty(out io.Writer, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	_, err := out.Write(data)
	return err
}

// BufferedSkeletonWrite reconstructs a document from a skeleton store and a
// resolved block map, writing to out. It is the buffered twin of
// StreamSkeletonWrite: the caller has already drained the Part stream into
// blocks, so a SkeletonRef resolves by map lookup rather than by pulling from
// the stream. Both take the same RefRenderer and LangRenderer, so a writer
// hands one renderer to both paths and the two produce identical bytes.
//
// A ref naming a block the map does not hold renders with a nil block, which is
// what StreamSkeletonWrite passes when no such block arrives. renderRef is
// required; renderLang may be nil, and a SkeletonLang entry then emits its raw
// stored bytes.
//
// The store must already be readable: a file-backed store needs its Flush
// before the first Next.
func BufferedSkeletonWrite(store *SkeletonStore, blocks map[string]*model.Block, out io.Writer, renderRef RefRenderer, renderLang LangRenderer) error {
	blockFor := func(id string) *model.Block { return blocks[id] }
	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := writeSkeletonEntry(out, entry, blockFor, renderRef, renderLang); err != nil {
			return err
		}
	}
}

// StreamSkeletonWrite reconstructs a document from a *streaming* skeleton store
// (NewStreamingSkeletonStore) interleaved with the arriving Part stream, writing
// to out. It is the bounded-memory twin of BufferedSkeletonWrite, which
// collects every block into a map first: instead of buffering, this one pulls
// each block referenced by a SkeletonRef from parts on demand. Because a
// StreamingReader emits skeleton refs and their blocks in the same order, the
// pending-block window stays small and the bytes written are identical to the
// buffered path.
//
// Non-Block parts (layer/data/media) carry no skeleton-referenced content and
// are discarded — exactly as the buffered writers' block map ignores them. After
// the skeleton reaches EOF, any remaining parts are drained so the executor's
// tool goroutines can finish.
//
// renderRef is required; renderLang may be nil (SkeletonLang entries then emit
// their raw stored bytes, matching a writer that does not retarget language).
func StreamSkeletonWrite(ctx context.Context, store *SkeletonStore, parts <-chan *model.Part, out io.Writer, renderRef RefRenderer, renderLang LangRenderer) error {
	pending := make(map[string]*model.Block)
	partsClosed := false

	// pullBlock returns the next Block from the stream, or false once the
	// channel is drained (or the context is cancelled). Non-Block parts are
	// skipped. It records cancellation in ctxErr.
	var ctxErr error
	pullBlock := func() (*model.Block, bool) {
		for {
			select {
			case <-ctx.Done():
				ctxErr = ctx.Err()
				partsClosed = true
				return nil, false
			case p, ok := <-parts:
				if !ok {
					partsClosed = true
					return nil, false
				}
				if p != nil && p.Type == model.PartBlock {
					if b, ok := p.Resource.(*model.Block); ok {
						return b, true
					}
				}
			}
		}
	}

	// blockFor returns the block with id, pulling parts until it appears. A
	// streaming reader emits a ref's block right after the ref, so this usually
	// pulls exactly one part; a buffering/reordering tool may make it pull more.
	blockFor := func(id string) *model.Block {
		if b, ok := pending[id]; ok {
			delete(pending, id)
			return b
		}
		for !partsClosed {
			b, ok := pullBlock()
			if !ok {
				break
			}
			if b.ID == id {
				return b
			}
			pending[b.ID] = b
		}
		return nil
	}

	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if err := writeSkeletonEntry(out, entry, blockFor, renderRef, renderLang); err != nil {
			return err
		}
	}

	// Drain any parts past the last skeleton ref so the tool goroutines feeding
	// this writer can complete and the executor can close the session.
	for !partsClosed {
		if _, ok := pullBlock(); !ok {
			break
		}
	}
	return ctxErr
}

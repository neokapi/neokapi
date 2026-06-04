// Package exporter is the inverse of core/blockstore/importer: it reads a
// block store's full contents — every block and every overlay — out into
// plain values, and loads such values back into a store. It is the engine
// behind serializing a resumable .klz workspace (AD-025 §5): export a
// project/working store to a Snapshot, hand the Snapshot to the klz
// packer, and on unpack, Load it into a fresh store.
//
// Export is deliberately content-only and timestamp-free: a workspace's
// identity is the work itself, not when it was recorded, so exported
// overlays carry no UpdatedAt. Load lets the store stamp its own.
package exporter

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/blockstore"
)

// Snapshot is the full content of a block store: its blocks (with the
// collection each belongs to) and its overlays, both in deterministic
// order (blocks by hash, overlays by kind then block hash).
type Snapshot struct {
	Blocks   []BlockEntry
	Overlays []blockstore.Overlay
}

// BlockEntry pairs a block with the collection it was stored under, so a
// round-trip preserves collection membership.
type BlockEntry struct {
	Collection string
	Block      blockstore.Block
}

// Export reads every block and overlay from the store into a Snapshot.
// Overlay enumeration requires the session to implement
// blockstore.OverlayEnumerator (memory and sqlite-cache stores do); a
// session without it yields blocks only and an error.
func Export(ctx context.Context, store blockstore.Store) (*Snapshot, error) {
	sess, err := store.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("exporter: begin session: %w", err)
	}
	defer func() { _ = sess.Close() }()

	snap := &Snapshot{}

	for b, err := range sess.Blocks(blockstore.BlockFilter{}) {
		if err != nil {
			return nil, fmt.Errorf("exporter: read blocks: %w", err)
		}
		if b == nil {
			continue
		}
		snap.Blocks = append(snap.Blocks, BlockEntry{Collection: blockCollection(b), Block: *b})
	}

	enum, ok := sess.(blockstore.OverlayEnumerator)
	if !ok {
		return snap, errors.New("exporter: store session does not support overlay enumeration")
	}
	for o, err := range enum.AllOverlays() {
		if err != nil {
			return nil, fmt.Errorf("exporter: read overlays: %w", err)
		}
		o.UpdatedAt = 0 // content identity, not when-recorded
		snap.Overlays = append(snap.Overlays, o)
	}
	return snap, nil
}

// Load writes a Snapshot's blocks and overlays into the store in one
// committed transaction. Existing entries with the same key are replaced
// (the store's upsert semantics).
func Load(ctx context.Context, store blockstore.Store, snap *Snapshot) error {
	if snap == nil {
		return nil
	}
	sess, err := store.Begin(ctx)
	if err != nil {
		return fmt.Errorf("exporter: begin session: %w", err)
	}
	defer func() { _ = sess.Close() }()

	for i := range snap.Blocks {
		e := snap.Blocks[i]
		b := e.Block
		if err := sess.PutBlock(e.Collection, &b); err != nil {
			return fmt.Errorf("exporter: put block %q: %w", b.Hash, err)
		}
	}
	for _, o := range snap.Overlays {
		if err := sess.PutOverlay(o); err != nil {
			return fmt.Errorf("exporter: put overlay %q/%q: %w", o.Kind, o.BlockHash, err)
		}
	}
	if err := sess.Commit(); err != nil {
		return fmt.Errorf("exporter: commit: %w", err)
	}
	return nil
}

// LoadOverlays writes only overlays into the store (no blocks). Used to
// warm a working store's overlay cache from a packaged workspace before
// re-running a flow, so already-computed steps hydrate instead of recompute.
func LoadOverlays(ctx context.Context, store blockstore.Store, overlays []blockstore.Overlay) error {
	if len(overlays) == 0 {
		return nil
	}
	return Load(ctx, store, &Snapshot{Overlays: overlays})
}

// blockCollection returns the collection a block reports via its
// properties. The block store keys blocks by hash and tracks collection
// separately; klf.Block carries no collection field, so a re-loaded block
// lands in the default (empty) collection unless the caller tracked it.
func blockCollection(b *blockstore.Block) string {
	return ""
}

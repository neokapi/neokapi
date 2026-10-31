package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// hydrateTargetsTool is the internal merge step: for each block it reads the
// stored `targets/<locale>` overlay and applies it to the in-flight block so
// the writer emits the translated output. It recomputes nothing (no tool
// re-runs, no network). It replays the work already in the workspace.
// An overlay whose source stamp names another source than the block's is a
// translation of a source that changed since: it is not applied, and Stale
// counts it.
type hydrateTargetsTool struct {
	tool.BaseTool
	locale model.LocaleID
	stale  atomic.Int64
	// unstamped counts, among the stale, the overlays an earlier kapi wrote
	// (blockstore.TargetOverlay.Unstamped).
	unstamped atomic.Int64

	// skeleton is set when the merge rebuilds the document from its skeleton
	// (a workspace packed without its source): each block carries its id
	// alone, so there is no source to compare a stamp with: every stamped
	// overlay applies as it is, and an unstamped one is counted and left out.
	skeleton bool
}

// Stale is how many blocks held a stored translation of another source, or
// one an earlier kapi wrote.
func (t *hydrateTargetsTool) Stale() int { return int(t.stale.Load()) }

// Unstamped is how many of the Stale blocks held a translation an earlier
// kapi wrote.
func (t *hydrateTargetsTool) Unstamped() int { return int(t.unstamped.Load()) }

func newHydrateTargetsTool(locale model.LocaleID) *hydrateTargetsTool {
	t := &hydrateTargetsTool{locale: locale}
	t.ToolName = "apply-targets"
	t.ToolDescription = "Apply stored target overlays onto blocks"
	return t
}

func (t *hydrateTargetsTool) SessionProcess(ctx context.Context, sess blockstore.Session, in <-chan *model.Part, out chan<- *model.Part) error {
	kind := blockstore.TargetOverlayKind(t.locale)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-in:
			if !ok {
				return nil
			}
			if b, isBlock := part.Resource.(*model.Block); isBlock && b != nil && b.Translatable && b.ID != "" {
				// Address overlays by the same globally-unique key the run wrote
				// (source-file-namespaced when the caller set it on ctx).
				key := blockstore.OverlayKey(ctx, b.ID, b.SourceText())
				// Absence and failure are different: ErrNotFound is "this block
				// has no translation yet" (ordinary pending work), anything else
				// is a store read that failed. Conflating them made a locked or
				// corrupt block store write the SOURCE text into the target file
				// and report the merge applied, the shipped-deliverable form of
				// #1449. absorbStoreTargets and core/convergence/coverage.go draw
				// the same distinction.
				o, err := sess.GetOverlay(kind, key)
				if err != nil && !errors.Is(err, blockstore.ErrNotFound) {
					return fmt.Errorf("read %s overlay for block %s: %w", kind, blockKey(b), err)
				}
				if err == nil && len(o.Payload) > 0 {
					if t.skeleton {
						// No source to compare a stamp with, but an overlay
						// with no stamp needs none to be told apart.
						p, derr := decodeTargetOverlay(b, t.locale, o.Payload)
						if derr != nil {
							return derr
						}
						if p.Unstamped() {
							t.stale.Add(1)
							t.unstamped.Add(1)
						} else {
							setTargetOverlay(b, t.locale, p)
						}
					} else {
						stale, aerr := applyCurrentTargetOverlay(b, b, t.locale, o.Payload)
						if aerr != nil {
							return aerr
						}
						if stale != overlayCurrent {
							t.stale.Add(1)
						}
						if stale == overlayUnstamped {
							t.unstamped.Add(1)
						}
					}
				}
			}
			select {
			case out <- part:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// applyTargetOverlay sets a block's target from a target-overlay payload,
// accepting the conventional {runs|text|target} shapes the translate-family
// tools write.
//
// An undecodable payload is RETURNED, not swallowed: returning silently left
// the block with no target, so the merged file shipped that segment as source
// text while the run still counted it applied. A stored overlay that cannot be
// read is a fault in the workspace, not an absent translation.
func applyTargetOverlay(b *model.Block, locale model.LocaleID, payload []byte) error {
	p, err := decodeTargetOverlay(b, locale, payload)
	if err != nil {
		return err
	}
	setTargetOverlay(b, locale, p)
	return nil
}

// applyCurrentTargetOverlay is applyTargetOverlay for a merge that holds the
// block it read from the file (read): it applies the overlay to dst, which may
// be read itself, only while the overlay's source stamp names read's source as
// read (blockstore.TargetOverlay.StaleFor). An overlay whose stamp names
// another source is a translation of a source that has changed since, and one
// with no stamp was written by an earlier kapi: dst is left as it is and the
// result says which, for the merge to count.
func applyCurrentTargetOverlay(read, dst *model.Block, locale model.LocaleID, payload []byte) (overlayStaleness, error) {
	p, err := decodeTargetOverlay(read, locale, payload)
	if err != nil {
		return overlayCurrent, err
	}
	switch {
	case p.Unstamped():
		return overlayUnstamped, nil
	case p.StaleFor(read):
		return overlayStale, nil
	}
	setTargetOverlay(dst, locale, p)
	return overlayCurrent, nil
}

// overlayStaleness is what a merge found of a stored translation against the
// block it read from the file.
type overlayStaleness int

const (
	// overlayCurrent is a translation of the source the block holds.
	overlayCurrent overlayStaleness = iota
	// overlayStale is a translation of a source the block no longer holds.
	overlayStale
	// overlayUnstamped is a translation an earlier kapi wrote, with no stamp
	// naming the source it answered.
	overlayUnstamped
)

// decodeTargetOverlay decodes a target-overlay payload stored for b.
func decodeTargetOverlay(b *model.Block, locale model.LocaleID, payload []byte) (blockstore.TargetOverlay, error) {
	var p blockstore.TargetOverlay
	if err := json.Unmarshal(payload, &p); err != nil {
		return p, fmt.Errorf("decode target overlay for block %s (%s): %w", blockKey(b), locale, err)
	}
	return p, nil
}

// setTargetOverlay sets b's target for locale from a decoded overlay.
func setTargetOverlay(b *model.Block, locale model.LocaleID, p blockstore.TargetOverlay) {
	switch {
	case len(p.Runs) > 0:
		b.SetTargetRuns(locale, p.Runs)
	case p.Text != "":
		b.SetTargetText(locale, p.Text)
	case p.Target != "":
		b.SetTargetText(locale, p.Target)
	}
	// Carry the lifecycle status and the provenance the overlay recorded
	// (additive; absent in older overlays) onto the hydrated target, so review
	// state and the governing context that produced the answer survive the
	// extract → workspace → merge round-trip. A merge that dropped the origin
	// would deliver a file whose translations claim to have been produced under
	// no governance at all.
	if p.Status != "" || p.Origin != nil {
		if t, ok := b.TargetEdition(locale); ok {
			if p.Status != "" {
				t.Status = model.Status(p.Status)
			}
			if p.Origin != nil {
				t.Origin = *p.Origin
			}
			b.SetTargetEdition(model.Variant(locale), t)
		}
	}
}

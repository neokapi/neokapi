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
// re-runs, no network) — it only replays the work already in the workspace.
// An overlay whose source stamp names another source than the block's is a
// translation of a source that changed since: it is not applied, and Stale
// counts it.
type hydrateTargetsTool struct {
	tool.BaseTool
	locale model.LocaleID
	stale  atomic.Int64
}

// Stale is how many blocks held a stored translation of another source.
func (t *hydrateTargetsTool) Stale() int { return int(t.stale.Load()) }

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
				// and report the merge applied — the shipped-deliverable form of
				// #1449. The same distinction absorbStoreTargets now draws, and
				// the one core/convergence/coverage.go already drew.
				o, err := sess.GetOverlay(kind, key)
				if err != nil && !errors.Is(err, blockstore.ErrNotFound) {
					return fmt.Errorf("read %s overlay for block %s: %w", kind, blockKey(b), err)
				}
				if err == nil && len(o.Payload) > 0 {
					stale, aerr := applyCurrentTargetOverlay(b, b, t.locale, o.Payload)
					if aerr != nil {
						return aerr
					}
					if stale {
						t.stale.Add(1)
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
// another source is a translation of a source that has changed since: dst is
// left as it is and stale is true, for the merge to count. An overlay with no
// stamp applies as it is.
func applyCurrentTargetOverlay(read, dst *model.Block, locale model.LocaleID, payload []byte) (stale bool, err error) {
	p, err := decodeTargetOverlay(read, locale, payload)
	if err != nil {
		return false, err
	}
	if p.StaleFor(read) {
		return true, nil
	}
	setTargetOverlay(dst, locale, p)
	return false, nil
}

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

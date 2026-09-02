package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// hydrateTargetsTool is the internal merge step: for each block it reads the
// stored `targets/<locale>` overlay and applies it to the in-flight block so
// the writer emits the translated output. It recomputes nothing (no tool
// re-runs, no network) — it only replays the work already in the workspace.
type hydrateTargetsTool struct {
	tool.BaseTool
	locale model.LocaleID
}

func newHydrateTargetsTool(locale model.LocaleID) *hydrateTargetsTool {
	t := &hydrateTargetsTool{locale: locale}
	t.ToolName = "apply-targets"
	t.ToolDescription = "Apply stored target overlays onto blocks"
	return t
}

func (t *hydrateTargetsTool) SessionProcess(ctx context.Context, sess blockstore.Session, in <-chan *model.Part, out chan<- *model.Part) error {
	kind := "targets/" + string(t.locale)
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
					if aerr := applyTargetOverlay(b, t.locale, o.Payload); aerr != nil {
						return aerr
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
	var p struct {
		Runs   []model.Run   `json:"runs"`
		Text   string        `json:"text"`
		Target string        `json:"target"`
		Status string        `json:"status"`
		Origin *model.Origin `json:"origin"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("decode target overlay for block %s (%s): %w", blockKey(b), locale, err)
	}
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
		if t := b.Target(locale); t != nil {
			if p.Status != "" {
				t.Status = model.TargetStatus(p.Status)
			}
			if p.Origin != nil {
				t.Origin = *p.Origin
			}
		}
	}
	return nil
}

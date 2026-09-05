package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// commitTargetsTool is the implicit trailing step of any run against a store
// that outlives it. For every block that carries a target for the run's locale it
// writes a `targets/<locale>` overlay to the session, so a later `kapi merge`
// (which replays those overlays via the hydrate step) materializes the localized
// file.
//
// It exists because the channel-based translate tools (recycle, and any
// other capability-typed Produce BaseTool) set the target on the in-flight
// block but do NOT implement SessionTool, so without this step a run would
// discard their work when the output stream is drained — and a file-writing run
// would leave the store claiming the block was never translated. Bespoke
// SessionTools (e.g. pseudo-translate) already write their own overlay; this
// step is idempotent and simply re-affirms the same `targets/<locale>` key from
// the block's final target text, so it is safe to append unconditionally.
type commitTargetsTool struct {
	tool.BaseTool
	locale model.LocaleID
}

func newCommitTargetsTool(locale model.LocaleID) *commitTargetsTool {
	t := &commitTargetsTool{locale: locale}
	t.ToolName = "commit-targets"
	t.ToolDescription = "Commit block target text as targets/<locale> overlays"
	return t
}

func (t *commitTargetsTool) SessionProcess(ctx context.Context, sess blockstore.Session, in <-chan *model.Part, out chan<- *model.Part) error {
	kind := blockstore.TargetOverlayKind(t.locale)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-in:
			if !ok {
				return nil
			}
			if err := t.commitOne(ctx, sess, kind, part); err != nil {
				return err
			}
			select {
			case out <- part:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func (t *commitTargetsTool) commitOne(ctx context.Context, sess blockstore.Session, kind string, part *model.Part) error {
	if part == nil || part.Resource == nil {
		return nil
	}
	b, ok := part.Resource.(*model.Block)
	if !ok || b == nil || !b.Translatable || b.ID == "" {
		return nil
	}
	tgt := b.Target(t.locale)
	if tgt == nil || len(tgt.Runs) == 0 {
		return nil
	}
	key := blockstore.OverlayKey(ctx, b.ID, b.SourceText())
	overlay := blockstore.TargetOverlay{
		Runs:   tgt.Runs,
		Status: string(tgt.Status),
		Source: blockstore.SourceStamp(b.SourceText()),
		Origin: blockstore.OverlayOrigin(tgt.Origin),
	}
	// The producer's own fingerprint of what it sent, carried forward from the
	// overlay it wrote earlier in this pass. That fingerprint is what lets the
	// NEXT run serve the translation instead of paying for it again, and a
	// re-affirming write without it costs the locale a full set of provider
	// calls per run (#2356). It travels only while the text still matches: a
	// target another step has since replaced is not the one that producer made.
	if prev, perr := sess.GetOverlay(kind, key); perr == nil && len(prev.Payload) > 0 {
		var cached blockstore.TargetOverlay
		if json.Unmarshal(prev.Payload, &cached) == nil && cached.TargetText() == model.RunsText(tgt.Runs) {
			overlay.Provider, overlay.Config = cached.Provider, cached.Config
		}
	}
	payload, err := json.Marshal(overlay)
	if err != nil {
		return fmt.Errorf("commit-targets: encode overlay: %w", err)
	}
	if err := sess.PutOverlay(blockstore.Overlay{Kind: kind, BlockHash: key, Payload: payload}); err != nil {
		// A read-only store carries the target on the in-flight block already;
		// the overlay write is best-effort caching for a later merge.
		if !errors.Is(err, blockstore.ErrReadOnly) {
			return fmt.Errorf("commit-targets: write overlay: %w", err)
		}
	}
	return nil
}

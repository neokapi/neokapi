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

// targetOverlayPayload is the canonical {runs}/{text} overlay shape the hydrate
// step understands. Runs are preferred so inline markup round-trips; plain text
// is the fallback for run-free targets. Status carries the target's lifecycle
// state, which is what makes re-affirming an already-hydrated block a true no-op
// rather than a silent downgrade of its review state.
//
// Origin carries the provenance the producer stamped: how the target was made,
// and the governing context it was made under. It travels here because most
// target formats have nowhere to keep it. A plain JSON catalog holds strings
// and nothing else, so for those the overlay is the only durable record of
// what governed the answer, and the staleness gate reads its stamp from there.
type targetOverlayPayload struct {
	Runs   []model.Run   `json:"runs,omitempty"`
	Text   string        `json:"text,omitempty"`
	Status string        `json:"status,omitempty"`
	Origin *model.Origin `json:"origin,omitempty"`
}

// overlayOrigin is the provenance an overlay carries for a target, or nil when
// the producer stamped none. A pointer, so an unstamped target writes no
// `origin` key at all and reads back as the absence it is.
func overlayOrigin(o model.Origin) *model.Origin {
	if o == (model.Origin{}) {
		return nil
	}
	return &o
}

func (t *commitTargetsTool) SessionProcess(ctx context.Context, sess blockstore.Session, in <-chan *model.Part, out chan<- *model.Part) error {
	kind := "targets/" + string(t.locale)
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
	payload, err := json.Marshal(targetOverlayPayload{
		Runs:   tgt.Runs,
		Status: string(tgt.Status),
		Origin: overlayOrigin(tgt.Origin),
	})
	if err != nil {
		return fmt.Errorf("commit-targets: encode overlay: %w", err)
	}
	key := blockstore.OverlayKey(ctx, b.ID, b.SourceText())
	if err := sess.PutOverlay(blockstore.Overlay{Kind: kind, BlockHash: key, Payload: payload}); err != nil {
		// A read-only store carries the target on the in-flight block already;
		// the overlay write is best-effort caching for a later merge.
		if !errors.Is(err, blockstore.ErrReadOnly) {
			return fmt.Errorf("commit-targets: write overlay: %w", err)
		}
	}
	return nil
}

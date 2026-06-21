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

// commitTargetsTool is the implicit trailing step of a process-only run. For
// every block that carries a target for the run's locale it writes a
// `targets/<locale>` overlay to the session, so a later `kapi merge` (which
// replays those overlays via the hydrate step) materializes the localized
// file.
//
// It exists because the channel-based translate tools (tm-leverage, and any
// other capability-typed Produce BaseTool) set the target on the in-flight
// block but do NOT implement SessionTool, so without this step a process-only
// run would discard their work when the output stream is drained. Bespoke
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
// is the fallback for run-free targets.
type targetOverlayPayload struct {
	Runs []model.Run `json:"runs,omitempty"`
	Text string      `json:"text,omitempty"`
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
			if err := t.commitOne(sess, kind, part); err != nil {
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

func (t *commitTargetsTool) commitOne(sess blockstore.Session, kind string, part *model.Part) error {
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
	payload, err := json.Marshal(targetOverlayPayload{Runs: tgt.Runs})
	if err != nil {
		return fmt.Errorf("commit-targets: encode overlay: %w", err)
	}
	if err := sess.PutOverlay(blockstore.Overlay{Kind: kind, BlockHash: b.ID, Payload: payload}); err != nil {
		// A read-only store carries the target on the in-flight block already;
		// the overlay write is best-effort caching for a later merge.
		if !errors.Is(err, blockstore.ErrReadOnly) {
			return fmt.Errorf("commit-targets: write overlay: %w", err)
		}
	}
	return nil
}

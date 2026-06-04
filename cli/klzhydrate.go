package cli

import (
	"context"
	"encoding/json"

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
				if o, err := sess.GetOverlay(kind, b.ID); err == nil && len(o.Payload) > 0 {
					applyTargetOverlay(b, t.locale, o.Payload)
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
func applyTargetOverlay(b *model.Block, locale model.LocaleID, payload []byte) {
	var p struct {
		Runs   []model.Run `json:"runs"`
		Text   string      `json:"text"`
		Target string      `json:"target"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return
	}
	switch {
	case len(p.Runs) > 0:
		b.SetTargetRuns(locale, p.Runs)
	case p.Text != "":
		b.SetTargetText(locale, p.Text)
	case p.Target != "":
		b.SetTargetText(locale, p.Target)
	}
}

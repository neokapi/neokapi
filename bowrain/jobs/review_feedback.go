package jobs

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/venue"
)

// Review feedback on the platform: a unit a reviewer rejected is drafted
// again, and the draft is told what was rejected and why, as kapi tells it
// (host.reviewFeedbackFor). The rules are the tool package's: the reasons'
// wording, the note's bound and the stop after tool.MaxRejections.
//
// The ledger row keeps one decision per unit, so a platform draft is told
// about the latest rejection: its reasons, the reviewer's note, and the
// rejected wording while the block still holds it. The row's tally
// (store.RejectionTally) counts the earlier rejections and the repeats toward
// the stop. Who rejected the draft is never sent.

// reviewFeedback builds the grant a translation of b.Blocks takes under
// tool.ReviewFeedbackKey. It reads the ledger rows of b's own project and
// stream only, through the same store calls the producers grade with. A store
// without the ledger, or a read that fails, grants nothing: the translation
// then runs without feedback, as it did before.
func (b TranslateBinding) reviewFeedback(ctx context.Context) tool.ReviewFeedbackGrant {
	if len(b.Blocks) == 0 || b.TargetLocale == "" || b.ProjectID == "" {
		return tool.ReviewFeedbackGrant{}
	}
	ds, ok := b.Store.(store.DecisionStore)
	if !ok {
		return tool.ReviewFeedbackGrant{}
	}
	stream := b.stream()
	records, err := ds.ListUnitDecisions(ctx, b.ProjectID, stream)
	if err != nil {
		slog.WarnContext(ctx, "read the decision ledger; drafting without review feedback",
			"project", b.ProjectID, "stream", stream, "error", err)
		return tool.ReviewFeedbackGrant{}
	}
	variant := variantText(model.Variant(b.TargetLocale))
	rejected := map[decisionUnitKey]venue.UnitDecision{}
	for _, d := range records {
		if d.Variant == variant && d.ReviewState == venue.ReviewStateRejected {
			rejected[decisionUnitKey{item: d.ItemName, unit: d.Unit, variant: d.Variant}] = d
		}
	}
	if len(rejected) == 0 {
		return tool.ReviewFeedbackGrant{}
	}
	tallies := map[decisionUnitKey]store.RejectionTally{}
	rl, counts := b.Store.(store.RejectionLedger)
	if counts {
		list, err := rl.ListRejectionTallies(ctx, b.ProjectID, stream)
		if err != nil {
			slog.WarnContext(ctx, "read the rejection tallies; drafting without the stop",
				"project", b.ProjectID, "stream", stream, "error", err)
		}
		for _, t := range list {
			tallies[decisionUnitKey{item: t.ItemName, unit: t.Unit, variant: t.Variant}] = t
		}
	}

	units := tool.ReviewFeedback{}
	for _, sb := range b.Blocks {
		if sb == nil || sb.Block == nil || sb.SourceID == "" || sb.ItemName == "" {
			continue
		}
		key := decisionUnitKey{item: sb.ItemName, unit: sb.SourceID, variant: variant}
		d, ok := rejected[key]
		if !ok {
			continue
		}
		fb, ok := unitFeedback(d, tallies[key], sb, b.TargetLocale)
		if !ok {
			continue
		}
		fb.Repeat = encodeRepeat(store.DraftBasis{ItemName: sb.ItemName, Unit: sb.SourceID, Variant: variant, Basis: sb.SourceRevision})
		units[tool.FeedbackUnitKey("", sb.Block.UnitKey())] = fb
	}
	if len(units) == 0 {
		return tool.ReviewFeedbackGrant{}
	}
	grant := tool.ReviewFeedbackGrant{Units: units}
	if counts {
		projectID := b.ProjectID
		grant.Repeated = func(repeat string) {
			draft, ok := decodeRepeat(repeat)
			if !ok {
				return
			}
			if err := rl.RecordRepeatedDrafts(ctx, projectID, stream, []store.DraftBasis{draft}); err != nil {
				slog.WarnContext(ctx, "count a draft that repeated a rejected one; the unit's stop may come a draft later",
					"project", projectID, "stream", stream, "unit", draft.Unit, "error", err)
			}
		}
	}
	return grant
}

// unitFeedback is what one unit's next draft is told: the latest rejection,
// while it judged a draft of the source the block holds now, or of no named
// source (a translation written outside kapi, which the rejection lowers all
// the same). A rejection of a draft of an earlier source says nothing about
// the source the block holds now.
func unitFeedback(d venue.UnitDecision, tally store.RejectionTally, sb *venue.StoredBlock, locale model.LocaleID) (tool.UnitReviewFeedback, bool) {
	if d.Basis != "" && d.Basis != sb.SourceRevision {
		return tool.UnitReviewFeedback{}, false
	}
	r := tool.ReviewRejection{
		Reasons: d.Reasons,
		Note:    venue.SplitNote(d).Note,
	}
	// The rejected wording is sent while the block still holds that draft;
	// its hash is what lets the tool tell a draft that repeats it.
	runs := localeTargetRuns(sb.Block, locale)
	if text := model.RunsText(runs); text != "" && d.Revision != "" && store.VariantRevision(d.Variant, runs) == d.Revision {
		r.Text = text
		r.TargetHash = state.TargetHash(text)
	}
	return tool.UnitReviewFeedback{
		Rejections: []tool.ReviewRejection{r},
		Held:       tally.Holds(sb.SourceRevision),
	}, true
}

// passRepeats is one pass's record of the units whose draft came back in
// wording a reviewer had rejected. The tool keeps no such draft, so the unit
// is still owed: the pass stamps no draft mark on it, and the next pass drafts
// it again until the tally reaches the stop.
type passRepeats struct {
	mu    sync.Mutex
	units map[decisionUnitKey]bool
}

// watchRepeats wraps the grant's Repeated so the pass also records each unit
// it hears of. The store's own count is unchanged.
func watchRepeats(grant *tool.ReviewFeedbackGrant) *passRepeats {
	r := &passRepeats{units: map[decisionUnitKey]bool{}}
	if len(grant.Units) == 0 {
		return r
	}
	next := grant.Repeated
	grant.Repeated = func(repeat string) {
		if d, ok := decodeRepeat(repeat); ok {
			r.mu.Lock()
			r.units[decisionUnitKey{item: d.ItemName, unit: d.Unit, variant: d.Variant}] = true
			r.mu.Unlock()
		}
		if next != nil {
			next(repeat)
		}
	}
	return r
}

// count is how many units' drafts repeated a rejected one in this pass.
func (r *passRepeats) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.units)
}

// without returns blocks less those whose draft repeated a rejected one.
func (r *passRepeats) without(blocks []*model.Block, byBlockID map[string]*venue.StoredBlock, locale model.LocaleID) []*model.Block {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.units) == 0 {
		return blocks
	}
	variant := variantText(model.Variant(locale))
	out := make([]*model.Block, 0, len(blocks))
	for _, b := range blocks {
		if b != nil {
			if sb := byBlockID[b.ID]; sb != nil && r.units[decisionUnitKey{item: sb.ItemName, unit: sb.SourceID, variant: variant}] {
				continue
			}
		}
		out = append(out, b)
	}
	return out
}

// encodeRepeat names a unit's repeated draft as tool.UnitReviewFeedback.Repeat,
// which the tool hands back unread.
func encodeRepeat(d store.DraftBasis) string {
	b, _ := json.Marshal(d)
	return string(b)
}

func decodeRepeat(repeat string) (store.DraftBasis, bool) {
	var d store.DraftBasis
	if err := json.Unmarshal([]byte(repeat), &d); err != nil || d.ItemName == "" || d.Unit == "" || d.Variant == "" {
		return store.DraftBasis{}, false
	}
	return d, true
}

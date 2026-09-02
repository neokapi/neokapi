package host

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
)

// The basis of an undecided translation.
//
// A decision carries the source it blessed, so a source rewrite under a decided
// translation is derived on read: the record's basis no longer matches the
// wording in front of the reader, the unit reads stale, coverage withholds the
// scope, and the loop re-drafts it. An undecided translation needs the same
// anchor, and nothing else durable provides one: the project block store
// remembers the previous read, but it is derived state outside git, so on a
// fresh clone the record absorber would pair a committed target with whatever
// sentence now sits beside it and recycle the old translation back over itself.
//
// So the loop records a basis for every unit it writes a target for: a
// state.UnitState with the source it translated and the translation it produced,
// and no Decision on it. Source drift on an undecided unit is then derived
// exactly as it is on a decided one, by the same comparison in the same place,
// and it survives a clone because the state record is committed.
//
// Two boundaries hold this to the units it is entitled to speak for:
//
//   - Only units in a target file THIS RUN wrote. A translation that was in the
//     tree before kapi ever ran has no record, grades basisNone, and is never
//     re-drafted. The loop does not get to claim authorship of somebody's work
//     by reading it.
//   - Never over a decision. A decision's basis is the decision's, and
//     overwriting it with the pass's own would silently restore an approval the
//     source edit withdrew.

// recordedBasisStatus is the rung a loop-written basis records. It is the
// presence baseline (a target exists) and never a decision: `draft` is what a
// rejection is recorded as, and writing it here would read every translation the
// loop produced as one somebody had turned down.
const recordedBasisStatus = model.TargetStatusTranslated

// writtenTargets is the set of target files a convergence run put its own output
// in, per locale.
type writtenTargets map[string]map[string]bool

// note records that the run wrote path for locale.
func (w writtenTargets) note(locale, path string) {
	if w == nil || locale == "" || path == "" {
		return
	}
	paths, ok := w[locale]
	if !ok {
		paths = map[string]bool{}
		w[locale] = paths
	}
	paths[path] = true
}

// wrote reports whether the run wrote this locale's copy of a target file.
func (w writtenTargets) wrote(locale, path string) bool {
	return w != nil && w[locale][path]
}

// producedTarget answers "did this run write the translation now sitting in this
// file". A convergence pass writes a locale's whole target document, so the
// answer is per file rather than per unit; the two runs of the loop that write
// answer it differently. Under a gate the pass drafts and only a locale that
// cleared its gate is delivered, so the delivered destinations are the answer.
// Ungated, the pass writes where the recipe points, so having run for the locale
// at all is.
type producedTarget func(locale, targetPath string) bool

// recordProducedBasis writes the basis for every unit whose translation this run
// produced, and makes the records durable.
//
// Best-effort in the same sense stampCommittedRecord is: it is how the NEXT run
// sees drift, so failing to write it costs a run's visibility, never the run's
// output. A run that fails here reports what it did and the following one
// records the basis it could not.
func (a *App) recordProducedBasis(ctx context.Context, proj *project.KapiProject, root string, produced producedTarget) error {
	if produced == nil {
		return nil
	}
	units, err := a.UnitsFromProject(proj, root, "")
	if err != nil {
		return fmt.Errorf("resolve project content: %w", err)
	}
	st, err := a.OpenProjectState(ctx, root)
	if err != nil {
		return err
	}
	docs := a.documentIndexOrEmpty(ctx, root)
	now := nowRFC3339()
	recorded := 0

	for _, u := range units {
		if !produced(u.Locale, u.TargetPath) {
			continue
		}
		blocks, missing, berr := a.bilingualBlocks(ctx, u)
		if berr != nil || missing {
			// An unreadable target (a compiled catalog) carries no per-unit
			// pairing to record, and a missing one carries no translation at
			// all. Neither is a run failure: coverage already reports both.
			continue
		}
		scope := docs.Scope(root, u.SourcePath)
		loc := model.LocaleID(u.Locale)
		for _, b := range blocks {
			if !b.Translatable {
				continue
			}
			target := b.TargetText(loc)
			if !model.RunsHaveContent(b.TargetRuns(loc)) {
				// Nothing was produced for this unit. Recording a basis would
				// claim a translation the file does not hold.
				continue
			}
			k := state.Key{Scope: scope, Unit: blockKey(b), Variant: model.Variant(loc)}
			prev, hadPrev := st.Get(ctx, k)
			if hadPrev && prev.Decision.ReviewState != "" {
				// The unit is decided. Its basis is the decision's, and the
				// decision is what a source edit has to invalidate.
				continue
			}
			th := targetHash(target)
			ch := state.SourceHash(b.SourceText())
			if hadPrev && prev.TargetHash == th && prev.ContentHash == ch {
				continue // already recorded for this exact pairing
			}
			next := state.UnitState{
				Unit:        blockKey(b),
				Variant:     model.Variant(loc),
				Status:      recordedBasisStatus,
				TargetHash:  th,
				ContentHash: ch,
				Updated:     now,
				Scope:       scope,
			}
			if hadPrev {
				// Advisory state rides along, exactly as it does across a
				// decision write: a run producing a translation is not a reason
				// to forget what was noted about the unit.
				next.Origin = prev.Origin
				next.SourceStatus = prev.SourceStatus
				next.ContextHash = prev.ContextHash
				if prev.AIReview.Fresh(th) {
					next.AIReview = prev.AIReview
				}
			}
			if perr := st.Record(ctx, next); perr != nil {
				return perr
			}
			recorded++
		}
	}
	if recorded == 0 {
		return nil
	}
	// The loop's own output, made durable by the loop. It is not staged, so it
	// never reads as a decision somebody owes a review, and the committed shards
	// it lands in are what carry the basis to the next clone.
	return st.PersistRecords(ctx)
}

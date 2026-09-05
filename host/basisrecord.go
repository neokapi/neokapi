package host

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/neokapi/neokapi/core/blockstore"
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
	origins := a.newProducedOrigins(ctx, root)
	defer origins.close()
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
			// What governed the translation now sitting in the file. The
			// producer's own stamp where it survives, and prev's where the run
			// produced nothing this record can speak for: a unit whose target
			// was written before the stamp existed keeps reading as unstamped
			// rather than acquiring a governance claim from a neighbour.
			origin := prev.Origin
			if o, ok := origins.at(u, b, loc); ok {
				origin = o
			}
			if hadPrev && prev.TargetHash == th && prev.ContentHash == ch && origin == prev.Origin &&
				prev.GoverningFingerprint == origin.ContextFingerprint {
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
				Origin:      origin,
				// What governed the translation is the producer's own stamp:
				// the run that wrote the target is the run recording it, so the
				// context in force at the write is the context it was produced
				// under. The record carries it as its own field because that is
				// where a reader of a JSON catalog or a .properties file finds
				// it (the file holds strings and nothing else), and where a
				// decision made later records its own.
				GoverningFingerprint: origin.ContextFingerprint,
			}
			if hadPrev {
				// Advisory state rides along, exactly as it does across a
				// decision write: a run producing a translation is not a reason
				// to forget what was noted about the unit.
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

// The provenance half of the record.
//
// A basis says which source a translation renders. The staleness gate asks a
// different question, what governed the answer when it was produced, and it
// asks it of the same store, so the record has to carry the producer's Origin
// as well. Every translation producer stamps one on the block it writes
// (core/ai/tools, core/mt/tools, recycle), but only a bilingual target format
// keeps it: a JSON catalog holds strings, so re-reading the delivered file
// finds a translation with no statement about what shaped it, and the gate had
// nothing to compare (#2344).
//
// So the origin comes from the file where the file carries one, and otherwise
// from the run's own `targets/<locale>` overlay in the project block store,
// which every convergence writes through the implicit commit-targets step and
// which carries the stamp beside the words.

// producedOrigins answers "what governed the translation this file now holds",
// per (unit, block, locale), for a run that has just finished writing.
//
// The block store session is opened on first use and held for the pass. A
// project with no store (the browser build, a run outside a project) has no
// fallback, and every unit is answered by the file itself.
type producedOrigins struct {
	ctx   context.Context
	root  string
	store blockstore.Store
	sess  blockstore.Session
	tried bool
}

func (a *App) newProducedOrigins(ctx context.Context, root string) *producedOrigins {
	return &producedOrigins{ctx: ctx, root: root, store: a.openProjectBlockStore(ctx)}
}

func (p *producedOrigins) close() {
	if p != nil && p.sess != nil {
		_ = p.sess.Close()
		p.sess = nil
	}
}

// session opens the block store session once, and reports nil when the project
// has no readable store.
func (p *producedOrigins) session() blockstore.Session {
	if p.sess != nil || p.tried {
		return p.sess
	}
	p.tried = true
	if p.store == nil {
		return nil
	}
	sess, err := p.store.Begin(p.ctx)
	if err != nil {
		return nil
	}
	p.sess = sess
	return p.sess
}

// at returns the origin to record for one block's target, and whether one was
// found at all. A false answer leaves the previous record's origin standing,
// which is the difference between "this run has nothing to say about the
// provenance here" and "this target was produced under no governance".
func (p *producedOrigins) at(u VerifyUnit, b *model.Block, locale model.LocaleID) (model.Origin, bool) {
	if t := b.Target(locale); t != nil && t.Origin != (model.Origin{}) {
		// The delivered artifact's own stamp. It is what absorbCommittedRecord
		// reads, and a format that keeps it makes the most direct statement
		// there is about the file in front of the reader.
		return t.Origin, true
	}
	sess := p.session()
	if sess == nil || b.ID == "" {
		return model.Origin{}, false
	}
	// The same key the run wrote under: the source file's project-relative
	// namespace plus the file-local block id (blockstore.StoreKey), which is
	// what core/flow's runner tags each file's context with.
	key := blockstore.StoreKey(blockstore.SourceNamespace(p.root, u.SourcePath), b.ID, b.SourceText())
	o, err := sess.GetOverlay(blockstore.TargetOverlayKind(locale), key)
	if err != nil || len(o.Payload) == 0 {
		return model.Origin{}, false
	}
	var payload blockstore.TargetOverlay
	if json.Unmarshal(o.Payload, &payload) != nil || payload.Origin == nil {
		return model.Origin{}, false
	}
	// Only when the overlay describes the words the file actually holds. An
	// overlay outlives the run that wrote it, so a target somebody edited by
	// hand since would otherwise be recorded as machine-produced under the
	// context of a run that never wrote those words.
	if payload.TargetText() != b.TargetText(locale) {
		return model.Origin{}, false
	}
	return *payload.Origin, true
}

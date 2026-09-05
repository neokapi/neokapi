package host

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/state"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/memory"
)

// The committed record is the other half of the rebuild AD-009 promises: the
// content memory reconstructs from "the committed translations plus an optional
// read-only seed", and a seed is only the accelerant. Compiling the seeds alone
// leaves every pair that a venue approved after the seed was written living in
// exactly one place — the per-locale target artifact in git — with nothing that
// reads it back. The next convergence from a cold store then produces less than
// the artifact it overwrites, and only a report notices.
//
// So after the seeds compile, the target documents themselves are absorbed: each
// committed target is paired with its source through the collection's own
// binding — the same reader, the same format config — and the resulting
// source→target pairs are written into the memory in one bulk transaction with
// one index rebuild, the same write discipline the merge path uses.
//
// Digest-keyed like the seeds: a target whose bytes have not moved since it was
// last absorbed costs two reads and no writes.

// MetaRecordDigests is the store-metadata key holding the digest each committed
// target artifact carried when its pairs were last absorbed — a JSON object of
// project-relative slash path to digest.
const MetaRecordDigests = "context.recordDigests"

// MetaRecordScheme is the store-metadata key holding the identity the
// absorber's own entries were last written under. A store written under an
// older one is re-learned rather than migrated: everything the absorber taught
// the corpus is, by construction, reproducible from the committed translations
// it read — that IS what this file does — so re-reading them costs one pass and
// carries no risk of a half-converted row.
const MetaRecordScheme = "context.recordScheme"

// recordSchemeCurrent names the identity in force: an entry per (source,
// approval point).
const recordSchemeCurrent = "point"

// recordOriginSource marks a content-memory origin whose pair was read back out
// of a committed target artifact, as opposed to a seed bundle (whose origins the
// bundle carries) or an approval promoted by `kapi apply` (Source "apply").
const recordOriginSource = "record"

// RecordAbsorbResult reports what one record-absorbing pass did.
type RecordAbsorbResult struct {
	// Documents and Pairs count the committed target artifacts read and the
	// source→target pairs they carried.
	Documents int `json:"documents,omitempty"`
	Pairs     int `json:"pairs,omitempty"`
	// Learned counts the new content-memory entries written; Reconciled counts
	// the entries already in the store whose target disagreed with the committed
	// record and was corrected to it. Both count entries, not pairs: a source
	// answered in several locales is one entry carrying a variant each.
	Learned    int `json:"learned,omitempty"`
	Reconciled int `json:"reconciled,omitempty"`
	// Refused counts pairs whose target does not carry its source's
	// placeholders, in either spelling — an inline code the reader lifted out
	// of the text, or an interpolation token left in it. Such a pair can only
	// produce a translation with a hole in it, so it is not absorbed.
	Refused int `json:"refused,omitempty"`
	// Contested counts source texts the record answers more than one way, and
	// ContestedSources names them: each answer, and the point it was approved
	// at. A count alone says a disagreement exists and leaves nobody able to
	// look at it; the resolver's choices are only auditable if the choices are
	// on the page.
	Contested        int               `json:"contested,omitempty"`
	ContestedSources []ContestedSource `json:"contestedSources,omitempty"`
	// Superseded counts pairs the project's own record says do not go together:
	// the committed target translates source wording that has since been
	// rewritten, so it is not a translation of the sentence now beside it. Such
	// a pair is absorbed against the source it actually translates when that
	// wording is still recoverable, and otherwise not at all — never against the
	// source now sitting beside it.
	Superseded int `json:"superseded,omitempty"`
	// Skipped counts target artifacts already absorbed at their current digest.
	Skipped int `json:"skipped,omitempty"`
	// Unreadable counts target artifacts whose format could not be read back
	// (a compiled catalog), which carry no pairs to absorb.
	Unreadable int `json:"unreadable,omitempty"`
}

// Absorbed reports whether the pass wrote anything into the store.
func (r RecordAbsorbResult) Absorbed() bool { return r.Learned > 0 || r.Reconciled > 0 }

// recordUnit is one (source document, committed target document, locale) the
// record absorber pairs.
type recordUnit struct {
	sourcePath, sourceRel string
	// scope is the document the source file IS — its durable key, or its path
	// where the project holds none. sourceRel is its ADDRESS, which is what the
	// block store places a block by; the two are the same string until a file
	// is renamed, and telling them apart is the whole point of holding both.
	scope                 string
	targetPath, targetRel string
	format                string
	config                map[string]any
	locale                model.LocaleID
	// point is where the source document sits in the project's context space —
	// the product, the channel and the collection that claim it. It is the
	// approval point of every pair this unit carries: a committed translation
	// was reviewed where its document lives, and that is what qualifies it
	// against a different answer approved somewhere else.
	point string
}

// recordAnswer is one answer the record gives for a source: the target wording,
// and the context point it was approved at.
type recordAnswer struct {
	runs  []model.Run
	text  string // the normalized target text, the answer's identity
	point string
	order int // first occurrence in the recipe's stable content order
}

// recordPair collects every answer the committed record gives for one source
// content key, in one locale.
type recordPair struct {
	order      int
	locale     model.LocaleID
	sourceRuns []model.Run
	// unit is the block this pairing was read off, so the entry it writes joins
	// that block's version chain rather than standing alone as a source string
	// somebody once approved. Empty when the format resolved no durable
	// identity, which is a true statement about the answer: nothing links it to
	// what came before.
	unit string
	// answers is keyed by (point, target text): one point can answer a source
	// twice — two keys of one catalog holding the same English and different
	// Norwegian — and two points can give the same answer.
	answers map[string]*recordAnswer
	origin  memory.Origin
}

// answerKey is the identity of one answer: the point that approved it and the
// wording it approved.
func answerKey(point, text string) string { return point + "\x00" + text }

// contested reports whether the record gives this source more than one answer.
// Two points giving the SAME wording are not a disagreement — the pick does not
// matter, and splitting them would put a copy of one answer in the store per
// collection that carries the string.
func (p *recordPair) contested() bool {
	var first *recordAnswer
	for _, a := range p.answers {
		if first == nil {
			first = a
			continue
		}
		if a.text != first.text {
			return true
		}
	}
	return false
}

// ordered returns the pair's answers in the recipe's stable content order, so
// every choice made over them is a function of the record and not of map
// iteration.
func (p *recordPair) ordered() []*recordAnswer {
	out := make([]*recordAnswer, 0, len(p.answers))
	for _, a := range p.answers {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].order < out[j].order })
	return out
}

// points returns the distinct context points the record answered this source
// at, in the order they were first seen — one entry per point is what lets a
// contested source keep every approval instead of collapsing to one.
func (p *recordPair) points() []string {
	var out []string
	seen := map[string]bool{}
	for _, a := range p.ordered() {
		if seen[a.point] {
			continue
		}
		seen[a.point] = true
		out = append(out, a.point)
	}
	return out
}

// resolve returns the answer that governs at the given point: the approval
// nearest it on the containment ladder, ties broken by the answer's own text
// (memory.NearerAnswer). Nil only for a pair with no answers at all.
func (p *recordPair) resolve(at string) *recordAnswer {
	var best *recordAnswer
	for _, a := range p.ordered() {
		if best == nil || memory.NearerAnswer(a.point, a.text, best.point, best.text, at) {
			best = a
		}
	}
	return best
}

// listContested renders the pair as the contested source a reader audits: every
// answer, where it was approved, and which one governs there.
func (p *recordPair) listContested() ContestedSource {
	c := ContestedSource{Source: model.FlattenRuns(p.sourceRuns), Locale: p.locale}
	for _, a := range p.ordered() {
		c.Answers = append(c.Answers, ContestedAnswer{
			Target:  model.FlattenRuns(a.runs),
			Point:   displayPoint(a.point),
			Governs: p.resolve(a.point) == a,
		})
	}
	sort.Slice(c.Answers, func(i, j int) bool {
		if c.Answers[i].Point != c.Answers[j].Point {
			return c.Answers[i].Point < c.Answers[j].Point
		}
		return c.Answers[i].Target < c.Answers[j].Target
	})
	return c
}

// ContestedSource is one source string the committed record answers more than
// one way, with every answer and the point that approved it.
type ContestedSource struct {
	Source  string            `json:"source"`
	Locale  model.LocaleID    `json:"locale"`
	Answers []ContestedAnswer `json:"answers"`
}

// ContestedAnswer is one of those answers.
type ContestedAnswer struct {
	Target string `json:"target"`
	// Point is where the answer was approved, as the recipe writes it
	// (`profile/channel/collection`). Empty at the project's default point.
	Point string `json:"point,omitempty"`
	// Governs reports that this answer is the one the resolver hands back at
	// its own point — false for an answer another approval outranks even there,
	// which is what a genuine tie at one point looks like.
	Governs bool `json:"governs,omitempty"`
}

// displayPoint renders a context point the way a recipe writes the binding it
// came from: `profile/channel/collection`, with the rungs nothing declared
// dropped. The stored form is separator-delimited so a rung may hold any name;
// this is the form a reader sees.
func displayPoint(point string) string {
	var rungs []string
	for i := range memory.PointRungs {
		if r := memory.PointRung(point, i); r != "" {
			rungs = append(rungs, r)
		}
	}
	return strings.Join(rungs, "/")
}

// absorbCommittedRecord reads every committed target artifact the recipe binds,
// pairs it with its source through the collection's own format binding, and
// writes the pairs into the project content memory.
//
// The digest stamp decides more than cost here: it decides who spoke last. An
// artifact whose bytes have not moved since it was absorbed has already had its
// say, and the store may legitimately have moved on since — a recompiled seed, a
// venue pull, an approval. Re-asserting the unchanged artifact over that later
// state would be the very erasure this reads the record to prevent. So a fresh
// clone (no stamps at all) absorbs everything and the record supersedes the
// seeds compiled moments earlier, while a later run only ever applies the
// artifacts that actually changed.
func (a *App) absorbCommittedRecord(ctx context.Context, db *projectdb.DB, proj *project.KapiProject, projectPath string, layout project.Layout) (RecordAbsorbResult, error) {
	var res RecordAbsorbResult
	tm := db.Memory()
	if tm == nil || a.FormatReg == nil {
		// No file-backed content memory in this build (the browser build), or no
		// format registry to read the documents with — the same posture the seed
		// phase takes: nothing to project into, and no stamp recorded, so the
		// record absorbs the first time a real store is there.
		return res, nil
	}

	pctx := project.NewProjectContext(proj, projectPath)
	sourceLocale := pctx.SourceLocale
	if sourceLocale == "" {
		sourceLocale = model.LocaleID("en")
	}
	units, err := recordUnits(ctx, a, proj, pctx, layout.Root)
	if err != nil {
		return res, err
	}
	if len(units) == 0 {
		return res, nil
	}

	// The decisions the project holds, so a pair the record's own basis
	// contradicts is not written into the memory as if it were a translation of
	// the source beside it. A state store that cannot be read yields an empty
	// index — no decisions, so no pair is contradicted — which is the same
	// posture every other reader of it takes.
	reviewed, rerr := a.loadReviewedCorrections(ctx, proj, layout.Root)
	if rerr != nil {
		return res, rerr
	}
	prior := newPriorSourceIndex(ctx, db)
	// What the corpus already answers, so a rewritten unit's committed target can
	// be recognized as the loop's own last output for the wording that is gone.
	corpus := &memoryAnswers{ctx: ctx, tm: tm, source: sourceLocale, cache: map[string][]memory.Entry{}}

	if err := relearnRecordIfRekeyed(ctx, db, tm); err != nil {
		return res, err
	}

	stamps := loadRecordDigests(ctx, db)
	next := make(map[string]string, len(units))
	pairs := map[string]*recordPair{}
	order := 0

	for _, u := range units {
		digest, ok, derr := recordDigest(u)
		if derr != nil {
			return res, derr
		}
		if !ok {
			continue // no committed target for this locale yet
		}
		next[u.targetRel] = digest
		if stamps[u.targetRel] == digest {
			res.Skipped++
			continue
		}
		blocks, uerr := a.pairedRecordBlocks(ctx, u, sourceLocale)
		if uerr != nil {
			if errors.Is(uerr, errTargetUnreadable) {
				// A target that cannot be parsed back (a compiled catalog)
				// carries no pairs. Stamp it anyway: re-reading it every run
				// would re-fail every run.
				res.Unreadable++
				continue
			}
			return res, uerr
		}
		res.Documents++
		for _, b := range blocks {
			e, basis, applies := reviewed.grade(u.scope, b, string(u.locale))
			srcRuns, tgtRuns, keep := recordRuns(b, u.locale, approvesTarget(e, applies))
			if !keep {
				continue
			}
			// A pairing the project's own record contradicts. This unit's key
			// survived a source rewrite, so its translation still sits beside a
			// sentence it was never a translation of — and reading that adjacency
			// as a pair is what taught the memory an exact answer for the NEW
			// wording, so every `kapi up` recycled the stale target back over
			// itself and reported the drift it had just confirmed.
			//
			// The pair is not simply dropped. The wording it does translate is
			// still in the block store (this runs before the pass re-extracts),
			// and recovering it is not a guess. Kept under that source it stays
			// reachable as leverage for the rewrite, which is what stops a
			// one-word source edit from throwing a careful translation away.
			// Unrecoverable — nothing extracted yet, or a store already re-read —
			// means nothing is written: a memory with one fewer entry costs a
			// lookup, an entry asserting a translation of the wrong sentence
			// costs the translation.
			//
			// Two things say the pairing is superseded, and a unit needs only one
			// of them.
			blessed, superseded, serr := supersededSource(b, u, e, basis, srcRuns, tgtRuns, prior, corpus)
			if serr != nil {
				return res, serr
			}
			if superseded {
				res.Superseded++
				if blessed == nil {
					continue
				}
				srcRuns = blessed
			}
			if !model.DiffRunCodes(srcRuns, tgtRuns).Balanced() ||
				!coretools.PlaceholdersCarried(model.RunsText(srcRuns), model.RunsText(tgtRuns)) {
				// The three-layer protection holds here too: an asymmetric pair
				// is one side of which has already lost a placeholder, and
				// recycle would refuse to fill from it anyway.
				//
				// Both spellings of a placeholder are asked about, because a
				// surface carries them one way or the other and the committed
				// record holds every surface. The JSX extraction lifts `{count}`
				// into an inline code, which DiffRunCodes reads; a plain JSON or
				// gettext catalog leaves it in the text, where only the text
				// comparison can see it. Reading one and not the other is how a
				// help string that gained `--target` documentation kept the
				// translation of the sentence before it: the pair paired by
				// scope, absorbed at full score against wording it does not
				// translate, and recycle filled it back over itself every run.
				res.Refused++
				continue
			}
			res.Pairs++
			key := recordKey(u.locale, srcRuns)
			p, ok := pairs[key]
			if !ok {
				order++
				p = &recordPair{
					order:      order,
					locale:     u.locale,
					sourceRuns: srcRuns,
					unit:       recordChainUnit(b),
					answers:    map[string]*recordAnswer{},
					origin: memory.Origin{
						Source:    recordOriginSource,
						Key:       u.targetRel,
						Reference: u.sourceRel,
						AddedBy:   "kapi-up",
						// What governed the answer. It travels with the answer
						// because reuse is only safe under the rules the answer
						// was approved under.
						ContextFingerprint: governingFingerprintOf(b, u.locale, reviewed, u.scope),
					},
				}
				pairs[key] = p
			}
			text := memory.NormalizeText(model.FlattenRuns(tgtRuns))
			if _, ok := p.answers[answerKey(u.point, text)]; !ok {
				order++
				p.answers[answerKey(u.point, text)] = &recordAnswer{
					runs: tgtRuns, text: text, point: u.point, order: order,
				}
			}
		}
	}

	if err := a.writeRecordPairs(ctx, tm, pairs, sourceLocale, &res); err != nil {
		return res, err
	}
	if err := saveRecordDigests(ctx, db, next); err != nil {
		return res, err
	}
	return res, nil
}

// stampCommittedRecord records every target artifact's current digest as
// absorbed, for a run that just wrote them itself.
//
// Without it a convergence launders its own output back in: the targets it
// materializes are, to the next run, artifacts it has never seen, so they read
// as a statement from git and are absorbed over whatever the store learned in
// the meantime — a seed edit pulled since, an approval. What the loop wrote is
// by definition already in the store; only a target that changed OUTSIDE the
// loop carries anything new.
//
// Best-effort: the stamp is an optimization of correctness, not its carrier, and
// a store that cannot record it re-absorbs its own output next run.
func (a *App) stampCommittedRecord(ctx context.Context, proj *project.KapiProject, projectPath string) {
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return
	}
	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil || db.Memory() == nil {
		return
	}
	units, err := recordUnits(ctx, a, proj, project.NewProjectContext(proj, projectPath), layout.Root)
	if err != nil {
		return
	}
	stamps := loadRecordDigests(ctx, db)
	for _, u := range units {
		digest, ok, derr := recordDigest(u)
		if derr != nil || !ok {
			continue
		}
		stamps[u.targetRel] = digest
	}
	_ = saveRecordDigests(ctx, db, stamps)
}

// recordSettlement is the set of committed target artifacts the absorber has
// already had its say about at the bytes they hold now — the ones the next pass
// stamps as skipped. It is read-only: the stamps and a digest, no store write.
//
// The plan needs it to read the content memory's silence. What a run spends a
// provider call on is a unit the corpus does not answer, but the corpus is only
// finished being taught once the absorber has read the artifacts: before that,
// silence about a produced unit means "not asked yet", and pricing it would quote
// a provider call for every translation the run is about to recycle. After it,
// silence is the absorber's answer — a pairing it declined — and the pass will
// draft the unit.
//
// Keyed by absolute target path, expanded exactly as recordUnits and
// UnitsFromProject both expand it, so the two resolutions cannot drift apart.
func (a *App) recordSettlement(ctx context.Context, db *projectdb.DB, proj *project.KapiProject, projectPath, root string) map[string]bool {
	settled := map[string]bool{}
	if db == nil {
		return settled
	}
	stamps := loadRecordDigests(ctx, db)
	if len(stamps) == 0 {
		return settled
	}
	units, err := recordUnits(ctx, a, proj, project.NewProjectContext(proj, projectPath), root)
	if err != nil {
		return settled
	}
	for _, u := range units {
		digest, ok, derr := recordDigest(u)
		if derr != nil || !ok {
			continue
		}
		if stamps[u.targetRel] == digest {
			settled[u.targetPath] = true
		}
	}
	return settled
}

// writeRecordPairs resolves each source's winning target, reconciles the entries
// already in the store that answer the same source differently, and writes the
// whole set in one transaction followed by one index rebuild.
func (a *App) writeRecordPairs(ctx context.Context, tm *memory.SQLiteStore, pairs map[string]*recordPair, sourceLocale model.LocaleID, res *RecordAbsorbResult) error {
	if len(pairs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return pairs[keys[i]].order < pairs[keys[j]].order })

	now := time.Now().UTC()
	var write []memory.Entry
	// One staged copy per entry id. An entry can be reached by two locales' pairs
	// — the same source string, a target in each — and each read returns the
	// store's PRE-RUN state. Appending both would write two copies whose locale
	// sets disagree, and since a write replaces variants per locale, the second
	// would put the first locale back the way it was. Correcting one staged copy
	// keeps both.
	staged := map[string]int{}
	stage := func(e memory.Entry) (entry *memory.Entry, fresh bool) {
		if i, ok := staged[e.ID]; ok {
			return &write[i], false
		}
		staged[e.ID] = len(write)
		write = append(write, e)
		return &write[len(write)-1], true
	}

	for _, k := range keys {
		p := pairs[k]
		existing, err := tm.FullScoreEntries(ctx, p.sourceRuns, sourceLocale)
		if err != nil {
			return fmt.Errorf("read content-memory entries for a committed pair: %w", err)
		}

		// The ordinary case: the record answers this source one way, wherever it
		// asked. One entry, bound to no point, exactly as before — an answer
		// every point agrees on is not a decision about a place, and giving it
		// one would put a copy of it in the store per collection that carries
		// the string.
		if !p.contested() {
			win := p.resolve("")
			held := false
			for _, e := range existing {
				// An entry with no variant in this locale is not a candidate the
				// lookup can return, so it is not part of the disagreement and is
				// left alone.
				if !e.HasLocale(p.locale) {
					continue
				}
				if memory.NormalizeText(e.VariantText(p.locale)) == win.text {
					held = true
					continue
				}
				// Full score, same source, different target: the committed
				// record is the later, reviewed answer, so the entry is
				// corrected to it rather than left to compete. Left competing,
				// the ambiguity rule (AD-009) would demote both and a full-score
				// fill policy would take neither — the disagreement would cost
				// the translation instead of resolving it.
				s, fresh := stage(e)
				s.Variants[p.locale] = win.runs
				s.Origins = withRecordOrigin(s.Origins, p.origin, now)
				s.UpdatedAt = now
				if fresh {
					res.Reconciled++
				}
				held = true
			}
			if held {
				continue
			}
			origin := p.origin
			origin.AddedAt = now
			// The id keys on the source alone, so a second locale for the same
			// source folds into the entry the first one staged rather than
			// opening a rival.
			s, fresh := stage(memory.Entry{
				ID:          recordEntryID(recordSourceKey(p.sourceRuns), ""),
				HintSrcLang: sourceLocale,
				Variants:    map[model.LocaleID][]model.Run{sourceLocale: p.sourceRuns},
				Origins:     []memory.Origin{origin},
				Unit:        p.unit,
				CreatedAt:   now,
				UpdatedAt:   now,
			})
			s.Variants[p.locale] = win.runs
			s.UpdatedAt = now
			if fresh {
				res.Learned++
			}
			continue
		}

		// The record answers this source more than one way. Every answer is a
		// translation a reader approved, so none of them can be discarded and no
		// gate can tell them apart on quality; what tells them apart is where
		// each was approved. So each point keeps an entry of its own, and the
		// fill resolves between them by asking from where it is — instead of the
		// whole project being handed whichever spelling the corpus repeats most.
		res.Contested++
		res.ContestedSources = append(res.ContestedSources, p.listContested())

		byID := make(map[string]memory.Entry, len(existing))
		for _, e := range existing {
			byID[e.ID] = e
		}
		written := map[string]bool{}
		for _, point := range p.points() {
			answer := p.resolve(point)
			id := recordEntryID(recordSourceKey(p.sourceRuns), point)
			written[id] = true
			if e, ok := byID[id]; ok {
				// The store already holds this point's entry: correct it in
				// place, so the other locales it carries survive the write.
				s, fresh := stage(e)
				s.Point = point
				s.Variants[p.locale] = answer.runs
				s.Origins = withRecordOrigin(s.Origins, p.origin, now)
				s.UpdatedAt = now
				if fresh && memory.NormalizeText(e.VariantText(p.locale)) != answer.text {
					res.Reconciled++
				}
				continue
			}
			origin := p.origin
			origin.AddedAt = now
			s, fresh := stage(memory.Entry{
				ID:          id,
				HintSrcLang: sourceLocale,
				Variants:    map[model.LocaleID][]model.Run{sourceLocale: p.sourceRuns},
				Origins:     []memory.Origin{origin},
				Point:       point,
				Unit:        p.unit,
				CreatedAt:   now,
				UpdatedAt:   now,
			})
			s.Variants[p.locale] = answer.runs
			s.UpdatedAt = now
			if fresh {
				res.Learned++
			}
		}

		// Every other entry the store answers this source with — a seed, an
		// import, an approval promoted by `kapi apply` — is corrected to the
		// answer that governs at ITS point rather than to a project-wide winner,
		// so an entry bound to no location does not carry one collection's
		// wording into every reader that finds it first.
		for _, e := range existing {
			if written[e.ID] || !e.HasLocale(p.locale) {
				continue
			}
			answer := p.resolve(e.Point)
			if memory.NormalizeText(e.VariantText(p.locale)) == answer.text {
				continue
			}
			s, fresh := stage(e)
			s.Variants[p.locale] = answer.runs
			s.Origins = withRecordOrigin(s.Origins, p.origin, now)
			s.UpdatedAt = now
			if fresh {
				res.Reconciled++
			}
		}
	}
	if len(write) == 0 {
		return nil
	}
	if err := tm.BulkAddWithStream(ctx, write, ""); err != nil {
		return fmt.Errorf("record %d content-memory entr%s from the committed translations: %w",
			len(write), map[bool]string{true: "y", false: "ies"}[len(write) == 1], err)
	}
	a.RebuildMemorySearchIndexes(ctx, tm)
	return nil
}

// supersededSource decides whether a committed pair translates the source now
// beside it, and recovers the wording it does translate when it does not.
//
// blessed is the source to absorb the target against; superseded with a nil
// blessed means the pairing is contradicted and the wording it belongs to cannot
// be recovered, so nothing is written for it at all.
//
// The two signals are not alternatives to each other, they answer for different
// units. A decision's basis is per locale and only the decided locale has one; a
// source rewrite is a fact about the source, and every locale of the unit is
// mispaired by it. Reading the record with only the first left the undecided
// locales silently serving the translation of a deleted sentence, which the loop
// then confirmed as caught up.
func supersededSource(
	b *model.Block, u recordUnit, e reviewedEntry, basis basisVerdict,
	srcRuns, tgtRuns []model.Run, prior *priorSourceIndex, corpus *memoryAnswers,
) (blessed []model.Run, superseded bool, err error) {
	// The record's own basis contradicts the adjacency: it names source wording
	// that is gone, and the translation it names is still on disk. Recovering
	// that wording by the basis hash is verified by construction — a block whose
	// hash IS the basis is the pairing the record was made against,
	// reconstructed rather than guessed at.
	//
	// A decision and a basis the loop recorded for a target it wrote answer here
	// alike, which is what lets a source rewrite under an undecided translation
	// be caught where the block store cannot help: on a fresh clone the prior
	// wording is unrecoverable, so nothing is written for the pair at all, and
	// the loop re-drafts against the source the project now has.
	//
	// Scoped to the record's other half still holding. Once the target has moved
	// on too — the loop re-drafted it, or a person rewrote it — the record
	// describes neither side of what is on disk, and the pair is absorbed as the
	// fresh pairing it is. That is also what keeps the re-draft from repeating:
	// the next run learns the fresh draft and recycles it rather than paying to
	// produce it again.
	if basis == basisStale && e.blessesTarget(b, u.locale) {
		runs, _ := prior.runsFor(e.contentHash)
		return runs, true, nil
	}

	// No decision, or one that no longer describes either half. The block store
	// still holds the source this unit had when the pass last read the working
	// tree, and the committed target was produced against THAT. A unit whose
	// source has moved since is mispaired in every locale, decided or not.
	//
	// Scoped the same way the basis is, and by the same evidence: the corpus
	// already answering the prior wording with exactly this target is what says
	// the target has not moved with the source — it is the loop's own last
	// output for the sentence that is gone. A target rewritten alongside its
	// source answers nothing, and is absorbed as the fresh pairing it is, so a
	// person who edits both halves together is not made to re-review their own
	// work.
	runs, ok := prior.runsForUnit(u.sourceRel, b.ID)
	if !ok || model.RunsText(runs) == model.RunsText(srcRuns) {
		return nil, false, nil
	}
	held, err := corpus.answers(runs, u.locale, tgtRuns)
	if err != nil || !held {
		return nil, false, err
	}
	return runs, true, nil
}

// priorSourceIndex answers "what wording is this translation of?" from the
// project block store, two ways: by the basis a decision recorded
// (state.SourceHash of a block's source text) and by the unit the block store
// keys the source under.
//
// The store is the only carrier of that wording: a decision keeps the basis
// hash, not the sentence, and an undecided unit keeps nothing at all. It holds
// it because the absorber runs BEFORE the pass re-extracts the working tree, so
// at this moment the store still describes the project as it was when the
// committed targets were last produced.
//
// The scan is deferred until something actually asks, and asked once for the
// run. Only a document the digest stamp did not skip puts the question at all,
// and reading that document's source and target back through their format
// readers — which the absorber has already done by then — costs more than the
// scan does.
type priorSourceIndex struct {
	ctx    context.Context
	db     *projectdb.DB
	loaded bool
	byHash map[string][]model.Run
	byUnit map[string][]model.Run
}

func newPriorSourceIndex(ctx context.Context, db *projectdb.DB) *priorSourceIndex {
	return &priorSourceIndex{ctx: ctx, db: db}
}

// runsFor returns the source runs whose basis hash is the given one, and
// ok=false when the store cannot supply them (nothing extracted yet, a build
// with no block store, or a store already re-read past the edit).
func (p *priorSourceIndex) runsFor(basis string) ([]model.Run, bool) {
	if basis == "" {
		return nil, false
	}
	p.load()
	runs, ok := p.byHash[basis]
	return runs, ok
}

// runsForUnit returns the source runs the store holds for one unit of one source
// document — the wording that unit had when the working tree was last read.
func (p *priorSourceIndex) runsForUnit(sourceRel, id string) ([]model.Run, bool) {
	if sourceRel == "" || id == "" {
		return nil, false
	}
	p.load()
	runs, ok := p.byUnit[priorUnitKey(sourceRel, id)]
	return runs, ok
}

// priorUnitKey is the identity the block store places a block by: the source
// document it was extracted from and its in-file id. It is what
// project.BlockStoreHash keys a stored block on, minus the source text — which
// is exactly the part a rewrite moves.
func priorUnitKey(sourceRel, id string) string { return sourceRel + "\x00" + id }

func (p *priorSourceIndex) load() {
	if p.loaded {
		return
	}
	p.loaded = true
	p.byHash, p.byUnit = p.scan()
}

// scan reads every translatable block the store holds and indexes its source
// runs by basis and by unit. Autocommit, not the session-transactional store:
// this is a read beside the run's own writes, and holding the write permit for
// the length of a full scan would stall them. Any failure yields empty indexes —
// the caller then absorbs no differently than it did before the store existed,
// rather than acting on half a scan.
func (p *priorSourceIndex) scan() (byHash, byUnit map[string][]model.Run) {
	byHash, byUnit = map[string][]model.Run{}, map[string][]model.Run{}
	if p.db == nil {
		return byHash, byUnit
	}
	store := p.db.BlocksAutocommit()
	if store == nil {
		return byHash, byUnit
	}
	sess, err := store.Begin(p.ctx)
	if err != nil {
		return byHash, byUnit
	}
	defer sess.Close()
	translatable := true
	for b, berr := range sess.Blocks(blockstore.BlockFilter{Translatable: &translatable}) {
		if berr != nil {
			return map[string][]model.Run{}, map[string][]model.Run{}
		}
		// model.RunsText, the same projection model.Block.SourceText makes, so a
		// stored block hashes to the number a decision recorded for it.
		text := model.RunsText(b.Source)
		if text == "" {
			continue
		}
		h := state.SourceHash(text)
		if _, seen := byHash[h]; !seen {
			byHash[h] = b.Source
		}
		byUnit[priorUnitKey(b.Properties.File, b.ID)] = b.Source
	}
	return byHash, byUnit
}

// memoryAnswers asks the content memory the question `recycle` asks: for this
// source, in this locale, does the corpus already give exactly this translation?
//
// It reads the store's PRE-RUN state, which is the point — an absorbing pass
// stages its writes and flushes them at the end, so what this sees is what the
// previous run left, not what this one is about to assert.
//
// Cached per source: a rewritten unit is asked about once per locale, and the
// full-score query is the same one for all of them.
type memoryAnswers struct {
	ctx    context.Context
	tm     *memory.SQLiteStore
	source model.LocaleID
	cache  map[string][]memory.Entry
}

// answers reports whether the corpus gives the given target for the given source
// in the locale.
func (m *memoryAnswers) answers(src []model.Run, locale model.LocaleID, tgt []model.Run) (bool, error) {
	key := recordSourceKey(src)
	entries, ok := m.cache[key]
	if !ok {
		var err error
		entries, err = m.tm.FullScoreEntries(m.ctx, src, m.source)
		if err != nil {
			return false, fmt.Errorf("read content-memory entries for a rewritten unit: %w", err)
		}
		m.cache[key] = entries
	}
	want := memory.NormalizeText(model.FlattenRuns(tgt))
	for _, e := range entries {
		if e.HasLocale(locale) && memory.NormalizeText(e.VariantText(locale)) == want {
			return true, nil
		}
	}
	return false, nil
}

// withRecordOrigin adds the record origin to an entry's origins, replacing the
// one this artifact left last time so a re-absorbed entry does not accumulate a
// row per run.
func withRecordOrigin(origins []memory.Origin, origin memory.Origin, now time.Time) []memory.Origin {
	origin.AddedAt = now
	for i, o := range origins {
		if o.Source == recordOriginSource && o.Key == origin.Key {
			origins[i] = origin
			return origins
		}
	}
	return append(origins, origin)
}

// recordRuns projects a paired block onto the source and target runs the memory
// stores, reporting keep=false for a block that carries no pair worth absorbing.
// approved says the project's committed record holds an approval for this unit
// in this locale, with both halves of what it blessed still on disk.
//
// A target identical to its source is not a pair unless a person decided it is.
// Unapproved, the identity is far more often a catalog carrying its untranslated
// leaves verbatim than a translation that happens to coincide, and absorbing one
// is worse than absorbing nothing: it teaches the memory nothing an unfilled
// target would not produce anyway, it competes at full score with the real
// translation another surface gives the same string, and — since the convergence
// flow skips what recycle matched — it would fill the unit with its own source
// and take it away from the AI step for good, freezing an untranslated leaf as
// translated.
//
// Approved, all three arguments fall away. The record is not guessing at what
// the identity means: a person read the pairing and said this wording is right,
// which is what proper nouns, product names and short labels look like when they
// are correct. Dropping it re-drafted them on every pass and overwrote the
// approval bound to each — the automated pass discarding the decision the unit
// ledger exists to keep.
func recordRuns(b *model.Block, locale model.LocaleID, approved bool) (src, tgt []model.Run, keep bool) {
	if !b.Translatable {
		return nil, nil, false
	}
	src = b.SourceRuns()
	tgt = b.TargetRuns(locale)
	if len(src) == 0 || len(tgt) == 0 {
		return nil, nil, false
	}
	srcText, tgtText := model.FlattenRuns(src), model.FlattenRuns(tgt)
	if srcText == "" || tgtText == "" {
		return nil, nil, false
	}
	if srcText == tgtText && !approved {
		return nil, nil, false
	}
	return src, tgt, true
}

// pairedRecordBlocks reads the source and the committed target through the same
// configured reader and overlays the target onto the source, so a pair is what
// the collection's own binding says it is. Reading the target with the source's
// format and config is the point: the target is the source document's shape in
// another language, and a reader configured differently would pair different
// blocks.
func (a *App) pairedRecordBlocks(ctx context.Context, u recordUnit, sourceLocale model.LocaleID) ([]*model.Block, error) {
	sourceBlocks, _, err := project.ReadSourceBlocks(ctx, a.FormatReg, u.format, u.sourcePath, sourceLocale, u.locale, u.config)
	if err != nil {
		return nil, fmt.Errorf("read source %s: %w", u.sourceRel, err)
	}
	targetBlocks, _, err := project.ReadSourceBlocks(ctx, a.FormatReg, u.format, u.targetPath, sourceLocale, u.locale, u.config)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", errTargetUnreadable, u.targetRel, err)
	}
	for _, sb := range sourceBlocks {
		sb.SourceLocale = sourceLocale
	}
	OverlayTargets(sourceBlocks, targetBlocks, u.locale)
	return sourceBlocks, nil
}

// recordUnits enumerates the (source, target, locale) triples whose target the
// recipe names — the same resolution `kapi up` writes through, so the absorber
// reads back exactly where the loop writes.
func recordUnits(ctx context.Context, a *App, proj *project.KapiProject, pctx *project.ProjectContext, root string) ([]recordUnit, error) {
	resolved, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return nil, fmt.Errorf("resolve content: %w", err)
	}
	var units []recordUnit
	points := map[string]string{}
	docs := a.documentIndexOrEmpty(ctx, root)
	for _, rf := range resolved {
		if rf.Item == nil || rf.Item.Target == "" {
			continue
		}
		cfg := mergedFormatConfig(proj, rf.Format, rf.Item)
		point, perr := a.approvalPoint(proj, points, rf)
		if perr != nil {
			return nil, perr
		}
		for _, loc := range rf.Item.ResolvedTargetLanguages(nil, proj.Defaults) {
			targetPath := expandTargetTemplate(rf.Item.Path, rf.Item.Base, rf.Item.Target, rf.Relative, string(loc), root, proj.Defaults.LocaleFormat)
			if blockedTargetPath(targetPath) != nil {
				continue // an obstructed destination holds no translation
			}
			units = append(units, recordUnit{
				sourcePath: rf.Path,
				sourceRel:  relSlash(root, rf.Path),
				scope:      docs.Scope(root, rf.Path),
				targetPath: targetPath,
				targetRel:  relSlash(root, targetPath),
				format:     rf.Format,
				config:     cfg,
				locale:     loc,
				point:      point,
			})
		}
	}
	sort.Slice(units, func(i, j int) bool {
		if units[i].sourceRel != units[j].sourceRel {
			return units[i].sourceRel < units[j].sourceRel
		}
		return units[i].locale < units[j].locale
	})
	return units, nil
}

// approvalPoint resolves the context point a resolved source file sits at, as
// the content memory stores it: the product profile and channel the recipe
// governs the file through, and the collection that claims it.
//
// It resolves through project.ResolveGovernanceFor, the same seam a run, a
// check and a push resolve through, so the point an answer is stored under and
// the point a fill asks from are one resolution rather than two that have to
// agree. Cached per collection+item channel, because a collection of a thousand
// files resolves to one point.
func (a *App) approvalPoint(proj *project.KapiProject, cache map[string]string, rf project.ResolvedFile) (string, error) {
	channel := rf.Collection
	if rf.Item != nil && rf.Item.Channel != "" {
		channel += "\x00" + rf.Item.Channel
	}
	if p, ok := cache[channel]; ok {
		return p, nil
	}
	gov, err := proj.ResolveGovernanceFor(a.GovernancePointFor(rf.Collection, rf.Relative))
	if err != nil {
		return "", fmt.Errorf("resolve the point %s sits at: %w", rf.Relative, err)
	}
	p := memory.NewPoint(gov.Profile, gov.Channel, rf.Collection)
	cache[channel] = p
	return p, nil
}

// recordDigest is the content identity one (source, target) pair is keyed by.
// Both sides matter: a source edit changes which blocks pair with which, and a
// target edit changes the answers. ok=false means there is no committed target
// yet, which is ordinary pending work rather than an error.
func recordDigest(u recordUnit) (string, bool, error) {
	targetBytes, err := os.ReadFile(u.targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read committed target %s: %w", u.targetRel, err)
	}
	sourceBytes, err := os.ReadFile(u.sourcePath)
	if err != nil {
		return "", false, fmt.Errorf("read source %s: %w", u.sourceRel, err)
	}
	h := sha256.New()
	h.Write(sourceBytes)
	h.Write([]byte{0})
	h.Write(targetBytes)
	return hex.EncodeToString(h.Sum(nil)), true, nil
}

// recordSourceKey identifies a source by the keys the memory matches on, so two
// occurrences of the same string in the same shape are one pair with two votes
// rather than two competing entries.
func recordSourceKey(runs []model.Run) string {
	return memory.NormalizeText(model.RunsStructuralText(runs)) + "\x00" +
		memory.NormalizeText(model.FlattenRuns(runs))
}

// recordKey scopes a source to one locale: the votes are per locale, because the
// answers are.
func recordKey(locale model.LocaleID, runs []model.Run) string {
	return string(locale) + "\x00" + recordSourceKey(runs)
}

// recordEntryPrefix marks an id the record absorber minted, and nothing else
// does — which is what lets a pass tell what it taught the corpus from what the
// corpus was taught elsewhere.
const recordEntryPrefix = "record:"

// recordEntryID is the deterministic id a record-derived entry carries, so a
// re-absorbed pair updates its own row instead of adding a rival.
//
// It keys on the source AND the point that approved the answer. The source
// alone would be right if a project answered each string once, and it is why a
// second reviewed answer could only ever displace the first: two collections
// reviewed apart wrote to one row and the last write won. The point is what
// gives each approval a row of its own; the locale is not part of it, because
// an entry is multilingual and every locale's answer at one point belongs in
// the same one.
func recordEntryID(sourceKey, point string) string {
	key := sourceKey
	if point != "" {
		key += "\x00" + point
	}
	sum := sha256.Sum256([]byte(key))
	return recordEntryPrefix + hex.EncodeToString(sum[:])[:24]
}

// relearnRecordIfRekeyed makes a store written under an older entry identity
// forget what the absorber taught it, so this pass teaches it again under the
// current one.
//
// It drops the absorber's own entries — the ids it mints, and nobody else does
// — and every absorb stamp, which is what makes the next loop read every
// committed target rather than skip the ones whose bytes have not moved. What
// the corpus learned elsewhere is left alone: a seed, an import and an approval
// are not the record's to forget, and the pass corrects them the same way it
// always did.
//
// Left in place, an entry keyed on its source alone would sit beside the
// point-keyed entries as a rival answer bound to no location — an answer at the
// default point, competing at full score with the approvals that actually
// govern, and demoting them for every reader who cannot say where they are
// asking from.
func relearnRecordIfRekeyed(ctx context.Context, db *projectdb.DB, tm *memory.SQLiteStore) error {
	scheme, _, err := db.Meta(ctx, MetaRecordScheme)
	if err == nil && scheme == recordSchemeCurrent {
		return nil
	}
	entries, eerr := tm.Entries(ctx)
	if eerr != nil {
		return fmt.Errorf("read the content memory to re-learn the committed record: %w", eerr)
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.ID, recordEntryPrefix) {
			continue
		}
		if derr := tm.Delete(ctx, e.ID); derr != nil {
			return fmt.Errorf("forget content-memory entry %s: %w", e.ID, derr)
		}
	}
	if serr := saveRecordDigests(ctx, db, map[string]string{}); serr != nil {
		return serr
	}
	if perr := db.PutMeta(ctx, MetaRecordScheme, recordSchemeCurrent); perr != nil &&
		!errors.Is(perr, projectdb.ErrNoStore) {
		return perr
	}
	return nil
}

// loadRecordDigests reads the per-artifact absorb stamps. Any uncertainty
// yields an empty map, so every artifact reads as needing an absorb — the safe
// direction, since absorbing again is idempotent while skipping leaves the store
// behind git.
func loadRecordDigests(ctx context.Context, db *projectdb.DB) map[string]string {
	v, ok, err := db.Meta(ctx, MetaRecordDigests)
	if err != nil || !ok {
		return map[string]string{}
	}
	var stamps map[string]string
	if json.Unmarshal([]byte(v), &stamps) != nil || stamps == nil {
		return map[string]string{}
	}
	return stamps
}

// saveRecordDigests records the per-artifact absorb stamps. The map is rebuilt
// from the artifacts this pass saw, so a target removed from the recipe stops
// being remembered.
func saveRecordDigests(ctx context.Context, db *projectdb.DB, stamps map[string]string) error {
	data, err := json.Marshal(stamps)
	if err != nil {
		return fmt.Errorf("encode record digests: %w", err)
	}
	if err := db.PutMeta(ctx, MetaRecordDigests, string(data)); err != nil && !errors.Is(err, projectdb.ErrNoStore) {
		return err
	}
	return nil
}

// recordChainUnit is the identity a block's version chain is keyed on: the
// answer to "is the thing I am looking at now the same thing I approved before".
//
// Three candidates, most durable first. A resolved Unit is matched rather than
// named, so it survives a sibling being deleted. A structural address is
// translation-invariant, which a structural NAME is not — a name carries its
// ancestors' words, so the same paragraph is named differently in each
// document's own language. A name is the right key for a format that has one
// naturally: a key path or a catalog id is already invariant.
//
// It deliberately stops there rather than falling through to the block's ID, as
// convergence.BlockKey does. An id is assigned per read, so keying a chain on
// one would braid unrelated answers together and fragment a real chain, both
// silently. For a version chain an unstable key is worse than no key: empty
// says "this block has no history", which is merely unhelpful, while a wrong
// key says "this block said that before", which is false.

// recordChainUnit is model.Block.ChainUnit, kept as a named call site because
// the reasoning about WHY a chain is keyed this way belongs beside the write
// path that stamps it.
//
// Unit, then structural address, then name. It deliberately stops there rather
// than falling through to the block's ID, as convergence.BlockKey does. An id
// is assigned per read, so keying a chain on one would braid unrelated answers
// together and fragment a real chain, both silently. For a version chain an
// unstable key is worse than no key: empty says "this block has no history",
// which is merely unhelpful, while a wrong key says "this block said that
// before", which is false.
func recordChainUnit(b *model.Block) string { return b.ChainUnit() }

// governingFingerprintOf reads the governing context a block's committed target
// stands under, from the two places the statement can live.
//
// The target's own stamp answers first: a bilingual format keeps the producer's
// Origin beside the words, and a stamp on the bytes in front of the reader is
// the most direct statement there is. Most delivered formats keep no such slot
// (a JSON catalog holds strings, a .properties file holds key=value), so the
// project's decision record answers next: the context the decider approved
// the unit under, or the producer's stamp the loop recorded when it wrote the
// target. The record is read for the unit and locale, and only while it
// describes the translation on disk.
//
// Empty when neither carries a statement: the answer was produced under no
// governing context, or before anything recorded one. Both are true statements
// about the answer rather than a missing value to paper over.
func governingFingerprintOf(b *model.Block, locale model.LocaleID, reviewed reviewedIndex, scope string) string {
	if b == nil {
		return ""
	}
	if t := b.Target(locale); t != nil && t.Origin.ContextFingerprint != "" {
		return t.Origin.ContextFingerprint
	}
	return reviewed.governingFingerprint(scope, b, string(locale))
}

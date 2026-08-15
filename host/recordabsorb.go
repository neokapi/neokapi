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
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/state"
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
	// Refused counts pairs whose target does not carry its source's inline
	// codes. Such a pair can only produce a translation with a hole in it, so
	// it is not absorbed.
	Refused int `json:"refused,omitempty"`
	// Contested counts source texts the record answers more than one way. The
	// pair the corpus repeats most often wins; the rest are reported here.
	Contested int `json:"contested,omitempty"`
	// Superseded counts pairs the project's own decision record says do not go
	// together: the decision blessed source wording that has since been
	// rewritten, so the committed target translates a sentence the project no
	// longer has. Such a pair is absorbed against the source it actually
	// translates when that wording is still recoverable, and otherwise not at
	// all — never against the source now sitting beside it.
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
	targetPath, targetRel string
	format                string
	config                map[string]any
	locale                model.LocaleID
}

// recordTarget is one answer the record gives for a source, with how often the
// corpus repeats it and where it was first seen.
type recordTarget struct {
	runs  []model.Run
	count int
	order int
}

// recordPair collects every answer the committed record gives for one source
// content key, in one locale.
type recordPair struct {
	order      int
	locale     model.LocaleID
	sourceRuns []model.Run
	targets    map[string]*recordTarget
	origin     memory.Origin
}

// winner returns the target the corpus repeats most often, ties broken by first
// occurrence in the recipe's stable content order, plus whether the record gave
// more than one answer.
func (p *recordPair) winner() (target *recordTarget, contested bool) {
	for _, t := range p.targets {
		if target == nil || t.count > target.count || (t.count == target.count && t.order < target.order) {
			target = t
		}
	}
	return target, len(p.targets) > 1
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
	units, err := recordUnits(a, proj, pctx, layout.Root)
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
			srcRuns, tgtRuns, keep := recordRuns(b, sourceLocale, u.locale)
			if !keep {
				continue
			}
			// A pairing the record's own basis contradicts. This unit's key
			// survived a source rewrite, so its translation still sits beside a
			// sentence it was never a translation of — and reading that adjacency
			// as a pair is what taught the memory an exact answer for the NEW
			// wording, so every `kapi up` recycled the stale target back over
			// itself and reported the drift it had just confirmed.
			//
			// The pair is not simply dropped. The wording the decision blessed is
			// still in the block store (this runs before the pass re-extracts),
			// and recovering it by its own basis hash is not a guess: it is the
			// pairing the reviewer approved, reconstructed. Kept under that
			// source it stays reachable as leverage for the rewrite, which is
			// what stops a one-word source edit from throwing a careful
			// translation away. Unrecoverable — nothing extracted yet, or a store
			// already re-read — means nothing is written: a memory with one fewer
			// entry costs a lookup, an entry asserting a translation of the wrong
			// sentence costs the translation.
			//
			// Scoped to the decision's other half still holding. Once the target
			// has moved on too — the loop re-drafted it, or a person rewrote it —
			// the record describes neither side of what is on disk, and the pair
			// is absorbed like any undecided one. That is also what keeps the
			// re-draft from repeating: the next run learns the fresh draft and
			// recycles it rather than paying to produce it again.
			if e, basis, _ := reviewed.grade(b, string(u.locale)); basis == basisStale &&
				e.blessesTarget(b, u.locale) {
				res.Superseded++
				blessed, ok := prior.runsFor(e.contentHash)
				if !ok {
					continue
				}
				srcRuns = blessed
			}
			if !model.DiffRunCodes(srcRuns, tgtRuns).Balanced() {
				// The three-layer protection holds here too: an asymmetric pair
				// is one side of which has already lost a placeholder, and
				// recycle would refuse to fill from it anyway.
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
					targets:    map[string]*recordTarget{},
					origin: memory.Origin{
						Source:    recordOriginSource,
						Key:       u.targetRel,
						Reference: u.sourceRel,
						AddedBy:   "kapi-up",
					},
				}
				pairs[key] = p
			}
			tk := memory.NormalizeText(model.FlattenRuns(tgtRuns))
			t, ok := p.targets[tk]
			if !ok {
				order++
				t = &recordTarget{runs: tgtRuns, order: order}
				p.targets[tk] = t
			}
			t.count++
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
	units, err := recordUnits(a, proj, project.NewProjectContext(proj, projectPath), layout.Root)
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
		win, contested := p.winner()
		if contested {
			res.Contested++
		}
		winText := memory.NormalizeText(model.FlattenRuns(win.runs))

		existing, err := tm.FullScoreEntries(ctx, p.sourceRuns, sourceLocale)
		if err != nil {
			return fmt.Errorf("read content-memory entries for a committed pair: %w", err)
		}
		held := false
		for _, e := range existing {
			// An entry with no variant in this locale is not a candidate the
			// lookup can return, so it is not part of the disagreement and is
			// left alone.
			if !e.HasLocale(p.locale) {
				continue
			}
			if memory.NormalizeText(e.VariantText(p.locale)) == winText {
				held = true
				continue
			}
			// Full score, same source, different target: the committed record
			// is the later, reviewed answer, so the entry is corrected to it
			// rather than left to compete. Left competing, the ambiguity rule
			// (AD-009) would demote both and a full-score fill policy would
			// take neither — the disagreement would cost the translation
			// instead of resolving it.
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
		// The id keys on the source alone, so a second locale for the same source
		// folds into the entry the first one staged rather than opening a rival.
		s, fresh := stage(memory.Entry{
			ID:          recordEntryID(recordSourceKey(p.sourceRuns)),
			HintSrcLang: sourceLocale,
			Variants:    map[model.LocaleID][]model.Run{sourceLocale: p.sourceRuns},
			Origins:     []memory.Origin{origin},
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		s.Variants[p.locale] = win.runs
		s.UpdatedAt = now
		if fresh {
			res.Learned++
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

// priorSourceIndex answers "what wording did this decision bless?" from the
// project block store, keyed by the basis itself (state.SourceHash of a block's
// source text — the same number a decision records).
//
// The store is the only carrier of that wording: a decision keeps the basis
// hash, not the sentence. It holds it because the absorber runs BEFORE the pass
// re-extracts the working tree, so at this moment the store still describes the
// project as it was when the decision was made. Keying on the hash rather than
// on the unit means a recovered source is verified by construction — a block
// whose hash is the basis IS the wording that was blessed, whatever document it
// has since moved to.
//
// The scan is deferred until something actually asks: most runs hold no
// superseded pairing, and a project's whole block set is not worth reading to
// answer a question nobody put.
type priorSourceIndex struct {
	ctx    context.Context
	db     *projectdb.DB
	loaded bool
	byHash map[string][]model.Run
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
	if !p.loaded {
		p.loaded = true
		p.byHash = p.scan()
	}
	runs, ok := p.byHash[basis]
	return runs, ok
}

// scan reads every translatable block the store holds and indexes its source
// runs by basis. Autocommit, not the session-transactional store: this is a read
// beside the run's own writes, and holding the write permit for the length of a
// full scan would stall them. Any failure yields an empty index — the caller
// then declines to absorb rather than absorbing something wrong.
func (p *priorSourceIndex) scan() map[string][]model.Run {
	out := map[string][]model.Run{}
	if p.db == nil {
		return out
	}
	store := p.db.BlocksAutocommit()
	if store == nil {
		return out
	}
	sess, err := store.Begin(p.ctx)
	if err != nil {
		return out
	}
	defer sess.Close()
	translatable := true
	for b, berr := range sess.Blocks(blockstore.BlockFilter{Translatable: &translatable}) {
		if berr != nil {
			return out
		}
		// model.RunsText, the same projection model.Block.SourceText makes, so a
		// stored block hashes to the number a decision recorded for it.
		text := model.RunsText(b.Source)
		if text == "" {
			continue
		}
		h := state.SourceHash(text)
		if _, seen := out[h]; !seen {
			out[h] = b.Source
		}
	}
	return out
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
//
// A target identical to its source is deliberately not a pair: it teaches the
// memory nothing an unfilled target would not produce anyway, and as an entry it
// would compete — at full score — with the real translation another surface
// gives the same string.
func recordRuns(b *model.Block, sourceLocale, locale model.LocaleID) (src, tgt []model.Run, keep bool) {
	if !b.Translatable {
		return nil, nil, false
	}
	src = b.SourceRuns()
	tgt = b.TargetRuns(locale)
	if len(src) == 0 || len(tgt) == 0 {
		return nil, nil, false
	}
	srcText, tgtText := model.FlattenRuns(src), model.FlattenRuns(tgt)
	if srcText == "" || tgtText == "" || srcText == tgtText {
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
func recordUnits(a *App, proj *project.KapiProject, pctx *project.ProjectContext, root string) ([]recordUnit, error) {
	resolved, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return nil, fmt.Errorf("resolve content: %w", err)
	}
	var units []recordUnit
	for _, rf := range resolved {
		if rf.Item == nil || rf.Item.Target == "" {
			continue
		}
		cfg := mergedFormatConfig(proj, rf.Format, rf.Item)
		for _, loc := range rf.Item.ResolvedTargetLanguages(nil, proj.Defaults) {
			targetPath := expandTargetTemplate(rf.Item.Path, rf.Item.Base, rf.Item.Target, rf.Relative, string(loc), root)
			if blockedTargetPath(targetPath) != nil {
				continue // an obstructed destination holds no translation
			}
			units = append(units, recordUnit{
				sourcePath: rf.Path,
				sourceRel:  relSlash(root, rf.Path),
				targetPath: targetPath,
				targetRel:  relSlash(root, targetPath),
				format:     rf.Format,
				config:     cfg,
				locale:     loc,
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

// recordEntryID is the deterministic id a record-derived entry carries, so a
// re-absorbed pair updates its own row instead of adding a rival. It keys on the
// source alone: an entry is multilingual, and every locale's answer for one
// source belongs in it.
func recordEntryID(sourceKey string) string {
	sum := sha256.Sum256([]byte(sourceKey))
	return "record:" + hex.EncodeToString(sum[:])[:24]
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

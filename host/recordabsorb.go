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

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
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
	// Learned counts pairs written as new content-memory entries; Reconciled
	// counts entries already in the store whose target for this locale
	// disagreed with the committed record and was corrected to it.
	Learned    int `json:"learned,omitempty"`
	Reconciled int `json:"reconciled,omitempty"`
	// Refused counts pairs whose target does not carry its source's inline
	// codes. Such a pair can only produce a translation with a hole in it, so
	// it is not absorbed.
	Refused int `json:"refused,omitempty"`
	// Contested counts source texts the record answers more than one way. The
	// pair the corpus repeats most often wins; the rest are reported here.
	Contested int `json:"contested,omitempty"`
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
			e.Variants[p.locale] = win.runs
			e.Origins = withRecordOrigin(e.Origins, p.origin, now)
			e.UpdatedAt = now
			write = append(write, e)
			res.Reconciled++
			held = true
		}
		if held {
			continue
		}
		origin := p.origin
		origin.AddedAt = now
		write = append(write, memory.Entry{
			ID:          recordEntryID(k),
			HintSrcLang: sourceLocale,
			Variants: map[model.LocaleID][]model.Run{
				sourceLocale: p.sourceRuns,
				p.locale:     win.runs,
			},
			Origins:   []memory.Origin{origin},
			CreatedAt: now,
			UpdatedAt: now,
		})
		res.Learned++
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

// recordKey identifies a source within one locale by the keys the memory
// matches on, so two occurrences of the same string in the same shape are one
// pair with two votes rather than two competing entries.
func recordKey(locale model.LocaleID, runs []model.Run) string {
	return string(locale) + "\x00" +
		memory.NormalizeText(model.RunsStructuralText(runs)) + "\x00" +
		memory.NormalizeText(model.FlattenRuns(runs))
}

// recordEntryID is the deterministic id a record-derived entry carries, so a
// re-absorbed pair updates its own row instead of adding a rival.
func recordEntryID(key string) string {
	sum := sha256.Sum256([]byte(key))
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

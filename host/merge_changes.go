package host

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workhome"
)

// A bilingual file kapi extract writes for a translator carries, on every
// unit, the two revisions the unit was extracted against: the revision of
// the unit's translation as the project held it (model.AbsentRevision when
// there was none) and the revision of the source the unit was read from. In
// XLIFF 2 they are the unit's <mda:meta> of category kapi, in PO the entry's
// kapi-if-match and kapi-basis comments, and in a .kpz the interchange task's
// revisions.
//
// kapi merge compiles a returned file into one set_content per translated
// unit, with those revisions as its if_match and basis, and applies them
// through the change service under require_basis. A unit whose source changed
// after the extraction is refused stale naming basis, from the file alone, so
// the guard holds when the extraction manifest is gone. A unit whose
// translation changed in the project since the extraction is refused stale
// naming if_match, and the recipe's conflict policy says which side keeps it.

// unitRevision is what one unit of a bilingual file was extracted against.
type unitRevision struct {
	// IfMatch is the revision of the unit's translation, or
	// model.AbsentRevision when the project held none.
	IfMatch string
	// Basis is the revision of the unit's source.
	Basis string
}

// The block properties a unit's revisions travel in between a carrier's
// reader or writer and the merge: the XLIFF 2 writer emits them as the unit's
// metadata and its reader reads them back.
var (
	unitIfMatchProperty = xliff2.UnitMetaKey(xliff2.FileNoteCategoryKapi, xliff2.UnitMetaIfMatch)
	unitBasisProperty   = xliff2.UnitMetaKey(xliff2.FileNoteCategoryKapi, xliff2.UnitMetaBasis)
)

// stampUnitRevision records rev on b, for the writer of a bilingual file. A
// revision rev leaves empty is not recorded.
func stampUnitRevision(b *model.Block, rev unitRevision) {
	if b.Properties == nil {
		b.Properties = map[string]string{}
	}
	for key, value := range map[string]string{unitIfMatchProperty: rev.IfMatch, unitBasisProperty: rev.Basis} {
		if value == "" {
			delete(b.Properties, key)
			continue
		}
		b.Properties[key] = value
	}
}

// extractedRevision is what the unit kapi extract writes for b, a block read
// in source, is extracted against. The basis is the revision of b's own
// source, so it names the source the unit carries whatever block the merge
// later finds under b's ID. The translation's revision comes from held, the
// revisions a read of the source with its translation joined gives
// (interchangeRevisions), and only when the block that read holds under b's
// ID has the same source. A block its reader left with no language is given
// source, as the change service gives every block it reads, so the two
// revisions of one source agree.
func extractedRevision(b *model.Block, source model.LocaleID, held map[string]unitRevision) unitRevision {
	if b.SourceLocale == "" {
		b.SourceLocale = source
	}
	rev := unitRevision{Basis: model.EditionRevision(b, b.Authoritative(model.AuthorityPolicy{}))}
	if h, ok := held[b.ID]; ok && h.Basis == rev.Basis {
		rev.IfMatch = h.IfMatch
	}
	return rev
}

// unitRevisionOf reads the revisions stampUnitRevision recorded on b, or a
// carrier's reader read.
func unitRevisionOf(b *model.Block) (unitRevision, bool) {
	rev := unitRevision{IfMatch: b.Properties[unitIfMatchProperty], Basis: b.Properties[unitBasisProperty]}
	return rev, rev.IfMatch != "" || rev.Basis != ""
}

// interchangeRevisions reads doc through svc with its translation into
// locale joined, and returns each block's revisions by block ID: the
// translation's, and the source's the read gives as the basis.
func interchangeRevisions(ctx context.Context, svc *change.Service, doc string, locale model.LocaleID) (map[string]unitRevision, error) {
	key := model.EditionKey{Locale: locale}
	revs := map[string]unitRevision{}
	_, err := svc.ReadEach(ctx, change.ReadRequest{Doc: doc, Editions: []model.EditionKey{key}}, func(b *model.Block, _ change.BlockRead) error {
		revs[b.ID] = unitRevision{
			IfMatch: model.EditionRevision(b, key),
			Basis:   model.EditionRevision(b, b.Authoritative(model.AuthorityPolicy{})),
		}
		return nil
	})
	return revs, err
}

// returnedFile is a bilingual file a translator returned, read into its
// units.
type returnedFile struct {
	// input is the file.
	input string
	// doc is the source document its units translate, project-relative.
	doc string
	// locale is the language of its translations.
	locale model.LocaleID
	// batch is the extraction batch the file came from, which names its
	// redaction vault.
	batch string
	// reference is what the content memory records the merged units as
	// coming from; empty is the batch.
	reference string
	// sourceHash is the source file's digest at the extraction, when the file
	// names one.
	sourceHash string
	// blocks are its units: each block's ID, the source the unit carries,
	// its translation in locale, and its revisions as unitRevisionOf reads
	// them.
	blocks []*model.Block
	// plainSource says the units carry their source as plain text, as a PO
	// msgid does, rather than as runs.
	plainSource bool
}

// mergeOutcome is what became of one unit of a returned file.
type mergeOutcome struct {
	Block string
	// Status is applied, unchanged, stale (the source moved, or the block is
	// gone), skipped (empty, or the project's translation kept by the
	// conflict policy) or refused.
	Status string
	// Error is the service's refusal, for stale, skipped by the policy and
	// refused.
	Error *change.Error
}

// The outcomes a merge reports for a unit.
const (
	mergeApplied   = "applied"
	mergeUnchanged = "unchanged"
	mergeStale     = "stale"
	mergeSkipped   = "skipped"
	mergeRefused   = "refused"
)

// mergeChangeActor is who a merge sends its change sets as: the person
// returning a translator's work.
var mergeChangeActor = change.Actor{Kind: change.ActorPerson}

// maxMergePasses bounds how often a merge sends a file's change set again
// after refusals. Each pass drops the units refused for good and resends the
// ones the conflict policy gives to the translator with the revision the
// project now holds; a translation that keeps moving under the merge stops
// it.
const maxMergePasses = 4

// mergeReturned applies rf to the project through the change service. Its
// redacted originals are restored first, then each translated unit is
// compiled into a set_content on the source's translation into rf.locale and
// the change set applied; the accepted units are absorbed into the content
// memory.
func (a *App) mergeReturned(ctx context.Context, task mergeTask, rf *returnedFile) (mergeStats, []mergeOutcome, error) {
	var stats mergeStats
	if err := checkPathSegment(string(rf.locale), "merge: target locale"); err != nil {
		return stats, nil, err
	}
	sourceAbs, err := containedJoin(task.layout.Root, rf.doc, "merge: source path")
	if err != nil {
		return stats, nil, err
	}
	if rf.batch != "" {
		if err := restoreRedactedBlocks(task.layout, rf.batch, rf.blocks, rf.locale, !task.noRestore); err != nil {
			return stats, nil, fmt.Errorf("merge: restore redaction for batch %s: %w", rf.batch, err)
		}
	}

	svc, err := a.ChangeService(ctx, ChangeServiceOptions{
		Project: task.recipe, Origin: "merge", SourceLocale: task.ctx.SourceLocale, TargetLocale: rf.locale, Materialize: true,
	})
	if err != nil {
		return stats, nil, err
	}
	key := model.EditionKey{Locale: rf.locale}

	// The source as it stands, with its translation joined.
	type held struct {
		block   *model.Block
		ref     string
		authRev string
		rev     string
	}
	current := map[string]held{}
	if _, err := svc.ReadEach(ctx, change.ReadRequest{Doc: rf.doc, Editions: []model.EditionKey{key}}, func(b *model.Block, r change.BlockRead) error {
		current[b.ID] = held{block: b, ref: r.Ref.Block, authRev: model.EditionRevision(b, b.Authoritative(model.AuthorityPolicy{})), rev: model.EditionRevision(b, key)}
		return nil
	}); err != nil {
		return stats, nil, fmt.Errorf("merge: read %s: %w", rf.doc, err)
	}
	unchangedFile := false
	if rf.sourceHash != "" {
		if h, herr := project.HashFile(sourceAbs); herr == nil && h == rf.sourceHash {
			unchangedFile = true
		}
	}

	var outcomes []mergeOutcome
	note := func(o mergeOutcome) { outcomes = append(outcomes, o) }
	// labels names each unit in a report by the block it translates, its key
	// and the edition, as a read names it, rather than by the id the reader
	// gave it.
	labels := map[string]string{}
	var ops []change.Op
	var units []*model.Block
	for _, u := range rf.blocks {
		target := u.Target(rf.locale)
		if target == nil || !hasAnyText(target.Runs) {
			note(mergeOutcome{Block: u.ID, Status: mergeSkipped})
			continue
		}
		cur, ok := current[u.ID]
		if ok {
			labels[u.ID] = cur.ref + "@" + string(rf.locale)
		}
		if !ok {
			note(mergeOutcome{Block: u.ID, Status: mergeStale, Error: &change.Error{Code: change.CodeNotFound, Field: "at/block",
				Message: fmt.Sprintf("%s holds no block %s", rf.doc, u.ID)}})
			continue
		}
		rev, _ := unitRevisionOf(u)
		// A unit that carries its source translates that source, so it
		// lands only on a block that still reads so, whatever basis it
		// names. A unit that carries no basis was made from the source it
		// carries, or, carrying none, from the file the return names.
		carried := len(u.Source) > 0
		var why string
		switch {
		case carried && !sameSource(u, cur.block, rf.plainSource, rev.Basis != ""):
			why = fmt.Sprintf("block %s no longer holds the source the unit carries", cur.ref)
		case rev.Basis == "" && !carried && !unchangedFile:
			why = fmt.Sprintf("the source of block %s changed since the unit was extracted", cur.ref)
		}
		if why != "" {
			note(mergeOutcome{Block: u.ID, Status: mergeStale, Error: &change.Error{Code: change.CodeStale, Field: "basis", Message: why}})
			continue
		}
		if flattensStructure(cur.block.SourceRuns(), target.Runs) {
			note(mergeOutcome{Block: u.ID, Status: mergeRefused, Error: &change.Error{Code: change.CodeGuard, Subcode: change.SubcodeStructureLost,
				Message: "the message holds a plural or select, and the file carries one branch of it, so its translation would flatten the message; " + warnStructures}})
			continue
		}
		if e := codesChanged(cur.block.SourceRuns(), target.Runs); e != nil {
			note(mergeOutcome{Block: u.ID, Status: mergeRefused, Error: e})
			continue
		}
		if rev.Basis == "" {
			rev.Basis = cur.authRev
		}
		ops = append(ops, change.Op{
			Kind: change.KindSetContent, At: change.Ref{Doc: rf.doc, Block: cur.ref, Edition: key},
			IfMatch: cmp.Or(rev.IfMatch, cur.rev), Basis: rev.Basis,
			Body: &change.SetContent{Runs: target.Runs},
		})
		units = append(units, u)
	}

	landed, err := a.applyReturned(ctx, svc, task, rf, ops, units, note)
	if err != nil {
		return stats, outcomes, err
	}

	// A refusal of one unit is reported and the rest land. Units refused for
	// one reason (a translation with no file, say) are reported once.
	var reasons []string
	byReason := map[string][]string{}
	for _, o := range outcomes {
		switch o.Status {
		case mergeApplied, mergeUnchanged:
			stats.Applied++
		case mergeStale:
			stats.Stale++
		case mergeSkipped:
			stats.Skipped++
		case mergeRefused:
			stats.Refused++
			why := "refused"
			if o.Error != nil {
				why = o.Error.Error()
			}
			if _, seen := byReason[why]; !seen {
				reasons = append(reasons, why)
			}
			byReason[why] = append(byReason[why], cmp.Or(labels[o.Block], o.Block))
		}
	}
	for _, why := range reasons {
		fmt.Fprintf(os.Stderr, "merge: %s: %s not merged: %s\n", filepath.Base(rf.input), blockList(byReason[why]), why)
	}
	if stats.Refused > 0 && stats.Applied == 0 {
		return stats, outcomes, fmt.Errorf("merge: nothing in %s was merged: %s", filepath.Base(rf.input), reasons[0])
	}

	var tmErr error
	if task.mem != nil {
		for _, u := range landed {
			cur := current[u.ID]
			b := &model.Block{ID: u.ID, Source: cur.block.Source, SourceLocale: cur.block.SourceLocale, Identity: cur.block.Identity}
			b.SetTargetRuns(rf.locale, u.Target(rf.locale).Runs)
			added, updated, aerr := task.mem.absorb(ctx, b, task.ctx.SourceLocale, rf.locale, cmp.Or(rf.reference, rf.batch, "merge"), rf.doc, rf.input)
			if aerr != nil && tmErr == nil {
				tmErr = aerr
			}
			stats.MemoryNew += added
			stats.MemoryUpdated += updated
		}
	}
	if tmErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: merge: record %s in the project content memory: %v (the merged translations were written)\n",
			filepath.Base(rf.input), tmErr)
	}
	return stats, outcomes, nil
}

// applyReturned applies ops, the set_content of each unit in units, as one
// change set under the enforce gate, and notes each unit's outcome. A refused
// unit is dropped and the rest sent again: a unit whose source moved is
// stale, one the conflict policy keeps the project's translation for is
// skipped, one the policy gives to the translator is sent again over the
// revision the project now holds, and any other refusal, a translation that
// breaks a rule governing it among them, is refused. It returns the units
// that landed.
func (a *App) applyReturned(ctx context.Context, svc *change.Service, task mergeTask, rf *returnedFile, ops []change.Op, units []*model.Block, note func(mergeOutcome)) ([]*model.Block, error) {
	set := change.Set{RequireBasis: true, Note: "merge " + filepath.Base(rf.input)}
	for pass := 0; len(ops) > 0; pass++ {
		if pass == maxMergePasses {
			return nil, fmt.Errorf("merge: %s: the translations of %s kept changing while the merge applied them; run it again", filepath.Base(rf.input), rf.doc)
		}
		set.Ops = ops
		res, err := svc.Apply(ctx, set, mergeChangeActor)
		if err != nil {
			return nil, fmt.Errorf("merge: apply %s: %w", filepath.Base(rf.input), err)
		}
		if res.Status != change.SetRefused {
			var landed []*model.Block
			for i, r := range res.Ops {
				status := mergeApplied
				if r.Status == change.OpUnchanged {
					status = mergeUnchanged
				}
				note(mergeOutcome{Block: units[i].ID, Status: status})
				landed = append(landed, units[i])
			}
			if res.Status == change.SetPartial {
				return landed, fmt.Errorf("merge: %s: an error interrupted writing the translations of %s, and part of them landed", filepath.Base(rf.input), rf.doc)
			}
			return landed, nil
		}
		var keepOps []change.Op
		var keepUnits []*model.Block
		refused := false
		for i, r := range res.Ops {
			if r.Status != change.OpRefused {
				keepOps, keepUnits = append(keepOps, ops[i]), append(keepUnits, units[i])
				continue
			}
			refused = true
			e := r.Error
			switch {
			case e != nil && e.Code == change.CodeStale && e.Field == "if_match" && r.Current != nil:
				if a.translatorWins(task, rf) {
					op := ops[i]
					op.IfMatch = r.Current.Rev
					keepOps, keepUnits = append(keepOps, op), append(keepUnits, units[i])
					continue
				}
				note(mergeOutcome{Block: units[i].ID, Status: mergeSkipped, Error: e})
			case e != nil && (e.Code == change.CodeStale || e.Code == change.CodeNotFound):
				note(mergeOutcome{Block: units[i].ID, Status: mergeStale, Error: e})
			case e != nil && e.Code == change.CodeGateFailed:
				note(mergeOutcome{Block: units[i].ID, Status: mergeRefused, Error: everyFinding(e, res.Docs, ops[i].At)})
			default:
				note(mergeOutcome{Block: units[i].ID, Status: mergeRefused, Error: e})
			}
		}
		if !refused {
			return nil, fmt.Errorf("merge: %s: the change set was refused with no operation refused", filepath.Base(rf.input))
		}
		ops, units = keepOps, keepUnits
	}
	return nil, nil
}

// codesChanged refuses a translation that drops an inline code its source
// holds as guard/codes_changed, with the codes as a read shows them: what the
// source holds and the translation lacks as expected, and what the
// translation adds besides as found. The commit check would refuse it too,
// naming one code at a time. A code the translation adds, such as a
// redaction placeholder a merge keeps with --no-restore, is the change
// service's to judge.
func codesChanged(source, target []model.Run) *change.Error {
	d := model.DiffRunCodes(source, target)
	if len(d.MissingCodes()) == 0 {
		return nil
	}
	tokens, order := codeTokens(source, target)
	render := func(sigs []string) string {
		if len(sigs) == 0 {
			return "none"
		}
		// In the order the codes stand in the source, then the translation.
		slices.SortStableFunc(sigs, func(a, b string) int { return order[a] - order[b] })
		out := make([]string, 0, len(sigs))
		for _, sig := range sigs {
			out = append(out, cmp.Or(tokens[sig], sig))
		}
		return strings.Join(out, " ")
	}
	expected, found := render(d.MissingCodes()), render(d.ExtraCodes())
	e := &change.Error{Code: change.CodeGuard, Subcode: change.SubcodeCodesChanged,
		Message: fmt.Sprintf("expected %s, found %s: keep every <x id=\"…\"/> code of the source in the translation", expected, found)}
	if expected != "none" {
		e.Expected = expected
	}
	if found != "none" {
		e.Found = found
	}
	return e
}

// codeTokens maps each code signature (model.RunCodeSignature) of runs to the
// <x id="…"/> token a read shows the code as, and to the place it first
// stands in them.
func codeTokens(runs ...[]model.Run) (map[string]string, map[string]int) {
	out, order := map[string]string{}, map[string]int{}
	var walk func([]model.Run)
	walk = func(rs []model.Run) {
		for _, r := range rs {
			if sig, ok := model.RunCodeSignature(r); ok {
				if _, seen := out[sig]; !seen {
					out[sig] = model.RunsPlaceholderText([]model.Run{r})
					order[sig] = len(order)
				}
				continue
			}
			switch {
			case r.Plural != nil:
				for _, f := range r.Plural.Forms {
					walk(f)
				}
			case r.Select != nil:
				for _, c := range r.Select.Cases {
					walk(c)
				}
			}
		}
	}
	for _, rs := range runs {
		walk(rs)
	}
	return out, order
}

// everyFinding is a gate refusal's error with every failing finding the
// commit check reported for the edition at, not only the first the
// operation's message names.
func everyFinding(e *change.Error, docs []change.DocResult, at change.Ref) *change.Error {
	var msgs []string
	for _, d := range docs {
		for _, f := range d.Findings {
			if !f.Fails || f.At == nil || f.At.Block != at.Block || f.At.Edition.Canonical() != at.Edition.Canonical() {
				continue
			}
			msgs = append(msgs, f.Message)
		}
	}
	if len(msgs) < 2 {
		return e
	}
	out := *e
	out.Message = fmt.Sprintf("%d failing findings: %s", len(msgs), strings.Join(msgs, "; "))
	return &out
}

// translatorWins says whether the conflict policy gives a unit whose
// translation changed in the project since the extraction to the translator:
// always under translator-wins, never under existing-wins, and under
// newest-wins when the returned file is newer than the translation's file.
func (a *App) translatorWins(task mergeTask, rf *returnedFile) bool {
	switch task.policy {
	case project.ConflictPolicyExistingWins:
		return false
	case project.ConflictPolicyNewestWins:
		in, err := os.Stat(rf.input)
		if err != nil {
			return false
		}
		target, err := resolveMergeOutputPath(&project.ExtractionFile{Source: rf.doc}, task.project, task.layout.Root, rf.locale)
		if err != nil {
			return false
		}
		held, err := os.Stat(target)
		if errors.Is(err, os.ErrNotExist) {
			return true
		}
		return err == nil && in.ModTime().After(held.ModTime())
	default:
		return true
	}
}

// materializeActor is who kapi merge's materializing form sends its change
// sets as: the tool that writes what the project's runs stored, whose drafts
// meet the ship gates later.
var materializeActor = change.Actor{Kind: change.ActorTool, Name: "merge"}

// materializeServices builds the change service a materialize pass writes
// each language's translations through, once per language.
type materializeServices struct {
	app    *App
	ctx    context.Context
	recipe string
	source model.LocaleID
	built  map[model.LocaleID]*change.Service
}

func (m *materializeServices) of(locale model.LocaleID) (*change.Service, error) {
	if svc, ok := m.built[locale]; ok {
		return svc, nil
	}
	svc, err := m.app.ChangeService(m.ctx, ChangeServiceOptions{
		Project: m.recipe, Origin: "merge", SourceLocale: m.source, TargetLocale: locale, Materialize: true,
	})
	if err != nil {
		return nil, err
	}
	if m.built == nil {
		m.built = map[model.LocaleID]*change.Service{}
	}
	m.built[locale] = svc
	return svc, nil
}

// materializeEdition gives the translation of doc into locale every block's
// edition the workspace home keeps (kept, read with its head) and, for a
// block it keeps none of, the target the block store holds, as one change set
// the service writes from the source's skeleton. A kept edition is written
// with the basis it was made from, so one made from a source that has changed
// since reads as stale in its file as it did where it was kept. It returns
// how many blocks have a translation to write, and whether the translation's
// file was written: a file that already holds every one of them and every
// block of the source is left as it is. ctx addresses the stored overlays by
// doc's key (blockstore.WithSourceRel).
func materializeEdition(ctx context.Context, svc *change.Service, store blockstore.Store, kept *workhome.Held, doc string, locale model.LocaleID) (int, bool, error) {
	sess, err := store.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer sess.Close()
	key := model.EditionKey{Locale: locale}
	kind := blockstore.TargetOverlayKind(locale)
	var ops []change.Op
	_, err = svc.ReadEach(ctx, change.ReadRequest{Doc: doc, Editions: []model.EditionKey{key}}, func(b *model.Block, r change.BlockRead) error {
		if !b.Translatable || b.ID == "" {
			return nil
		}
		at := change.Ref{Doc: doc, Block: r.Ref.Block, Edition: key}
		if kept != nil {
			if ed, ok := kept.Blocks[change.BlockKey(b)]; ok && model.RunsHaveContent(ed.Runs) {
				// The workspace home is the edition's home until this
				// delivery: what it keeps is the translation, whatever the
				// cache holds.
				ops = append(ops, change.Op{Kind: change.KindSetContent, At: at,
					IfMatch: model.EditionRevision(b, key), Basis: kept.Rows[change.BlockKey(b)].Basis,
					Body: &change.SetContent{Runs: ed.Runs}})
				return nil
			}
		}
		// Absence and failure differ: ErrNotFound is a block with no
		// translation yet, anything else a store that could not be read.
		o, gerr := sess.GetOverlay(kind, blockstore.OverlayKey(ctx, b.ID, b.SourceText()))
		if errors.Is(gerr, blockstore.ErrNotFound) {
			return nil
		}
		if gerr != nil {
			return fmt.Errorf("read %s overlay for block %s: %w", kind, r.Ref.Block, gerr)
		}
		if len(o.Payload) == 0 {
			return nil
		}
		stored := &model.Block{ID: b.ID}
		if err := applyTargetOverlay(stored, locale, o.Payload); err != nil {
			return err
		}
		t := stored.Target(locale)
		if t == nil {
			return nil
		}
		ops = append(ops, change.Op{
			Kind: change.KindSetContent, At: at,
			IfMatch: model.EditionRevision(b, key),
			Body:    &change.SetContent{Runs: t.Runs},
		})
		return nil
	})
	if err != nil || len(ops) == 0 {
		return 0, false, err
	}
	res, err := svc.Apply(ctx, change.Set{Gate: change.GateReport, Note: "materialize " + string(locale), Ops: ops}, materializeActor)
	if err != nil {
		return 0, false, err
	}
	if res.Status != change.SetApplied {
		for _, r := range res.Ops {
			if r.Status == change.OpRefused && r.Error != nil {
				return 0, false, fmt.Errorf("block %s: %w", refBlock(r.At), r.Error)
			}
		}
		return 0, false, fmt.Errorf("the change set was %s", res.Status)
	}
	written := false
	for _, d := range res.Docs {
		written = written || (d.Written && d.Edition != "")
	}
	return len(ops), written, nil
}

// refBlock names the block a result addresses.
func refBlock(at *change.Ref) string {
	if at == nil {
		return "?"
	}
	return at.Block
}

// blockList names up to three blocks in a message.
func blockList(ids []string) string {
	const shown = 3
	switch {
	case len(ids) == 1:
		return "block " + ids[0]
	case len(ids) <= shown:
		return "blocks " + strings.Join(ids, ", ")
	}
	return fmt.Sprintf("blocks %s and %d more", strings.Join(ids[:shown], ", "), len(ids)-shown)
}

// sameSource reports whether the source a unit carries is the source block
// cur holds: compared as plain text for a carrier that holds plain text, and
// otherwise as its original data, codes included, the way a returned file
// carries the markup. A unit that names its basis (based) may also carry each
// code as the code it is, the way an XLIFF extract does (a <pc> or <ph> with
// its native form in originalData): the basis then answers for the codes'
// data, and the change service refuses the unit as stale when the source
// moved. A unit with no basis has nothing else to answer for that data, so a
// source whose only change is a code's data, such as a link's address, is not
// the source it carries.
func sameSource(unit, cur *model.Block, plain, based bool) bool {
	if plain {
		return unit.SourceText() == cur.SourceText()
	}
	if based && model.RunsPlaceholderText(unit.Source) == model.RunsPlaceholderText(cur.Source) {
		return true
	}
	return model.RenderRunsWithData(unit.Source) == model.RenderRunsWithData(cur.Source)
}

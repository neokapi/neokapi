package host

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
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

// stampUnitRevision records rev on b, for the writer of a bilingual file.
func stampUnitRevision(b *model.Block, rev unitRevision) {
	if b.Properties == nil {
		b.Properties = map[string]string{}
	}
	b.Properties[unitIfMatchProperty] = rev.IfMatch
	b.Properties[unitBasisProperty] = rev.Basis
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
	var ops []change.Op
	var units []*model.Block
	for _, u := range rf.blocks {
		target := u.Target(rf.locale)
		if target == nil || !hasAnyText(target.Runs) {
			note(mergeOutcome{Block: u.ID, Status: mergeSkipped})
			continue
		}
		cur, ok := current[u.ID]
		if !ok {
			note(mergeOutcome{Block: u.ID, Status: mergeStale, Error: &change.Error{Code: change.CodeNotFound, Field: "at/block",
				Message: fmt.Sprintf("%s holds no block %s", rf.doc, u.ID)}})
			continue
		}
		rev, _ := unitRevisionOf(u)
		if rev.Basis == "" {
			// A unit that carries no basis was made from the source it
			// carries, or, carrying none, from the file the return names.
			switch {
			case len(u.Source) > 0 && sameSource(u, cur.block, rf.plainSource), len(u.Source) == 0 && unchangedFile:
				rev.Basis = cur.authRev
			default:
				note(mergeOutcome{Block: u.ID, Status: mergeStale, Error: &change.Error{Code: change.CodeStale, Field: "basis",
					Message: fmt.Sprintf("the source of block %s changed since the unit was extracted", cur.ref)}})
				continue
			}
		}
		ops = append(ops, change.Op{
			Kind: change.KindSetContent, At: change.Ref{Doc: rf.doc, Block: cur.ref, Edition: key},
			IfMatch: cmp.Or(rev.IfMatch, cur.rev), Basis: rev.Basis,
			Body: &change.SetContent{Content: change.Content{Runs: target.Runs}},
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
			byReason[why] = append(byReason[why], o.Block)
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
// change set, and notes each unit's outcome. A refused unit is dropped and
// the rest sent again: a unit whose source moved is stale, one the conflict
// policy keeps the project's translation for is skipped, one the policy
// gives to the translator is sent again over the revision the project now
// holds, and any other refusal is refused. It returns the units that landed.
func (a *App) applyReturned(ctx context.Context, svc *change.Service, task mergeTask, rf *returnedFile, ops []change.Op, units []*model.Block, note func(mergeOutcome)) ([]*model.Block, error) {
	set := change.Set{RequireBasis: true, Gate: change.GateReport, Note: "merge " + filepath.Base(rf.input)}
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

// materializeEdition gives the translation of doc into locale every target
// the block store holds for its blocks, as one change set the service writes
// from the source's skeleton, and returns how many blocks the store holds a
// target for. ctx addresses the stored overlays by doc's key
// (blockstore.WithSourceRel).
func materializeEdition(ctx context.Context, svc *change.Service, store blockstore.Store, doc string, locale model.LocaleID) (int, error) {
	sess, err := store.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer sess.Close()
	key := model.EditionKey{Locale: locale}
	kind := blockstore.TargetOverlayKind(locale)
	var ops []change.Op
	_, err = svc.ReadEach(ctx, change.ReadRequest{Doc: doc, Editions: []model.EditionKey{key}}, func(b *model.Block, r change.BlockRead) error {
		if !b.Translatable || b.ID == "" {
			return nil
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
			Kind: change.KindSetContent, At: change.Ref{Doc: doc, Block: r.Ref.Block, Edition: key},
			IfMatch: model.EditionRevision(b, key),
			Body:    &change.SetContent{Content: change.Content{Runs: t.Runs}},
		})
		return nil
	})
	if err != nil || len(ops) == 0 {
		return 0, err
	}
	res, err := svc.Apply(ctx, change.Set{Gate: change.GateReport, Note: "materialize " + string(locale), Ops: ops}, materializeActor)
	if err != nil {
		return 0, err
	}
	if res.Status != change.SetApplied {
		for _, r := range res.Ops {
			if r.Status == change.OpRefused && r.Error != nil {
				return 0, fmt.Errorf("block %s: %w", refBlock(r.At), r.Error)
			}
		}
		return 0, fmt.Errorf("the change set was %s", res.Status)
	}
	return len(ops), nil
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
// otherwise with inline codes rendered as their original data, which is how a
// returned XLIFF carries them.
func sameSource(unit, cur *model.Block, plain bool) bool {
	if plain {
		return unit.SourceText() == cur.SourceText()
	}
	return model.RenderRunsWithData(unit.Source) == model.RenderRunsWithData(cur.Source)
}

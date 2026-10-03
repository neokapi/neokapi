package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/workhome"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/kpz"
	"github.com/neokapi/neokapi/memory"
)

// restoreRedactedBlocks restores redacted originals into the incoming
// translated blocks using the batch's vault sidecar, if one exists. A missing
// sidecar (batch wasn't redacted) is a no-op. Merge runs it before it compiles
// the units into a change set.
//
// The incoming source is ALWAYS restored: a unit that carries no basis is
// compared with the source as it stands, which holds the originals, so the
// placeholders must be reverted for that comparison to hold. The translated
// target is restored only when restoreTarget is set — passing false (the
// --no-restore flag) leaves placeholders in the merged output.
func restoreRedactedBlocks(layout project.Layout, batchID string, blocks []*model.Block, targetLocale model.LocaleID, restoreTarget bool) error {
	sidecar := layout.RedactionSidecarPath(batchID)
	if _, err := os.Stat(sidecar); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	vault, err := redaction.OpenFileVault(sidecar)
	if err != nil {
		return err
	}
	for _, b := range blocks {
		get := func(token string) (string, bool) {
			v, ok := vault.Get(b.ID, token)
			return v.Original, ok
		}
		entries := redaction.ValuesForBlock(vault, b.ID)
		restore := func(runs []model.Run) ([]model.Run, int) {
			runs, n1 := redaction.Restore(runs, get)
			runs, n2 := redaction.RestoreText(runs, entries)
			return runs, n1 + n2
		}
		if sr, n := restore(b.SourceRuns()); n > 0 {
			b.SetSourceRuns(sr)
		}
		if restoreTarget {
			if tr, n := restore(b.TargetRuns(targetLocale)); n > 0 {
				b.SetTargetRuns(targetLocale, tr)
			}
		}
	}
	return nil
}

// MergeCmdOptions exists so bowrain/kapi callers can inject hooks later;
// nothing is needed today.
type MergeCmdOptions struct{}

func (a *App) RunMerge(cmd Command) error {
	projectPath, err := RequireProjectPath(cmd)
	if err != nil {
		return err
	}
	proj, err := a.LoadProjectInteractive(cmd.Context(), projectPath, LoadProjectInteractiveOptions{
		AssumeYes: a.AssumeYes,
	})
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	ctx := project.NewProjectContext(proj, projectPath)
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return fmt.Errorf("resolve project layout: %w", err)
	}

	inputs, _ := cmd.Flags().GetStringArray("input")
	if len(inputs) == 0 {
		return errors.New("merge: -i <file|glob|dir> is required (repeatable)")
	}
	expanded, err := expandMergeInputs(inputs, layout.Root)
	if err != nil {
		return err
	}
	if len(expanded) == 0 {
		return errors.New("merge: no input files matched. Check -i paths and globs")
	}

	noMemoryUpdate := BoolFlag(cmd, "no-memory-update")
	noRestore, _ := cmd.Flags().GetBool("no-restore")

	var tm *projector.Memory
	if !noMemoryUpdate {
		w, derr := a.Projector(CmdContext(cmd), layout.Root)
		if derr != nil {
			fmt.Fprintf(os.Stderr, "Warning: merge: open project store: %v (continuing with --no-memory-update semantics)\n", derr)
		} else {
			tm = w.With(projector.Origin{By: "merge"}).Memory()
		}
	}
	// One write for the whole run, not one per merged block — see memoryAbsorber.
	absorber := a.newMemoryAbsorber(tm)

	policy := proj.Defaults.Merge.ResolvedConflictPolicy()

	// The structured result (output.MergeOutput): its FormatText renders the
	// historical human report byte-for-byte, so text mode is unchanged while
	// --json gets a documented document on stdout.
	res := output.MergeOutput{ConflictPolicy: policy}
	sink, progressReport := progressSink(cmd)
	emit := func(ev FlowRunEvent) {
		if sink != nil {
			ev.Flow = "merge"
			sink(ev)
		}
	}
	start := time.Now()

	var totals mergeStats
	failures := 0

	for i, in := range expanded {
		rel := relOrAbs(layout.Root, in)
		emit(FlowRunEvent{Type: FlowEventProgress, FileIndex: i, FileCount: len(expanded), FilePath: in})
		stats, err := a.mergeOne(cmd.Context(), mergeTask{
			layout:    layout,
			ctx:       ctx,
			input:     in,
			policy:    policy,
			mem:       absorber,
			project:   proj,
			recipe:    projectPath,
			noRestore: noRestore,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "merge: %s: %v\n", rel, err)
			failures++
			res.Files = append(res.Files, output.MergeFileOutput{Input: rel, Error: err.Error()})
			continue
		}
		totals.accumulate(stats)
		res.Files = append(res.Files, output.MergeFileOutput{
			Input:   rel,
			Applied: stats.Applied, Stale: stats.Stale, Skipped: stats.Skipped, Refused: stats.Refused,
			MemoryNew: stats.MemoryNew, MemoryUpdated: stats.MemoryUpdated,
		})
		emit(FlowRunEvent{Type: FlowEventFileDone, FilePath: in})
	}

	// Reported, not fatal: the merged files are the deliverable and they are
	// already written. Losing the leverage silently is what must not happen.
	if ferr := absorber.flush(cmd.Context()); ferr != nil {
		fmt.Fprintf(os.Stderr, "Warning: merge: %v (the merged files were written)\n", ferr)
	}

	res.Applied, res.Stale, res.Skipped, res.Refused = totals.Applied, totals.Stale, totals.Skipped, totals.Refused
	res.MemoryNew, res.MemoryUpdated = totals.MemoryNew, totals.MemoryUpdated
	res.Failures = failures

	// The complete event closes the stream whatever happened (errors travel as
	// the returned error, per the FlowRunEvent contract) — but its message must
	// not read like an unqualified success when inputs failed.
	message := fmt.Sprintf("Merged %d file(s)", len(expanded)-failures)
	if failures > 0 {
		message = fmt.Sprintf("Merged %d file(s), %d failed", len(expanded)-failures, failures)
	}
	emit(FlowRunEvent{
		Type:       FlowEventComplete,
		DurationMs: time.Since(start).Milliseconds(), FilesProcessed: len(expanded) - failures,
		Message: message,
	})

	if err := output.Print(cmd, res); err != nil {
		return err
	}

	if failures > 0 {
		return fmt.Errorf("merge: %d input file(s) failed. See errors above", failures)
	}
	if err := progressReport(); err != nil {
		return err
	}
	// A refused unit is work left: its target file holds the source, or the
	// translation it held, where the returned translation was meant to be. The
	// merge exits 3, as a refused change set does, so a command chained after
	// it does not go on with a file half in the source language.
	if totals.Refused > 0 {
		return WithExitCode(ExitGate, fmt.Errorf("merge: %d unit(s) refused; fix each target the lines above name and merge again", totals.Refused))
	}
	return nil
}

// MergeFromProjectStore materializes localized files from the project block
// store (AD-026 §3): for each project source × target locale it gives the
// source's translation the stored `targets/<locale>` overlays (recomputing
// nothing) through the change service, which writes the file the source's
// target template names. This is the sink half of the process-only loop —
// `kapi run flow -i src.json` (in a project, no -o) commits overlays; `kapi
// merge` (no -i) writes the files.
func (a *App) MergeFromProjectStore(cmd Command) error {
	ctx := cmd.Context()
	projectPath, err := RequireProjectPath(cmd)
	if err != nil {
		return err
	}
	proj, err := a.LoadProjectInteractive(ctx, projectPath, LoadProjectInteractiveOptions{AssumeYes: a.AssumeYes})
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	locales := proj.Defaults.TargetLanguages
	if len(locales) == 0 {
		return errors.New("merge: project declares no target languages (defaults.target_languages)")
	}
	noMemoryUpdate := BoolFlag(cmd, "no-memory-update")

	// In JSON mode the per-file "Merged X → Y" lines are suppressed (stdout
	// carries only the result document); text mode streams them live as before.
	lineOut := cmd.OutOrStdout()
	if output.ResolveFormat(cmd) == output.FormatJSON {
		lineOut = io.Discard
	}
	// A collection in a format no installed reader opens has nothing in the
	// store and cannot be rewritten, so the merge leaves it out and names it.
	unread := a.newUnreadSetFor("materialized")
	written, err := a.materializeProject(ctx, lineOut, proj, projectPath, locales, noMemoryUpdate, unread)
	if err != nil {
		return err
	}
	unread.warn(a, cmd)
	return output.Print(cmd, output.MergeStoreOutput{Written: written, FromProjectStore: true, Warnings: unread.warnings()})
}

// materializeFromProjectStore is the shared materialize path (#1078 C2/C3):
// it writes the localized files for the given locales from the project block
// store — for each source and locale one change set of set_content operations
// carrying the stored `targets/<locale>` overlays, which the change service
// writes from the source's skeleton (materializeEdition). A collection that
// names no target is source-only and gets no file, and a translation the
// store holds nothing for is left as it is. `kapi merge` (no -i) calls it over
// every target language; `kapi up` calls it after the loop for the shippable
// locales when the materialize policy (defaults.materialize / --materialize)
// says so. Returns the number of files written.
func (a *App) materializeFromProjectStore(ctx context.Context, out io.Writer, proj *project.KapiProject, projectPath string, locales []model.LocaleID, noMemoryUpdate bool) (int, error) {
	return a.materializeProject(ctx, out, proj, projectPath, locales, noMemoryUpdate, nil)
}

// materializeProject is materializeFromProjectStore for a run that set content
// aside: the files unread names were never read, so the run produced nothing
// for them and writes nothing for them.
func (a *App) materializeProject(ctx context.Context, out io.Writer, proj *project.KapiProject, projectPath string, locales []model.LocaleID, noMemoryUpdate bool, unread *UnreadSet) (int, error) {
	pctx := project.NewProjectContext(proj, projectPath)
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return 0, fmt.Errorf("resolve project layout: %w", err)
	}
	if len(locales) == 0 {
		return 0, errors.New("merge: no target locales to materialize")
	}

	if err := project.EnsureLayout(layout); err != nil {
		return 0, fmt.Errorf("merge: ensure project layout: %w", err)
	}
	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return 0, fmt.Errorf("merge: open project store: %w", err)
	}
	store := a.projectBlocksAutocommit(db)
	if store == nil {
		return 0, fmt.Errorf("merge: read the block cache: %w", projectdb.ErrNoStore)
	}

	files, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return 0, fmt.Errorf("merge: resolve project content: %w", err)
	}
	// A file declared for its comments alone has no target to materialize, and
	// neither has a file in a format no installed reader opens, nor one whose
	// collection names no target: that content is source-only.
	kept := files[:0]
	sourceOnly := 0
	for _, f := range files {
		switch {
		case f.CommentsOnly():
		case f.Item == nil || f.Item.Target == "":
			sourceOnly++
		case !a.setAside(unread, filepath.Dir(projectPath), f):
			kept = append(kept, f)
		}
	}
	files = kept
	if len(files) == 0 {
		switch {
		case !unread.empty():
			return 0, fmt.Errorf("merge: nothing was materialized: %s", unread.summary())
		case sourceOnly > 0:
			// Every file is source-only: there is no translation to write.
			return 0, nil
		}
		return 0, errors.New("merge: project has no source files to materialize (check content patterns)")
	}

	// Admission: refuse the whole materialize pass when any destination is
	// blocked (#1449), rather than writing some locales and then failing —
	// nothing is written yet, and every blocked path is named at once.
	if berr := checkMaterializeTargetsWritable(proj, layout.Root, files, locales); berr != nil {
		return 0, fmt.Errorf("merge: %w", berr)
	}

	var tm *projector.Memory
	if !noMemoryUpdate {
		w, werr := a.Projector(ctx, db.Layout().Root)
		if werr != nil {
			return 0, fmt.Errorf("merge: %w", werr)
		}
		tm = w.With(projector.Origin{By: "merge"}).Memory()
	}
	// One write for the whole pass, not one per materialized block.
	absorber := a.newMemoryAbsorber(tm)

	// Materializing means writing what the store produced. A locale the store
	// holds no target for produces source fallback for every block, and writing
	// that over the committed translations destroys them — the exact loss on a
	// fresh clone, where the catalogs are complete (so the loop correctly runs no
	// pass, having nothing pending) and the block store is empty (so it has
	// nothing to say). Silence is the honest output for a store with nothing to
	// say; the locale's standing is what coverage already reports.
	keptLocales, err := a.keptLocales(ctx, layout.Root)
	if err != nil {
		return 0, fmt.Errorf("merge: read the workspace home: %w", err)
	}
	locales, err = localesWithStoredTargets(ctx, store, keptLocales, locales)
	if err != nil {
		return 0, fmt.Errorf("merge: read the stored targets: %w", err)
	}
	if len(locales) == 0 {
		return 0, nil
	}

	// The change service writes each translation from its source's skeleton,
	// one service per language, for a bilingual source whose reader has to
	// be told the language it holds.
	services := &materializeServices{app: a, ctx: ctx, recipe: projectPath, source: pctx.SourceLocale}

	written := 0
	for _, f := range files {
		srcFormat := f.Format
		if srcFormat == "" {
			srcFormat = detectSourceFormat(a.FormatReg, pctx, f.Relative, f.Path)
		}
		if srcFormat == "" {
			return written, fmt.Errorf("merge: cannot detect format for source %s", f.Path)
		}
		for _, locale := range locales {
			entry := &project.ExtractionFile{Source: f.Relative}
			targetPath, terr := resolveMergeOutputPath(entry, proj, layout.Root, locale)
			if terr != nil {
				return written, terr
			}

			// Whole-image (target-asset) replacement: an existing localized
			// binary-asset variant is authoritative — keep it rather than clobber
			// it by re-materializing the source.
			if preserveAssetVariant(srcFormat, f.Path, targetPath) {
				written++
				continue
			}

			// Address the stored overlays by the same source-file-namespaced key
			// the run wrote them under (blockstore.StoreKey).
			fileCtx := blockstore.WithSourceRel(ctx, f.Relative)
			svc, serr := services.of(locale)
			if serr != nil {
				return written, fmt.Errorf("merge: %w", serr)
			}
			ref := filepath.ToSlash(f.Relative)
			held, wrote, keptEd, merr := a.deliverEdition(ctx, fileCtx, svc, store, layout.Root, ref, locale)
			if merr != nil {
				return written, fmt.Errorf("merge: materialize %s → %s: %w", f.Relative, locale, merr)
			}
			if held == 0 {
				// Neither the workspace home nor the store holds a
				// translation for this file: there is nothing to write.
				continue
			}
			if wrote {
				written++
			}

			// Absorb the materialized targets into the project content memory with merge
			// provenance, mirroring the XLIFF/PO/.kpz merge paths. TM write-back
			// is best-effort — the localized file is already on disk and is the
			// deliverable — but a failure is REPORTED rather than dropped: the
			// TM is how the next run recycles this work, and a silent failure to
			// record it looks like the translation never happened.
			if absorber != nil {
				if _, _, aerr := absorbStoreTargets(fileCtx, a.FormatReg, srcFormat, f.Path, pctx.SourceLocale, locale, store, keptBlocksOf(keptEd), absorber, f.Relative, pctx.FormatConfigFor(srcFormat, f.Item)); aerr != nil {
					fmt.Fprintf(os.Stderr, "Warning: merge: record %s → %s in the project content memory: %v (the target file was written)\n",
						f.Relative, locale, aerr)
				}
			}

			if wrote {
				fmt.Fprintf(out, "Merged %s → %s\n", f.Relative, targetPath)
			}
		}
	}

	// Reported, not fatal: the target files are already on disk.
	if ferr := absorber.flush(ctx); ferr != nil {
		fmt.Fprintf(os.Stderr, "Warning: merge: %v (the target files were written)\n", ferr)
	}

	return written, nil
}

// deliverEdition writes the translation of ref into locale from what the
// workspace home keeps of it and the targets the block store holds
// (materializeEdition), then releases from the workspace home exactly what it
// read there. The release expects the head the delivery read, so an edit to
// the kept edition that lands in between is never released unwritten: the
// delivery reads the edition again and writes it. It returns how many blocks
// had a translation to write, whether the file was written, and what the
// workspace home kept that the file now holds.
func (a *App) deliverEdition(ctx, fileCtx context.Context, svc *change.Service, store blockstore.Store, root, ref string, locale model.LocaleID) (int, bool, *workhome.Held, error) {
	edition := model.EditionKey{Locale: locale}
	wroteAny := false
	for attempt := 0; ; attempt++ {
		kept, err := a.keptEdition(ctx, root, ref, edition)
		if err != nil {
			return 0, false, nil, fmt.Errorf("read the %s drafts of %s: %w", locale, ref, err)
		}
		n, wrote, err := materializeEdition(fileCtx, svc, store, kept, ref, locale)
		if err != nil {
			return 0, wroteAny, nil, err
		}
		wroteAny = wroteAny || wrote
		err = a.releaseKept(ctx, root, ref, edition, kept, nil, materializeActor, "merge")
		if errors.Is(err, workspace.ErrHeadMoved) && attempt < 2 {
			continue
		}
		if err != nil {
			return n, wroteAny, kept, fmt.Errorf("release the %s drafts of %s from the workspace home: %w", locale, ref, err)
		}
		return n, wroteAny, kept, nil
	}
}

// keptBlocksOf is each block's edition a read of the workspace home found,
// nil for none.
func keptBlocksOf(kept *workhome.Held) map[string]model.Edition {
	if kept == nil {
		return nil
	}
	return kept.Blocks
}

// localesWithStoredTargets narrows a materialize pass to the locales the
// workspace home keeps an edition of (kept) or the block store holds a target
// for, preserving the caller's order. One overlay is enough: the question is
// whether the store has anything to say about the locale at all, not how far
// along it is.
func localesWithStoredTargets(ctx context.Context, store blockstore.Store, kept map[model.LocaleID]bool, locales []model.LocaleID) ([]model.LocaleID, error) {
	sess, err := store.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	var out []model.LocaleID
	for _, locale := range locales {
		if kept[model.NormalizeLocale(locale)] {
			out = append(out, locale)
			continue
		}
		for _, oerr := range sess.ListOverlays(blockstore.TargetOverlayKind(locale)) {
			if oerr != nil {
				return nil, oerr
			}
			out = append(out, locale)
			break
		}
	}
	return out, nil
}

// absorbStoreTargets reads the source blocks, applies each block's edition the
// workspace home kept (kept, by block key) or else the stored
// `targets/<locale>` overlay, and writes accepted source+target pairs into
// the project content memory with kapi-merge provenance. Returns (new,
// updated) counts.
func absorbStoreTargets(ctx context.Context, reg *registry.FormatRegistry, srcFormat, sourceAbs string, source, target model.LocaleID, store blockstore.Store, kept map[string]model.Edition, absorber *memoryAbsorber, sourceRel string, formatCfg map[string]any) (int, int, error) {
	// The recipe's configuration for this item, not an unconfigured read: the
	// overlays are addressed by the file-local block id, so a read that splits
	// the document differently pairs each block's source text with another
	// block's translation and writes that into the content memory.
	blocks, _, err := project.ReadSourceBlocks(ctx, reg, srcFormat, sourceAbs, source, target, formatCfg)
	if err != nil {
		return 0, 0, err
	}
	sess, err := store.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer sess.Close()
	kind := blockstore.TargetOverlayKind(target)
	newCount, updatedCount := 0, 0
	for _, b := range blocks {
		if !b.Translatable || b.ID == "" {
			continue
		}
		if ed, ok := kept[change.BlockKey(b)]; ok && model.RunsHaveContent(ed.Runs) {
			// The workspace home held this translation until the delivery
			// that wrote it: what it kept is what was delivered.
			b.SetTargetRuns(target, ed.Runs)
			n, u, aerr := absorber.absorb(ctx, b, source, target, "store", sourceRel, sourceAbs)
			if aerr != nil {
				return newCount, updatedCount, aerr
			}
			newCount += n
			updatedCount += u
			continue
		}
		o, oerr := sess.GetOverlay(kind, blockstore.OverlayKey(ctx, b.ID, b.SourceText()))
		if oerr != nil {
			// "No target overlay for this block" and "the block store could not
			// be read" are DIFFERENT outcomes and must not share a branch: the
			// first is ordinary pending work, the second loses translations that
			// exist. Conflating them made every absorb report a short count as
			// success — the same shape as #1449, one layer up. ErrNotFound is the
			// documented absence sentinel (core/blockstore); anything else is a
			// read failure and fails the absorb.
			if errors.Is(oerr, blockstore.ErrNotFound) {
				continue
			}
			return newCount, updatedCount, fmt.Errorf("read %s overlay for block %s of %s: %w", kind, blockKey(b), sourceRel, oerr)
		}
		if len(o.Payload) == 0 {
			continue // recorded but empty — nothing to absorb
		}
		if oaerr := applyTargetOverlay(b, target, o.Payload); oaerr != nil {
			return newCount, updatedCount, oaerr
		}
		n, u, aerr := absorber.absorb(ctx, b, source, target, "store", sourceRel, sourceAbs)
		if aerr != nil {
			return newCount, updatedCount, aerr
		}
		newCount += n
		updatedCount += u
	}
	return newCount, updatedCount, nil
}

type mergeTask struct {
	layout  project.Layout
	ctx     *project.ProjectContext
	input   string
	policy  string
	mem     *memoryAbsorber
	project *project.KapiProject
	// recipe is the project's recipe file, which the change service edits.
	recipe string

	// noRestore disables restoring redacted originals from the batch vault.
	noRestore bool
}

type mergeStats struct {
	Applied       int
	Stale         int
	Skipped       int
	Refused       int
	MemoryNew     int
	MemoryUpdated int
}

func (s *mergeStats) accumulate(o mergeStats) {
	s.Applied += o.Applied
	s.Stale += o.Stale
	s.Skipped += o.Skipped
	s.Refused += o.Refused
	s.MemoryNew += o.MemoryNew
	s.MemoryUpdated += o.MemoryUpdated
}

// MergeOneKpz ingests a bilingual interchange .kpz returned by a translator
// (kind=kapi-interchange, AD-025 §7): it validates the profile and merges the
// target overlays of each source it carries through the change service, as
// kapi merge -i merges an XLIFF or PO file, with the revisions the
// interchange task records for each unit as their if_match and basis.
func (a *App) MergeOneKpz(cmd Command, kpzInput string) error {
	ctx := cmd.Context()
	pkg, err := LoadWorkspace(kpzInput)
	if err != nil {
		return err
	}
	if pkg.Kind != kpz.KindInterchange {
		return fmt.Errorf("merge: %s is not a bilingual interchange .kpz (kind=%q)", filepath.Base(kpzInput), pkg.Kind)
	}
	if pkg.InterchangeTask == nil {
		return fmt.Errorf("merge: %s has no interchange task metadata", filepath.Base(kpzInput))
	}

	projectPath, err := RequireProjectPath(cmd)
	if err != nil {
		return err
	}
	proj, err := a.LoadProjectInteractive(ctx, projectPath, LoadProjectInteractiveOptions{AssumeYes: a.AssumeYes})
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	pctx := project.NewProjectContext(proj, projectPath)
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return fmt.Errorf("resolve project layout: %w", err)
	}

	targetLocale := model.LocaleID(pkg.InterchangeTask.TargetLocale)
	if targetLocale == "" {
		return errors.New("merge: interchange package has no target locale")
	}
	// The locale is interpolated into the output path as one segment, so a
	// package cannot be allowed to spell it as a path.
	if err := checkPathSegment(string(targetLocale), "merge: package target locale"); err != nil {
		return err
	}
	policy := proj.Defaults.Merge.ResolvedConflictPolicy()

	var tm *projector.Memory
	if !BoolFlag(cmd, "no-memory-update") {
		// Warned, like the two sibling merge paths above: a content memory that
		// failed to open reported `memory_new=0 memory_updated=0`, which reads as
		// "nothing new to learn" rather than "it was never opened" — so the
		// leverage is lost for this bundle with no way to tell.
		if w, derr := a.Projector(CmdContext(cmd), layout.Root); derr != nil {
			fmt.Fprintf(os.Stderr, "Warning: merge: open project store: %v (continuing without write-back)\n", derr)
		} else {
			tm = w.With(projector.Origin{By: "merge"}).Memory()
		}
	}
	// One write for the whole package, not one per merged block.
	absorber := a.newMemoryAbsorber(tm)
	task := mergeTask{layout: layout, ctx: pctx, input: kpzInput, policy: policy, mem: absorber, project: proj, recipe: projectPath}

	var stats mergeStats
	for _, si := range pkg.Sources {
		// The package names which of the project's sources it carries work
		// for, and that name decides the document the merge writes, so it is
		// contained first.
		if _, err := containedJoin(layout.Root, si.SourcePath, "merge: package source path"); err != nil {
			return err
		}
		member := kpz.SourceDir + filepath.Base(si.SourcePath)
		rf := &returnedFile{input: kpzInput, doc: si.SourcePath, locale: targetLocale, reference: "kpz", sourceHash: si.ContentHash}
		for _, ov := range pkg.Overlays {
			if blockstore.CanonicalOverlayKind(ov.Kind) != blockstore.TargetOverlayKind(targetLocale) || (ov.Source != "" && ov.Source != member) {
				continue
			}
			b := &model.Block{ID: ov.BlockHash}
			if err := applyTargetOverlay(b, targetLocale, ov.Payload); err != nil {
				return err
			}
			if rev, ok := pkg.InterchangeTask.Revisions[ov.BlockHash]; ok {
				stampUnitRevision(b, unitRevision{IfMatch: rev.IfMatch, Basis: rev.Basis})
			}
			rf.blocks = append(rf.blocks, b)
		}
		s, _, err := a.mergeReturned(ctx, task, rf)
		if err != nil {
			return err
		}
		stats.accumulate(s)
	}

	if ferr := absorber.flush(ctx); ferr != nil {
		fmt.Fprintf(os.Stderr, "Warning: merge: record %s in the project content memory: %v (the merged files were written)\n",
			filepath.Base(kpzInput), ferr)
	}
	fmt.Fprintf(cmd.OutOrStdout(),
		"Merged %s → %s: %s\n", filepath.Base(kpzInput), targetLocale, stats.summary(policy))
	return nil
}

// summary is the counts of a merge as its report prints them: refused only
// when a unit was refused.
func (s mergeStats) summary(policy string) string {
	refused := ""
	if s.Refused > 0 {
		refused = fmt.Sprintf(" refused=%d", s.Refused)
	}
	return fmt.Sprintf("applied=%d stale=%d skipped=%d%s memory_new=%d memory_updated=%d (conflict_policy=%s)",
		s.Applied, s.Stale, s.Skipped, refused, s.MemoryNew, s.MemoryUpdated, policy)
}

// BoolFlag reads a bool flag, defaulting to false on error.
func BoolFlag(cmd Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}

// mergeOne merges a single returned XLIFF or PO file.
func (a *App) mergeOne(ctx context.Context, task mergeTask) (mergeStats, error) {
	var rf *returnedFile
	var err error
	switch ext := format.Ext(task.input); ext {
	case ".xliff", ".xlf":
		rf, err = readReturnedXLIFF(ctx, task.input)
	case ".po":
		if rf, err = readReturnedPO(task.input); err == nil {
			err = settlePOLanguage(task, rf)
		}
	default:
		return mergeStats{}, fmt.Errorf("merge: unsupported input extension %q (supported: .xliff, .xlf, .po)", ext)
	}
	if err != nil {
		return mergeStats{}, err
	}
	stats, _, err := a.mergeReturned(ctx, task, rf)
	return stats, err
}

// readReturnedXLIFF reads a returned XLIFF 2 file: the source it translates
// from its kapi source-file note, the language of its translations from its
// trgLang, and a block per unit with the revisions its metadata carries.
func readReturnedXLIFF(ctx context.Context, path string) (*returnedFile, error) {
	reader := xliff2.NewReader()
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := reader.Open(ctx, &model.RawDocument{URI: path, Reader: f, FormatID: "xliff2"}); err != nil {
		return nil, fmt.Errorf("xliff2 open: %w", err)
	}
	defer reader.Close()
	var layer *model.Layer
	var blocks []*model.Block
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			return nil, fmt.Errorf("xliff2 read: %w", res.Error)
		}
		switch res.Part.Type {
		case model.PartLayerStart:
			if l, ok := res.Part.Resource.(*model.Layer); ok && layer == nil {
				layer = l
			}
		case model.PartBlock:
			if b, ok := res.Part.Resource.(*model.Block); ok {
				blocks = append(blocks, b)
			}
		}
	}
	srcRel := xliff2.FilePropertyFromLayer(layer, xliff2.FileNoteCategoryKapi, xliff2.FileNoteIDSourceFile)
	if srcRel == "" {
		return nil, fmt.Errorf("merge: %s names no source file. Was it produced by kapi extract?", path)
	}
	locale := model.LocaleID("")
	if layer != nil {
		locale = model.LocaleID(strings.TrimSpace(layer.Properties["target-language"]))
	}
	if locale == "" {
		return nil, fmt.Errorf("merge: %s names no target language (trgLang)", path)
	}
	return &returnedFile{
		input: path, doc: srcRel, locale: locale,
		batch:      xliff2.BatchIDFromLayer(layer),
		sourceHash: xliff2.FilePropertyFromLayer(layer, xliff2.FileNoteCategoryKapi, xliff2.FileNoteIDSourceHash),
		blocks:     blocks,
	}, nil
}

// readReturnedPO reads a returned PO file: the source it translates from its
// kapi-source-file comment, the language of its translations from its
// header, and a block per entry with the revisions its comments carry. An
// entry with no kapi-block comment cannot be matched to a block and is read
// with no translation, so the merge skips it.
func readReturnedPO(path string) (*returnedFile, error) {
	po, err := ReadPOForMerge(path)
	if err != nil {
		return nil, fmt.Errorf("po read: %w", err)
	}
	if po.SourceFile == "" {
		return nil, fmt.Errorf("merge: %s has no kapi-source-file comment. Was it produced by kapi extract?", path)
	}
	if po.Language == "" {
		return nil, fmt.Errorf("merge: %s names no language in its header", path)
	}
	locale := model.LocaleID(po.Language)
	rf := &returnedFile{input: path, doc: po.SourceFile, locale: locale, batch: po.BatchID, sourceHash: po.SourceHash, plainSource: true}
	for _, mb := range po.Blocks {
		b := &model.Block{ID: mb.BlockID, Source: []model.Run{{Text: &model.TextRun{Text: mb.MsgID}}}}
		if mb.BlockID != "" && mb.MsgStr != "" {
			b.SetTargetText(locale, mb.MsgStr)
		}
		if mb.IfMatch != "" || mb.Basis != "" {
			stampUnitRevision(b, unitRevision{IfMatch: mb.IfMatch, Basis: mb.Basis})
		}
		rf.blocks = append(rf.blocks, b)
	}
	return rf, nil
}

// settlePOLanguage settles the language of a returned PO file's
// translations. Its header names it, and the merge takes the recipe's
// spelling of that language. A header that names none of the recipe's target
// languages (an editor rewrote it) gives way to the extraction pair that
// wrote the file, found by the name extract gave it in the batch's manifest;
// with no such pair the merge is refused.
func settlePOLanguage(task mergeTask, rf *returnedFile) error {
	if task.project == nil || len(task.project.Defaults.TargetLanguages) == 0 {
		return nil
	}
	targets := task.project.Defaults.TargetLanguages
	settled := model.LocaleID("")
	for _, t := range targets {
		if model.NormalizeLocale(t) == model.NormalizeLocale(rf.locale) {
			settled = t
		}
	}
	if settled == "" && rf.batch != "" {
		if m, err := project.LoadExtractionManifest(task.layout, rf.batch); err == nil {
			src := project.ResolvedFile{Relative: filepath.FromSlash(rf.doc)}
			for _, p := range m.Pairs {
				if bilingualOutputName(src, task.ctx.SourceLocale, p.TargetLocale, extractFormatPO) == filepath.Base(rf.input) {
					settled = p.TargetLocale
				}
			}
		}
	}
	if settled == "" {
		names := make([]string, len(targets))
		for i, t := range targets {
			names[i] = string(t)
		}
		return fmt.Errorf("merge: %s names %q as its language, which is none of the project's target languages (%s), and no extraction this project holds wrote it",
			filepath.Base(rf.input), rf.locale, strings.Join(names, ", "))
	}
	if settled != rf.locale {
		for _, b := range rf.blocks {
			if t := b.Target(rf.locale); t != nil {
				runs := t.Runs
				b.RemoveEdition(model.EditionKey{Locale: rf.locale})
				b.SetTargetRuns(settled, runs)
			}
		}
		rf.locale = settled
	}
	return nil
}

// detectSourceFormat picks the format for a source path: the format the
// claiming content item declares, or detection when it declares none. The
// claim is project.KapiProject.ItemForPath, the same rule content resolution
// applies, so a later item's explicit format is never read into a file an
// earlier item claimed.
func detectSourceFormat(reg *registry.FormatRegistry, ctx *project.ProjectContext, rel, abs string) string {
	if ctx != nil && ctx.Project != nil {
		if item, _, ok := ctx.Project.ItemForPath(rel); ok && item.Format != nil && item.Format.Name != "" {
			return item.Format.Name
		}
	}
	return ctx.DetectFormat(reg, abs)
}

// resolveMergeOutputPath returns the path to write the merged target
// source to. Falls back to a sensible default next to the source when
// the recipe does not declare a target template.
//
// Two inputs decide the destination and they carry different trust. The target
// template comes from the LOCAL recipe — the project owner's own file — so an
// absolute template stays a supported way to write outside the tree. The source
// path may come from a package, and it feeds both the default layout and the
// template's {path}/{filename} substitutions, so it is contained first: a
// package may only name a source inside the project it is merged into.
func resolveMergeOutputPath(entry *project.ExtractionFile, proj *project.KapiProject, root string, locale model.LocaleID) (string, error) {
	if _, err := containedJoin(root, entry.Source, "merge: source path"); err != nil {
		return "", err
	}
	if err := checkPathSegment(string(locale), "merge: target locale"); err != nil {
		return "", err
	}
	// The content item that claims entry.Source decides the destination; one
	// that declares no target falls through to the default below. The target
	// template supports {lang}, {path}, {filename}, {basename}, and the legacy
	// bare `*` — see project.ResolveTargetPath.
	if proj != nil {
		if item, _, ok := proj.ItemForPath(entry.Source); ok && item.Target != "" {
			tmpl := project.ResolveTargetPathIn(item.Path, item.Base, item.Target, entry.Source, string(locale), proj.Defaults.LocaleFormat)
			if !filepath.IsAbs(tmpl) {
				tmpl = filepath.Join(root, tmpl)
			}
			return tmpl, nil
		}
	}
	// Default: <source-dir>/<locale>/<basename>
	base := filepath.Base(entry.Source)
	return filepath.Join(root, filepath.Dir(entry.Source), string(locale), base), nil
}

// memoryAbsorber stages merged source/target pairs with kapi-merge provenance
// and writes them in one transaction at the end of the run. Batching reduces
// contention from repeated small write transactions.
//
// The bulk path skips per-row FTS5 maintenance and rebuilds search and fuzzy
// indexes afterward. This scans tm_variants even for a small merge, but avoids
// holding the writer throughout file processing.
//
// Write errors are returned for callers to report. They do not invalidate the
// merged files. The absorber is used by the sequential merge loop and is not
// safe for concurrent use.
type memoryAbsorber struct {
	app     *App
	tm      *projector.Memory
	entries []memory.Entry
	// staged maps an entry id to its position in entries. Entry ids are
	// content-derived, so one string merged from two files — or the same source
	// merged for a second target locale — is one entry, and the bulk path would
	// otherwise carry it twice.
	staged map[string]int
}

// newMemoryAbsorber returns an absorber over tm, or nil when there is no
// content memory to write to — every call site already guards on nil, which is
// the "--no-memory-update, or the store would not open" case.
func (a *App) newMemoryAbsorber(tm *projector.Memory) *memoryAbsorber {
	if tm == nil {
		return nil
	}
	return &memoryAbsorber{app: a, tm: tm, staged: map[string]int{}}
}

// absorb stages one merged block. The new/updated split is decided here, while
// the store still reflects the run's starting state — a read, so it takes no
// write lock.
func (m *memoryAbsorber) absorb(ctx context.Context, block *model.Block, source, target model.LocaleID, batchID, sourceRel, xliffPath string) (newCount, updatedCount int, err error) {
	if m == nil {
		return 0, 0, nil
	}
	entry, ok := mergeMemoryEntry(block, source, target, batchID, sourceRel, xliffPath)
	if !ok {
		return 0, 0, nil
	}
	if i, staged := m.staged[entry.ID]; staged {
		// A second target locale for the same source teaches another variant of
		// the entry already staged, not a competing entry: fold it in so the one
		// write carries every locale this run learned. The counts stay per
		// entry, so the fold adds neither a new nor an updated one.
		maps.Copy(m.entries[i].Variants, entry.Variants)
		return 0, 0, nil
	}
	_, existed, gerr := m.tm.GetEntry(ctx, entry.ID)
	if gerr != nil {
		return 0, 0, fmt.Errorf("read content-memory entry %s: %w", entry.ID, gerr)
	}
	m.staged[entry.ID] = len(m.entries)
	m.entries = append(m.entries, entry)
	if existed {
		return 0, 1, nil
	}
	return 1, 0, nil
}

// flush writes everything the run learned, then repopulates the search and
// fuzzy indexes the bulk path skipped. A run that learned nothing writes
// nothing — including no rebuild.
func (m *memoryAbsorber) flush(ctx context.Context) error {
	if m == nil || len(m.entries) == 0 {
		return nil
	}
	if err := m.tm.BulkAddWithStream(ctx, m.entries, ""); err != nil {
		return fmt.Errorf("record %d content-memory entr%s: %w",
			len(m.entries), map[bool]string{true: "y", false: "ies"}[len(m.entries) == 1], err)
	}
	m.entries, m.staged = nil, map[string]int{}
	m.app.RebuildMemorySearchIndexes(ctx, m.tm)
	return nil
}

// mergeMemoryEntry builds the content-memory entry a merged block teaches, or
// reports ok=false when the block carries no translatable pair.
func mergeMemoryEntry(block *model.Block, source, target model.LocaleID, batchID, sourceRel, xliffPath string) (memory.Entry, bool) {
	srcText := block.SourceText()
	tgtText := block.TargetText(target)
	if srcText == "" || tgtText == "" {
		return memory.Entry{}, false
	}
	// block.Identity can be nil on blocks built by readers that don't
	// compute content hashes eagerly; fall back to hashing the source
	// text so the TU id is still deterministic.
	contentHash := ""
	if block.Identity != nil {
		contentHash = block.Identity.ContentHash
	}
	if contentHash == "" {
		contentHash = model.ComputeContentHash(srcText)
	}
	now := time.Now().UTC()
	entry := memory.Entry{
		ID: fmt.Sprintf("merge:%s:%s", batchID, contentHash),
		Variants: map[model.LocaleID][]model.Run{
			source: {{Text: &model.TextRun{Text: srcText}}},
			target: {{Text: &model.TextRun{Text: tgtText}}},
		},
		HintSrcLang: source,
		Origins: []memory.Origin{{
			Source:    "merge",
			Key:       sourceRel,
			Reference: batchID,
			AddedAt:   now,
			AddedBy:   "kapi-merge",
		}},
		Properties: map[string]string{
			"kapi-merge:xliff-original":     filepath.Base(xliffPath),
			"kapi-merge:block-content-hash": contentHash,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	return entry, true
}

// bilingualReturnExts are the bilingual file extensions a directory argument
// contributes to `kapi merge`. A returned-translation directory routinely also
// holds notes, zips and the translator's scratch files, so a bare directory
// means "the bilingual files in here", not "everything in here".
var bilingualReturnExts = map[string]bool{".xliff": true, ".xlf": true, ".po": true}

// expandMergeInputs turns a mixed list of files/globs/dirs into a flat,
// de-duplicated list of regular files, relative to the project root. Globs and
// directories expand through the shared resolver (inputs.go), so `**` recurses
// here exactly as it does for every other command; a directory argument is
// additionally narrowed to the bilingual extensions merge can actually read.
func expandMergeInputs(inputs []string, root string) ([]string, error) {
	seen := make(map[string]bool)
	var out []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}

	for _, in := range inputs {
		abs := in
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(root, in)
		}
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			walked, werr := walkDirFiles(abs)
			if werr != nil {
				return nil, fmt.Errorf("merge: %w", werr)
			}
			for _, p := range walked {
				if bilingualReturnExts[format.Ext(p)] {
					add(p)
				}
			}
			continue
		}
		// A glob or a plain path: the shared expander handles both, and a
		// pattern that matches nothing simply contributes nothing (merge
		// reports the empty result itself).
		matches, err := expandArgs([]string{abs}, InputOptions{})
		if err != nil {
			return nil, fmt.Errorf("merge: %w", err)
		}
		for _, m := range matches {
			add(m)
		}
	}
	return out, nil
}

func hasAnyText(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text != nil && strings.TrimSpace(r.Text.Text) != "" {
			return true
		}
	}
	return false
}

func relOrAbs(root, abs string) string {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return abs
	}
	return rel
}

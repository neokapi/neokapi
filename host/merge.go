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

	"github.com/bmatcuk/doublestar/v4"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/kpz"
	"github.com/neokapi/neokapi/memory"
)

// restoreRedactedBlocks restores redacted originals into the incoming
// translated blocks using the batch's vault sidecar, if one exists. A missing
// sidecar (batch wasn't redacted) is a no-op.
//
// The incoming source is ALWAYS restored: the per-block staleness check in
// merge compares the XLIFF source text against the (unredacted) re-read source
// file, so the placeholders must be reverted for that comparison to hold. The
// translated target is restored only when restoreTarget is set — passing
// false (the --no-restore flag) leaves placeholders in the merged output.
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

	noMemoryUpdate := BoolFlagAny(cmd, "no-memory-update", "no-tm-update")
	noRestore, _ := cmd.Flags().GetBool("no-restore")

	var tm *memory.SQLiteStore
	// In the browser/seeded build (a.MemoryBackend set) there is no file-backed
	// SQLite driver and no project content memory to write back to, so skip
	// write-back silently rather than surfacing a driver error. The native CLI
	// (MemoryBackend == nil) writes to the project store.
	if !noMemoryUpdate && a.MemoryBackend == nil {
		db, derr := a.ProjectDB(CmdContext(cmd), layout.Root)
		if derr != nil {
			fmt.Fprintf(os.Stderr, "Warning: merge: open project store: %v (continuing with --no-memory-update semantics)\n", derr)
		} else {
			tm = db.Memory()
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
			Applied: stats.Applied, Stale: stats.Stale, Skipped: stats.Skipped,
			MemoryNew: stats.MemoryNew, MemoryUpdated: stats.MemoryUpdated,
		})
		emit(FlowRunEvent{Type: FlowEventFileDone, FilePath: in})
	}

	// Reported, not fatal: the merged files are the deliverable and they are
	// already written. Losing the leverage silently is what must not happen.
	if ferr := absorber.flush(cmd.Context()); ferr != nil {
		fmt.Fprintf(os.Stderr, "Warning: merge: %v (the merged files were written)\n", ferr)
	}

	res.Applied, res.Stale, res.Skipped = totals.Applied, totals.Stale, totals.Skipped
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
	// A truncated progress feed is reported after the result, so the deliverable
	// still lands and the consumer still learns its feed was incomplete.
	return progressReport()
}

// MergeFromProjectStore materializes localized files from the project block
// store (AD-026 §3): for each project source × target locale it reads the
// source, applies the stored `targets/<locale>` overlays via the
// hydrateTargetsTool (recomputing nothing), and writes the localized file to
// the source's output template. This is the sink half of the process-only
// loop — `kapi run flow -i src.json` (in a project, no -o) commits overlays;
// `kapi merge` (no -i) writes the files.
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
	noMemoryUpdate := BoolFlagAny(cmd, "no-memory-update", "no-tm-update")

	// In JSON mode the per-file "Merged X → Y" lines are suppressed (stdout
	// carries only the result document); text mode streams them live as before.
	lineOut := cmd.OutOrStdout()
	if output.ResolveFormat(cmd) == output.FormatJSON {
		lineOut = io.Discard
	}
	written, err := a.materializeFromProjectStore(ctx, lineOut, proj, projectPath, locales, noMemoryUpdate)
	if err != nil {
		return err
	}
	return output.Print(cmd, output.MergeStoreOutput{Written: written, FromProjectStore: true})
}

// materializeFromProjectStore is the shared materialize path (#1078 C2/C3):
// it writes the localized files for the given locales from the project block
// store — each source read once, the stored `targets/<locale>` overlays
// hydrated onto it, the localized file written via the source format's
// skeleton round-trip. `kapi merge` (no -i) calls it over every target
// language; `kapi up` calls it after the loop for the shippable locales when
// the materialize policy (defaults.materialize / --materialize) says so.
// Returns the number of files written.
func (a *App) materializeFromProjectStore(ctx context.Context, out io.Writer, proj *project.KapiProject, projectPath string, locales []model.LocaleID, noMemoryUpdate bool) (int, error) {
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
	store := db.BlocksAutocommit()
	if store == nil {
		return 0, fmt.Errorf("merge: read the block cache: %w", projectdb.ErrNoStore)
	}

	files, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return 0, fmt.Errorf("merge: resolve project content: %w", err)
	}
	if len(files) == 0 {
		return 0, errors.New("merge: project has no source files to materialize (check content patterns)")
	}

	// Admission: refuse the whole materialize pass when any destination is
	// blocked (#1449), rather than writing some locales and then failing —
	// nothing is written yet, and every blocked path is named at once.
	if berr := checkMaterializeTargetsWritable(proj, layout.Root, files, locales); berr != nil {
		return 0, fmt.Errorf("merge: %w", berr)
	}

	var tm *memory.SQLiteStore
	// Browser/seeded build (a.MemoryBackend set): no file-backed SQLite driver
	// and no project content memory — skip write-back silently. The native CLI
	// writes to the project store.
	if !noMemoryUpdate && a.MemoryBackend == nil {
		tm = db.Memory()
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
	locales, err = localesWithStoredTargets(ctx, store, locales)
	if err != nil {
		return 0, fmt.Errorf("merge: read the stored targets: %w", err)
	}
	if len(locales) == 0 {
		return 0, nil
	}

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

			runner := flow.NewFileRunner(flow.FileRunnerConfig{
				FormatReg:    a.FormatReg,
				SourceLocale: pctx.SourceLocale,
				Encoding:     pctx.Encoding,
				Store:        store,
				DetectFormat: func(string) registry.FormatID { return registry.FormatID(srcFormat) },
				// The recipe's configuration for THIS item, on both halves of the
				// round-trip. Materializing re-reads the source to rebuild its
				// skeleton, so a reader configured any other way splits the
				// document into a different set of blocks than extraction did and
				// the stored targets land on the wrong ones.
				ConfigureReader: func(reader format.DataFormatReader, detectedFmt registry.FormatID) error {
					return pctx.ConfigureReaderFor(reader, string(detectedFmt), f.Item)
				},
				ConfigureWriter: func(writer format.DataFormatWriter, fmtName registry.FormatID) error {
					return pctx.ConfigureWriterFor(writer, string(fmtName), f.Item)
				},
			})
			// Address the stored overlays by the same source-file-namespaced key
			// the run wrote them under (blockstore.StoreKey).
			fileCtx := blockstore.WithSourceRel(ctx, f.Relative)
			tools := []tool.Tool{newHydrateTargetsTool(locale)}
			if rerr := runner.RunFile(fileCtx, "merge", tools, f.Path, targetPath, string(locale)); rerr != nil {
				return written, fmt.Errorf("merge: materialize %s → %s: %w", f.Relative, locale, rerr)
			}
			written++

			// Absorb the materialized targets into the project content memory with merge
			// provenance, mirroring the XLIFF/PO/.kpz merge paths. TM write-back
			// is best-effort — the localized file is already on disk and is the
			// deliverable — but a failure is REPORTED rather than dropped: the
			// TM is how the next run recycles this work, and a silent failure to
			// record it looks like the translation never happened.
			if absorber != nil {
				if _, _, aerr := absorbStoreTargets(fileCtx, a.FormatReg, srcFormat, f.Path, pctx.SourceLocale, locale, store, absorber, f.Relative, pctx.FormatConfigFor(srcFormat, f.Item)); aerr != nil {
					fmt.Fprintf(os.Stderr, "Warning: merge: record %s → %s in the project content memory: %v (the target file was written)\n",
						f.Relative, locale, aerr)
				}
			}

			fmt.Fprintf(out, "Merged %s → %s\n", f.Relative, targetPath)
		}
	}

	// Reported, not fatal: the target files are already on disk.
	if ferr := absorber.flush(ctx); ferr != nil {
		fmt.Fprintf(os.Stderr, "Warning: merge: %v (the target files were written)\n", ferr)
	}

	return written, nil
}

// localesWithStoredTargets narrows a materialize pass to the locales the block
// store actually holds a target for, preserving the caller's order. One overlay
// is enough: the question is whether the store has anything to say about the
// locale at all, not how far along it is.
func localesWithStoredTargets(ctx context.Context, store blockstore.Store, locales []model.LocaleID) ([]model.LocaleID, error) {
	sess, err := store.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	var out []model.LocaleID
	for _, locale := range locales {
		for _, oerr := range sess.ListOverlays("targets/" + string(locale)) {
			if oerr != nil {
				return nil, oerr
			}
			out = append(out, locale)
			break
		}
	}
	return out, nil
}

// absorbStoreTargets reads the source blocks, applies the stored
// `targets/<locale>` overlays, and writes accepted source+target pairs into
// the project content memory with kapi-merge provenance. Returns (new, updated) counts.
func absorbStoreTargets(ctx context.Context, reg *registry.FormatRegistry, srcFormat, sourceAbs string, source, target model.LocaleID, store blockstore.Store, absorber *memoryAbsorber, sourceRel string, formatCfg map[string]any) (int, int, error) {
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
	kind := "targets/" + string(target)
	newCount, updatedCount := 0, 0
	for _, b := range blocks {
		if !b.Translatable || b.ID == "" {
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

	// noRestore disables restoring redacted originals from the batch vault.
	noRestore bool
}

type mergeStats struct {
	Applied       int
	Stale         int
	Skipped       int
	MemoryNew     int
	MemoryUpdated int
}

func (s *mergeStats) accumulate(o mergeStats) {
	s.Applied += o.Applied
	s.Stale += o.Stale
	s.Skipped += o.Skipped
	s.MemoryNew += o.MemoryNew
	s.MemoryUpdated += o.MemoryUpdated
}

// MergeOneKpz ingests a bilingual interchange .kpz returned by a translator
// (kind=kapi-interchange, AD-025 §7): it validates the profile, hydrates the
// target overlays onto the current source blocks (matched by id), staleness-
// checks each block against the current source, applies the project conflict
// policy, writes the merged target via the package's inline skeleton, and
// absorbs accepted targets into the project content memory.
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

	var tm *memory.SQLiteStore
	if !BoolFlagAny(cmd, "no-memory-update", "no-tm-update") {
		// Warned, like the two sibling merge paths above: a content memory that
		// failed to open reported `tm_new=0 tm_updated=0`, which reads as
		// "nothing new to learn" rather than "it was never opened" — so the
		// leverage is lost for this bundle with no way to tell.
		if db, derr := a.ProjectDB(CmdContext(cmd), layout.Root); derr != nil {
			fmt.Fprintf(os.Stderr, "Warning: merge: open project store: %v (continuing without write-back)\n", derr)
		} else {
			tm = db.Memory()
		}
	}
	// One write for the whole package, not one per merged block.
	absorber := a.newMemoryAbsorber(tm)

	// Index the package's target overlays by block id.
	overlayByID := make(map[string][]byte)
	for _, ov := range pkg.Overlays {
		if ov.Kind == "targets/"+string(targetLocale) {
			overlayByID[ov.BlockHash] = ov.Payload
		}
	}

	var stats mergeStats
	// TM write-back is best-effort here (the merged file is the deliverable),
	// but the FIRST failure is kept and reported once at the end rather than
	// discarded per block.
	var tmErr error
	for _, si := range pkg.Sources {
		srcRel := si.SourcePath
		// The package names which of the project's sources it carries work for.
		// That name decides both the file re-read here and the file written
		// below, so it is contained before either happens.
		sourceAbs, err := containedJoin(layout.Root, srcRel, "merge: package source path")
		if err != nil {
			return err
		}
		srcFormat := si.FormatID
		if srcFormat == "" {
			srcFormat = detectSourceFormat(a.FormatReg, pctx, srcRel, sourceAbs)
		}
		if srcFormat == "" {
			return fmt.Errorf("merge: cannot detect format for source %s", sourceAbs)
		}

		currentHash, herr := project.HashFile(sourceAbs)
		if herr != nil {
			return fmt.Errorf("hash current source %s: %w", sourceAbs, herr)
		}
		fileStale := si.ContentHash != "" && currentHash != si.ContentHash

		currentBlocks, _, rerr := project.ReadSourceBlocks(ctx, a.FormatReg, srcFormat, sourceAbs, pctx.SourceLocale, targetLocale,
			formatConfigForSource(pctx.Project, srcFormat, srcRel))
		if rerr != nil {
			return fmt.Errorf("re-read source %s: %w", sourceAbs, rerr)
		}

		for _, b := range currentBlocks {
			payload, ok := overlayByID[b.ID]
			if !ok {
				continue
			}
			// Staleness: a whole-file hash drift is advisory here; per-block
			// identity (the block id and its source text) is the real guard.
			// We applied the overlay by id; if the file is stale we still apply
			// when the id matched, matching the XLIFF path's per-block tolerance.
			_ = fileStale

			existing := b.Target(targetLocale)
			hasExisting := existing != nil && hasAnyText(existing.Runs)
			apply := true
			switch policy {
			case project.ConflictPolicyExistingWins:
				if hasExisting {
					apply = false
				}
			case project.ConflictPolicyNewestWins:
				if hasExisting {
					srcInfo, _ := os.Stat(sourceAbs)
					kpzInfo, _ := os.Stat(kpzInput)
					if srcInfo != nil && kpzInfo != nil && !kpzInfo.ModTime().After(srcInfo.ModTime()) {
						apply = false
					}
				}
			}
			if !apply {
				stats.Skipped++
				continue
			}
			if aerr := applyTargetOverlay(b, targetLocale, payload); aerr != nil {
				return aerr
			}
			if b.Target(targetLocale) == nil {
				stats.Skipped++
				continue
			}
			stats.Applied++
			if absorber != nil {
				added, updated, aerr := absorber.absorb(ctx, b, pctx.SourceLocale, targetLocale, "kpz", srcRel, kpzInput)
				if aerr != nil && tmErr == nil {
					tmErr = aerr
				}
				stats.MemoryNew += added
				stats.MemoryUpdated += updated
			}
		}

		// Write the merged target via the package's skeleton. The skeleton
		// stream is bounded; read it from its parcel reference to feed the
		// reconstructing writer.
		var skelBytes []byte
		for _, s := range pkg.Skeletons {
			if s.SourcePath == srcRel {
				b, rerr := kpz.ReadAll(s.Content)
				if rerr != nil {
					return fmt.Errorf("read skeleton for %s: %w", srcRel, rerr)
				}
				skelBytes = b
				break
			}
		}
		entry := &project.ExtractionFile{Source: srcRel}
		targetPath, terr := resolveMergeOutputPath(entry, pctx.Project, layout.Root, targetLocale)
		if terr != nil {
			return terr
		}
		if werr := writeMergedSourceWithSkeleton(ctx, a.FormatReg, srcFormat, sourceAbs, targetPath, targetLocale, currentBlocks, "", skelBytes, formatConfigForSource(pctx.Project, srcFormat, srcRel)); werr != nil {
			return fmt.Errorf("write merged target %s: %w", targetPath, werr)
		}
	}

	if ferr := absorber.flush(ctx); ferr != nil && tmErr == nil {
		tmErr = ferr
	}
	if tmErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: merge: record %s in the project content memory: %v (the merged files were written)\n",
			filepath.Base(kpzInput), tmErr)
	}
	fmt.Fprintf(cmd.OutOrStdout(),
		"Merged %s → %s: applied=%d skipped=%d tm_new=%d tm_updated=%d (conflict_policy=%s)\n",
		filepath.Base(kpzInput), targetLocale, stats.Applied, stats.Skipped, stats.MemoryNew, stats.MemoryUpdated, policy)
	return nil
}

// BoolFlag reads a bool flag, defaulting to false on error.
func BoolFlag(cmd Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}

// BoolFlagAny reports whether any of the named bool flags is set. It reads a
// flag that carries an accepted-but-hidden alias alongside its current name.
func BoolFlagAny(cmd Command, names ...string) bool {
	for _, name := range names {
		if v, err := cmd.Flags().GetBool(name); err == nil && v {
			return true
		}
	}
	return false
}

// mergeOne handles a single returning XLIFF / PO file.
func (a *App) mergeOne(ctx context.Context, task mergeTask) (mergeStats, error) {
	var stats mergeStats

	ext := format.Ext(task.input)
	switch ext {
	case ".xliff", ".xlf":
		return a.mergeOneXLIFF(ctx, task)
	case ".po":
		return a.mergeOnePO(ctx, task)
	default:
		return stats, fmt.Errorf("merge: unsupported input extension %q (supported: .xliff, .xlf, .po)", ext)
	}
}

// mergeOneXLIFF is the original XLIFF 2 merge path. Split out from
// mergeOne so the dispatch is a cheap switch on the extension.
func (a *App) mergeOneXLIFF(ctx context.Context, task mergeTask) (mergeStats, error) {
	var stats mergeStats
	var tmErr error

	// 1. Read the incoming XLIFF — blocks + layer metadata.
	reader := xliff2.NewReader()
	f, err := os.Open(task.input)
	if err != nil {
		return stats, err
	}
	defer f.Close()
	doc := &model.RawDocument{
		URI:      task.input,
		Reader:   f,
		FormatID: "xliff2",
	}
	if err := reader.Open(ctx, doc); err != nil {
		return stats, fmt.Errorf("xliff2 open: %w", err)
	}
	var layer *model.Layer
	var translatedBlocks []*model.Block
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			return stats, fmt.Errorf("xliff2 read: %w", res.Error)
		}
		switch res.Part.Type {
		case model.PartLayerStart:
			if l, ok := res.Part.Resource.(*model.Layer); ok && layer == nil {
				layer = l
			}
		case model.PartBlock:
			if b, ok := res.Part.Resource.(*model.Block); ok {
				translatedBlocks = append(translatedBlocks, b)
			}
		}
	}
	_ = reader.Close()

	// 2. Resolve the extraction batch via the file-level note.
	batchID := xliff2.BatchIDFromLayer(layer)
	if batchID == "" {
		return stats, fmt.Errorf("merge: no kapi batch id in %s. Was this file produced by kapi extract?", task.input)
	}
	manifest, err := project.LoadExtractionManifest(task.layout, batchID)
	if err != nil {
		return stats, fmt.Errorf("merge: load extraction manifest for batch %s: %w", batchID, err)
	}

	// 3. Find the matching source entry in the manifest.
	srcRel := xliff2.FilePropertyFromLayer(layer, xliff2.FileNoteCategoryKapi, xliff2.FileNoteIDSourceFile)
	if srcRel == "" {
		return stats, fmt.Errorf("merge: no source-file note in %s", task.input)
	}
	targetLocale := model.LocaleID(strings.TrimSpace(layer.Properties["target-language"]))
	if targetLocale == "" {
		// Try to derive from XLIFF <xliff trgLang>
		targetLocale = layer.Locale // fallback — reader sets srcLang on layer.Locale
	}

	pair, entry, ok := findManifestEntry(manifest, srcRel, targetLocale)
	if !ok {
		return stats, fmt.Errorf("merge: source %q / target %q not found in batch %s", srcRel, targetLocale, batchID)
	}
	_ = pair

	// Restore redacted originals: if this batch was extracted with --redact,
	// a vault sidecar maps each placeholder token back to its original. We
	// restore both the incoming source (so the staleness comparison sees the
	// original text, matching the re-read source file) and the translated
	// target before applying it. The originals never left the machine.
	if err := restoreRedactedBlocks(task.layout, batchID, translatedBlocks, targetLocale, !task.noRestore); err != nil {
		return stats, fmt.Errorf("merge: restore redaction for batch %s: %w", batchID, err)
	}

	// 4. Re-read the current source (for per-block staleness detection).
	sourceAbs, err := containedJoin(task.layout.Root, entry.Source, "merge: manifest source path")
	if err != nil {
		return stats, err
	}
	currentHash, err := project.HashFile(sourceAbs)
	if err != nil {
		return stats, fmt.Errorf("hash current source %s: %w", sourceAbs, err)
	}
	fileStale := currentHash != entry.SourceHash

	srcFormat := detectSourceFormat(a.FormatReg, task.ctx, entry.Source, sourceAbs)
	if srcFormat == "" {
		return stats, fmt.Errorf("merge: cannot detect format for source %s", sourceAbs)
	}
	currentSourceBlocks, currentSourceLayer, err := project.ReadSourceBlocks(ctx, a.FormatReg, srcFormat, sourceAbs, task.ctx.SourceLocale, targetLocale,
		formatConfigForSource(task.ctx.Project, srcFormat, entry.Source))
	if err != nil {
		return stats, fmt.Errorf("re-read source %s: %w", sourceAbs, err)
	}
	_ = currentSourceLayer

	currentByID := make(map[string]*model.Block, len(currentSourceBlocks))
	for _, b := range currentSourceBlocks {
		currentByID[b.ID] = b
	}

	// 5. Apply translations per conflict policy with per-block stale check.
	for _, tb := range translatedBlocks {
		target := tb.Target(targetLocale)
		if target == nil || !hasAnyText(target.Runs) {
			// Translator returned no target for this block — leave existing.
			stats.Skipped++
			continue
		}

		srcBlock, ok := currentByID[tb.ID]
		if !ok {
			stats.Stale++
			continue
		}

		// Per-block staleness: compare the block's source text between
		// extract-time (preserved in the XLIFF's <source>) and current source.
		// Both sides render through RenderRunsWithData: the XLIFF carries
		// inline codes flattened to their original data (the markdown/HTML
		// markers), while the freshly read source block keeps them as code
		// runs that plain SourceText() would drop — comparing unlike
		// renderings marked every block with inline markup stale.
		xliffSourceText := model.RenderRunsWithData(tb.Source)
		currentSourceText := model.RenderRunsWithData(srcBlock.Source)
		if xliffSourceText != currentSourceText {
			stats.Stale++
			continue
		}
		if fileStale {
			// File hash drift doesn't block if per-block text still matches —
			// noop path, but record separately so callers can see the file
			// changed even if not at this block.
			_ = fileStale
		}

		// Conflict policy.
		existing := srcBlock.Target(targetLocale)
		hasExisting := existing != nil
		apply := true
		switch task.policy {
		case project.ConflictPolicyExistingWins:
			if hasExisting && hasAnyText(existing.Runs) {
				apply = false
			}
		case project.ConflictPolicyNewestWins:
			// At this layer we only know about the returning XLIFF vs the
			// (re-read) source file's existing target. Prefer the XLIFF if
			// the source file's mtime is older than the XLIFF's mtime,
			// otherwise keep existing.
			if hasExisting && hasAnyText(existing.Runs) {
				srcInfo, _ := os.Stat(sourceAbs)
				xliffInfo, _ := os.Stat(task.input)
				if srcInfo != nil && xliffInfo != nil && !xliffInfo.ModTime().After(srcInfo.ModTime()) {
					apply = false
				}
			}
		case project.ConflictPolicyTranslatorWins, "":
			// Always apply the translator's target.
		}
		if !apply {
			stats.Skipped++
			continue
		}
		srcBlock.SetTarget(targetLocale, target)
		stats.Applied++

		// TM absorb with provenance. Best-effort, but reported: see
		// absorbBlockIntoTM.
		if task.mem != nil {
			added, updated, aerr := task.mem.absorb(ctx, srcBlock, task.ctx.SourceLocale, targetLocale, batchID, entry.Source, task.input)
			if aerr != nil && tmErr == nil {
				tmErr = aerr
			}
			stats.MemoryNew += added
			stats.MemoryUpdated += updated
		}
	}
	if tmErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: merge: record %s in the project content memory: %v (the merged file was written)\n",
			filepath.Base(task.input), tmErr)
	}

	// 6. Write the merged target file via the project's writer + skeleton.
	targetPath, err := resolveMergeOutputPath(entry, task.ctx.Project, task.layout.Root, targetLocale)
	if err != nil {
		return stats, err
	}
	if err := writeMergedSource(ctx, a.FormatReg, srcFormat, sourceAbs, targetPath, task.layout, batchID, entry, targetLocale, currentSourceBlocks, task.ctx.Project); err != nil {
		return stats, fmt.Errorf("write merged target %s: %w", targetPath, err)
	}

	return stats, nil
}

// mergeOnePO handles a returning PO (gettext) file. It shares all the
// conflict policy, stale detection, and content memory absorb machinery with
// mergeOneXLIFF — the only differences are parsing and target-locale
// discovery (PO has no intrinsic src/trg attribute; we pull the target
// from the extraction manifest via the pair that named the PO output).
func (a *App) mergeOnePO(ctx context.Context, task mergeTask) (mergeStats, error) {
	var stats mergeStats
	var tmErr error

	po, err := ReadPOForMerge(task.input)
	if err != nil {
		return stats, fmt.Errorf("po read: %w", err)
	}
	if po.BatchID == "" {
		return stats, fmt.Errorf("merge: no kapi-batch comment in %s. Was this file produced by kapi extract?", task.input)
	}
	manifest, err := project.LoadExtractionManifest(task.layout, po.BatchID)
	if err != nil {
		return stats, fmt.Errorf("merge: load extraction manifest for batch %s: %w", po.BatchID, err)
	}
	if po.SourceFile == "" {
		return stats, fmt.Errorf("merge: no kapi-source-file comment in %s", task.input)
	}

	// Target locale: resolved by finding the pair whose files list
	// contains this source path. PO has no inherent target-locale attr,
	// so we trust the extraction manifest.
	pair, entry, ok := findPOManifestEntry(manifest, po.SourceFile, task.input, task.layout.Root)
	if !ok {
		return stats, fmt.Errorf("merge: source %q not found in batch %s", po.SourceFile, po.BatchID)
	}
	targetLocale := pair.TargetLocale

	// Re-read the current source.
	sourceAbs, err := containedJoin(task.layout.Root, entry.Source, "merge: manifest source path")
	if err != nil {
		return stats, err
	}
	srcFormat := detectSourceFormat(a.FormatReg, task.ctx, entry.Source, sourceAbs)
	if srcFormat == "" {
		return stats, fmt.Errorf("merge: cannot detect format for source %s", sourceAbs)
	}
	currentSourceBlocks, _, err := project.ReadSourceBlocks(ctx, a.FormatReg, srcFormat, sourceAbs, task.ctx.SourceLocale, targetLocale,
		formatConfigForSource(task.ctx.Project, srcFormat, entry.Source))
	if err != nil {
		return stats, fmt.Errorf("re-read source %s: %w", sourceAbs, err)
	}
	currentByID := make(map[string]*model.Block, len(currentSourceBlocks))
	for _, b := range currentSourceBlocks {
		currentByID[b.ID] = b
	}

	// Apply per-entry.
	for _, mb := range po.Blocks {
		if mb.MsgStr == "" {
			stats.Skipped++
			continue
		}
		if mb.BlockID == "" {
			// No kapi-block hint — we can't correlate cleanly. Skip
			// rather than risk misapplying.
			stats.Skipped++
			continue
		}
		srcBlock, ok := currentByID[mb.BlockID]
		if !ok {
			stats.Stale++
			continue
		}
		// Per-block staleness: compare source text between extract-time
		// (carried in the PO's msgid) and the current source.
		if mb.MsgID != srcBlock.SourceText() {
			stats.Stale++
			continue
		}
		// Conflict policy.
		existing := srcBlock.Target(targetLocale)
		hasExisting := existing != nil
		apply := true
		switch task.policy {
		case project.ConflictPolicyExistingWins:
			if hasExisting && hasAnyText(existing.Runs) {
				apply = false
			}
		case project.ConflictPolicyNewestWins:
			if hasExisting && hasAnyText(existing.Runs) {
				srcInfo, _ := os.Stat(sourceAbs)
				poInfo, _ := os.Stat(task.input)
				if srcInfo != nil && poInfo != nil && !poInfo.ModTime().After(srcInfo.ModTime()) {
					apply = false
				}
			}
		}
		if !apply {
			stats.Skipped++
			continue
		}
		// Stash target text (PO v1 = one msgid per block).
		srcBlock.SetTargetText(targetLocale, mb.MsgStr)
		stats.Applied++

		if task.mem != nil {
			added, updated, aerr := task.mem.absorb(ctx, srcBlock, task.ctx.SourceLocale, targetLocale, po.BatchID, entry.Source, task.input)
			if aerr != nil && tmErr == nil {
				tmErr = aerr
			}
			stats.MemoryNew += added
			stats.MemoryUpdated += updated
		}
	}
	if tmErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: merge: record %s in the project content memory: %v (the merged file was written)\n",
			filepath.Base(task.input), tmErr)
	}

	// Write merged target via source format writer + captured skeleton.
	targetPath, err := resolveMergeOutputPath(entry, task.ctx.Project, task.layout.Root, targetLocale)
	if err != nil {
		return stats, err
	}
	if err := writeMergedSource(ctx, a.FormatReg, srcFormat, sourceAbs, targetPath, task.layout, po.BatchID, entry, targetLocale, currentSourceBlocks, task.ctx.Project); err != nil {
		return stats, fmt.Errorf("write merged target %s: %w", targetPath, err)
	}
	return stats, nil
}

// findPOManifestEntry is the PO counterpart to findManifestEntry. Since
// PO files carry no trgLang attribute, we locate the pair by matching
// the output path (or falling back to the source file path) in the
// manifest — whichever pair claims this PO as its output wins.
func findPOManifestEntry(m *project.ExtractionManifest, sourceRel, inputPath, root string) (*project.ExtractionPair, *project.ExtractionFile, bool) {
	absInput, _ := filepath.Abs(inputPath)
	for i := range m.Pairs {
		p := &m.Pairs[i]
		// Primary: match by the pair's output path.
		if p.Output != "" {
			absOut := p.Output
			if !filepath.IsAbs(absOut) {
				absOut = filepath.Join(root, p.Output)
			}
			if absOut == absInput {
				for j := range p.Files {
					if p.Files[j].Source == sourceRel {
						return p, &p.Files[j], true
					}
				}
			}
		}
		// Fallback: source-file match within the pair (useful for
		// single-source projects where the pair output is the only file).
		for j := range p.Files {
			if p.Files[j].Source == sourceRel {
				return p, &p.Files[j], true
			}
		}
	}
	return nil, nil, false
}

func findManifestEntry(m *project.ExtractionManifest, sourceRel string, target model.LocaleID) (*project.ExtractionPair, *project.ExtractionFile, bool) {
	for i := range m.Pairs {
		p := &m.Pairs[i]
		if target != "" && p.TargetLocale != target {
			continue
		}
		for j := range p.Files {
			if p.Files[j].Source == sourceRel {
				return p, &p.Files[j], true
			}
		}
	}
	return nil, nil, false
}

// detectSourceFormat picks the format for a source path, preferring the
// recipe's declared format when available.
func detectSourceFormat(reg *registry.FormatRegistry, ctx *project.ProjectContext, rel, abs string) string {
	if ctx != nil && ctx.Project != nil {
		for _, coll := range ctx.Project.Collections {
			for _, item := range coll.EffectiveItems() {
				if item.Format == nil || item.Format.Name == "" {
					continue
				}
				// Patterns use doublestar, matching content resolution —
				// filepath.Match has no `**` support, so deep paths fell
				// back to extension detection (mdx read as markdown).
				if ok, _ := doublestar.Match(item.Path, rel); ok {
					return item.Format.Name
				}
			}
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
	// Search the recipe for the ContentItem whose Path matches entry.Source.
	// Patterns use doublestar (matching ExpandGlob's `**` semantics), and the
	// target template supports {lang}, {path}, {filename}, {basename}, and the
	// legacy bare `*` — see project.ResolveTargetPath.
	if proj != nil {
		for _, coll := range proj.Collections {
			for _, item := range coll.EffectiveItems() {
				ok, _ := doublestar.Match(item.Path, entry.Source)
				if !ok {
					continue
				}
				if item.Target == "" {
					break
				}
				tmpl := project.ResolveTargetPathIn(item.Path, item.Base, item.Target, entry.Source, string(locale), proj.Defaults.LocaleFormat)
				if !filepath.IsAbs(tmpl) {
					tmpl = filepath.Join(root, tmpl)
				}
				return tmpl, nil
			}
		}
	}
	// Default: <source-dir>/<locale>/<basename>
	base := filepath.Base(entry.Source)
	return filepath.Join(root, filepath.Dir(entry.Source), string(locale), base), nil
}

// writeMergedSource writes the merged blocks to the target file using the
// source format's writer, plus the captured skeleton when available. proj
// (optional) supplies the project's per-format config so the shared writer
// output options (output.bom / output.newline / output.encoding) apply on
// merge too.
func writeMergedSource(ctx context.Context, reg *registry.FormatRegistry, formatName, sourceAbs, targetPath string, layout project.Layout, batchID string, entry *project.ExtractionFile, locale model.LocaleID, blocks []*model.Block, proj *project.KapiProject) error {
	skelPath := ""
	relSource := ""
	if entry != nil {
		relSource = entry.Source
		if entry.Skeleton != "" {
			skelPath = filepath.Join(project.ExtractionDir(layout, batchID), entry.Skeleton)
		}
	}
	outputCfg := formatConfigForSource(proj, formatName, relSource)
	return writeMergedSourceWithSkeleton(ctx, reg, formatName, sourceAbs, targetPath, locale, blocks, skelPath, nil, outputCfg)
}

// writeMergedSourceWithSkeleton is the underlying writer that takes the
// skeleton as either a file path (skelPath, for the XLIFF/PO extraction flow)
// or raw bytes (skelBytes, for a bilingual interchange .kpz that carries the
// skeleton inline). When both are empty the writer re-serializes from its parse
// tree (lower fidelity). skelBytes takes precedence.
func writeMergedSourceWithSkeleton(ctx context.Context, reg *registry.FormatRegistry, formatName, sourceAbs, targetPath string, locale model.LocaleID, blocks []*model.Block, skelPath string, skelBytes []byte, outputCfg map[string]any) error {
	writer, err := reg.NewWriter(registry.FormatID(formatName))
	if err != nil {
		return err
	}
	writer.SetLocale(locale)
	if err := applyWriterOutputConfig(writer, outputCfg); err != nil {
		return err
	}
	// Refuse a blocked destination before opening anything (#1449), so a merge
	// onto an occupied path reports what is in the way rather than a bare
	// "is a directory" from the writer's open.
	if err := flow.CheckOutputPath(targetPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return flow.ClassifyOutputPathError(targetPath, filepath.Dir(targetPath), err)
	}
	if err := writer.SetOutput(targetPath); err != nil {
		return flow.ClassifyOutputPathError(targetPath, filepath.Dir(targetPath), err)
	}

	if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
		switch {
		case len(skelBytes) > 0:
			// Inline skeleton (from a .kpz): read-mode store over the bytes.
			store := format.NewSkeletonStoreFromBytes(skelBytes)
			consumer.SetSkeletonStore(store)
			defer store.Close()
		case skelPath != "":
			// A skeleton that is NOT there is the documented lower-fidelity
			// fallback (the extraction state dir is regenerable). A skeleton that
			// is there but will not open is a fault, and its error used to be
			// discarded — the merge then wrote a re-serialized file, losing the
			// source's exact formatting, and reported success. Faithful
			// write-back is the point; fail instead of degrading silently.
			if _, statErr := os.Stat(skelPath); statErr == nil {
				store, oerr := format.OpenSkeletonStore(skelPath)
				if oerr != nil {
					return fmt.Errorf("cannot write %s: its skeleton %s exists but could not be opened, so the source's exact formatting cannot be restored. Re-run `kapi extract`: %w",
						targetPath, skelPath, oerr)
				}
				consumer.SetSkeletonStore(store)
				defer store.Close()
			}
		}
	}

	// Emit layer + blocks.
	parts := make(chan *model.Part, len(blocks)+1)
	parts <- &model.Part{Type: model.PartLayerStart, Resource: &model.Layer{
		ID:             "file-merged",
		Name:           filepath.Base(sourceAbs),
		Format:         formatName,
		Locale:         locale,
		IsMultilingual: true,
	}}
	for _, b := range blocks {
		parts <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(parts)

	if err := writer.Write(ctx, parts); err != nil {
		return err
	}
	return writer.Close()
}

// absorbBlockIntoMemory writes a block's source+target into the project content memory
// with kapi-merge provenance. Returns (new, updated) counts. Today both
// are 1-or-0 since we write one entry per block; tracking them separately
// matters once we widen to per-segment.
//
// A write failure is RETURNED, not swallowed. It used to be discarded (an
// errored Add read as "0 new, 0 updated"), so a project content memory that had gone
// read-only, or run out of disk, absorbed nothing while every merge reported
// success. Callers treat it as non-fatal — the merged file is the deliverable —
// but they report it.
// memoryAbsorber collects what a merge run teaches the content memory and
// writes it ONCE, at the end, in a single transaction.
//
// Per-block writes were the natural shape when the content memory had a
// database to itself: N small write transactions on a file nothing else touched.
// In the merged store they contend with the block cache and the working set for
// the same writer, and SQLite's deferred write transactions do not queue fairly
// — a run of hundreds of one-row commits starves whichever writer arrives
// second. One bulk transaction per run removes the contention rather than
// tuning it.
//
// The trade is the index rebuild. The bulk path deliberately skips per-row FTS5
// maintenance (it dominates on large corpora), so the search and fuzzy indexes
// are rebuilt set-wise afterwards — one pass over tm_variants instead of a
// tokenization per row. That is a whole-table pass for a run that may have
// learned three entries, which is the cost of not holding the writer for the
// length of the merge.
//
// Not safe for concurrent use: the merge loop is sequential, one file at a time.
type memoryAbsorber struct {
	app     *App
	tm      *memory.SQLiteStore
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
func (a *App) newMemoryAbsorber(tm *memory.SQLiteStore) *memoryAbsorber {
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

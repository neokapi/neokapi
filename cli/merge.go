package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/klz"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/spf13/cobra"
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

// NewMergeCmd returns the `kapi merge` command (AD-017, issue #416).
// Applies a translator-returned XLIFF back onto the project's source
// files using the captured skeleton, records stale segments, and
// absorbs accepted targets into the project TM.
func (a *App) NewMergeCmd(_ MergeCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "merge",
		Short:   "Apply a returned bilingual file (.klz/XLIFF/PO) back onto the project source",
		GroupID: "localization",
		Long: `Materialize localized files for a project, or apply bilingual files
returned by a translator back onto the project's source locales.

With no -i in a project, merge writes the localized files from the
project block store: a process-only "kapi run" (in a project, no -o)
commits its work as targets/<locale> overlays, and merge is the matching
sink — it reads each source, applies the stored overlays, and writes the
localized file via the source format's skeleton round-trip.

With -i, merge applies one or more bilingual files returned by a
translator back onto the project's source locales, using the skeleton
captured by kapi extract. Each input carries the extraction
batch id in a file-level <note>, so merge finds the right extraction
manifest without guessing from the filename. Mixed target locales in one
batch are fine — merge handles each input independently.`,
		Example: `  kapi merge                     # materialize localized files from the project store
  kapi merge -i out/app.en-US-to-fr-FR.xliff
  kapi merge -i file1.xliff -i file2.xliff
  kapi merge -i vendor-return/ --no-tm-update
  kapi merge work.klz -o l10n/   # emit localized files from a .klz workspace`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// A single .klz positional arg is either a bilingual interchange
			// file (kind=kapi-interchange — ingest the translator's targets) or
			// an ad-hoc workspace (emit its localized files).
			if len(args) == 1 && isKlzPath(args[0]) {
				out, _ := cmd.Flags().GetString("output")
				if pkg, err := loadWorkspace(args[0]); err == nil && pkg.Kind == klz.KindInterchange {
					return a.mergeOneKlz(cmd, args[0])
				}
				return a.mergeFromKlz(cmd.Context(), args[0], out)
			}
			// In a project with no -i input, materialize the localized files
			// from the project block store: a process-only `kapi run` lands its
			// work as `targets/<locale>` overlays, and this is the matching sink
			// (AD-026 §3). Explicit -i keeps the XLIFF/PO/dir merge path below.
			if inputs, _ := cmd.Flags().GetStringArray("input"); len(inputs) == 0 {
				return a.mergeFromProjectStore(cmd)
			}
			return a.runMerge(cmd)
		},
	}
	AddProjectFlag(cmd)
	cmd.Flags().StringArrayP("input", "i", nil, "input XLIFF file, glob, or directory (repeatable)")
	cmd.Flags().StringP("output", "o", "", "output directory or template when merging a .klz workspace")
	cmd.Flags().Bool("no-tm-update", false, "skip TM write-back")
	cmd.Flags().Bool("no-restore", false, "skip restoring redacted originals from the batch vault")
	return cmd
}

func (a *App) runMerge(cmd *cobra.Command) error {
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
		return errors.New("merge: no input files matched — check -i paths and globs")
	}

	noTMUpdate, _ := cmd.Flags().GetBool("no-tm-update")
	noRestore, _ := cmd.Flags().GetBool("no-restore")

	var tm *sievepen.SQLiteTM
	// In the browser/seeded build (a.TMBackend set) there is no SQLite driver and
	// no on-disk project TM to write back to, so skip TM write-back silently
	// rather than surfacing a driver error. The native CLI (TMBackend == nil)
	// opens the project's SQLite TM as before.
	if !noTMUpdate && a.TMBackend == nil {
		tmPath := filepath.Join(layout.StateDir, "tm.db")
		loaded, err := sievepen.NewSQLiteTM(tmPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: merge: open project TM at %s: %v (continuing with --no-tm-update semantics)\n", tmPath, err)
		} else {
			defer loaded.Close()
			tm = loaded
		}
	}

	policy := proj.Defaults.Merge.ResolvedConflictPolicy()

	var totals mergeStats
	failures := 0

	for _, in := range expanded {
		fmt.Fprintf(cmd.OutOrStdout(), "Merging %s\n", relOrAbs(layout.Root, in))
		stats, err := a.mergeOne(cmd.Context(), mergeTask{
			layout:    layout,
			ctx:       ctx,
			input:     in,
			policy:    policy,
			tm:        tm,
			project:   proj,
			noRestore: noRestore,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "merge: %s: %v\n", relOrAbs(layout.Root, in), err)
			failures++
			continue
		}
		totals.accumulate(stats)
		fmt.Fprintf(cmd.OutOrStdout(),
			"  applied=%d stale=%d skipped=%d tm_new=%d tm_updated=%d\n",
			stats.Applied, stats.Stale, stats.Skipped, stats.TMNew, stats.TMUpdated)
	}

	fmt.Fprintf(cmd.OutOrStdout(),
		"\nMerge complete. applied=%d stale=%d skipped=%d tm_new=%d tm_updated=%d (conflict_policy=%s)\n",
		totals.Applied, totals.Stale, totals.Skipped, totals.TMNew, totals.TMUpdated, policy)

	if failures > 0 {
		return fmt.Errorf("merge: %d input file(s) failed — see errors above", failures)
	}
	return nil
}

// mergeFromProjectStore materializes localized files from the project block
// store (AD-026 §3): for each project source × target locale it reads the
// source, applies the stored `targets/<locale>` overlays via the
// hydrateTargetsTool (recomputing nothing), and writes the localized file to
// the source's output template. This is the sink half of the process-only
// loop — `kapi run flow -i src.json` (in a project, no -o) commits overlays;
// `kapi merge` (no -i) writes the files.
func (a *App) mergeFromProjectStore(cmd *cobra.Command) error {
	ctx := cmd.Context()
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

	locales := proj.Defaults.TargetLanguages
	if len(locales) == 0 {
		return errors.New("merge: project declares no target languages (defaults.target_languages)")
	}

	if err := project.EnsureLayout(layout); err != nil {
		return fmt.Errorf("merge: ensure project layout: %w", err)
	}
	store, err := sqlitestore.New(layout.BlockStorePath())
	if err != nil {
		return fmt.Errorf("merge: open project block store: %w", err)
	}
	defer store.Close()

	files, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return fmt.Errorf("merge: resolve project content: %w", err)
	}
	if len(files) == 0 {
		return errors.New("merge: project has no source files to materialize (check content patterns)")
	}

	noTMUpdate, _ := cmd.Flags().GetBool("no-tm-update")
	var tm *sievepen.SQLiteTM
	// Browser/seeded build (a.TMBackend set): no SQLite driver / on-disk TM —
	// skip write-back silently. Native CLI opens the project SQLite TM as before.
	if !noTMUpdate && a.TMBackend == nil {
		tmPath := filepath.Join(layout.StateDir, "tm.db")
		if loaded, lerr := sievepen.NewSQLiteTM(tmPath); lerr != nil {
			fmt.Fprintf(os.Stderr, "Warning: merge: open project TM at %s: %v (continuing without TM write-back)\n", tmPath, lerr)
		} else {
			defer loaded.Close()
			tm = loaded
		}
	}

	written := 0
	for _, f := range files {
		srcFormat := f.Format
		if srcFormat == "" {
			srcFormat = detectSourceFormat(a.FormatReg, pctx, f.Relative, f.Path)
		}
		if srcFormat == "" {
			return fmt.Errorf("merge: cannot detect format for source %s", f.Path)
		}
		for _, locale := range locales {
			entry := &project.ExtractionFile{Source: f.Relative}
			targetPath := resolveMergeOutputPath(entry, proj, layout.Root, locale)

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
				ConfigureReader: func(reader format.DataFormatReader, detectedFmt registry.FormatID) error {
					return pctx.ConfigureReader(reader, string(detectedFmt))
				},
			})
			tools := []tool.Tool{newHydrateTargetsTool(locale)}
			if rerr := runner.RunFile(ctx, "merge", tools, f.Path, targetPath, string(locale)); rerr != nil {
				return fmt.Errorf("merge: materialize %s → %s: %w", f.Relative, locale, rerr)
			}
			written++

			// Absorb the materialized targets into the project TM with merge
			// provenance, mirroring the XLIFF/PO/.klz merge paths.
			if tm != nil {
				if added, updated, aerr := absorbStoreTargets(ctx, a.FormatReg, srcFormat, f.Path, pctx.SourceLocale, locale, store, tm, f.Relative); aerr == nil {
					_ = added
					_ = updated
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Merged %s → %s\n", f.Relative, targetPath)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nMerge complete. wrote=%d file(s) from the project store.\n", written)
	return nil
}

// absorbStoreTargets reads the source blocks, applies the stored
// `targets/<locale>` overlays, and writes accepted source+target pairs into
// the project TM with kapi-merge provenance. Returns (new, updated) counts.
func absorbStoreTargets(ctx context.Context, reg *registry.FormatRegistry, srcFormat, sourceAbs string, source, target model.LocaleID, store blockstore.Store, tm *sievepen.SQLiteTM, sourceRel string) (int, int, error) {
	blocks, _, err := readSourceBlocks(ctx, reg, srcFormat, sourceAbs, source, target, nil)
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
		o, oerr := sess.GetOverlay(kind, b.ID)
		if oerr != nil || len(o.Payload) == 0 {
			continue
		}
		applyTargetOverlay(b, target, o.Payload)
		n, u := absorbBlockIntoTM(ctx, tm, b, source, target, "store", sourceRel, sourceAbs)
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
	tm      *sievepen.SQLiteTM
	project *project.KapiProject

	// noRestore disables restoring redacted originals from the batch vault.
	noRestore bool
}

type mergeStats struct {
	Applied   int
	Stale     int
	Skipped   int
	TMNew     int
	TMUpdated int
}

func (s *mergeStats) accumulate(o mergeStats) {
	s.Applied += o.Applied
	s.Stale += o.Stale
	s.Skipped += o.Skipped
	s.TMNew += o.TMNew
	s.TMUpdated += o.TMUpdated
}

// mergeOneKlz ingests a bilingual interchange .klz returned by a translator
// (kind=kapi-interchange, AD-025 §7): it validates the profile, hydrates the
// target overlays onto the current source blocks (matched by id), staleness-
// checks each block against the current source, applies the project conflict
// policy, writes the merged target via the package's inline skeleton, and
// absorbs accepted targets into the project TM.
func (a *App) mergeOneKlz(cmd *cobra.Command, klzInput string) error {
	ctx := cmd.Context()
	pkg, err := loadWorkspace(klzInput)
	if err != nil {
		return err
	}
	if pkg.Kind != klz.KindInterchange {
		return fmt.Errorf("merge: %s is not a bilingual interchange .klz (kind=%q)", filepath.Base(klzInput), pkg.Kind)
	}
	if pkg.InterchangeTask == nil {
		return fmt.Errorf("merge: %s has no interchange task metadata", filepath.Base(klzInput))
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
	policy := proj.Defaults.Merge.ResolvedConflictPolicy()

	var tm *sievepen.SQLiteTM
	if !boolFlag(cmd, "no-tm-update") {
		if loaded, lerr := sievepen.NewSQLiteTM(filepath.Join(layout.StateDir, "tm.db")); lerr == nil {
			defer loaded.Close()
			tm = loaded
		}
	}

	// Index the package's target overlays by block id.
	overlayByID := make(map[string][]byte)
	for _, ov := range pkg.Overlays {
		if ov.Kind == "targets/"+string(targetLocale) {
			overlayByID[ov.BlockHash] = ov.Payload
		}
	}

	var stats mergeStats
	for _, si := range pkg.Sources {
		srcRel := si.SourcePath
		sourceAbs := filepath.Join(layout.Root, srcRel)
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

		currentBlocks, _, rerr := readSourceBlocks(ctx, a.FormatReg, srcFormat, sourceAbs, pctx.SourceLocale, targetLocale,
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
					klzInfo, _ := os.Stat(klzInput)
					if srcInfo != nil && klzInfo != nil && !klzInfo.ModTime().After(srcInfo.ModTime()) {
						apply = false
					}
				}
			}
			if !apply {
				stats.Skipped++
				continue
			}
			applyTargetOverlay(b, targetLocale, payload)
			if b.Target(targetLocale) == nil {
				stats.Skipped++
				continue
			}
			stats.Applied++
			if tm != nil {
				added, updated := absorbBlockIntoTM(ctx, tm, b, pctx.SourceLocale, targetLocale, "klz", srcRel, klzInput)
				stats.TMNew += added
				stats.TMUpdated += updated
			}
		}

		// Write the merged target via the package's skeleton. The skeleton
		// stream is bounded; read it from its parcel reference to feed the
		// reconstructing writer.
		var skelBytes []byte
		for _, s := range pkg.Skeletons {
			if s.SourcePath == srcRel {
				b, rerr := klz.ReadAll(s.Content)
				if rerr != nil {
					return fmt.Errorf("read skeleton for %s: %w", srcRel, rerr)
				}
				skelBytes = b
				break
			}
		}
		entry := &project.ExtractionFile{Source: srcRel}
		targetPath := resolveMergeOutputPath(entry, pctx.Project, layout.Root, targetLocale)
		if werr := writeMergedSourceWithSkeleton(ctx, a.FormatReg, srcFormat, sourceAbs, targetPath, targetLocale, currentBlocks, "", skelBytes); werr != nil {
			return fmt.Errorf("write merged target %s: %w", targetPath, werr)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(),
		"Merged %s → %s: applied=%d skipped=%d tm_new=%d tm_updated=%d (conflict_policy=%s)\n",
		filepath.Base(klzInput), targetLocale, stats.Applied, stats.Skipped, stats.TMNew, stats.TMUpdated, policy)
	return nil
}

// boolFlag reads a bool flag, defaulting to false on error.
func boolFlag(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}

// mergeOne handles a single returning XLIFF / PO file.
func (a *App) mergeOne(ctx context.Context, task mergeTask) (mergeStats, error) {
	var stats mergeStats

	ext := strings.ToLower(filepath.Ext(task.input))
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
		return stats, fmt.Errorf("merge: no kapi batch id in %s — was this file produced by kapi extract?", task.input)
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
	sourceAbs := filepath.Join(task.layout.Root, entry.Source)
	currentHash, err := project.HashFile(sourceAbs)
	if err != nil {
		return stats, fmt.Errorf("hash current source %s: %w", sourceAbs, err)
	}
	fileStale := currentHash != entry.SourceHash

	srcFormat := detectSourceFormat(a.FormatReg, task.ctx, entry.Source, sourceAbs)
	if srcFormat == "" {
		return stats, fmt.Errorf("merge: cannot detect format for source %s", sourceAbs)
	}
	currentSourceBlocks, currentSourceLayer, err := readSourceBlocks(ctx, a.FormatReg, srcFormat, sourceAbs, task.ctx.SourceLocale, targetLocale,
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

		// TM absorb with provenance.
		if task.tm != nil {
			added, updated := absorbBlockIntoTM(ctx, task.tm, srcBlock, task.ctx.SourceLocale, targetLocale, batchID, entry.Source, task.input)
			stats.TMNew += added
			stats.TMUpdated += updated
		}
	}

	// 6. Write the merged target file via the project's writer + skeleton.
	targetPath := resolveMergeOutputPath(entry, task.ctx.Project, task.layout.Root, targetLocale)
	if err := writeMergedSource(ctx, a.FormatReg, srcFormat, sourceAbs, targetPath, task.layout, batchID, entry, targetLocale, currentSourceBlocks); err != nil {
		return stats, fmt.Errorf("write merged target %s: %w", targetPath, err)
	}

	return stats, nil
}

// mergeOnePO handles a returning PO (gettext) file. It shares all the
// conflict policy, stale detection, and TM absorb machinery with
// mergeOneXLIFF — the only differences are parsing and target-locale
// discovery (PO has no intrinsic src/trg attribute; we pull the target
// from the extraction manifest via the pair that named the PO output).
func (a *App) mergeOnePO(ctx context.Context, task mergeTask) (mergeStats, error) {
	var stats mergeStats

	po, err := readPOForMerge(task.input)
	if err != nil {
		return stats, fmt.Errorf("po read: %w", err)
	}
	if po.BatchID == "" {
		return stats, fmt.Errorf("merge: no kapi-batch comment in %s — was this file produced by kapi extract?", task.input)
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
	sourceAbs := filepath.Join(task.layout.Root, entry.Source)
	srcFormat := detectSourceFormat(a.FormatReg, task.ctx, entry.Source, sourceAbs)
	if srcFormat == "" {
		return stats, fmt.Errorf("merge: cannot detect format for source %s", sourceAbs)
	}
	currentSourceBlocks, _, err := readSourceBlocks(ctx, a.FormatReg, srcFormat, sourceAbs, task.ctx.SourceLocale, targetLocale,
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

		if task.tm != nil {
			added, updated := absorbBlockIntoTM(ctx, task.tm, srcBlock, task.ctx.SourceLocale, targetLocale, po.BatchID, entry.Source, task.input)
			stats.TMNew += added
			stats.TMUpdated += updated
		}
	}

	// Write merged target via source format writer + captured skeleton.
	targetPath := resolveMergeOutputPath(entry, task.ctx.Project, task.layout.Root, targetLocale)
	if err := writeMergedSource(ctx, a.FormatReg, srcFormat, sourceAbs, targetPath, task.layout, po.BatchID, entry, targetLocale, currentSourceBlocks); err != nil {
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
		for _, coll := range ctx.Project.Content {
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

func readSourceBlocks(ctx context.Context, reg *registry.FormatRegistry, formatName, path string, src, tgt model.LocaleID, cfg map[string]any) ([]*model.Block, *model.Layer, error) {
	reader, err := reg.NewReader(registry.FormatID(formatName))
	if err != nil {
		return nil, nil, err
	}
	// Per-item format config (e.g. translateFrontMatter on a docs item)
	// must apply on the merge-time re-read exactly as it did at extract
	// time — block numbering depends on it.
	if err := applyFormatConfig(reader, cfg); err != nil {
		return nil, nil, fmt.Errorf("apply format config: %w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: src,
		TargetLocale: tgt,
		FormatID:     formatName,
		Reader:       f,
	}
	if err := reader.Open(ctx, doc); err != nil {
		return nil, nil, err
	}
	defer reader.Close()

	var blocks []*model.Block
	var layer *model.Layer
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			return nil, nil, res.Error
		}
		switch res.Part.Type {
		case model.PartBlock:
			if b, ok := res.Part.Resource.(*model.Block); ok {
				blocks = append(blocks, b)
			}
		case model.PartLayerStart:
			if l, ok := res.Part.Resource.(*model.Layer); ok && layer == nil {
				layer = l
			}
		}
	}
	return blocks, layer, nil
}

// resolveMergeOutputPath returns the path to write the merged target
// source to. Falls back to a sensible default next to the source when
// the recipe does not declare a target template.
func resolveMergeOutputPath(entry *project.ExtractionFile, proj *project.KapiProject, root string, locale model.LocaleID) string {
	// Search the recipe for the ContentItem whose Path matches entry.Source.
	// Patterns use doublestar (matching ExpandGlob's `**` semantics), and the
	// target template supports {lang}, {path}, {filename}, {basename}, and the
	// legacy bare `*` — see project.ResolveTargetPath.
	if proj != nil {
		for _, coll := range proj.Content {
			for _, item := range coll.EffectiveItems() {
				ok, _ := doublestar.Match(item.Path, entry.Source)
				if !ok {
					continue
				}
				if item.Target == "" {
					break
				}
				tmpl := project.ResolveTargetPath(item.Path, item.Base, item.Target, entry.Source, string(locale))
				if !filepath.IsAbs(tmpl) {
					tmpl = filepath.Join(root, tmpl)
				}
				return tmpl
			}
		}
	}
	// Default: <source-dir>/<locale>/<basename>
	base := filepath.Base(entry.Source)
	return filepath.Join(root, filepath.Dir(entry.Source), string(locale), base)
}

// writeMergedSource writes the merged blocks to the target file using the
// source format's writer, plus the captured skeleton when available.
func writeMergedSource(ctx context.Context, reg *registry.FormatRegistry, formatName, sourceAbs, targetPath string, layout project.Layout, batchID string, entry *project.ExtractionFile, locale model.LocaleID, blocks []*model.Block) error {
	skelPath := ""
	if entry != nil && entry.Skeleton != "" {
		skelPath = filepath.Join(project.ExtractionDir(layout, batchID), entry.Skeleton)
	}
	return writeMergedSourceWithSkeleton(ctx, reg, formatName, sourceAbs, targetPath, locale, blocks, skelPath, nil)
}

// writeMergedSourceWithSkeleton is the underlying writer that takes the
// skeleton as either a file path (skelPath, for the XLIFF/PO extraction flow)
// or raw bytes (skelBytes, for a bilingual interchange .klz that carries the
// skeleton inline). When both are empty the writer re-serializes from its parse
// tree (lower fidelity). skelBytes takes precedence.
func writeMergedSourceWithSkeleton(ctx context.Context, reg *registry.FormatRegistry, formatName, sourceAbs, targetPath string, locale model.LocaleID, blocks []*model.Block, skelPath string, skelBytes []byte) error {
	writer, err := reg.NewWriter(registry.FormatID(formatName))
	if err != nil {
		return err
	}
	writer.SetLocale(locale)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	if err := writer.SetOutput(targetPath); err != nil {
		return err
	}

	if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
		switch {
		case len(skelBytes) > 0:
			// Inline skeleton (from a .klz): read-mode store over the bytes.
			store := format.NewSkeletonStoreFromBytes(skelBytes)
			consumer.SetSkeletonStore(store)
			defer store.Close()
		case skelPath != "":
			if _, statErr := os.Stat(skelPath); statErr == nil {
				if store, oerr := format.OpenSkeletonStore(skelPath); oerr == nil {
					consumer.SetSkeletonStore(store)
					defer store.Close()
				}
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

// absorbBlockIntoTM writes a block's source+target into the project TM
// with kapi-merge provenance. Returns (new, updated) counts. Today both
// are 1-or-0 since we write one entry per block; tracking them separately
// matters once we widen to per-segment.
func absorbBlockIntoTM(ctx context.Context, tm *sievepen.SQLiteTM, block *model.Block, source, target model.LocaleID, batchID, sourceRel, xliffPath string) (newCount, updatedCount int) {
	srcText := block.SourceText()
	tgtText := block.TargetText(target)
	if srcText == "" || tgtText == "" {
		return 0, 0
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
	entry := sievepen.TMEntry{
		ID: fmt.Sprintf("merge:%s:%s", batchID, contentHash),
		Variants: map[model.LocaleID][]model.Run{
			source: {{Text: &model.TextRun{Text: srcText}}},
			target: {{Text: &model.TextRun{Text: tgtText}}},
		},
		HintSrcLang: source,
		Origins: []sievepen.Origin{{
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
	if _, existed, _ := tm.GetEntry(ctx, entry.ID); existed {
		if err := tm.Add(ctx, entry); err == nil {
			return 0, 1
		}
		return 0, 0
	}
	if err := tm.Add(ctx, entry); err == nil {
		return 1, 0
	}
	return 0, 0
}

// expandMergeInputs turns a mixed list of files/globs/dirs into a flat,
// de-duplicated list of regular files.
func expandMergeInputs(inputs []string, root string) ([]string, error) {
	seen := make(map[string]bool)
	var out []string
	for _, in := range inputs {
		abs := in
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(root, in)
		}
		info, statErr := os.Stat(abs)
		if statErr == nil && info.IsDir() {
			// Directory: include every .xliff / .xlf within.
			entries, err := os.ReadDir(abs)
			if err != nil {
				return nil, fmt.Errorf("merge: read dir %s: %w", abs, err)
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				ext := strings.ToLower(filepath.Ext(name))
				if ext != ".xliff" && ext != ".xlf" && ext != ".po" {
					continue
				}
				p := filepath.Join(abs, name)
				if !seen[p] {
					seen[p] = true
					out = append(out, p)
				}
			}
			continue
		}
		// Try glob first.
		matches, err := filepath.Glob(abs)
		if err == nil && len(matches) > 0 {
			for _, m := range matches {
				if !seen[m] {
					seen[m] = true
					out = append(out, m)
				}
			}
			continue
		}
		// Fall through: treat as plain file path.
		if statErr == nil && !info.IsDir() && !seen[abs] {
			seen[abs] = true
			out = append(out, abs)
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

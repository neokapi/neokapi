package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"encoding/json"

	"github.com/google/uuid"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/klz"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/sievepen/klftm"
	"github.com/neokapi/neokapi/termbase"
	"github.com/neokapi/neokapi/termbase/klftb"
	"github.com/spf13/cobra"
)

// ExtractCmdOptions lets callers (like the bowrain binary) inject extra
// hooks into the extract command. None are required today.
type ExtractCmdOptions struct{}

// NewExtractCmd returns the `kapi extract` command (AD-017, issue #415).
// Emits one XLIFF 2.x (or PO) file per source → target-locale pair with
// TM pre-fill, under .kapi/cache/extractions/<batch-id>/ bookkeeping.
func (a *App) NewExtractCmd(_ ExtractCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "extract",
		Short:   "Emit a bilingual file for a translator — native .klz or XLIFF/PO",
		GroupID: "content",
		Long: `Emit bilingual XLIFF 2.x (default) or PO files for each target locale
declared in a .kapi project, pre-filled from the project's translation
memory.

Each invocation writes one batch of outputs under .kapi/cache/extractions/<batch-id>/
plus one bilingual file per source → target pair in --out-dir (default "out/").`,
		Example: `  kapi extract -p app.kapi --no-tm
  kapi extract -p app.kapi --target-lang fr
  kapi extract -p app.kapi --target-lang fr,de,es
  kapi extract src/*.json -o work.klz --target-lang fr,qps   # ad-hoc .klz workspace`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Ad-hoc workspace: `extract <sources> -o work.klz` ingests source
			// documents into a portable .klz workspace (no project needed).
			out, _ := cmd.Flags().GetString("output")
			format, _ := cmd.Flags().GetString("format")
			withSource, _ := cmd.Flags().GetBool("with-source")
			// Bilingual interchange .klz: `extract -i <src> --format klz` emits a
			// task-scoped (kind=kapi-interchange) package per source→target pair.
			if format == ExtractFormatKLZ {
				return a.runExtractKlz(cmd)
			}
			if isKlzPath(out) || len(args) > 0 {
				if !isKlzPath(out) {
					return errors.New("extract: writing source files needs -o <work.klz>")
				}
				tl, _ := cmd.Flags().GetString("target-lang")
				layout, _ := cmd.Flags().GetString("out")
				return a.extractToKlz(cmd.Context(), args, out, tl, layout, withSource)
			}
			return a.runExtract(cmd)
		},
	}
	AddProjectFlag(cmd)
	cmd.Flags().StringP("output", "o", "", "write an ad-hoc .klz workspace from the given source files")
	cmd.Flags().String("out", "", "merge-time output layout recorded in the .klz (e.g. 'l10n/{lang}/{name}.{ext}')")
	cmd.Flags().String("target-lang", "", "comma-separated target locales (default: all in recipe)")
	cmd.Flags().String("only", "", "restrict to a single content collection by name")
	cmd.Flags().String("pattern", "", "extra glob pattern restricting which source files to include")
	cmd.Flags().String("format", ExtractFormatXLIFF2, "bilingual output format (xliff2 | po | klz)")
	cmd.Flags().String("xliff-version", "", "XLIFF 2.x version to emit (2.0, 2.1, 2.2; default 2.2)")
	cmd.Flags().Bool("no-tm", false, "skip TM pre-fill on extract")
	cmd.Flags().Bool("with-source", false, "embed raw source bytes in the .klz (default: identity + skeleton only)")
	cmd.Flags().String("out-dir", "out", "directory for emitted bilingual files (relative to project)")
	cmd.Flags().Bool("redact", false, "replace sensitive content with placeholders; originals stay in a local vault for merge")
	cmd.Flags().String("redact-rules", "", "path to a redaction rules YAML file (implies --redact)")
	return cmd
}

// resolveRedaction determines the effective redaction spec for an extract
// run, combining the project recipe's defaults with the --redact /
// --redact-rules flags. Returns nil when redaction is off.
func resolveRedaction(cmd *cobra.Command, ctx *project.ProjectContext, rootDir string) (*project.RedactionSpec, error) {
	redactFlag, _ := cmd.Flags().GetBool("redact")
	redactRules, _ := cmd.Flags().GetString("redact-rules")

	var base *project.RedactionSpec
	if ctx.Project != nil {
		base = ctx.Project.Defaults.Redaction
	}
	enabled := redactFlag || redactRules != "" || (base != nil && base.Enabled)
	if !enabled {
		return nil, nil
	}

	eff := project.RedactionSpec{}
	if base != nil {
		eff = *base
	}
	eff.Enabled = true
	if redactRules != "" {
		eff.Rules = redactRules
	}
	if len(eff.Detectors) == 0 {
		eff.Detectors = []string{"rules"}
	}
	if eff.Rules == "" {
		return nil, errors.New("extract: redaction enabled but no rules file — set defaults.redaction.rules in the recipe or pass --redact-rules")
	}
	if !filepath.IsAbs(eff.Rules) {
		eff.Rules = filepath.Join(rootDir, eff.Rules)
	}
	return &eff, nil
}

// Supported extract output formats (AD-017). PO is tracked as a
// follow-up and will land behind the same flag surface.
const (
	ExtractFormatXLIFF2 = project.ExtractionFormatXLIFF2
	ExtractFormatPO     = project.ExtractionFormatPO
	// ExtractFormatKLZ selects the bilingual interchange .klz output
	// (kind=kapi-interchange) — neokapi's lossless interchange format for a
	// translator or reviewer (AD-025 §7).
	ExtractFormatKLZ = "klz"
)

func (a *App) runExtract(cmd *cobra.Command) error {
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
	if status := project.CheckPlugins(proj, a.InstalledPluginList()); !status.Satisfied {
		for _, issue := range status.Issues {
			switch issue.Type {
			case "missing":
				fmt.Fprintf(os.Stderr, "Warning: plugin %q required by project but not installed\n", issue.Plugin)
			case "version_mismatch":
				fmt.Fprintf(os.Stderr, "Warning: plugin %q version mismatch — requires %s, installed %s\n",
					issue.Plugin, issue.Required, issue.InstalledVersion)
			}
		}
		return fmt.Errorf("project plugin requirements not met — install missing plugins or adjust version constraints in %s", projectPath)
	}
	// pctx is a *project.ProjectContext (not context.Context); renamed to avoid
	// shadowing the cancellation context (cmd.Context()) used later in this function.
	pctx := project.NewProjectContext(proj, projectPath)

	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return fmt.Errorf("resolve project layout: %w", err)
	}
	if err := project.EnsureLayout(layout); err != nil {
		return err
	}

	format, _ := cmd.Flags().GetString("format")
	switch format {
	case ExtractFormatXLIFF2, ExtractFormatPO:
		// ok
	default:
		return fmt.Errorf("extract: unknown --format %q (supported: %s, %s)", format, ExtractFormatXLIFF2, ExtractFormatPO)
	}
	xliffVersion, _ := cmd.Flags().GetString("xliff-version")
	if xliffVersion != "" && !xliff2.IsSupportedVersion(xliffVersion) {
		return fmt.Errorf("extract: unsupported --xliff-version %q (expected %v)", xliffVersion, xliff2.SupportedXLIFFVersions)
	}

	noTM, _ := cmd.Flags().GetBool("no-tm")
	only, _ := cmd.Flags().GetString("only")
	pattern, _ := cmd.Flags().GetString("pattern")

	targets, err := resolveTargetLocales(cmd, pctx)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("extract: no target locales — set defaults.target_languages in %s or pass --target-lang", projectPath)
	}

	files, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return fmt.Errorf("extract: resolve content: %w", err)
	}
	files = filterFiles(files, only, pattern, layout.Root)
	if len(files) == 0 {
		return errors.New("extract: no source files matched — check content patterns / --only / --pattern")
	}

	outDir, _ := cmd.Flags().GetString("out-dir")
	if outDir == "" {
		outDir = "out"
	}
	absOut := outDir
	if !filepath.IsAbs(absOut) {
		absOut = filepath.Join(layout.Root, absOut)
	}
	if err := os.MkdirAll(absOut, 0o755); err != nil {
		return fmt.Errorf("extract: create out dir: %w", err)
	}

	batchID := uuid.NewString()
	batchDir, err := project.EnsureExtractionDir(layout, batchID)
	if err != nil {
		return err
	}

	redactionSpec, err := resolveRedaction(cmd, pctx, layout.Root)
	if err != nil {
		return err
	}
	redactionVault := ""
	if redactionSpec != nil {
		redactionVault = layout.RedactionSidecarPath(batchID)
		fmt.Fprintf(cmd.OutOrStdout(), "Redaction enabled (rules=%s) — originals stay in %s\n",
			redactionSpec.Rules, redactionVault)
	}

	var tm sievepen.TranslationMemory
	if !noTM {
		if a.TMBackend != nil {
			tm = a.TMBackend
		} else {
			tmPath := filepath.Join(layout.StateDir, "tm.db")
			loaded, err := sievepen.NewSQLiteTM(tmPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: extract: open project TM at %s: %v (continuing with no TM)\n", tmPath, err)
			} else {
				defer loaded.Close()
				tm = loaded
			}
		}
	}

	manifest := &project.ExtractionManifest{
		SchemaVersion: project.ExtractionSchemaVersion,
		Kind:          project.ExtractionManifestKind,
		BatchID:       batchID,
		Generator: project.ExtractionGenerator{
			ID:      "kapi",
			Version: version.Version,
		},
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		SourceLocale: pctx.SourceLocale,
		Options: project.ExtractionOptions{
			Format:       format,
			XLIFFVersion: effectiveXLIFFVersion(xliffVersion),
			NoTM:         noTM,
			Only:         only,
			Pattern:      pattern,
			Segmentation: pctx.Project != nil && pctx.Project.Defaults.Segmentation.Source,
		},
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Extracting batch %s (format=%s, targets=%v, sources=%d)\n",
		batchID, format, targets, len(files))

	failures := 0

	for _, tgt := range targets {
		pair := project.ExtractionPair{TargetLocale: tgt}
		pairOutDir := absOut

		for _, src := range files {
			outName := bilingualOutputName(src, pctx.SourceLocale, tgt, format)
			outPath := filepath.Join(pairOutDir, outName)

			sourceHash, err := project.HashFile(src.Path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "extract: hash %s: %v\n", src.Path, err)
				failures++
				continue
			}

			ef, err := a.extractOne(cmd.Context(), extractTask{
				ctx:            pctx,
				layout:         layout,
				source:         src,
				sourceHash:     sourceHash,
				targetLocale:   tgt,
				outputPath:     outPath,
				batchDir:       batchDir,
				batchID:        batchID,
				format:         format,
				xliffVersion:   xliffVersion,
				tm:             tm,
				redaction:      redactionSpec,
				redactionVault: redactionVault,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "extract: %s → %s: %v\n", src.Relative, tgt, err)
				failures++
				continue
			}
			rel, _ := filepath.Rel(layout.Root, outPath)
			ef.Skeleton = project.SkeletonFilename(strings.TrimPrefix(sourceHash, "sha256:"))
			if pair.Output == "" {
				pair.Output = rel
			}
			_ = rel
			pair.Files = append(pair.Files, ef)

			manifest.Totals.Add(ef.Leverage)
		}

		manifest.Pairs = append(manifest.Pairs, pair)

		fmt.Fprintf(cmd.OutOrStdout(), "  %s: %d files, %d blocks, TM exact=%d fuzzy=%d new=%d\n",
			tgt,
			len(pair.Files),
			sumBlocks(pair.Files),
			sumLeverage(pair.Files).Exact,
			sumLeverage(pair.Files).Fuzzy,
			sumLeverage(pair.Files).New)
	}

	if err := project.SaveExtractionManifest(layout, manifest); err != nil {
		return fmt.Errorf("extract: save manifest: %w", err)
	}

	total := manifest.Totals
	fmt.Fprintf(cmd.OutOrStdout(), "\nBatch %s complete. Manifest: %s\n",
		batchID, filepath.Join(batchDir, project.ExtractionManifestFilename))
	fmt.Fprintf(cmd.OutOrStdout(), "Aggregate TM leverage: exact=%d fuzzy=%d new=%d (total=%d)\n",
		total.Exact, total.Fuzzy, total.New, total.Total())

	if failures > 0 {
		return fmt.Errorf("extract: %d source/target pair(s) failed — see errors above", failures)
	}
	return nil
}

type extractTask struct {
	ctx          *project.ProjectContext
	layout       project.Layout
	source       project.ResolvedFile
	sourceHash   string
	targetLocale model.LocaleID
	outputPath   string
	batchDir     string
	batchID      string
	format       string // xliff2 | po
	xliffVersion string
	tm           sievepen.TranslationMemory

	// redaction, when non-nil, redacts source blocks before TM pre-fill and
	// write; originals are persisted to redactionVault for merge.
	redaction      *project.RedactionSpec
	redactionVault string
}

// extractOne processes a single source file for a single target locale:
// reads blocks (capturing the source skeleton to the batch dir), applies
// TM pre-fill, and writes an XLIFF 2.x file stamped with the batch id.
func (a *App) extractOne(ctx context.Context, task extractTask) (project.ExtractionFile, error) {
	reader, err := a.FormatReg.NewReader(registry.FormatID(task.source.Format))
	if err != nil {
		return project.ExtractionFile{}, fmt.Errorf("open source format %q: %w", task.source.Format, err)
	}
	// Apply the project's per-item format config (defaults.formats overlaid
	// by the item's format.config) so recipe options like
	// translateFrontMatter actually reach the reader.
	if err := applyFormatConfig(reader, mergedFormatConfig(task.ctx.Project, task.source.Format, task.source.Item)); err != nil {
		return project.ExtractionFile{}, fmt.Errorf("apply format config for %s: %w", task.source.Relative, err)
	}

	// Persist the source skeleton for merge — only when the source reader
	// supports skeleton emission (most text formats do, including the keyed
	// catalog formats: JSON/YAML/.properties, Android XML, .resx, Apple
	// .strings/.stringsdict/.xcstrings, .arb, i18next, design tokens).
	// Formats without a skeleton emitter (e.g. binary gettext MO, PDF)
	// re-read the source at merge time; stale detection still works via the
	// source hash carried in the XLIFF file notes.
	skeletonHash := strings.TrimPrefix(task.sourceHash, "sha256:")
	skeletonPath := filepath.Join(task.batchDir, project.SkeletonFilename(skeletonHash))
	var skelStore *format.SkeletonStore
	if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
		// Only capture if we don't already have one from an earlier pair
		// for the same source (batch may extract N target locales off
		// the same source file).
		if _, err := os.Stat(skeletonPath); os.IsNotExist(err) {
			skelStore, err = format.NewSkeletonStoreAt(skeletonPath)
			if err != nil {
				return project.ExtractionFile{}, fmt.Errorf("create skeleton store: %w", err)
			}
			emitter.SetSkeletonStore(skelStore)
		}
	}

	sourceFile, err := os.Open(task.source.Path)
	if err != nil {
		return project.ExtractionFile{}, fmt.Errorf("open %s: %w", task.source.Path, err)
	}
	defer sourceFile.Close()

	doc := &model.RawDocument{
		URI:          task.source.Path,
		SourceLocale: task.ctx.SourceLocale,
		TargetLocale: task.targetLocale,
		FormatID:     task.source.Format,
		Reader:       sourceFile,
	}
	if err := reader.Open(ctx, doc); err != nil {
		if skelStore != nil {
			_ = skelStore.Close()
		}
		return project.ExtractionFile{}, fmt.Errorf("reader.Open: %w", err)
	}
	defer reader.Close()

	// Collect blocks from the source; preserve layer so we can re-emit it.
	var sourceLayer *model.Layer
	var blocks []*model.Block
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			if skelStore != nil {
				_ = skelStore.Close()
			}
			return project.ExtractionFile{}, fmt.Errorf("reader.Read: %w", res.Error)
		}
		switch res.Part.Type {
		case model.PartBlock:
			if block, ok := res.Part.Resource.(*model.Block); ok {
				blocks = append(blocks, block)
			}
		case model.PartLayerStart:
			if l, ok := res.Part.Resource.(*model.Layer); ok && sourceLayer == nil {
				sourceLayer = l
			}
		}
	}
	if skelStore != nil {
		_ = skelStore.Close()
	}

	// Redaction: replace sensitive source spans with protected placeholders
	// before TM pre-fill and write, persisting the originals to the batch
	// vault sidecar. Running before TM keeps the redacted text out of TM
	// lookups (and thus out of pre-filled targets), so nothing sensitive
	// reaches the XLIFF.
	if task.redaction != nil {
		if err := applyRedaction(blocks, task.redaction, task.layout.Root, task.redactionVault, task.ctx.SourceLocale); err != nil {
			return project.ExtractionFile{}, fmt.Errorf("redaction: %w", err)
		}
	}

	// Segmentation overlay: when the recipe opts in, run the SRX tool
	// over each block's source before TM pre-fill. Block identity is
	// preserved (hash is over SourceText(), which concatenates segments),
	// so on/off toggles between extractions are safe.
	if task.ctx.Project != nil && task.ctx.Project.Defaults.Segmentation.Source {
		if err := applySegmentation(blocks, task.ctx.Project.Defaults.Segmentation); err != nil {
			return project.ExtractionFile{}, fmt.Errorf("segmentation: %w", err)
		}
	}

	// TM pre-fill: fill block.Targets[targetLocale] for any exact/fuzzy
	// match. Leverage stats reflect one decision per block (counting the
	// first segment's pre-fill outcome for that block).
	leverage := project.ExtractionLeverageStats{}
	threshold := float64(task.ctx.Project.Defaults.TM.ResolvedFuzzyThreshold()) / 100.0
	for _, b := range blocks {
		if !b.Translatable {
			continue
		}
		outcome := applyTMPrefill(ctx, task.tm, b, task.ctx.SourceLocale, task.targetLocale, threshold)
		switch outcome {
		case prefillExact:
			leverage.Exact++
		case prefillFuzzy:
			leverage.Fuzzy++
		default:
			leverage.New++
		}
	}

	// Write the bilingual output file in the requested format.
	if err := os.MkdirAll(filepath.Dir(task.outputPath), 0o755); err != nil {
		return project.ExtractionFile{}, fmt.Errorf("mkdir output: %w", err)
	}
	outFile, err := os.Create(task.outputPath)
	if err != nil {
		return project.ExtractionFile{}, fmt.Errorf("create %s: %w", task.outputPath, err)
	}

	switch task.format {
	case ExtractFormatPO:
		if err := writePOExtract(outFile, task.targetLocale, task.batchID, task.source.Relative, task.sourceHash, blocks); err != nil {
			_ = outFile.Close()
			_ = os.Remove(task.outputPath)
			return project.ExtractionFile{}, fmt.Errorf("po writer: %w", err)
		}
	default: // xliff2
		writer := xliff2.NewWriter()
		if err := writer.SetOutputWriter(outFile); err != nil {
			_ = outFile.Close()
			return project.ExtractionFile{}, err
		}
		if task.xliffVersion != "" {
			if err := writer.SetVersion(task.xliffVersion); err != nil {
				_ = outFile.Close()
				return project.ExtractionFile{}, err
			}
		}
		writer.SetLocale(task.targetLocale)
		writer.SetFileNotes([]xliff2.FileNote{
			xliff2.BatchIDNote(task.batchID),
			xliff2.SourceFileNote(task.source.Relative),
			xliff2.SourceHashNote(task.sourceHash),
		})

		// Feed parts: emit a synthetic Layer (so writer picks up source lang +
		// target lang) then the blocks.
		parts := make(chan *model.Part, len(blocks)+1)
		parts <- &model.Part{Type: model.PartLayerStart, Resource: &model.Layer{
			ID:             "file-" + sanitizeFileID(task.source.Relative),
			Name:           sanitizeFileID(task.source.Relative),
			Format:         "xliff2",
			Locale:         task.ctx.SourceLocale,
			IsMultilingual: true,
			Properties: map[string]string{
				"target-language": string(task.targetLocale),
			},
		}}
		for _, b := range blocks {
			parts <- &model.Part{Type: model.PartBlock, Resource: b}
		}
		close(parts)

		if err := writer.Write(ctx, parts); err != nil {
			_ = outFile.Close()
			return project.ExtractionFile{}, fmt.Errorf("writer.Write: %w", err)
		}
	}

	if err := outFile.Close(); err != nil {
		return project.ExtractionFile{}, err
	}

	segments := 0
	for _, b := range blocks {
		segments += b.SourceSegmentCount()
	}

	return project.ExtractionFile{
		Source:     task.source.Relative,
		SourceHash: task.sourceHash,
		Format:     task.format,
		Blocks:     len(blocks),
		Segments:   segments,
		Leverage:   leverage,
	}, nil
}

type prefillOutcome int

const (
	prefillNone prefillOutcome = iota
	prefillExact
	prefillFuzzy
)

// applyTMPrefill queries the TM and fills the block's target for the locale
// with the best match when it exceeds the threshold. Returns whether the
// block was covered by an exact or fuzzy match. The source segment spans are
// the unit of lookup — one lookup per span — and the pre-filled target is
// written as a run sequence plus a target segmentation overlay index-aligned
// to the source spans (so a segmented block round-trips per-segment targets).
func applyTMPrefill(ctx context.Context, tm sievepen.TranslationMemory, block *model.Block, source, target model.LocaleID, threshold float64) prefillOutcome {
	if tm == nil || block == nil || len(block.Source) == 0 {
		return prefillNone
	}
	opts := sievepen.LookupOptions{MinScore: threshold, MaxResults: 1}

	segCount := block.SourceSegmentCount()
	srcSeg := block.SourceSegmentation() // nil when unsegmented (one implicit span)

	var targetRuns []model.Run
	var targetSpans []model.Span
	matched := 0
	anyExact := false
	for i := range segCount {
		spanID := fmt.Sprintf("s%d", i+1)
		if srcSeg != nil && i < len(srcSeg.Spans) {
			spanID = srcSeg.Spans[i].ID
		}
		start := len(targetRuns)
		// Ambiguous matches (several full-score exacts with differing
		// targets) are never pre-filled: an unattended merge would turn an
		// arbitrary pick into published content. Left empty, they surface
		// as untranslated for a human (or higher-context tool) to decide.
		if matches, err := tm.LookupSegment(ctx, block, i, source, target, opts); err == nil && len(matches) > 0 && !matches[0].Ambiguous {
			if text := matches[0].Entry.VariantText(target); text != "" {
				targetRuns = append(targetRuns, model.Run{Text: &model.TextRun{Text: text}})
				matched++
				if matches[0].Score >= 1.0 {
					anyExact = true
				}
			}
		}
		// Span covers this segment's (0 or 1) target runs, preserving id and
		// index alignment with the source span even when unmatched (empty).
		targetSpans = append(targetSpans, model.Span{ID: spanID, Range: model.RunRange{StartRun: start, EndRun: len(targetRuns)}})
	}
	if matched == 0 {
		return prefillNone
	}
	block.SetTargetRuns(target, targetRuns)
	if segCount > 1 {
		key := model.Variant(target)
		block.SetSegmentation(&key, targetSpans)
	}
	// Stash the match type on the block so downstream writers can surface
	// it in format-appropriate ways (PO's `#, fuzzy` flag; XLIFF 2's
	// segment state — not yet emitted, tracked as a follow-up).
	if block.Properties == nil {
		block.Properties = make(map[string]string, 1)
	}
	if anyExact && matched == segCount {
		block.Properties["kapi-tm-match"] = "exact"
		return prefillExact
	}
	block.Properties["kapi-tm-match"] = "fuzzy"
	return prefillFuzzy
}

// applySegmentation runs the existing SRX segmentation tool over each
// block's source — the overlay path from AD-017 / #417. The tool is a
// regular kapi tool.Tool but we call its block handler directly here to
// avoid wiring a one-stage channel pipeline into the extract flow.
// applyRedaction runs the redact tool over the source blocks in external
// mode, writing originals to the batch vault sidecar at vaultPath. Rule paths
// in the spec are resolved relative to rootDir.
func applyRedaction(blocks []*model.Block, spec *project.RedactionSpec, rootDir, vaultPath string, sourceLocale model.LocaleID) error {
	cfg := &tools.RedactConfig{
		Detectors:    spec.Detectors,
		Placeholder:  spec.Placeholder,
		VaultPath:    vaultPath,
		SourceLocale: sourceLocale,
	}
	if len(cfg.Detectors) == 0 {
		cfg.Detectors = []string{tools.DetectRules}
	}
	if spec.Rules != "" {
		if filepath.IsAbs(spec.Rules) {
			cfg.RulesPath = spec.Rules
		} else {
			cfg.RulesPath = filepath.Join(rootDir, spec.Rules)
		}
	}
	rt, err := tools.NewRedactTool(cfg)
	if err != nil {
		return err
	}
	for _, b := range blocks {
		part := &model.Part{Type: model.PartBlock, Resource: b}
		if _, err := rt.Apply(part); err != nil {
			return err
		}
	}
	return rt.Flush()
}

func applySegmentation(blocks []*model.Block, conf project.SegmentationDefaults) error {
	cfg := &tools.SegmentationConfig{
		SegmentSource: true,
	}
	if conf.SRX != "" {
		cfg.EngineParams = map[string]any{"rulesPath": conf.SRX}
	}
	t := tools.NewSegmentationTool(cfg)
	for _, b := range blocks {
		part := &model.Part{Type: model.PartBlock, Resource: b}
		if _, err := t.Apply(part); err != nil {
			return err
		}
	}
	return nil
}

// --- helpers ---

func resolveTargetLocales(cmd *cobra.Command, ctx *project.ProjectContext) ([]model.LocaleID, error) {
	raw, _ := cmd.Flags().GetString("target-lang")
	if raw == "" {
		return append([]model.LocaleID(nil), ctx.TargetLocales...), nil
	}
	var out []model.LocaleID
	seen := make(map[model.LocaleID]bool)
	for item := range strings.SplitSeq(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		loc := model.LocaleID(item)
		if seen[loc] {
			continue
		}
		seen[loc] = true
		out = append(out, loc)
	}
	return out, nil
}

func filterFiles(files []project.ResolvedFile, only, pattern, root string) []project.ResolvedFile {
	if only == "" && pattern == "" {
		return files
	}
	var out []project.ResolvedFile
	for _, f := range files {
		if only != "" && f.Collection != only {
			continue
		}
		if pattern != "" {
			abs := f.Path
			patAbs := pattern
			if !filepath.IsAbs(patAbs) {
				patAbs = filepath.Join(root, pattern)
			}
			if ok, _ := filepath.Match(patAbs, abs); !ok {
				// also try relative match
				if ok2, _ := filepath.Match(pattern, f.Relative); !ok2 {
					continue
				}
			}
		}
		out = append(out, f)
	}
	return out
}

// bilingualOutputName constructs the output filename for one source → target
// pair. Format: <source-rel-slug>.<src>-to-<tgt>.<ext>. Slashes in the
// relative path become dashes; the extension is stripped so the bilingual
// extension wins.
func bilingualOutputName(src project.ResolvedFile, source, target model.LocaleID, ext string) string {
	stem := src.Relative
	stem = strings.TrimSuffix(stem, filepath.Ext(stem))
	slug := strings.ReplaceAll(stem, string(filepath.Separator), "-")
	slug = strings.ReplaceAll(slug, "/", "-")
	out := fmt.Sprintf("%s.%s-to-%s", slug, source, target)
	switch ext {
	case ExtractFormatPO:
		return out + ".po"
	case ExtractFormatKLZ:
		return out + ".klz"
	default:
		return out + ".xliff"
	}
}

func sanitizeFileID(rel string) string {
	rel = strings.ReplaceAll(rel, string(filepath.Separator), "/")
	return rel
}

func sumBlocks(files []project.ExtractionFile) int {
	n := 0
	for _, f := range files {
		n += f.Blocks
	}
	return n
}

func sumLeverage(files []project.ExtractionFile) project.ExtractionLeverageStats {
	var s project.ExtractionLeverageStats
	for _, f := range files {
		s.Add(f.Leverage)
	}
	return s
}

func effectiveXLIFFVersion(flag string) string {
	if flag != "" {
		return flag
	}
	return xliff2.DefaultXLIFFVersion
}

// ─── bilingual interchange .klz (kind=kapi-interchange) ─────────
//
// `kapi extract --format klz` emits one task-scoped .klz per source→target
// pair (AD-025 §7): the source blocks' targets pre-filled from TM (as
// `targets/<locale>` overlays, hydrated by `kapi merge` exactly like the
// workspace flow), the per-source round-trip skeleton, a minimal recipe
// carrying the locale pair, and the relevant TM/termbase context subset. It is
// neokapi's lossless interchange format for a translator or reviewer.

// runExtractKlz drives the bilingual-interchange extract over a project's
// content × target locales, writing one <slug>.<src>-to-<tgt>.klz per pair.
func (a *App) runExtractKlz(cmd *cobra.Command) error {
	projectPath, err := RequireProjectPath(cmd)
	if err != nil {
		return err
	}
	proj, err := a.LoadProjectInteractive(cmd.Context(), projectPath, LoadProjectInteractiveOptions{AssumeYes: a.AssumeYes})
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	pctx := project.NewProjectContext(proj, projectPath)
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return fmt.Errorf("resolve project layout: %w", err)
	}
	if err := project.EnsureLayout(layout); err != nil {
		return err
	}

	noTM, _ := cmd.Flags().GetBool("no-tm")
	only, _ := cmd.Flags().GetString("only")
	pattern, _ := cmd.Flags().GetString("pattern")

	targets, err := resolveTargetLocales(cmd, pctx)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("extract: no target locales — set defaults.target_languages in %s or pass --target-lang", projectPath)
	}

	files, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return fmt.Errorf("extract: resolve content: %w", err)
	}
	files = filterFiles(files, only, pattern, layout.Root)
	if len(files) == 0 {
		return errors.New("extract: no source files matched — check content patterns / --only / --pattern")
	}

	outDir, _ := cmd.Flags().GetString("out-dir")
	if outDir == "" {
		outDir = "out"
	}
	absOut := outDir
	if !filepath.IsAbs(absOut) {
		absOut = filepath.Join(layout.Root, absOut)
	}
	if err := os.MkdirAll(absOut, 0o755); err != nil {
		return fmt.Errorf("extract: create out dir: %w", err)
	}

	var tm sievepen.TranslationMemory
	if !noTM {
		if a.TMBackend != nil {
			tm = a.TMBackend
		} else if loaded, lerr := sievepen.NewSQLiteTM(filepath.Join(layout.StateDir, "tm.db")); lerr == nil {
			defer loaded.Close()
			tm = loaded
		}
	}

	// Termbase context (optional): bound termbase or project termbase.db.
	var tb termbase.TermBase
	if tbLoaded, terr := termbase.NewSQLiteTermBase(filepath.Join(layout.StateDir, "termbase.db")); terr == nil {
		defer tbLoaded.Close()
		tb = tbLoaded
	}

	written := 0
	for _, tgt := range targets {
		for _, src := range files {
			outName := bilingualOutputName(src, pctx.SourceLocale, tgt, ExtractFormatKLZ)
			outPath := filepath.Join(absOut, outName)
			if err := a.extractOneKlz(cmd.Context(), klzInterchangeTask{
				ctx: pctx, source: src, targetLocale: tgt, outputPath: outPath, tm: tm, tb: tb,
			}); err != nil {
				return fmt.Errorf("extract: %s → %s: %w", src.Relative, tgt, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  %s → %s: %s\n", src.Relative, tgt, outName)
			written++
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nExtracted %d bilingual interchange package(s) into %s\n", written, outDir)
	return nil
}

type klzInterchangeTask struct {
	ctx          *project.ProjectContext
	source       project.ResolvedFile
	targetLocale model.LocaleID
	outputPath   string
	tm           sievepen.TranslationMemory
	tb           termbase.TermBase
}

// extractOneKlz assembles a KindInterchange package for one (source, target)
// pair: reads the source blocks (capturing the skeleton), TM-pre-fills the
// target as a `targets/<locale>` overlay per block, attaches the relevant
// TM/termbase context, and writes the .klz.
func (a *App) extractOneKlz(ctx context.Context, task klzInterchangeTask) error {
	srcAbs := task.source.Path
	data, err := os.ReadFile(srcAbs)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	formatID := registry.FormatID(task.source.Format)
	sourceHash := project.HashBytes(data)

	blocks, _, err := readSourceBlocks(ctx, a.FormatReg, string(formatID), srcAbs, task.ctx.SourceLocale, task.targetLocale,
		mergedFormatConfig(task.ctx.Project, string(formatID), task.source.Item))
	if err != nil {
		return fmt.Errorf("read blocks: %w", err)
	}

	// Capture the round-trip skeleton (best effort).
	var skeletons []klz.SkeletonDoc
	skelMember := ""
	if skel, serr := captureSkeletonBytes(ctx, a.FormatReg, formatID, srcAbs, data, task.ctx.SourceLocale); serr == nil && len(skel) > 0 {
		skelMember = klz.SkeletonDir + filepath.Base(task.source.Relative)
		skeletons = append(skeletons, klz.SkeletonDoc{
			Path: skelMember, SourcePath: task.source.Relative, FormatID: task.source.Format,
			ContentHash: sourceHash, Content: klz.BytesContent(skel),
		})
	}

	// TM pre-fill the target as a per-block overlay (keyed by block ID, so
	// `kapi merge`'s hydrate step applies it the same way the workspace flow
	// does). Also gather the TM entries actually consulted as inline context.
	threshold := float64(task.ctx.Project.Defaults.TM.ResolvedFuzzyThreshold()) / 100.0
	srcArchive := "source/" + filepath.Base(task.source.Relative)
	var overlays []klz.OverlayDoc
	contextEntries := map[string]sievepen.TMEntry{}
	for _, b := range blocks {
		if !b.Translatable || b.ID == "" {
			continue
		}
		if task.tm != nil {
			if matches, merr := task.tm.Lookup(ctx, b, task.ctx.SourceLocale, task.targetLocale, sievepen.LookupOptions{MinScore: threshold, MaxResults: 1}); merr == nil && len(matches) > 0 && !matches[0].Ambiguous {
				if text := matches[0].Entry.VariantText(task.targetLocale); text != "" {
					payload, _ := json.Marshal(map[string]string{"text": text})
					overlays = append(overlays, klz.OverlayDoc{
						Kind: "targets/" + string(task.targetLocale), BlockHash: b.ID, Payload: payload, Source: srcArchive,
					})
				}
				contextEntries[matches[0].Entry.ID] = matches[0].Entry
			}
		}
	}

	// TM context subset (the consulted entries) so the package is
	// self-contained for offline review.
	var tmFile *klftm.File
	if len(contextEntries) > 0 {
		entries := make([]sievepen.TMEntry, 0, len(contextEntries))
		for _, e := range contextEntries {
			entries = append(entries, e)
		}
		tmFile = klftm.FromModel(entries, nil)
	}

	// Termbase context: the whole bound termbase (terms are small and the
	// reviewer needs the glossary).
	var tbFile *klftb.File
	if task.tb != nil {
		if concepts, cerr := task.tb.Concepts(ctx); cerr == nil && len(concepts) > 0 {
			tbFile = klftb.FromConcepts(concepts)
		}
	}

	recipe := newInterchangeRecipe(string(task.ctx.SourceLocale), string(task.targetLocale))

	pkg := &klz.Package{
		Kind:      klz.KindInterchange,
		Generator: &klz.GeneratorInfo{ID: "kapi", Version: version.Version},
		Recipe:    recipe,
		Skeletons: skeletons,
		Overlays:  overlays,
		TM:        tmFile,
		Termbase:  tbFile,
		Sources: []klz.SourceIdentity{{
			SourcePath: task.source.Relative, FormatID: task.source.Format,
			ContentHash: sourceHash, SkeletonPath: skelMember,
		}},
		InterchangeTask: &klz.InterchangeTask{
			SourceLocale: string(task.ctx.SourceLocale),
			TargetLocale: string(task.targetLocale),
			SourceFiles:  []string{task.source.Relative},
		},
	}
	if err := os.MkdirAll(filepath.Dir(task.outputPath), 0o755); err != nil {
		return err
	}
	return saveWorkspace(pkg, task.outputPath)
}

// Unused imports workaround — bytes/sort/io are here for helpers that
// may be added during merge/segmentation work. Referencing them here
// keeps the imports stable.
var (
	_ = bytes.NewReader
	_ = sort.Strings
	_ = io.Discard
)

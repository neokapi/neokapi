package host

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
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/kpz"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

// ExtractCmdOptions lets callers (like the bowrain binary) inject extra
// hooks into the extract command. None are required today.
type ExtractCmdOptions struct{}

// resolveRedaction determines the effective redaction spec for an extract
// run, combining the project recipe's defaults with the --redact /
// --redact-rules flags. Returns nil when redaction is off.
func resolveRedaction(cmd Command, ctx *project.ProjectContext, rootDir string) (*project.RedactionSpec, error) {
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
		return nil, errors.New("extract: redaction is enabled but no rules file is set. Set defaults.redaction.rules in the recipe or pass --redact-rules")
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
	extractFormatPO     = project.ExtractionFormatPO
	// ExtractFormatKPZ selects the bilingual interchange .kpz output
	// (kind=kapi-interchange) — neokapi's lossless interchange format for a
	// translator or reviewer (AD-025 §7).
	ExtractFormatKPZ = "kpz"
)

func (a *App) RunExtract(cmd Command) error {
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
				fmt.Fprintf(os.Stderr, "Warning: plugin %q version mismatch: requires %s, installed %s\n",
					issue.Plugin, issue.Required, issue.InstalledVersion)
			}
		}
		return fmt.Errorf("project plugin requirements not met. Install missing plugins or adjust version constraints in %s", projectPath)
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
	case ExtractFormatXLIFF2, extractFormatPO:
		// ok
	default:
		return fmt.Errorf("extract: unknown --format %q (supported: %s, %s)", format, ExtractFormatXLIFF2, extractFormatPO)
	}
	xliffVersion, _ := cmd.Flags().GetString("xliff-version")
	if xliffVersion != "" && !xliff2.IsSupportedVersion(xliffVersion) {
		return fmt.Errorf("extract: unsupported --xliff-version %q (expected %v)", xliffVersion, xliff2.SupportedXLIFFVersions)
	}

	noMemory := BoolFlagAny(cmd, "no-memory", "no-tm")
	only, _ := cmd.Flags().GetString("only")
	pattern, _ := cmd.Flags().GetString("pattern")

	targets, err := resolveTargetLocales(cmd, pctx)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("extract: no target locales. Set defaults.target_languages in %s or pass --target-lang", projectPath)
	}

	files, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return fmt.Errorf("extract: resolve content: %w", err)
	}
	files = filterFiles(files, only, pattern, layout.Root)
	if len(files) == 0 {
		return errors.New("extract: no source files matched. Check content patterns / --only / --pattern")
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
	}

	var tm memory.ContentMemory
	if !noMemory {
		if a.MemoryBackend != nil {
			tm = a.MemoryBackend
		} else if db, derr := a.ProjectDB(cmd.Context(), layout.Root); derr != nil {
			fmt.Fprintf(os.Stderr, "Warning: extract: open project store: %v (continuing with no content memory)\n", derr)
		} else if mem := db.Memory(); mem != nil {
			tm = mem
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
			NoMemory:     noMemory,
			Only:         only,
			Pattern:      pattern,
			Segmentation: pctx.Project != nil && pctx.Project.Defaults.Segmentation.Source,
		},
	}

	manifest.InputsHash = project.ExtractionInputsHash(layout, manifest.Options)

	// Incremental: reuse the latest prior batch's result for any source whose
	// bytes AND inputs (recipe, content memory, options) are unchanged, so a re-extract only
	// re-parses what actually changed. --force re-extracts everything.
	force, _ := cmd.Flags().GetBool("force")
	prior := loadReusablePrior(layout, manifest.InputsHash, force)

	// The structured result (output.ExtractOutput): its FormatText renders the
	// historical human report byte-for-byte, so text mode is unchanged while
	// --json gets a documented document on stdout.
	res := output.ExtractOutput{
		BatchID: batchID,
		Format:  format,
		Targets: localeStrings(targets),
		Sources: len(files),
	}
	if redactionSpec != nil {
		res.RedactionRules = redactionSpec.Rules
		res.RedactionVault = redactionVault
	}

	sink, progressReport := progressSink(cmd)
	emit := func(ev FlowRunEvent) {
		if sink != nil {
			ev.Flow = "extract"
			sink(ev)
		}
	}
	start := time.Now()
	totalPairs := len(targets) * len(files)
	pairIndex := 0

	failures := 0
	reused := 0

	prog := a.NewProgress(cmd, "extracting", totalPairs)
	defer prog.Done()

	for _, tgt := range targets {
		pair := project.ExtractionPair{TargetLocale: tgt}
		pairOutDir := absOut

		for _, src := range files {
			outName := bilingualOutputName(src, pctx.SourceLocale, tgt, format)
			outPath := filepath.Join(pairOutDir, outName)

			emit(FlowRunEvent{
				Type: FlowEventProgress, Locale: string(tgt),
				FileIndex: pairIndex, FileCount: totalPairs, FilePath: src.Path,
			})
			prog.Step(string(tgt) + " · " + src.Relative)
			prog.Advance()
			pairIndex++

			sourceHash, err := project.HashFile(src.Path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "extract: hash %s: %v\n", src.Path, err)
				failures++
				continue
			}

			// Reuse a prior batch's output when the source bytes are unchanged
			// (inputs already matched). Copies the content-addressed skeleton into
			// this batch dir — a byte copy, no re-parse — so merge resolves it here.
			if pe, ok := prior.reuse(string(tgt), src.Relative, sourceHash); ok {
				if err := pe.copySkeleton(layout, batchDir, outPath); err == nil {
					if pair.Output == "" {
						rel, _ := filepath.Rel(layout.Root, outPath)
						pair.Output = rel
					}
					pair.Files = append(pair.Files, pe.ef)
					manifest.Totals.Add(pe.ef.Leverage)
					reused++
					emit(FlowRunEvent{
						Type: FlowEventFileDone, Locale: string(tgt),
						FilePath: src.Path, OutputPath: outPath,
					})
					continue
				}
				// Skeleton copy failed — fall through to a fresh extract.
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
			emit(FlowRunEvent{
				Type: FlowEventFileDone, Locale: string(tgt),
				FilePath: src.Path, OutputPath: outPath,
			})
		}

		manifest.Pairs = append(manifest.Pairs, pair)

		lev := sumLeverage(pair.Files)
		res.Pairs = append(res.Pairs, output.ExtractPairOutput{
			TargetLocale: string(tgt),
			Files:        len(pair.Files),
			Blocks:       sumBlocks(pair.Files),
			Leverage:     output.LeverageOutput{Exact: lev.Exact, Fuzzy: lev.Fuzzy, New: lev.New},
		})
	}

	prog.Done()

	if err := project.SaveExtractionManifest(layout, manifest); err != nil {
		return fmt.Errorf("extract: save manifest: %w", err)
	}

	total := manifest.Totals
	res.Manifest = filepath.Join(batchDir, project.ExtractionManifestFilename)
	res.Reused = reused
	res.Leverage = output.LeverageOutput{Exact: total.Exact, Fuzzy: total.Fuzzy, New: total.New}
	res.Failures = failures

	emit(FlowRunEvent{
		Type:       FlowEventComplete,
		DurationMs: time.Since(start).Milliseconds(), FilesProcessed: totalPairs - failures,
		Message: fmt.Sprintf("Extracted %d pair(s) in batch %s", totalPairs-failures, batchID),
	})

	// Final report: FormatText renders the historical human lines; --json
	// renders the ExtractOutput document.
	if err := output.Print(cmd, res); err != nil {
		return err
	}

	if failures > 0 {
		return fmt.Errorf("extract: %d source/target pair(s) failed. See errors above", failures)
	}
	// A truncated progress feed is reported after the result, so the deliverable
	// still lands and the consumer still learns its feed was incomplete.
	return progressReport()
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
	tm           memory.ContentMemory

	// redaction, when non-nil, redacts source blocks before content memory pre-fill and
	// write; originals are persisted to redactionVault for merge.
	redaction      *project.RedactionSpec
	redactionVault string
}

// extractOne processes a single source file for a single target locale:
// reads blocks (capturing the source skeleton to the batch dir), applies
// content memory pre-fill, and writes an XLIFF 2.x file stamped with the batch id.
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
	// The success path: Close is where the skeleton's deferred write error and
	// its buffered flush surface (the extract path never calls Flush/Bytes), so
	// checking it is what stops a TRUNCATED skeleton being recorded in the
	// manifest as if it were whole. A later `kapi merge` would otherwise splice
	// the short skeleton and write a truncated deliverable, reporting success.
	if skelStore != nil {
		if err := skelStore.Close(); err != nil {
			return project.ExtractionFile{}, fmt.Errorf("capture skeleton for %s: %w", task.source.Relative, err)
		}
	}

	// Redaction: replace sensitive source spans with protected placeholders
	// before content memory pre-fill and write, persisting the originals to the batch
	// vault sidecar. Running before content memory keeps the redacted text out of content memory
	// lookups (and thus out of pre-filled targets), so nothing sensitive
	// reaches the XLIFF.
	if task.redaction != nil {
		if err := applyRedaction(ctx, blocks, task.redaction, task.layout.Root, task.redactionVault, task.ctx.SourceLocale); err != nil {
			return project.ExtractionFile{}, fmt.Errorf("redaction: %w", err)
		}
	}

	// Segmentation overlay: when the recipe opts in, run the SRX tool
	// over each block's source before content memory pre-fill. Block identity is
	// preserved (hash is over SourceText(), which concatenates segments),
	// so on/off toggles between extractions are safe.
	if task.ctx.Project != nil && task.ctx.Project.Defaults.Segmentation.Source {
		if err := applySegmentation(ctx, blocks, task.ctx.Project.Defaults.Segmentation); err != nil {
			return project.ExtractionFile{}, fmt.Errorf("segmentation: %w", err)
		}
	}

	// content memory pre-fill: fill block.Targets[targetLocale] for any exact/fuzzy
	// match. Leverage stats reflect one decision per block (counting the
	// first segment's pre-fill outcome for that block).
	leverage := project.ExtractionLeverageStats{}
	threshold := float64(task.ctx.Project.Defaults.Memory.ResolvedFuzzyThreshold()) / 100.0
	for _, b := range blocks {
		if !b.Translatable {
			continue
		}
		outcome := applyMemoryPrefill(ctx, task.tm, b, task.ctx.SourceLocale, task.targetLocale, threshold)
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
	case extractFormatPO:
		if err := WritePOExtract(outFile, task.targetLocale, task.batchID, task.source.Relative, task.sourceHash, blocks); err != nil {
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

// applyMemoryPrefill queries the content memory and fills the block's target for the locale
// with the best match when it exceeds the threshold. Returns whether the
// block was covered by an exact or fuzzy match. The source segment spans are
// the unit of lookup — one lookup per span — and the pre-filled target is
// written as a run sequence plus a target segmentation overlay index-aligned
// to the source spans (so a segmented block round-trips per-segment targets).
func applyMemoryPrefill(ctx context.Context, tm memory.ContentMemory, block *model.Block, source, target model.LocaleID, threshold float64) prefillOutcome {
	if tm == nil || block == nil || len(block.Source) == 0 {
		return prefillNone
	}
	opts := memory.LookupOptions{MinScore: threshold, MaxResults: 1}

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
		targetSpans = append(targetSpans, model.Span{ID: spanID, Range: model.SpanAnchor(model.RunPos{Run: start}, model.RunPos{Run: len(targetRuns)})})
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
func applyRedaction(ctx context.Context, blocks []*model.Block, spec *project.RedactionSpec, rootDir, vaultPath string, sourceLocale model.LocaleID) error {
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
		if _, err := rt.ApplyContext(ctx, part); err != nil {
			return err
		}
	}
	return rt.Flush()
}

func applySegmentation(ctx context.Context, blocks []*model.Block, conf project.SegmentationDefaults) error {
	cfg := &tools.SegmentationConfig{
		SegmentSource: true,
	}
	if conf.SRX != "" {
		cfg.EngineParams = map[string]any{"rulesPath": conf.SRX}
	}
	t := tools.NewSegmentationTool(cfg)
	for _, b := range blocks {
		part := &model.Part{Type: model.PartBlock, Resource: b}
		if _, err := t.ApplyContext(ctx, part); err != nil {
			return err
		}
	}
	return nil
}

// --- helpers ---

func resolveTargetLocales(cmd Command, ctx *project.ProjectContext) ([]model.LocaleID, error) {
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
	stem = format.TrimExt(stem)
	slug := strings.ReplaceAll(stem, string(filepath.Separator), "-")
	slug = strings.ReplaceAll(slug, "/", "-")
	out := fmt.Sprintf("%s.%s-to-%s", slug, source, target)
	switch ext {
	case extractFormatPO:
		return out + ".po"
	case ExtractFormatKPZ:
		return out + ".kpz"
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

// priorEntry is one reusable per-file result and the batch dir its skeleton
// lives in.
type priorEntry struct {
	ef      project.ExtractionFile
	batchID string
}

// priorBatch indexes every reusable per-file result across all prior batches
// whose inputs (recipe, options) match the current run, keyed by content — the
// (target, source, sourceHash) triple. Indexing by source hash (not just path)
// means a file is reused exactly when its bytes match a prior extraction,
// regardless of which batch produced it, so same-second batch timestamps can't
// pick the wrong one.
type priorBatch struct {
	byKey map[string]priorEntry
}

func reuseKey(tgt, srcRel, sourceHash string) string {
	return tgt + "\x00" + srcRel + "\x00" + sourceHash
}

// loadReusablePrior indexes the reusable results of every prior batch whose
// InputsHash matches. Returns nil (everything re-extracts) when forced, when
// there are no batches, or when none matches the current inputs.
func loadReusablePrior(layout project.Layout, inputsHash string, force bool) *priorBatch {
	if force || inputsHash == "" {
		return nil
	}
	// List the batch dirs directly and skip any that fail to load — notably the
	// current run's own dir, which exists (created above) but has no manifest yet.
	entries, err := os.ReadDir(project.ExtractionsRoot(layout))
	if err != nil {
		return nil
	}
	pb := &priorBatch{byKey: map[string]priorEntry{}}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := project.LoadExtractionManifest(layout, e.Name())
		if err != nil || m.InputsHash != inputsHash {
			continue
		}
		for _, pair := range m.Pairs {
			for _, ef := range pair.Files {
				if ef.Skeleton == "" {
					continue
				}
				pb.byKey[reuseKey(string(pair.TargetLocale), ef.Source, ef.SourceHash)] = priorEntry{ef: ef, batchID: m.BatchID}
			}
		}
	}
	if len(pb.byKey) == 0 {
		return nil
	}
	return pb
}

// reuse reports whether a prior batch holds a result for (tgt, srcRel) at exactly
// this sourceHash — i.e. the source bytes are unchanged — and returns it plus the
// batch dir its skeleton lives in.
func (p *priorBatch) reuse(tgt, srcRel, sourceHash string) (priorEntry, bool) {
	if p == nil {
		return priorEntry{}, false
	}
	e, ok := p.byKey[reuseKey(tgt, srcRel, sourceHash)]
	return e, ok
}

// copySkeleton brings the prior batch's content-addressed skeleton into the new
// batch dir and verifies the bilingual output still exists, so the reused result
// is fully resolvable from this batch. A byte copy — no re-parse.
func (e priorEntry) copySkeleton(layout project.Layout, batchDir, outPath string) error {
	if _, err := os.Stat(outPath); err != nil {
		return err // the prior bilingual output is gone — re-extract
	}
	src := filepath.Join(layout.ExtractionsDir(), e.batchID, e.ef.Skeleton)
	dst := filepath.Join(batchDir, e.ef.Skeleton)
	if src == dst {
		return nil
	}
	return copyFileContents(src, dst)
}

func copyFileContents(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func effectiveXLIFFVersion(flag string) string {
	if flag != "" {
		return flag
	}
	return xliff2.DefaultXLIFFVersion
}

// ─── bilingual interchange .kpz (kind=kapi-interchange) ─────────
//
// `kapi extract --format kpz` emits one task-scoped .kpz per source→target
// pair (AD-025 §7): the source blocks' targets pre-filled from content memory (as
// `targets/<locale>` overlays, hydrated by `kapi merge` exactly like the
// workspace flow), the per-source round-trip skeleton, a minimal recipe
// carrying the locale pair, and the relevant content memory/terms context subset. It is
// neokapi's lossless interchange format for a translator or reviewer.

// RunExtractKpz drives the bilingual-interchange extract over a project's
// content × target locales, writing one <slug>.<src>-to-<tgt>.kpz per pair.
func (a *App) RunExtractKpz(cmd Command) error {
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

	noMemory := BoolFlagAny(cmd, "no-memory", "no-tm")
	only, _ := cmd.Flags().GetString("only")
	pattern, _ := cmd.Flags().GetString("pattern")

	targets, err := resolveTargetLocales(cmd, pctx)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("extract: no target locales. Set defaults.target_languages in %s or pass --target-lang", projectPath)
	}

	files, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return fmt.Errorf("extract: resolve content: %w", err)
	}
	files = filterFiles(files, only, pattern, layout.Root)
	if len(files) == 0 {
		return errors.New("extract: no source files matched. Check content patterns / --only / --pattern")
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

	// Content-memory / terms leverage context, both read from the project store.
	// A store that will not open is REPORTED, never dropped: the store is
	// CREATED when it is absent, so a failure here can only mean "a store that
	// exists and cannot be read" (locked, corrupt, an FTS5 tokenizer mismatch
	// between builds, bad permissions). Swallowing it shipped .kpz packages
	// whose recycling context was silently empty — the translator sees no
	// matches and no vocabulary on a project that has both, and nothing explains
	// why. Reported and continued rather than fatal, matching the two siblings on
	// the same call in this file (RunExtract, MergeOneKpz): the packages are
	// still usable, just without leverage.
	var mem memory.ContentMemory
	var tb terms.Terminology
	db, derr := a.ProjectDB(cmd.Context(), layout.Root)
	if derr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: extract: open project store: %v "+
			"(continuing with no leverage: the packages will report zero recycling and no vocabulary)\n", derr)
	}
	if !noMemory && a.MemoryBackend != nil {
		mem = a.MemoryBackend
	} else if !noMemory && db != nil && db.Memory() != nil {
		mem = db.Memory()
	}
	if db != nil && db.Terms() != nil {
		tb = db.Terms()
	}

	written := 0
	for _, tgt := range targets {
		for _, src := range files {
			outName := bilingualOutputName(src, pctx.SourceLocale, tgt, ExtractFormatKPZ)
			outPath := filepath.Join(absOut, outName)
			if err := a.extractOneKpz(cmd.Context(), kpzInterchangeTask{
				ctx: pctx, source: src, targetLocale: tgt, outputPath: outPath, tm: mem, tb: tb,
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

type kpzInterchangeTask struct {
	ctx          *project.ProjectContext
	source       project.ResolvedFile
	targetLocale model.LocaleID
	outputPath   string
	tm           memory.ContentMemory
	tb           terms.Terminology
}

// extractOneKpz assembles a KindInterchange package for one (source, target)
// pair: reads the source blocks (capturing the skeleton), content memory-pre-fills the
// target as a `targets/<locale>` overlay per block, attaches the relevant
// content memory/terms context, and writes the .kpz.
func (a *App) extractOneKpz(ctx context.Context, task kpzInterchangeTask) error {
	srcAbs := task.source.Path
	data, err := os.ReadFile(srcAbs)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	formatID := registry.FormatID(task.source.Format)
	sourceHash := project.HashBytes(data)

	blocks, _, err := project.ReadSourceBlocks(ctx, a.FormatReg, string(formatID), srcAbs, task.ctx.SourceLocale, task.targetLocale,
		mergedFormatConfig(task.ctx.Project, string(formatID), task.source.Item))
	if err != nil {
		return fmt.Errorf("read blocks: %w", err)
	}

	// Capture the round-trip skeleton. A capture FAILURE is not the same thing
	// as "this format has no skeleton": captureSkeletonBytes already reports
	// genuine absence as (nil, nil) — no emitter, or an emitter that wrote
	// nothing — so any error here means the skeleton exists and could not be
	// taken. Shipping the .kpz anyway leaves merge with nothing to write back
	// through, so it re-serializes from the content model and silently loses the
	// source's exact formatting, while the extract reports success. Fail instead;
	// faithful write-back is the contract (the doccache half of #1459, one layer
	// out).
	var skeletons []kpz.SkeletonDoc
	skelMember := ""
	skel, serr := captureSkeletonBytes(ctx, a.FormatReg, formatID, srcAbs, data, task.ctx.SourceLocale)
	if serr != nil {
		return fmt.Errorf("capture round-trip skeleton for %s: %w", task.source.Relative, serr)
	}
	if len(skel) > 0 {
		skelMember = kpz.SkeletonDir + filepath.Base(task.source.Relative)
		skeletons = append(skeletons, kpz.SkeletonDoc{
			Path: skelMember, SourcePath: task.source.Relative, FormatID: task.source.Format,
			ContentHash: sourceHash, Content: kpz.BytesContent(skel),
		})
	}

	// content memory pre-fill the target as a per-block overlay (keyed by block ID, so
	// `kapi merge`'s hydrate step applies it the same way the workspace flow
	// does). Also gather the content-memory entries actually consulted as inline
	// context.
	threshold := float64(task.ctx.Project.Defaults.Memory.ResolvedFuzzyThreshold()) / 100.0
	srcArchive := kpz.SourceDir + filepath.Base(task.source.Relative)
	var overlays []kpz.OverlayDoc
	contextEntries := map[string]memory.Entry{}
	for _, b := range blocks {
		if !b.Translatable || b.ID == "" {
			continue
		}
		if task.tm != nil {
			if matches, merr := task.tm.Lookup(ctx, b, task.ctx.SourceLocale, task.targetLocale, memory.LookupOptions{MinScore: threshold, MaxResults: 1}); merr == nil && len(matches) > 0 && !matches[0].Ambiguous {
				if text := matches[0].Entry.VariantText(task.targetLocale); text != "" {
					// A content memory-leveraged prefill is a draft; carry that status so the
					// workspace → merge round-trip preserves it (the matching read
					// is applyTargetOverlay).
					payload, _ := json.Marshal(map[string]string{
						"text":   text,
						"status": string(model.TargetStatusDraft),
					})
					overlays = append(overlays, kpz.OverlayDoc{
						Kind: "targets/" + string(task.targetLocale), BlockHash: b.ID, Payload: payload, Source: srcArchive,
					})
				}
				contextEntries[matches[0].Entry.ID] = matches[0].Entry
			}
		}
	}

	// content memory context subset (the consulted entries) so the package is
	// self-contained for offline review.
	var memoryFile *kmb.File
	if len(contextEntries) > 0 {
		entries := make([]memory.Entry, 0, len(contextEntries))
		for _, e := range contextEntries {
			entries = append(entries, e)
		}
		memoryFile = kmb.FromModel(entries, nil)
	}

	// Terms context: the whole bound terms (terms are small and the
	// reviewer needs the terms).
	var tbFile *ktb.File
	if task.tb != nil {
		if concepts, cerr := task.tb.Concepts(ctx); cerr == nil && len(concepts) > 0 {
			tbFile = ktb.FromConcepts(concepts)
		}
	}

	recipe := newInterchangeRecipe(string(task.ctx.SourceLocale), string(task.targetLocale))

	pkg := &kpz.Package{
		Kind:      kpz.KindInterchange,
		Generator: &kpz.GeneratorInfo{ID: "kapi", Version: version.Version},
		Recipe:    recipe,
		Skeletons: skeletons,
		Overlays:  overlays,
		Memory:    memoryFile,
		Terms:     tbFile,
		Sources: []kpz.SourceIdentity{{
			SourcePath: task.source.Relative, FormatID: task.source.Format,
			ContentHash: sourceHash, SkeletonPath: skelMember,
		}},
		InterchangeTask: &kpz.InterchangeTask{
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

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/exporter"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/klz"
)

// A .klz is a single-file, serverless localization workspace — the portable
// equivalent of a .kapi project's working state. Three pipeline-stage verbs
// operate on it (AD-025 §5):
//
//   - extract <sources> -o work.klz   reader: ingest source documents
//   - <tool|run> work.klz             transform: run a tool/flow IN PLACE
//   - merge work.klz [-o ...]         writer: emit the localized files
//
// The package carries the sources, the per-source overlays (the work), and a
// small Recipe (target locales + output layout) so config travels with the
// file. This file implements those three operations; the commands route to
// them when a .klz is on the command line.

// isKlzPath reports whether a path names a .klz package.
func isKlzPath(p string) bool {
	return strings.EqualFold(filepath.Ext(p), WorkspaceExt)
}

// klzWorkspaceInput reports whether the inputs are a single .klz workspace —
// the signal to transform it in place rather than run the plain pipeline.
func klzWorkspaceInput(inputs []string) bool {
	return len(inputs) == 1 && isKlzPath(inputs[0])
}

// errKlzTransformOutput explains that a tool/flow on a .klz mutates it in
// place; emitting files is `kapi merge`'s job.
var errKlzTransformOutput = errors.New("running a tool on a .klz updates it in place — use `kapi merge <work.klz> -o <dir>` to write output files")

// errKlzCreateWithExtract explains that a .klz workspace is created with
// `kapi extract`, not by giving a tool/flow a .klz output.
var errKlzCreateWithExtract = errors.New("to create a .klz workspace use `kapi extract <sources> -o work.klz`, then run tools on it")

// toolChainBuilder builds the tool chain (and a cleanup) for a klz transform.
// It abstracts over `run` (a composed flow) and a top-level tool command (a
// single tool).
type toolChainBuilder func() ([]tool.Tool, func(), error)

// ─── extract: ingest sources into a workspace ───────────────────

// extractToKlz writes a fresh workspace package from source files plus a
// recipe (target locales + output layout). No tools run yet — that is what
// the transform verbs are for.
func (a *App) extractToKlz(sources []string, outKlz, targetLang, outLayout string) error {
	files, err := resolveFiles(sources)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("extract: no source files")
	}
	if !isKlzPath(outKlz) {
		outKlz += WorkspaceExt
	}

	recipe := &klz.Recipe{SourceLang: a.SourceLang, Out: outLayout}
	for _, tl := range splitLocales(targetLang) {
		recipe.AddTargetLang(tl)
	}

	pkg := &klz.Package{Generator: &klz.GeneratorInfo{ID: "kapi"}, Recipe: recipe}
	for _, f := range files {
		if isKlzPath(f) {
			return errors.New("extract: source must be a document, not a .klz")
		}
		data, rerr := os.ReadFile(f)
		if rerr != nil {
			return fmt.Errorf("read %q: %w", filepath.Base(f), rerr)
		}
		pkg.Source = append(pkg.Source, klz.SourceDoc{Path: "source/" + filepath.Base(f), Data: data})
	}
	if err := saveWorkspace(pkg, outKlz); err != nil {
		return err
	}
	a.printlnUnlessQuiet(fmt.Sprintf("Extracted %d document(s) → %s", len(pkg.Source), outKlz))
	return nil
}

// ─── transform: run a tool/flow in place ────────────────────────

// transformKlzInPlace runs the tool chain over every source in the workspace
// against per-source warm stores and folds the resulting overlays back into
// the package — mutating it in place. Locales accumulate: each transform adds
// its target locale to the recipe.
func (a *App) transformKlzInPlace(ctx context.Context, klzPath, flowName string, build toolChainBuilder, targetLang, toolDefaultLocale string) error {
	pkg, err := loadWorkspace(klzPath)
	if err != nil {
		return err
	}
	if pkg.Recipe == nil {
		pkg.Recipe = &klz.Recipe{SourceLang: a.SourceLang}
	}
	// Resolve the target locale: explicit flag, else the recipe's first
	// locale, else the tool's default (e.g. pseudo-translate → qps).
	locale := targetLang
	if locale == "" && len(pkg.Recipe.TargetLangs) > 0 {
		locale = pkg.Recipe.TargetLangs[0]
	}
	if locale == "" {
		locale = toolDefaultLocale
	}
	if locale == "" {
		return errors.New("transform: --target-lang is required (none recorded in the workspace)")
	}
	a.TargetLang = locale
	pkg.Recipe.AddTargetLang(locale)

	tools, cleanup, err := build()
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	work, err := os.MkdirTemp("", "kapi-klz-xform-*")
	if err != nil {
		return fmt.Errorf("transform: %w", err)
	}
	defer os.RemoveAll(work)

	var all []klz.OverlayDoc
	for i, src := range pkg.Source {
		prior := overlaysForSource(pkg.Overlays, src.Path)
		out, err := a.runKlzSource(ctx, work, i, flowName, tools, src, prior, filepath.Join(work, "discard-"+filepath.Base(src.Path)), locale)
		if err != nil {
			return err
		}
		all = append(all, out...)
	}
	pkg.Overlays = all

	if err := saveWorkspace(pkg, klzPath); err != nil {
		return err
	}
	a.printlnUnlessQuiet(fmt.Sprintf("Updated %s (%d document(s), locales: %s)", klzPath, len(pkg.Source), strings.Join(pkg.Recipe.TargetLangs, ", ")))
	return nil
}

// ─── merge: emit the localized files ────────────────────────────

// mergeFromKlz writes the finished documents from a workspace: for each
// source × target locale it hydrates the stored target overlays onto the
// source and writes the localized file. Output layout comes from -o, else
// the recipe's Out, else a default per-locale layout.
func (a *App) mergeFromKlz(ctx context.Context, klzPath, outOverride string) error {
	pkg, err := loadWorkspace(klzPath)
	if err != nil {
		return err
	}
	locales := mergeLocales(pkg)
	if len(locales) == 0 {
		return errors.New("merge: workspace has no translated locales yet — run a tool on it first")
	}
	layout := outOverride
	if layout == "" && pkg.Recipe != nil {
		layout = pkg.Recipe.Out
	}
	if pkg.Recipe != nil && pkg.Recipe.SourceLang != "" {
		a.SourceLang = pkg.Recipe.SourceLang
	}

	work, err := os.MkdirTemp("", "kapi-klz-merge-*")
	if err != nil {
		return fmt.Errorf("merge: %w", err)
	}
	defer os.RemoveAll(work)

	written := 0
	for i, src := range pkg.Source {
		prior := overlaysForSource(pkg.Overlays, src.Path)
		for _, locale := range locales {
			docOut := mergeOutputPath(layout, src.Path, locale, len(locales) > 1)
			tools := []tool.Tool{newHydrateTargetsTool(model.LocaleID(locale))}
			if _, err := a.runKlzSource(ctx, work, i, "merge", tools, src, prior, docOut, locale); err != nil {
				return err
			}
			written++
		}
	}
	a.printlnUnlessQuiet(fmt.Sprintf("Merged %s → %d file(s) (%s)", klzPath, written, strings.Join(locales, ", ")))
	return nil
}

// ─── shared per-source runner ───────────────────────────────────

// runKlzSource runs one source through the tool chain against a fresh store
// warmed with its prior overlays, writing the document to docOut. It returns
// the store's overlays tagged with the source path (the full set: prior +
// anything the tools added).
func (a *App) runKlzSource(ctx context.Context, work string, idx int, flowName string, tools []tool.Tool, src klz.SourceDoc, prior []klz.OverlayDoc, docOut, targetLang string) ([]klz.OverlayDoc, error) {
	store, err := blockstore.NewCacheStore(filepath.Join(work, fmt.Sprintf("blocks-%d.db", idx)))
	if err != nil {
		return nil, fmt.Errorf("klz: open store: %w", err)
	}
	defer store.Close()
	if len(prior) > 0 {
		if err := exporter.LoadOverlays(ctx, store, klzToStoreOverlays(prior)); err != nil {
			return nil, fmt.Errorf("klz: warm store: %w", err)
		}
	}

	srcPath := filepath.Join(work, fmt.Sprintf("src-%d-%s", idx, filepath.Base(src.Path)))
	if err := os.WriteFile(srcPath, src.Data, 0o644); err != nil {
		return nil, fmt.Errorf("materialize source: %w", err)
	}

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:       a.FormatReg,
		SourceLocale:    model.LocaleID(a.SourceLang),
		Encoding:        a.Encoding,
		Store:           store,
		DetectFormat:    a.klzDetectFormat(src.Path),
		ConfigureReader: a.klzConfigureReader(),
	})
	if err := runner.RunFile(ctx, flowName, tools, srcPath, docOut, targetLang); err != nil {
		return nil, err
	}

	snap, err := exporter.Export(ctx, store)
	if err != nil {
		return nil, fmt.Errorf("klz: export overlays: %w", err)
	}
	out := storeToKlzOverlays(snap.Overlays)
	for i := range out {
		out[i].Source = src.Path
	}
	return out, nil
}

// ─── helpers ────────────────────────────────────────────────────

// overlaysForSource returns the overlays tagged for a given source path.
func overlaysForSource(overlays []klz.OverlayDoc, sourcePath string) []klz.OverlayDoc {
	var out []klz.OverlayDoc
	for _, o := range overlays {
		if o.Source == sourcePath {
			out = append(out, o)
		}
	}
	return out
}

// mergeLocales returns the locales to emit: the recipe's, else the distinct
// locales found across the overlays' "targets/<locale>" kinds.
func mergeLocales(pkg *klz.Package) []string {
	if pkg.Recipe != nil && len(pkg.Recipe.TargetLangs) > 0 {
		return pkg.Recipe.TargetLangs
	}
	seen := map[string]bool{}
	var out []string
	for _, o := range pkg.Overlays {
		if l, ok := strings.CutPrefix(o.Kind, "targets/"); ok && !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	return out
}

// mergeOutputPath computes one source × locale output path. A template
// ({name} {lang} {ext} {dir}) is expanded; a bare directory receives
// <dir>/[<lang>/]<name>; the default is ./<lang>/<name>.
func mergeOutputPath(layout, sourcePath, locale string, multiLocale bool) string {
	base := filepath.Base(sourcePath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	extNoDot := strings.TrimPrefix(ext, ".")

	if strings.Contains(layout, "{") {
		out := expandOutputTemplate(layout, name, locale, extNoDot, ".")
		if d := filepath.Dir(out); d != "." {
			_ = os.MkdirAll(d, 0o755)
		}
		return out
	}
	if layout != "" {
		if multiLocale {
			return filepath.Join(layout, locale, base)
		}
		return filepath.Join(layout, base)
	}
	return filepath.Join(locale, base)
}

// splitLocales splits a comma-separated locale flag into a trimmed list.
func splitLocales(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// klzDetectFormat returns a flag/extension-aware format detector. The path
// argument is the source member path, used for extension detection of the
// embedded document.
func (a *App) klzDetectFormat(sourcePath string) func(string) registry.FormatID {
	if a.FormatFlag != "" {
		name := preset.ParseFormatRef(a.FormatFlag).RegistryName()
		return func(string) registry.FormatID { return registry.FormatID(name) }
	}
	if det, err := a.FormatReg.DetectByExtension(filepath.Ext(sourcePath)); err == nil && det != "" {
		return func(string) registry.FormatID { return det }
	}
	return nil
}

// klzConfigureReader applies preset config to each reader the workspace
// runner opens.
func (a *App) klzConfigureReader() func(format.DataFormatReader, registry.FormatID) error {
	return func(reader format.DataFormatReader, _ registry.FormatID) error {
		if a.FormatFlag == "" {
			return nil
		}
		ref := preset.ParseFormatRef(a.FormatFlag)
		if !ref.IsPreset() {
			return nil
		}
		presetReg := preset.NewPresetRegistry()
		preset.RegisterBuiltins(presetReg)
		resolver := preset.NewConfigResolver(presetReg, a.SchemaReg)
		mergedConfig, err := resolver.ResolveFormatConfig(ref.Name, ref.Preset, nil, nil)
		if err != nil {
			return fmt.Errorf("resolve format config: %w", err)
		}
		if cfg := reader.Config(); cfg != nil && len(mergedConfig) > 0 {
			if err := cfg.ApplyMap(mergedConfig); err != nil {
				return fmt.Errorf("apply format config: %w", err)
			}
		}
		return nil
	}
}

// toolDefaultLocale returns a tool's declared default target locale (e.g.
// pseudo-translate → "qps"), or "" when none.
func (a *App) toolDefaultLocale(toolName string) string {
	if a.ToolReg == nil {
		return ""
	}
	if info := a.ToolReg.GetToolInfo(registry.ToolID(toolName)); info != nil {
		return string(info.DefaultLocale)
	}
	return ""
}

func (a *App) printlnUnlessQuiet(msg string) {
	if !a.Quiet {
		fmt.Fprintln(os.Stdout, msg)
	}
}

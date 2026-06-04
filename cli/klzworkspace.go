package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/klz"
	"github.com/spf13/cobra"
)

// A .klz is a single-file, serverless localization workspace — the portable
// twin of a .kapi project's working state. It is operated on through three
// pipeline-stage verbs against a persistent shadow cache (see klzcache.go):
//
//   - extract <sources> -o work.klz   reader: ingest sources (+ a recipe)
//   - <tool|run> work.klz             transform: run a tool/flow IN PLACE
//   - merge work.klz [-o ...]         writer: emit the localized files
//
// Transforms touch only the cache (fast, incremental); the .klz is rewritten
// by `kapi pack` (or a transform's --pack), the git-bundle eject. `kapi info`
// shows whether the cache is dirty (diverged from the packed .klz).

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
type toolChainBuilder func() ([]tool.Tool, func(), error)

// ─── extract: ingest sources into a workspace ───────────────────

// extractToKlz writes a fresh .klz from source files plus a recipe (target
// locales + output layout) and builds its working cache. No tools run yet.
func (a *App) extractToKlz(ctx context.Context, sources []string, outKlz, targetLang, outLayout string) error {
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
	if _, err := buildKlzCache(ctx, outKlz); err != nil {
		return err
	}
	a.printlnUnlessQuiet(fmt.Sprintf("Extracted %d document(s) → %s", len(pkg.Source), outKlz))
	return nil
}

// ─── transform: run a tool/flow in place ────────────────────────

// transformKlzInPlace runs the tool chain over every source in the workspace
// against the cache's persistent per-source block stores — incrementally,
// without rewriting the .klz. Locales accumulate in the recipe. With doPack
// it also ejects the result to the .klz (--pack); otherwise the cache is left
// dirty for an explicit `kapi pack`.
func (a *App) transformKlzInPlace(ctx context.Context, klzPath, flowName string, build toolChainBuilder, targetLang, toolDefaultLocale string, doPack bool) error {
	c, err := a.ensureKlzCache(ctx, klzPath)
	if err != nil {
		return err
	}
	if c.meta.Recipe == nil {
		c.meta.Recipe = &klz.Recipe{SourceLang: a.SourceLang}
	}
	locale := targetLang
	if locale == "" && len(c.meta.Recipe.TargetLangs) > 0 {
		locale = c.meta.Recipe.TargetLangs[0]
	}
	if locale == "" {
		locale = toolDefaultLocale
	}
	if locale == "" {
		return errors.New("transform: --target-lang is required (none recorded in the workspace)")
	}
	a.TargetLang = locale
	c.meta.Recipe.AddTargetLang(locale)
	if c.meta.Recipe.SourceLang != "" {
		a.SourceLang = c.meta.Recipe.SourceLang
	}

	tools, cleanup, err := build()
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Document output during a transform is a throwaway (the work that
	// persists is the overlays the SessionTools cache). Write it under the
	// cache dir rather than the OS temp dir — the latter doesn't exist in the
	// wasm sandbox.
	discard := filepath.Join(c.dir, "discard")
	if err := os.MkdirAll(discard, 0o755); err != nil {
		return fmt.Errorf("transform: %w", err)
	}
	defer os.RemoveAll(discard)

	for _, src := range c.meta.Sources {
		if err := a.runCacheSource(ctx, c, src, flowName, tools, filepath.Join(discard, src.Name), locale); err != nil {
			return err
		}
	}
	if err := c.save(); err != nil {
		return err
	}

	if doPack {
		if err := c.pack(ctx); err != nil {
			return err
		}
		a.printlnUnlessQuiet(fmt.Sprintf("Updated and packed %s (%d document(s), locales: %s)", klzPath, len(c.meta.Sources), strings.Join(c.meta.Recipe.TargetLangs, ", ")))
		return nil
	}
	a.printlnUnlessQuiet(fmt.Sprintf("Updated %s [dirty] (%d document(s), locales: %s) — run `kapi pack %s` to share", klzPath, len(c.meta.Sources), strings.Join(c.meta.Recipe.TargetLangs, ", "), filepath.Base(klzPath)))
	return nil
}

// ─── merge: emit the localized files ────────────────────────────

// mergeFromKlz writes the finished documents from the workspace cache: for
// each source × target locale it hydrates the stored target overlays and
// writes the localized file. Layout comes from -o, else the recipe's Out,
// else a default per-locale layout. Reads the cache (freshest state); does
// not require a pack.
func (a *App) mergeFromKlz(ctx context.Context, klzPath, outOverride string) error {
	c, err := a.ensureKlzCache(ctx, klzPath)
	if err != nil {
		return err
	}
	var locales []string
	if c.meta.Recipe != nil {
		locales = c.meta.Recipe.TargetLangs
	}
	if len(locales) == 0 {
		return errors.New("merge: workspace has no translated locales yet — run a tool on it first")
	}
	layout := outOverride
	if layout == "" && c.meta.Recipe != nil {
		layout = c.meta.Recipe.Out
	}
	if c.meta.Recipe != nil && c.meta.Recipe.SourceLang != "" {
		a.SourceLang = c.meta.Recipe.SourceLang
	}

	written := 0
	for _, src := range c.meta.Sources {
		for _, locale := range locales {
			docOut := mergeOutputPath(layout, src.Path, locale, len(locales) > 1)
			tools := []tool.Tool{newHydrateTargetsTool(model.LocaleID(locale))}
			if err := a.runCacheSource(ctx, c, src, "merge", tools, docOut, locale); err != nil {
				return err
			}
			written++
		}
	}
	a.printlnUnlessQuiet(fmt.Sprintf("Merged %s → %d file(s) (%s)", klzPath, written, strings.Join(locales, ", ")))
	return nil
}

// ─── info: show workspace status (dirty?) ───────────────────────

// WorkspaceInfo is the structured state of a .klz workspace: its sources,
// recipe, per-locale overlay coverage, and dirty flag. Emitted as text or,
// with --json, as JSON — the latter drives the docs lab's inspection panel.
type WorkspaceInfo struct {
	Workspace   string         `json:"workspace"`
	SourceLang  string         `json:"sourceLang,omitempty"`
	TargetLangs []string       `json:"targetLangs,omitempty"`
	Out         string         `json:"out,omitempty"`
	Documents   []string       `json:"documents"`
	Overlays    map[string]int `json:"overlays"` // locale → translated-block count
	Dirty       bool           `json:"dirty"`
}

// FormatText renders the workspace info for humans.
func (o WorkspaceInfo) FormatText(w io.Writer) error {
	state := "clean (packed)"
	if o.Dirty {
		state = "dirty — run `kapi pack " + filepath.Base(o.Workspace) + "` to update the .klz"
	}
	fmt.Fprintf(w, "%s\n  documents: %d (%s)\n  locales:   %s\n  output:    %s\n",
		o.Workspace, len(o.Documents), strings.Join(o.Documents, ", "),
		strings.Join(o.TargetLangs, ", "), o.Out)
	if len(o.Overlays) > 0 {
		for _, l := range o.TargetLangs {
			fmt.Fprintf(w, "  translated[%s]: %d\n", l, o.Overlays[l])
		}
	}
	fmt.Fprintf(w, "  state:     %s\n", state)
	return nil
}

// infoKlz prints the workspace's state (text, or JSON with --json). Named
// `info` rather than `status` (which the bowrain plugin owns).
func (a *App) infoKlz(cmd *cobra.Command, klzPath string) error {
	ctx := cmd.Context()
	c, err := a.ensureKlzCache(ctx, klzPath)
	if err != nil {
		return err
	}
	dirty, err := c.dirty(ctx)
	if err != nil {
		return err
	}
	info := WorkspaceInfo{Workspace: klzPath, Dirty: dirty, Overlays: map[string]int{}}
	for _, s := range c.meta.Sources {
		info.Documents = append(info.Documents, s.Name)
	}
	if c.meta.Recipe != nil {
		info.SourceLang = c.meta.Recipe.SourceLang
		info.TargetLangs = c.meta.Recipe.TargetLangs
		info.Out = c.meta.Recipe.Out
	}
	// Per-locale translated-block counts (overlays of kind targets/<locale>).
	pkg, err := c.toPackage(ctx)
	if err != nil {
		return err
	}
	for _, ov := range pkg.Overlays {
		if l, ok := strings.CutPrefix(ov.Kind, "targets/"); ok {
			info.Overlays[l]++
		}
	}
	return output.Print(cmd, info)
}

// packKlz ejects a workspace cache into its .klz (the explicit hand-off
// boundary).
func (a *App) packKlz(ctx context.Context, klzPath string) error {
	c, err := a.ensureKlzCache(ctx, klzPath)
	if err != nil {
		return err
	}
	if err := c.pack(ctx); err != nil {
		return err
	}
	a.printlnUnlessQuiet("Packed " + klzPath)
	return nil
}

// unpackKlz rebuilds a workspace cache from its .klz, discarding any unpacked
// work in the existing cache.
func (a *App) unpackKlz(ctx context.Context, klzPath string) error {
	if _, err := buildKlzCache(ctx, klzPath); err != nil {
		return err
	}
	a.printlnUnlessQuiet(fmt.Sprintf("Unpacked %s into its working cache", klzPath))
	return nil
}

// ─── shared per-source runner ───────────────────────────────────

// runCacheSource runs one source through the tool chain against its
// persistent cache store, writing the document to docOut. The store is
// persistent, so SessionTools read prior overlays (skip recompute) and write
// new ones directly — no per-invocation load/export.
func (a *App) runCacheSource(ctx context.Context, c *klzCache, src klzCacheSource, flowName string, tools []tool.Tool, docOut, targetLang string) error {
	store, err := blockstore.NewCacheStore(c.storePath(src.Key))
	if err != nil {
		return fmt.Errorf("klz: open store: %w", err)
	}
	defer store.Close()
	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:       a.FormatReg,
		SourceLocale:    model.LocaleID(a.SourceLang),
		Encoding:        a.Encoding,
		Store:           store,
		DetectFormat:    a.klzDetectFormat(src.Path),
		ConfigureReader: a.klzConfigureReader(),
	})
	return runner.RunFile(ctx, flowName, tools, c.sourcePath(src.Name), docOut, targetLang)
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

// klzDetectFormat returns a flag/extension-aware format detector for the
// embedded source document.
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

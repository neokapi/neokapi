package host

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/kpz"
)

// A .kpz is a single-file, serverless localization workspace — the portable
// twin of a .kapi project's working state. It is operated on through three
// pipeline-stage verbs against a persistent shadow cache (see kpzcache.go):
//
//   - extract <sources> -o work.kpz   reader: ingest sources (+ a recipe)
//   - <tool|run> work.kpz             transform: run a tool/flow IN PLACE
//   - merge work.kpz [-o ...]         writer: emit the localized files
//
// Transforms touch only the cache (fast, incremental); the .kpz is rewritten
// by `kapi pack` (or a transform's --pack), the git-bundle eject. `kapi info`
// shows whether the cache is dirty (diverged from the packed .kpz).

// IsKpzPath reports whether a bare path names a .kpz package. Detection routes
// through the binding resolver (AD-026): a bare path with a .kpz extension binds
// to the block store. Scheme-prefixed locators (store:/kpz:) are handled by the
// binding layer, not here, so this stays equivalent to a plain extension check.
func IsKpzPath(p string) bool {
	l := flow.ParseLocator(p)
	return l.Scheme == "" && l.Kind() == flow.BindingStore
}

// kpzWorkspaceInput reports whether the inputs are a single .kpz workspace —
// the signal to transform it in place rather than run the plain pipeline.
func kpzWorkspaceInput(inputs []string) bool {
	return len(inputs) == 1 && IsKpzPath(inputs[0])
}

// errKpzTransformOutput explains that a tool/flow on a .kpz mutates it in
// place; emitting files is `kapi merge`'s job.
var errKpzTransformOutput = errors.New("running a tool on a .kpz updates it in place. Use `kapi merge <work.kpz> -o <dir>` to write output files")

// errKpzCreateWithExtract explains that a .kpz workspace is created with
// `kapi extract`, not by giving a tool/flow a .kpz output.
var errKpzCreateWithExtract = errors.New("to create a .kpz workspace use `kapi extract <sources> -o work.kpz`, then run tools on it")

// toolChainBuilder builds the tool chain (and a cleanup) for a kpz transform.
type toolChainBuilder func() ([]tool.Tool, func(), error)

// ─── extract: ingest sources into a workspace ───────────────────

// ExtractToKpz writes a fresh .kpz from source files plus the full project
// recipe (synthesized from source/target locales + output layout) and builds
// its working cache. No tools run yet.
//
// Source retention follows AD-025 §6: each source's identity (path, format,
// content hash) and its round-trip skeleton always travel; the raw source
// bytes ride in the .kpz only with withSource. The working cache always holds
// the raw source locally (it is the runtime), so the extract → transform →
// merge loop works regardless of the packed retention.
func (a *App) ExtractToKpz(ctx context.Context, sources []string, outKpz, targetLang, outLayout string, withSource bool) error {
	files, err := resolveFiles(sources)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("extract: no source files")
	}
	if !IsKpzPath(outKpz) {
		outKpz += workspaceExt
	}
	// The layout is recorded IN the package, so it has to mean the same thing
	// wherever the package is merged. Say so now rather than at merge time in
	// someone else's checkout.
	if !kpz.IsLocalOutTemplate(outLayout) {
		return fmt.Errorf("extract: --out %q must be relative to the merge directory (e.g. 'l10n/{lang}/{name}.{ext}'). A workspace records this layout for whoever merges it; pass an absolute destination to `kapi merge -o` instead", outLayout)
	}

	recipe := newWorkspaceRecipe(a.SourceLocale(), splitLocales(targetLang), outLayout)

	// Build a full in-memory package (raw source always present) so the cache
	// holds it. The packed .kpz then drops raw source unless withSource.
	full := &kpz.Package{Generator: &kpz.GeneratorInfo{ID: "kapi"}, Recipe: recipe}
	for _, f := range files {
		if IsKpzPath(f) {
			return errors.New("extract: source must be a document, not a .kpz")
		}
		data, rerr := os.ReadFile(f)
		if rerr != nil {
			return fmt.Errorf("read %q: %w", filepath.Base(f), rerr)
		}
		base := filepath.Base(f)
		archivePath := kpz.SourceDir + base
		formatID := ""
		if det := a.kpzDetectFormat(f); det != nil {
			formatID = string(det(f))
		}
		si := kpz.SourceIdentity{
			SourcePath:   base,
			FormatID:     formatID,
			ContentHash:  project.HashBytes(data),
			HasRawSource: withSource,
		}
		// Capture the round-trip skeleton (default source-retention). As in
		// extractOneKpz: genuine absence is (nil, nil), so an error means the
		// skeleton exists and could not be taken — a workspace built without it
		// writes back re-serialized, lower-fidelity files while reporting
		// success. Fail rather than degrade silently.
		if formatID != "" {
			skel, serr := captureSkeletonBytes(ctx, a.FormatReg, registry.FormatID(formatID), f, data, model.LocaleID(a.SourceLocale()))
			if serr != nil {
				return fmt.Errorf("capture round-trip skeleton for %s: %w", base, serr)
			}
			if len(skel) > 0 {
				skelPath := kpz.SkeletonDir + base
				full.Skeletons = append(full.Skeletons, kpz.SkeletonDoc{
					Path: skelPath, SourcePath: base, FormatID: formatID,
					ContentHash: si.ContentHash, Content: kpz.BytesContent(skel),
				})
				si.SkeletonPath = skelPath
			}
		}
		// Stream the original source file into the parcel rather than retaining
		// the bytes (already read above only to hash + capture the skeleton).
		full.Source = append(full.Source, kpz.SourceDoc{Path: archivePath, Content: kpz.FileContent(f)})
		full.Sources = append(full.Sources, si)
	}

	// Build the working cache from the full package (raw source local).
	if _, err := buildKpzCacheFromPackage(ctx, outKpz, full); err != nil {
		return err
	}

	// Write the packed .kpz: identical content, but drop the raw source members
	// unless withSource. The cache is then re-synced to the packed rootHash.
	packed := *full
	if !withSource {
		packed.Source = nil
	}
	if err := saveWorkspace(&packed, outKpz); err != nil {
		return err
	}
	// Re-open the cache and re-sync its PackedRootHash to the saved file so a
	// fresh extract is "clean", not spuriously dirty.
	if c, ok, oerr := openKpzCache(outKpz); oerr == nil && ok {
		if root, rerr := packed.RootHash(); rerr == nil {
			c.meta.PackedRootHash = root
			_ = c.save()
		}
	}

	a.printlnUnlessQuiet(fmt.Sprintf("Extracted %d document(s) → %s", len(full.Sources), outKpz))
	return nil
}

// captureSkeletonBytes reads a source document through its format reader with
// a SkeletonStore wired, returning the serialized skeleton stream (the
// round-trip template merge reuses; AD-025 §6). Returns (nil, nil) when the
// reader does not emit a skeleton (binary formats, etc.) — callers treat that
// as "no skeleton, re-read source at merge time". The data argument is the raw
// source bytes (so callers that already read the file don't read it twice).
func captureSkeletonBytes(ctx context.Context, reg *registry.FormatRegistry, formatID registry.FormatID, path string, data []byte, srcLocale model.LocaleID) ([]byte, error) {
	reader, err := reg.NewReader(formatID)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	emitter, ok := reader.(format.SkeletonStoreEmitter)
	if !ok {
		return nil, nil // format has no skeleton emitter
	}
	store, err := format.NewSkeletonStore()
	if err != nil {
		return nil, err
	}
	defer store.Close()
	emitter.SetSkeletonStore(store)

	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: srcLocale,
		FormatID:     string(formatID),
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		return nil, err
	}
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			return nil, res.Error
		}
	}
	if store.EntriesWritten() == 0 {
		return nil, nil // reader registered but emitted nothing
	}
	return store.Bytes()
}

// ─── transform: run a tool/flow in place ────────────────────────

// transformKpzInPlace runs the tool chain over every source in the workspace
// against the cache's persistent per-source block stores — incrementally,
// without rewriting the .kpz. Locales accumulate in the recipe. With doPack
// it also ejects the result to the .kpz (--pack); otherwise the cache is left
// dirty for an explicit `kapi pack`.
func (a *App) transformKpzInPlace(ctx context.Context, kpzPath, flowName string, build toolChainBuilder, targetLang, toolDefaultLocale string, doPack bool) error {
	c, err := a.ensureKpzCache(ctx, kpzPath)
	if err != nil {
		return err
	}
	if c.meta.Recipe == nil {
		c.meta.Recipe = newWorkspaceRecipe(a.SourceLocale(), nil, "")
	}
	targets := recipeTargetLangs(c.meta.Recipe)
	locale := targetLang
	if locale == "" && len(targets) > 0 {
		locale = targets[0]
	}
	if locale == "" {
		locale = toolDefaultLocale
	}
	if locale == "" {
		return errors.New("transform: --target-lang is required (none recorded in the workspace)")
	}
	a.TargetLang = locale
	recipeAddTargetLang(c.meta.Recipe, locale)
	if sl := recipeSourceLang(c.meta.Recipe); sl != "" {
		a.SourceLang = sl
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
		a.printlnUnlessQuiet(fmt.Sprintf("Updated and packed %s (%d document(s), locales: %s)", kpzPath, len(c.meta.Sources), strings.Join(recipeTargetLangs(c.meta.Recipe), ", ")))
		return nil
	}
	a.printlnUnlessQuiet(fmt.Sprintf("Updated %s [dirty] (%d document(s), locales: %s). Run `kapi pack %s` to share", kpzPath, len(c.meta.Sources), strings.Join(recipeTargetLangs(c.meta.Recipe), ", "), filepath.Base(kpzPath)))
	return nil
}

// ─── merge: emit the localized files ────────────────────────────

// MergeFromKpz writes the finished documents from the workspace cache: for
// each source × target locale it hydrates the stored target overlays and
// writes the localized file. Layout comes from -o, else the recipe's Out,
// else a default per-locale layout. Reads the cache (freshest state); does
// not require a pack.
func (a *App) MergeFromKpz(ctx context.Context, kpzPath, outOverride string) error {
	c, err := a.ensureKpzCache(ctx, kpzPath)
	if err != nil {
		return err
	}
	locales := recipeTargetLangs(c.meta.Recipe)
	if len(locales) == 0 {
		return errors.New("merge: workspace has no translated locales yet. Run a tool on it first")
	}
	// -o is the user speaking, and an absolute destination there is legitimate.
	// The recipe's own layout is the PACKAGE speaking, and a package describes
	// where its output goes relative to wherever it is merged — never an
	// absolute or climbing destination. LoadWorkspace already drops a non-local
	// layout on ingest; this is the check at the point of use, so a cache built
	// by an older build cannot slip one through.
	layout := outOverride
	if layout == "" {
		layout = recipeOut(c.meta.Recipe)
		if !kpz.IsLocalOutTemplate(layout) {
			return fmt.Errorf("merge: the workspace's recorded output layout %q names a destination outside the merge directory. Pass `-o <dir>` to choose one deliberately", layout)
		}
	}
	if sl := recipeSourceLang(c.meta.Recipe); sl != "" {
		a.SourceLang = sl
	}

	written := 0
	for _, src := range c.meta.Sources {
		for _, locale := range locales {
			docOut, derr := mergeOutputPath(layout, src.Path, locale, len(locales) > 1)
			if derr != nil {
				return derr
			}
			tools := []tool.Tool{newHydrateTargetsTool(model.LocaleID(locale))}
			if err := a.runCacheSource(ctx, c, src, "merge", tools, docOut, locale); err != nil {
				return err
			}
			written++
		}
	}
	a.printlnUnlessQuiet(fmt.Sprintf("Merged %s → %d file(s) (%s)", kpzPath, written, strings.Join(locales, ", ")))
	return nil
}

// ─── info: show workspace status (dirty?) ───────────────────────

// workspaceInfo is the structured state of a .kpz workspace: its sources,
// recipe, per-locale overlay coverage, and dirty flag. Emitted as text or,
// with --json, as JSON — the latter drives the docs lab's inspection panel.
type workspaceInfo struct {
	Workspace   string         `json:"workspace"`
	SourceLang  string         `json:"sourceLang,omitempty"`
	TargetLangs []string       `json:"targetLangs,omitempty"`
	Out         string         `json:"out,omitempty"`
	Documents   []string       `json:"documents"`
	Overlays    map[string]int `json:"overlays"` // locale → translated-block count
	Dirty       bool           `json:"dirty"`
}

// FormatText renders the workspace info for humans.
func (o workspaceInfo) FormatText(w io.Writer) error {
	state := "clean (packed)"
	if o.Dirty {
		state = "dirty: run `kapi pack " + filepath.Base(o.Workspace) + "` to update the .kpz"
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

// InfoKpz prints a `.kpz`'s state (text, or JSON with --json). Named `info`
// rather than `status` (which the bowrain plugin owns).
//
// Two archive profiles, two answers. A **workspace** `.kpz` is a live thing with
// a shadow cache, so the question worth asking is whether that cache holds work
// not yet packed back — which is what this reports. A **project snapshot** has no
// cache and no such state; asking it produces one anyway (building a cache the
// snapshot never had, then calling it "dirty" moments after `pack` wrote it).
// A snapshot is answered by [App.RunArchiveInfo] instead: the same parts, in the
// same words, as `kapi info` on the project it came from.
func (a *App) InfoKpz(cmd Command, kpzPath string) error {
	ctx := cmd.Context()
	pkg, err := LoadWorkspace(kpzPath)
	if err != nil {
		return err
	}
	if !isWorkspacePackage(pkg) {
		return a.RunArchiveInfo(cmd, kpzPath, pkg)
	}
	c, err := a.ensureKpzCache(ctx, kpzPath)
	if err != nil {
		return err
	}
	dirty, err := c.dirty(ctx)
	if err != nil {
		return err
	}
	info := workspaceInfo{Workspace: kpzPath, Dirty: dirty, Overlays: map[string]int{}}
	for _, s := range c.meta.Sources {
		info.Documents = append(info.Documents, s.Name)
	}
	if c.meta.Recipe != nil {
		info.SourceLang = recipeSourceLang(c.meta.Recipe)
		info.TargetLangs = recipeTargetLangs(c.meta.Recipe)
		info.Out = recipeOut(c.meta.Recipe)
	}
	// Per-locale translated-block counts (overlays of kind targets/<locale>),
	// read from the *cache* rather than the on-disk archive — the point of a
	// workspace report is the work the cache holds.
	cached, err := c.toPackage(ctx)
	if err != nil {
		return err
	}
	for _, ov := range cached.Overlays {
		if l, ok := strings.CutPrefix(ov.Kind, "targets/"); ok {
			info.Overlays[l]++
		}
	}
	return output.Print(cmd, info)
}

// PackKpz ejects a workspace cache into its .kpz (the explicit hand-off
// boundary).
func (a *App) PackKpz(ctx context.Context, kpzPath string) error {
	c, err := a.ensureKpzCache(ctx, kpzPath)
	if err != nil {
		return err
	}
	if err := c.pack(ctx); err != nil {
		return err
	}
	a.printlnUnlessQuiet("Packed " + kpzPath)
	return nil
}

// unpackKpz rebuilds a workspace cache from its .kpz, discarding any unpacked
// work in the existing cache.
func (a *App) unpackKpz(ctx context.Context, kpzPath string) error {
	if _, err := buildKpzCache(ctx, kpzPath); err != nil {
		return err
	}
	a.printlnUnlessQuiet(fmt.Sprintf("Unpacked %s into its working cache", kpzPath))
	return nil
}

// ─── shared per-source runner ───────────────────────────────────

// runCacheSource runs one source through the tool chain against its
// persistent cache store, writing the document to docOut. The store is
// persistent, so SessionTools read prior overlays (skip recompute) and write
// new ones directly — no per-invocation load/export.
func (a *App) runCacheSource(ctx context.Context, c *kpzCache, src kpzCacheSource, flowName string, tools []tool.Tool, docOut, targetLang string) error {
	store, err := sqlitestore.New(c.storePath(src.Key))
	if err != nil {
		return fmt.Errorf("kpz: open store: %w", err)
	}
	defer store.Close()
	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:       a.FormatReg,
		SourceLocale:    model.LocaleID(a.SourceLocale()),
		Encoding:        a.InputEncoding(),
		Store:           store,
		DetectFormat:    a.kpzDetectFormat(src.Path),
		ConfigureReader: a.kpzConfigureReader(),
	})

	// When the cache holds the raw source bytes (the runtime default, or a
	// .kpz packed --with-source), run the normal read → process → write
	// pipeline over them.
	if c.hasSource(src) {
		return runner.RunFile(ctx, flowName, tools, c.sourcePath(src.Name), docOut, targetLang)
	}

	// Skeleton-only handoff (AD-025 §6): the shipped .kpz dropped the raw
	// source, so reconstruct the document from its round-trip skeleton +
	// cached overlays instead of reading a source file that isn't there.
	if src.Skeleton == "" {
		return fmt.Errorf("kpz: source %q has neither raw bytes nor a skeleton to reconstruct from", src.Name)
	}
	skelBytes, err := os.ReadFile(c.skeletonPath(src.Skeleton))
	if err != nil {
		return fmt.Errorf("kpz: read skeleton: %w", err)
	}
	formatID := registry.FormatID(src.FormatID)
	if formatID == "" {
		if det := a.kpzDetectFormat(src.Path); det != nil {
			formatID = det(src.Path)
		}
	}
	if formatID == "" {
		return fmt.Errorf("kpz: cannot determine format for %q to reconstruct from its skeleton", src.Name)
	}
	return runner.RunSkeletonReconstruct(ctx, flowName, tools, formatID, skelBytes, docOut, targetLang)
}

// ─── helpers ────────────────────────────────────────────────────

// overlaysForSource returns the overlays tagged for a given source path.
func overlaysForSource(overlays []kpz.OverlayDoc, sourcePath string) []kpz.OverlayDoc {
	var out []kpz.OverlayDoc
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
//
// The locale is a package-supplied value that becomes a path segment in every
// branch, so it is checked before it can redirect the write — the layout may be
// as absolute as the user asked for, but the locale is an identifier.
func mergeOutputPath(layout, sourcePath, locale string, multiLocale bool) (string, error) {
	if err := checkPathSegment(locale, "merge: target locale"); err != nil {
		return "", err
	}
	base := filepath.Base(sourcePath)
	ext := format.Ext(base)
	name := strings.TrimSuffix(base, ext)
	extNoDot := strings.TrimPrefix(ext, ".")

	if strings.Contains(layout, "{") {
		out := expandOutputTemplate(layout, name, locale, extNoDot, ".")
		if d := filepath.Dir(out); d != "." {
			_ = os.MkdirAll(d, 0o755)
		}
		return out, nil
	}
	if layout != "" {
		if multiLocale {
			return filepath.Join(layout, locale, base), nil
		}
		return filepath.Join(layout, base), nil
	}
	return filepath.Join(locale, base), nil
}

// splitLocales splits a comma-separated locale flag into a trimmed list.
func splitLocales(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// kpzDetectFormat returns a flag/extension-aware format detector for the
// embedded source document.
func (a *App) kpzDetectFormat(sourcePath string) func(string) registry.FormatID {
	if a.FormatFlag != "" {
		name := preset.ParseFormatRef(a.FormatFlag).RegistryName()
		return func(string) registry.FormatID { return registry.FormatID(name) }
	}
	if det, err := a.FormatReg.Detect(sourcePath, registry.DetectOptions{ExtensionOnly: true}); err == nil && det != "" {
		return func(string) registry.FormatID { return det }
	}
	return nil
}

// kpzConfigureReader applies preset config to each reader the workspace
// runner opens.
func (a *App) kpzConfigureReader() func(format.DataFormatReader, registry.FormatID) error {
	return func(reader format.DataFormatReader, _ registry.FormatID) error {
		if a.FormatFlag == "" {
			return nil
		}
		_, mergedConfig, err := a.resolveFormatRef(a.FormatFlag)
		if err != nil {
			return err
		}
		return applyFormatConfig(reader, mergedConfig)
	}
}

// toolDefaultLocale returns a tool's declared default target locale (e.g.
// pseudo-translate → "qps"), or "" when none.
func (a *App) toolDefaultLocale(toolName string) string {
	if a.ToolReg == nil {
		return ""
	}
	if info := a.ToolReg.ToolInfo(registry.ToolID(toolName)); info != nil {
		return string(info.DefaultLocale)
	}
	return ""
}

func (a *App) printlnUnlessQuiet(msg string) {
	if !a.Quiet {
		fmt.Fprintln(os.Stdout, msg)
	}
}

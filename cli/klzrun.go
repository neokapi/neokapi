package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/exporter"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/klz"
	"github.com/spf13/cobra"
)

// isKlzPath reports whether a path names a .klz package.
func isKlzPath(p string) bool {
	return strings.EqualFold(filepath.Ext(p), WorkspaceExt)
}

// klzInvolved reports whether a run reads from or writes to a .klz — the
// signal to route through the klz-aware workflow instead of the plain
// read→tools→write path.
func klzInvolved(inputs []string, output string) bool {
	if isKlzPath(output) {
		return true
	}
	for _, in := range inputs {
		if isKlzPath(in) {
			return true
		}
	}
	return false
}

// toolChainBuilder builds the tool chain (and a cleanup) for a klz run. It
// abstracts over the `run` command (a composed flow) and a top-level tool
// command (a single tool).
type toolChainBuilder func() ([]tool.Tool, func(), error)

// runKlzWorkflow drives a flow with a .klz on either side of the pipeline,
// making .klz a first-class in-progress I/O format:
//
//   - Writing `-o work.klz`: run the flow against a persistent store so the
//     tools cache their per-block work as overlays, then pack the original
//     source bytes + those overlays into the package. The store is the work;
//     the package is its portable form.
//   - Reading `-i work.klz`: warm a fresh store from the package's overlays
//     and re-stream its source(s) through the flow — already-done steps
//     hydrate instead of recompute — writing the finished document(s) to the
//     `-o` target (or, when that is itself a .klz, re-packing to continue).
//
// This works with no project: the .klz *is* the workspace, so ad-hoc resume
// needs nothing but the file.
func (a *App) runKlzWorkflow(ctx context.Context, cmd *cobra.Command, flowName string, build toolChainBuilder, inputs []string, output, targetLang string) error {
	if targetLang != "" {
		a.TargetLang = targetLang
	}
	outIsKlz := isKlzPath(output)
	if !outIsKlz && output == "" {
		return errors.New("klz run: -o <output> is required when reading a .klz")
	}

	tools, cleanup, err := build()
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	work, err := os.MkdirTemp("", "kapi-klz-run-*")
	if err != nil {
		return fmt.Errorf("klz run: %w", err)
	}
	defer os.RemoveAll(work)

	// Resolve the source documents and, when resuming, the overlays already
	// computed for each.
	var sources []klz.SourceDoc
	var priorOverlays []klz.OverlayDoc
	if len(inputs) == 1 && isKlzPath(inputs[0]) {
		pkg, err := loadWorkspace(inputs[0])
		if err != nil {
			return err
		}
		sources = pkg.Source
		priorOverlays = pkg.Overlays
		if len(sources) == 0 {
			return fmt.Errorf("klz run: %q carries no source document to resume", filepath.Base(inputs[0]))
		}
	} else {
		files, err := resolveFiles(inputs)
		if err != nil {
			return err
		}
		for _, f := range files {
			if isKlzPath(f) {
				return errors.New("klz run: cannot mix a .klz input with other inputs")
			}
			data, rerr := os.ReadFile(f)
			if rerr != nil {
				return fmt.Errorf("read input %q: %w", filepath.Base(f), rerr)
			}
			sources = append(sources, klz.SourceDoc{Path: "source/" + filepath.Base(f), Data: data})
		}
	}
	if len(sources) == 0 {
		return errors.New("klz run: no input documents")
	}

	inDir := filepath.Join(work, "in")
	outDir := filepath.Join(work, "out")
	if err := os.MkdirAll(inDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	// Each source gets its OWN store: block IDs are only unique within one
	// document, so a shared keyspace would let one document's overlay
	// hydrate another's same-numbered block. Overlays are tagged with their
	// source path in the package so resume warms the right store.
	var allOverlays []klz.OverlayDoc
	for i, src := range sources {
		store, err := blockstore.NewCacheStore(filepath.Join(work, fmt.Sprintf("blocks-%d.db", i)))
		if err != nil {
			return fmt.Errorf("klz run: open store: %w", err)
		}
		if warm := overlaysForSource(priorOverlays, src.Path); len(warm) > 0 {
			if err := exporter.LoadOverlays(ctx, store, klzToStoreOverlays(warm)); err != nil {
				_ = store.Close()
				return fmt.Errorf("klz run: warm store: %w", err)
			}
		}
		runner := flow.NewFileRunner(flow.FileRunnerConfig{
			FormatReg:       a.FormatReg,
			SourceLocale:    model.LocaleID(a.SourceLang),
			Encoding:        a.Encoding,
			Store:           store,
			DetectFormat:    a.klzDetectFormat(),
			ConfigureReader: a.klzConfigureReader(),
		})

		base := filepath.Base(src.Path)
		srcPath := filepath.Join(inDir, base)
		if err := os.WriteFile(srcPath, src.Data, 0o644); err != nil {
			_ = store.Close()
			return fmt.Errorf("materialize source: %w", err)
		}

		docOut := filepath.Join(outDir, base) // throwaway when writing a package
		if !outIsKlz {
			docOut = a.resolveKlzDocOutput(output, srcPath, len(sources) > 1)
		}
		if err := runner.RunFile(ctx, flowName, tools, srcPath, docOut, targetLang); err != nil {
			_ = store.Close()
			return err
		}

		if outIsKlz {
			snap, err := exporter.Export(ctx, store)
			if err != nil {
				_ = store.Close()
				return fmt.Errorf("klz run: export overlays: %w", err)
			}
			for _, o := range storeToKlzOverlays(snap.Overlays) {
				o.Source = src.Path
				allOverlays = append(allOverlays, o)
			}
		}
		_ = store.Close()
	}

	if outIsKlz {
		pkg := &klz.Package{
			Generator: &klz.GeneratorInfo{ID: "kapi"},
			Source:    sources,
			Overlays:  allOverlays,
		}
		if err := saveWorkspace(pkg, output); err != nil {
			return err
		}
		if !a.Quiet {
			return output1(cmd, output, len(sources), true)
		}
		return nil
	}

	if !a.Quiet {
		return output1(cmd, output, len(sources), false)
	}
	return nil
}

// resolveKlzDocOutput maps one resumed source to its document output path:
//
//   - a template ("{name}", …) is expanded per source;
//   - a directory (trailing separator, an existing dir, or implied because
//     several sources are being written) receives "<dir>/<source-base>";
//   - otherwise the literal path is used (single-source convert).
//
// This makes `-o qps/` write ./qps/<name> for every input — the natural way
// to say "write the finished files back into this directory".
func (a *App) resolveKlzDocOutput(output, srcPath string, multi bool) string {
	if strings.Contains(output, "{") {
		return a.resolveOutputPath(srcPath, output)
	}
	isDir := strings.HasSuffix(output, "/") || strings.HasSuffix(output, string(os.PathSeparator))
	if !isDir {
		if fi, err := os.Stat(output); err == nil && fi.IsDir() {
			isDir = true
		}
	}
	if isDir || multi {
		return filepath.Join(output, filepath.Base(srcPath))
	}
	return output
}

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

// klzDetectFormat returns a project/flag-aware format detector for the klz
// runner, or nil to fall back to extension detection.
func (a *App) klzDetectFormat() func(string) registry.FormatID {
	if a.FormatFlag != "" {
		name := preset.ParseFormatRef(a.FormatFlag).RegistryName()
		return func(string) registry.FormatID { return registry.FormatID(name) }
	}
	if a.projectContext != nil {
		return func(path string) registry.FormatID {
			return registry.FormatID(a.projectContext.DetectFormat(a.FormatReg, path))
		}
	}
	return nil
}

// klzConfigureReader applies preset config + project format defaults to each
// reader the klz runner opens, matching the plain run path.
func (a *App) klzConfigureReader() func(format.DataFormatReader, registry.FormatID) error {
	return func(reader format.DataFormatReader, detectedFmt registry.FormatID) error {
		if a.FormatFlag != "" {
			ref := preset.ParseFormatRef(a.FormatFlag)
			if ref.IsPreset() {
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
			}
		}
		if a.projectContext != nil {
			if err := a.projectContext.ConfigureReader(reader, string(detectedFmt)); err != nil {
				return fmt.Errorf("apply project format config: %w", err)
			}
		}
		return nil
	}
}

// output1 prints the result line for a klz run. cmd may be nil (the tool
// command path), in which case it prints plainly to stdout.
func output1(cmd *cobra.Command, target string, n int, wrotePackage bool) error {
	verb := "Wrote"
	if wrotePackage {
		verb = "Packed"
	}
	msg := fmt.Sprintf("%s %s (%d document(s))", verb, target, n)
	if cmd == nil {
		fmt.Fprintln(os.Stdout, msg)
		return nil
	}
	return output.Print(cmd, simpleMessage{Message: msg})
}

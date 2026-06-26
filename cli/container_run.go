package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/neokapi/neokapi/core/container"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
)

// containerSkipFormats are entry formats the container binding never processes:
// binary assets (whole-asset localisation, pointless to re-encode inside a
// container) and nested containers (copied verbatim rather than recursed).
var containerSkipFormats = map[string]bool{
	"image": true, "audio": true, "video": true,
	"pdf": true, "archive": true, "mo": true,
}

// runContainer localizes an archive (ZIP/TAR/TAR.GZ) as a namespace of inner
// documents (AD-026 §6). It streams the container — never loading the whole
// archive into memory — visiting one entry at a time: each eligible entry is run
// as its own file (normal reader/writer with skeleton round-trip, so a DOCX/EPUB
// inside the archive round-trips faithfully) and spliced into the output as it is
// produced; every other entry is copied through. The output is written
// atomically (temp file then rename).
func (a *App) runContainer(ctx context.Context, cfg ToolRunConfig, inputPath, outputPath string, progress progressGroup) error {
	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:       a.FormatReg,
		SourceLocale:    model.LocaleID(a.SourceLang),
		Encoding:        a.Encoding,
		ConfigureReader: a.containerConfigureReader(),
	})

	work, err := os.MkdirTemp("", "kapi-container-*")
	if err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}
	defer os.RemoveAll(work)

	base := filepath.Base(inputPath)
	seq := 0
	return writeAtomic(outputPath, func(f *os.File) error {
		return container.Transform(inputPath, f, func(name string, read func() ([]byte, error)) ([]byte, bool, error) {
			if _, eligible := a.containerEntryFormat(name); !eligible {
				return nil, false, nil // copied through without being read into memory
			}
			content, rerr := read()
			if rerr != nil {
				return nil, false, rerr
			}
			seq++
			out, eerr := a.runContainerEntry(ctx, cfg, runner, work, seq, container.Entry{Name: name, Data: content})
			if eerr != nil {
				if cfg.FailOnUnknown {
					return nil, false, fmt.Errorf("%s!%s: %w", base, name, eerr)
				}
				if !cfg.NoWarn {
					warnf(progress, "Warning: leaving %s!%s unchanged: %v\n", base, name, eerr)
				}
				return nil, false, nil // pass the original entry through unchanged
			}
			return out, true, nil
		})
	})
}

// runContainerEntry runs one archive entry through a single-file pipeline and
// returns the localized bytes. The entry is materialised under a per-entry work
// dir so the shared FileRunner (which reads/writes paths and wires the skeleton
// store) can drive it exactly as it would a standalone file. Only this one entry
// is on disk/in memory at a time.
func (a *App) runContainerEntry(ctx context.Context, cfg ToolRunConfig, runner *flow.FileRunner, work string, seq int, e container.Entry) ([]byte, error) {
	dir := filepath.Join(work, strconv.Itoa(seq))
	inPath := filepath.Join(dir, "in", filepath.FromSlash(e.Name))
	outPath := filepath.Join(dir, "out", filepath.FromSlash(e.Name))
	if err := os.MkdirAll(filepath.Dir(inPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(inPath, e.Data, 0o644); err != nil {
		return nil, err
	}

	t, err := cfg.NewTool()
	if err != nil {
		return nil, fmt.Errorf("create tool: %w", err)
	}
	if cfg.ParallelBlocks > 1 {
		t = tool.NewParallelBlockTool(t, cfg.ParallelBlocks)
	}

	if err := runner.RunFile(ctx, cfg.ToolName, []tool.Tool{t}, inPath, outPath, cfg.TargetLang); err != nil {
		return nil, err
	}
	out, err := os.ReadFile(outPath)
	if err != nil {
		return nil, err
	}
	// Free the entry's on-disk footprint promptly rather than at the end of the run.
	_ = os.RemoveAll(dir)
	return out, nil
}

// containerEntryFormat decides, by extension alone (no content read, so an
// untouched entry is never loaded), whether the container binding should process
// an entry: it must have both a reader and a writer, not be a bilingual
// interchange format (those belong to extract/merge, not in-place rewriting),
// and not be a binary asset or nested container.
func (a *App) containerEntryFormat(name string) (string, bool) {
	ext := filepath.Ext(name)
	if ext == "" {
		return "", false
	}
	detected, err := a.FormatReg.DetectByExtension(ext)
	if err != nil || detected == "" {
		return "", false
	}
	fmtName := string(detected)
	if containerSkipFormats[fmtName] {
		return fmtName, false
	}
	info := a.FormatReg.FormatInfo(detected)
	if info == nil || !info.HasReader || !info.HasWriter || info.Interchange {
		return fmtName, false
	}
	return fmtName, true
}

// containerConfigureReader applies project/preset format config to each inner
// entry's reader, so per-format settings declared in the recipe reach content
// inside the container too.
func (a *App) containerConfigureReader() func(format.DataFormatReader, registry.FormatID) error {
	if a.projectContext == nil {
		return nil
	}
	return func(reader format.DataFormatReader, detectedFmt registry.FormatID) error {
		return a.projectContext.ConfigureReader(reader, string(detectedFmt))
	}
}

// writeAtomic writes via a sibling temp file then renames, so a failure never
// leaves a partial container at path.
func writeAtomic(path string, write func(*os.File) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".kapi-container-*")
	if err != nil {
		return fmt.Errorf("create temp output: %w", err)
	}
	tmpPath := tmp.Name()
	if err := write(tmp); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("finalize %s: %w", path, err)
	}
	return nil
}

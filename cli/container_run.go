package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

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
// documents (AD-026 §6). Each eligible entry is run as its own file — through
// the normal reader/writer with skeleton round-trip, so a DOCX/EPUB inside the
// archive round-trips faithfully — and the results are repacked over the
// original container, copying every other member byte-for-byte. The output is
// written atomically (temp file then rename).
func (a *App) runContainer(ctx context.Context, cfg ToolRunConfig, inputPath, outputPath string, progress progressGroup) error {
	original, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", inputPath, err)
	}
	kind, entries, err := container.Enumerate(original)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(inputPath), err)
	}

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

	replacements := make(map[string][]byte)
	for _, e := range entries {
		fmtName, eligible := a.containerEntryFormat(e.Name, e.Data)
		if !eligible {
			continue // copied verbatim by Repack
		}

		out, err := a.runContainerEntry(ctx, cfg, runner, work, e)
		if err != nil {
			if cfg.FailOnUnknown {
				return fmt.Errorf("%s!%s: %w", filepath.Base(inputPath), e.Name, err)
			}
			if !cfg.NoWarn {
				warnf(progress, "Warning: skipping %s!%s (%s): %v\n", filepath.Base(inputPath), e.Name, fmtName, err)
			}
			continue // leave the entry unchanged
		}
		replacements[e.Name] = out
	}

	return writeAtomic(outputPath, func(f *os.File) error {
		return container.Repack(kind, original, replacements, f)
	})
}

// runContainerEntry runs one archive entry through a single-file pipeline and
// returns the localized bytes. The entry is materialised under a work dir so the
// shared FileRunner (which reads/writes paths and wires the skeleton store) can
// drive it exactly as it would a standalone file.
func (a *App) runContainerEntry(ctx context.Context, cfg ToolRunConfig, runner *flow.FileRunner, work string, e container.Entry) ([]byte, error) {
	inPath := filepath.Join(work, "in", filepath.FromSlash(e.Name))
	outPath := filepath.Join(work, "out", filepath.FromSlash(e.Name))
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
	return os.ReadFile(outPath)
}

// containerEntryFormat detects an entry's format and reports whether the
// container binding should process it: it must have both a reader and a writer,
// not be a bilingual interchange format (those belong to extract/merge, not
// in-place rewriting), and not be a binary asset or nested container.
func (a *App) containerEntryFormat(name string, content []byte) (string, bool) {
	detected, err := a.FormatReg.Detector().Detect(name, bytes.NewReader(content), "")
	if err != nil || detected == "" {
		return "", false
	}
	fmtName := detected
	if containerSkipFormats[fmtName] {
		return fmtName, false
	}
	info := a.FormatReg.FormatInfo(registry.FormatID(detected))
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

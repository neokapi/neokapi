package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/gokapi/gokapi/core/flow"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/loader"
	"github.com/gokapi/gokapi/core/tool"
	"github.com/gokapi/gokapi/platform/cli/output"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"golang.org/x/sync/errgroup"
)

// ToolRunConfig configures RunToolOnFiles.
type ToolRunConfig struct {
	ToolName       string
	Files          []string
	Concurrency    int
	JSONOutput     bool
	FailOnUnknown  bool
	NoWarn         bool
	Progress       bool
	OutputTemplate string
	TargetLang     string
	NewTool        func() (tool.Tool, error)
	NewCollector   func() flow.Collector
}

// RunToolOnFiles processes each file through a single-tool flow and
// aggregates results via the collector. Files are processed in parallel.
func (a *App) RunToolOnFiles(ctx context.Context, cfg ToolRunConfig) error {
	files, err := resolveFiles(cfg.Files)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no files to process")
	}

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	var collector flow.Collector
	if cfg.NewCollector != nil {
		collector = cfg.NewCollector()
	}

	var commonDir string
	if cfg.OutputTemplate != "" {
		commonDir = commonDirPrefix(files)
	}

	var bar *mpb.Bar
	var progress *mpb.Progress
	var active atomic.Int64
	if cfg.Progress {
		progress = mpb.New(mpb.WithOutput(os.Stderr))
		bar = progress.New(int64(len(files)),
			mpb.BarStyle().Lbound("[").Filler("=").Tip(">").Padding(" ").Rbound("]"),
			mpb.PrependDecorators(decor.Elapsed(decor.ET_STYLE_GO)),
			mpb.AppendDecorators(
				decor.CountersNoUnit("[%d/%d]"),
				decor.Any(func(s decor.Statistics) string {
					n := active.Load()
					if n > 0 {
						return fmt.Sprintf(" (%d active)", n)
					}
					return ""
				}),
			),
		)
	}

	g, ctx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, concurrency)

	for _, file := range files {
		sem <- struct{}{}
		g.Go(func() error {
			defer func() { <-sem }()
			active.Add(1)
			err := a.processOneFile(ctx, cfg, file, collector, commonDir, progress)
			active.Add(-1)
			if bar != nil {
				bar.Increment()
			}
			return err
		})
	}

	if err := g.Wait(); err != nil {
		if progress != nil {
			progress.Wait()
		}
		return err
	}

	if progress != nil {
		progress.Wait()
	}

	if collector == nil {
		return nil
	}

	result, err := collector.Result()
	if err != nil {
		return fmt.Errorf("collector result: %w", err)
	}
	return output.FormatCollectorResult(cfg.JSONOutput, result.Data)
}

func (a *App) processOneFile(ctx context.Context, cfg ToolRunConfig, filePath string, collector flow.Collector, commonDir string, progress *mpb.Progress) error {
	fmtName := a.FormatFlag
	if fmtName == "" {
		ext := filepath.Ext(filePath)
		detected, err := a.FormatReg.Detector().DetectByExtension(ext)
		if err != nil {
			if !cfg.FailOnUnknown {
				if !cfg.NoWarn {
					warnf(progress, "Warning: skipping %q: %v\n", filePath, err)
				}
				return nil
			}
			return fmt.Errorf("unable to detect format for %q: %w", filePath, err)
		}
		fmtName = detected
	}

	reader, err := a.FormatReg.NewReader(fmtName)
	if err != nil {
		if !cfg.FailOnUnknown {
			if !cfg.NoWarn {
				warnf(progress, "Warning: skipping %q: %v\n", filePath, err)
			}
			return nil
		}
		return fmt.Errorf("no reader for format %q: %w", fmtName, err)
	}

	if cfg.OutputTemplate != "" {
		if _, err := a.FormatReg.NewWriter(fmtName); err != nil {
			if !cfg.FailOnUnknown {
				if !cfg.NoWarn {
					warnf(progress, "Warning: skipping %q: %v\n", filePath, err)
				}
				return nil
			}
			return fmt.Errorf("no writer for format %q: %w", fmtName, err)
		}
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", filePath, err)
	}

	doc := &model.RawDocument{
		URI:          filePath,
		SourceLocale: model.LocaleID(a.SourceLang),
		Encoding:     a.Encoding,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}

	if err := reader.Open(ctx, doc); err != nil {
		if !cfg.FailOnUnknown {
			if !cfg.NoWarn {
				warnf(progress, "Warning: skipping %q: %v\n", filePath, err)
			}
			return nil
		}
		return fmt.Errorf("open %s: %w", filePath, err)
	}

	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			reader.Close()
			if !cfg.FailOnUnknown {
				if !cfg.NoWarn {
					warnf(progress, "Warning: skipping %q: %v\n", filePath, result.Error)
				}
				return nil
			}
			return fmt.Errorf("read %s: %w", filePath, result.Error)
		}
		parts = append(parts, result.Part)
	}
	reader.Close()

	t, err := cfg.NewTool()
	if err != nil {
		return fmt.Errorf("create tool: %w", err)
	}

	fb := flow.NewFlow(cfg.ToolName)
	fb.AddTool(t)
	f := fb.Build()

	executor := flow.NewFlowExecutor()
	inCh, outCh, wait := executor.ExecuteWithChannels(ctx, f)

	go func() {
		for _, p := range parts {
			inCh <- p
		}
		close(inCh)
	}()

	var outputParts []*model.Part
	for p := range outCh {
		outputParts = append(outputParts, p)
	}

	if err := wait(); err != nil {
		return fmt.Errorf("tool execution on %s: %w", filePath, err)
	}

	if cfg.OutputTemplate != "" {
		outputPath := expandOutputPath(cfg.OutputTemplate, filePath, commonDir, cfg.TargetLang)

		writer, err := a.FormatReg.NewWriter(fmtName)
		if err != nil {
			return fmt.Errorf("no writer for format %q: %w", fmtName, err)
		}

		if err := writer.SetOutput(outputPath); err != nil {
			return fmt.Errorf("set output %s: %w", outputPath, err)
		}

		if ocs, ok := writer.(loader.OriginalContentSetter); ok {
			ocs.SetOriginalContent(content)
		}

		locale := model.LocaleID(cfg.TargetLang)
		writer.SetLocale(locale)

		ch := make(chan *model.Part, len(outputParts))
		for _, p := range outputParts {
			ch <- p
		}
		close(ch)

		if err := writer.Write(ctx, ch); err != nil {
			return fmt.Errorf("write output %s: %w", outputPath, err)
		}
		if err := writer.Close(); err != nil {
			return fmt.Errorf("close writer %s: %w", outputPath, err)
		}

		if !a.Quiet {
			msg := fmt.Sprintf("%s → %s\n", filePath, outputPath)
			if progress != nil {
				fmt.Fprint(progress, msg)
			} else {
				fmt.Fprint(os.Stderr, msg)
			}
		}
		return nil
	}

	if collector != nil {
		item := &flow.FlowItem{
			Input:        doc,
			TargetLocale: model.LocaleID(cfg.TargetLang),
		}
		if err := collector.Collect(ctx, item, outputParts); err != nil {
			return fmt.Errorf("collect %s: %w", filePath, err)
		}
	}

	return nil
}

func resolveFiles(patterns []string) ([]string, error) {
	var files []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}
		if matches == nil {
			matches = []string{pattern}
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil {
				return nil, fmt.Errorf("stat %q: %w", m, err)
			}
			if info.IsDir() {
				continue
			}
			if isJunkFile(filepath.Base(m)) {
				continue
			}
			files = append(files, m)
		}
	}
	return files, nil
}

func isJunkFile(name string) bool {
	return strings.HasPrefix(name, "~$") || strings.HasPrefix(name, "._")
}

func commonDirPrefix(files []string) string {
	if len(files) == 0 {
		return ""
	}
	if len(files) == 1 {
		return filepath.Dir(files[0]) + string(filepath.Separator)
	}

	prefix := filepath.Dir(files[0])
	for _, f := range files[1:] {
		dir := filepath.Dir(f)
		prefix = commonPath(prefix, dir)
		if prefix == "" {
			return ""
		}
	}
	if prefix != "" && !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return prefix
}

func commonPath(a, b string) string {
	aParts := strings.Split(filepath.ToSlash(a), "/")
	bParts := strings.Split(filepath.ToSlash(b), "/")
	n := min(len(bParts), len(aParts))
	var common []string
	for i := range n {
		if aParts[i] != bParts[i] {
			break
		}
		common = append(common, aParts[i])
	}
	if len(common) == 0 {
		return ""
	}
	return filepath.FromSlash(strings.Join(common, "/"))
}

func expandOutputPath(tmpl, filePath, commonDir, lang string) string {
	rel := filePath
	if commonDir != "" {
		if r, err := filepath.Rel(commonDir, filePath); err == nil {
			rel = r
		}
	}

	ext := filepath.Ext(rel)
	name := strings.TrimSuffix(rel, ext)
	extNoDot := strings.TrimPrefix(ext, ".")

	result := tmpl
	result = strings.ReplaceAll(result, "{name}", name)
	result = strings.ReplaceAll(result, "{ext}", extNoDot)
	result = strings.ReplaceAll(result, "{lang}", lang)

	isDir := strings.HasSuffix(result, "/") || strings.HasSuffix(result, string(filepath.Separator))
	if !isDir {
		if info, err := os.Stat(result); err == nil && info.IsDir() {
			isDir = true
		}
	}
	if !isDir && filepath.Ext(result) == "" {
		isDir = true
	}
	if isDir {
		result = filepath.Join(result, rel)
	}

	dir := filepath.Dir(result)
	_ = os.MkdirAll(dir, 0o755)

	return result
}

func warnf(progress *mpb.Progress, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if progress != nil {
		fmt.Fprint(progress, msg)
	} else {
		fmt.Fprint(os.Stderr, msg)
	}
}

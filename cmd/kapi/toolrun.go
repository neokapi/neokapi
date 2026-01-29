package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/asgeirf/gokapi/core/flow"
	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/core/tool"
	"golang.org/x/sync/errgroup"
)

// ToolRunConfig configures RunToolOnFiles.
type ToolRunConfig struct {
	ToolName     string
	Files        []string
	Concurrency  int                       // 0 = NumCPU
	JSONOutput   bool
	NewTool      func() (tool.Tool, error) // factory for fresh tool per file
	NewCollector func() flow.Collector     // nil = no collection
}

// RunToolOnFiles processes each file through a single-tool flow and
// aggregates results via the collector. Files are processed in parallel.
func RunToolOnFiles(ctx context.Context, cfg ToolRunConfig) error {
	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	var collector flow.Collector
	if cfg.NewCollector != nil {
		collector = cfg.NewCollector()
	}

	g, ctx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, concurrency)

	for _, file := range cfg.Files {
		file := file
		sem <- struct{}{}
		g.Go(func() error {
			defer func() { <-sem }()
			return processOneFile(ctx, cfg, file, collector)
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	if collector == nil {
		return nil
	}

	result, err := collector.Result()
	if err != nil {
		return fmt.Errorf("collector result: %w", err)
	}
	return formatOutput(cfg.JSONOutput, result)
}

// processOneFile reads a single file, pipes parts through the tool, and
// feeds the output to the collector.
func processOneFile(ctx context.Context, cfg ToolRunConfig, filePath string, collector flow.Collector) error {
	// Detect format.
	fmtName := formatFlag
	if fmtName == "" {
		ext := filepath.Ext(filePath)
		detected, err := formatReg.Detector().DetectByExtension(ext)
		if err != nil {
			return fmt.Errorf("unable to detect format for %q: %w", filePath, err)
		}
		fmtName = detected
	}

	reader, err := formatReg.NewReader(fmtName)
	if err != nil {
		return fmt.Errorf("no reader for format %q: %w", fmtName, err)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", filePath, err)
	}

	doc := &model.RawDocument{
		URI:          filePath,
		SourceLocale: model.LocaleID(sourceLang),
		Encoding:     encoding,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}

	if err := reader.Open(ctx, doc); err != nil {
		return fmt.Errorf("open %s: %w", filePath, err)
	}
	defer reader.Close()

	// Read all parts from the format reader.
	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			return fmt.Errorf("read %s: %w", filePath, result.Error)
		}
		parts = append(parts, result.Part)
	}

	// Create a single-tool flow and execute it.
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

	// Feed collector.
	if collector != nil {
		item := &flow.FlowItem{
			Input:        doc,
			TargetLocale: model.LocaleID(targetLang),
		}
		if err := collector.Collect(ctx, item, outputParts); err != nil {
			return fmt.Errorf("collect %s: %w", filePath, err)
		}
	}

	return nil
}

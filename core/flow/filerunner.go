package flow

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
)

// FileRunnerConfig configures a FileRunner.
type FileRunnerConfig struct {
	// FormatReg is the format registry for reader/writer creation.
	FormatReg *registry.FormatRegistry

	// SourceLocale is the BCP-47 source locale.
	SourceLocale model.LocaleID

	// Encoding is the file encoding (default: "UTF-8").
	Encoding string

	// ConfigureReader is an optional callback applied to each reader after
	// creation. Use this to apply project format defaults or preset config.
	// The formatName parameter is the detected format name (e.g., "json").
	ConfigureReader func(reader format.DataFormatReader, formatName string) error

	// ConfigureWriter is an optional callback applied to each writer after
	// creation. Use this to apply project encoding or other defaults.
	ConfigureWriter func(writer format.DataFormatWriter)
}

// FileRunner runs a full read → process → write pipeline for a single file.
// It handles format detection, reader/writer creation, skeleton store wiring,
// tool execution, and output writing. Shared by CLI, desktop, and MCP.
type FileRunner struct {
	cfg FileRunnerConfig
}

// NewFileRunner creates a FileRunner with the given configuration.
func NewFileRunner(cfg FileRunnerConfig) *FileRunner {
	if cfg.Encoding == "" {
		cfg.Encoding = "UTF-8"
	}
	return &FileRunner{cfg: cfg}
}

// RunFile detects the format, creates a reader, and routes to the appropriate
// pipeline — bridge (Java single-pass) or native (Go read→process→write).
// Consumers don't need to know which execution model is used.
func (r *FileRunner) RunFile(ctx context.Context, flowName string, tools []tool.Tool, inputPath, outputPath, targetLang string) error {
	reg := r.cfg.FormatReg

	ext := filepath.Ext(inputPath)
	fmtName, err := reg.DetectByExtension(ext)
	if err != nil {
		return fmt.Errorf("detect format for %q: %w", filepath.Base(inputPath), err)
	}

	reader, err := reg.NewReader(fmtName)
	if err != nil {
		return fmt.Errorf("no reader for %q: %w", fmtName, err)
	}

	// Apply reader configuration (project defaults, presets).
	if r.cfg.ConfigureReader != nil {
		if err := r.cfg.ConfigureReader(reader, fmtName); err != nil {
			reader.Close()
			return fmt.Errorf("configure reader for %q: %w", fmtName, err)
		}
	}

	// Bridge format: Java controls read/write, Go processes parts inline.
	if bridgeReader, ok := reader.(*bridge.BridgeFormatReader); ok {
		br := NewBridgeRunner(BridgeRunnerConfig{
			SourceLocale: string(r.cfg.SourceLocale),
			TargetLocale: targetLang,
			Encoding:     r.cfg.Encoding,
		})
		return br.RunFile(ctx, flowName, tools, bridgeReader, inputPath, outputPath)
	}

	// Native format: Go controls full lifecycle.
	writer, err := reg.NewWriter(fmtName)
	if err != nil {
		reader.Close()
		return fmt.Errorf("no writer for %q: %w", fmtName, err)
	}

	// Apply writer configuration (encoding, project defaults).
	if r.cfg.ConfigureWriter != nil {
		r.cfg.ConfigureWriter(writer)
	}

	return r.RunFileWithReaderWriter(ctx, flowName, tools, inputPath, outputPath, targetLang, reader, writer)
}

// RunFileWithReaderWriter runs the pipeline with pre-created reader and writer.
// The caller is responsible for configuring the reader (presets, project
// defaults) before calling. This is the primary integration point for CLI
// and MCP which need to apply format presets and project config.
func (r *FileRunner) RunFileWithReaderWriter(ctx context.Context, flowName string, tools []tool.Tool, inputPath, outputPath, targetLang string, reader format.DataFormatReader, writer format.DataFormatWriter) error {
	// Wire skeleton store if both support it.
	var skeletonStore *format.SkeletonStore
	if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
		if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
			if store, storeErr := format.NewSkeletonStore(); storeErr == nil {
				skeletonStore = store
				emitter.SetSkeletonStore(store)
				consumer.SetSkeletonStore(store)
			}
		}
	}

	// Read input file.
	inputContent, err := os.ReadFile(inputPath)
	if err != nil {
		reader.Close()
		if skeletonStore != nil {
			skeletonStore.Close()
		}
		return fmt.Errorf("read file: %w", err)
	}

	doc := &model.RawDocument{
		URI:          inputPath,
		SourceLocale: r.cfg.SourceLocale,
		TargetLocale: model.LocaleID(targetLang),
		Encoding:     r.cfg.Encoding,
		Reader:       io.NopCloser(bytes.NewReader(inputContent)),
	}

	if err := reader.Open(ctx, doc); err != nil {
		reader.Close()
		if skeletonStore != nil {
			skeletonStore.Close()
		}
		return fmt.Errorf("open %q: %w", filepath.Base(inputPath), err)
	}

	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			reader.Close()
			if skeletonStore != nil {
				skeletonStore.Close()
			}
			return fmt.Errorf("read %q: %w", filepath.Base(inputPath), result.Error)
		}
		parts = append(parts, result.Part)
	}
	// Close reader immediately after reading — for bridge formats this releases
	// the JVM back to the pool so the writer can reuse it.
	reader.Close()

	// Build and execute tool pipeline.
	fb := NewFlow(flowName)
	for _, t := range tools {
		fb.AddTool(t)
	}
	f, err := fb.Build()
	if err != nil {
		if skeletonStore != nil {
			skeletonStore.Close()
		}
		return fmt.Errorf("build flow: %w", err)
	}

	executor := NewExecutor()
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
		if skeletonStore != nil {
			skeletonStore.Close()
		}
		return fmt.Errorf("execute flow: %w", err)
	}

	// Ensure output directory exists.
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		if skeletonStore != nil {
			skeletonStore.Close()
		}
		return fmt.Errorf("create output dir: %w", err)
	}

	if err := writer.SetOutput(outputPath); err != nil {
		if skeletonStore != nil {
			skeletonStore.Close()
		}
		return fmt.Errorf("set output: %w", err)
	}

	// Pass original content for skeleton-based writers (e.g., OpenXML).
	if sps, ok := writer.(format.SourcePathSetter); ok && filepath.IsAbs(inputPath) {
		sps.SetSourcePath(inputPath)
	} else if ocs, ok := writer.(format.OriginalContentSetter); ok {
		ocs.SetOriginalContent(inputContent)
	}

	writer.SetLocale(model.LocaleID(targetLang))

	ch := make(chan *model.Part, len(outputParts))
	for _, p := range outputParts {
		ch <- p
	}
	close(ch)

	if err := writer.Write(ctx, ch); err != nil {
		if skeletonStore != nil {
			skeletonStore.Close()
		}
		return fmt.Errorf("write %q: %w", filepath.Base(outputPath), err)
	}
	writer.Close()

	if skeletonStore != nil {
		skeletonStore.Close()
	}

	return nil
}

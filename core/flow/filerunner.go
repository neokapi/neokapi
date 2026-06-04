package flow

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
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

	// DetectFormat is an optional callback for project-scoped format detection.
	// When set, it replaces the default FormatReg.DetectByExtension call.
	// Use this to restrict detection to project-declared plugin sources.
	DetectFormat func(path string) registry.FormatID

	// ConfigureReader is an optional callback applied to each reader after
	// creation. Use this to apply project format defaults or preset config.
	// The formatName parameter is the detected format name (e.g., "json").
	ConfigureReader func(reader format.DataFormatReader, formatName registry.FormatID) error

	// ConfigureWriter is an optional callback applied to each writer after
	// creation. Use this to apply project encoding or other defaults.
	ConfigureWriter func(writer format.DataFormatWriter)

	// Recorder, when non-nil, captures a flow trace: an initial snapshot of
	// each Part as it leaves the reader plus reader-exit events, and
	// writer enter/exit events. The caller is responsible for wrapping the
	// tools with TracingTool to capture per-tool snapshots. This is what makes
	// `kapi run <flow> --trace` produce the same rich trace as a single tool.
	Recorder *TraceRecorder

	// Store, when non-nil, is the block store the executor runs the tool
	// chain against. A persistent store (e.g. a workspace's blocks.db) makes
	// SessionTools cache per-block work as overlays and skip already-done
	// steps on a later run — the substrate of resumable .klz workspaces
	// (AD-025 §5). nil (the default) uses an ephemeral in-memory store, so
	// one-shot runs are unchanged.
	Store blockstore.Store
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

// RunFile detects the format, creates a reader and writer, and runs the
// standard read → process → write pipeline. Mode-C plugin formats are
// transparently routed through their daemon by the registered factories.
func (r *FileRunner) RunFile(ctx context.Context, flowName string, tools []tool.Tool, inputPath, outputPath, targetLang string) error {
	reg := r.cfg.FormatReg

	var fmtName registry.FormatID
	if r.cfg.DetectFormat != nil {
		fmtName = r.cfg.DetectFormat(inputPath)
	}
	if fmtName == "" {
		ext := filepath.Ext(inputPath)
		var err error
		fmtName, err = reg.DetectByExtension(ext)
		if err != nil {
			return fmt.Errorf("detect format for %q: %w", filepath.Base(inputPath), err)
		}
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
		// Record the reader-stage trace: an initial snapshot (so per-tool
		// TracingTool snapshots have a set to attach to) plus a reader-exit
		// event as the Part heads into the pipeline.
		if r.cfg.Recorder != nil && result.Part != nil && result.Part.Resource != nil {
			id := result.Part.Resource.ResourceID()
			r.cfg.Recorder.SnapshotPart(result.Part, "reader", "initial")
			r.cfg.Recorder.Record(TraceExit, "reader", id, nil)
		}
	}
	// Close reader immediately after reading — for daemon-backed plugin
	// formats this lets the daemon enter its idle state, freeing the
	// stream for the subsequent writer call.
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

	// Ensure output directory exists.
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		if skeletonStore != nil {
			skeletonStore.Close()
		}
		return fmt.Errorf("create output dir: %w", err)
	}

	// Open the output file here and hand the writer a buffered io.Writer
	// rather than letting it open the file directly (#608, S4). Skeleton-
	// driven writers emit one (often tiny) write per skeleton entry; an
	// unbuffered *os.File turns each into a syscall. A 64 KiB buffer
	// coalesces them. The buffer is flushed AFTER writer.Close() returns —
	// some writers (e.g. the KLF writer) only emit their payload in Close,
	// so the buffer must outlive Close. Output bytes are unchanged.
	//
	// Write into a sibling temp file and rename on success (#608, S1).
	// The executor and writer now run concurrently — the writer drains
	// the tool output channel directly instead of buffering every output
	// Part into a slice and re-feeding a third channel. Because output is
	// produced incrementally, a tool/writer error could leave a partial
	// file at outputPath; the temp-then-rename keeps the destination
	// all-or-nothing, matching the pre-S1 contract where a tool error
	// produced no output file at all.
	tmpFile, err := os.CreateTemp(filepath.Dir(outputPath), ".kapi-out-*")
	if err != nil {
		if skeletonStore != nil {
			skeletonStore.Close()
		}
		return fmt.Errorf("set output: %w", err)
	}
	tmpPath := tmpFile.Name()
	// failTmp closes + removes the temp file (so outputPath is never left
	// with partial bytes), closes the skeleton store, and returns the
	// formatted error. Used on every error path before the final rename.
	failTmp := func(format string, args ...any) error {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		if skeletonStore != nil {
			skeletonStore.Close()
		}
		return fmt.Errorf(format, args...)
	}

	bw := bufio.NewWriterSize(tmpFile, 64*1024)
	if err := writer.SetOutputWriter(bw); err != nil {
		return failTmp("set output: %w", err)
	}

	// Pass original content for skeleton-based writers (e.g., OpenXML).
	if sps, ok := writer.(format.SourcePathSetter); ok && filepath.IsAbs(inputPath) {
		sps.SetSourcePath(inputPath)
	} else if ocs, ok := writer.(format.OriginalContentSetter); ok {
		ocs.SetOriginalContent(inputContent)
	}

	writer.SetLocale(model.LocaleID(targetLang))

	// Single concurrent pipeline: feed the read parts into the executor's
	// input channel from a goroutine while the writer drains the executor's
	// output channel directly. The reader has already been fully read and
	// closed above, which preserves the read-then-write ordering required
	// by daemon-backed plugin formats (one Process stream at a time) and
	// guarantees every skeleton entry is written before the writer's
	// internal skeleton Flush().
	var execOpts []ExecutorOption
	if r.cfg.Store != nil {
		execOpts = append(execOpts, WithBlockStore(r.cfg.Store))
	}
	executor := NewExecutor(execOpts...)

	// Derive a cancellable context for the feeder so a tool error (which
	// cancels the executor's own internal context and stops the tools)
	// can also unblock the feeder if it's parked on an inCh send. Without
	// this the feeder goroutine would leak. feedDone lets us join it.
	feedCtx, feedCancel := context.WithCancel(ctx)
	defer feedCancel()

	inCh, outCh, wait := executor.ExecuteWithChannels(ctx, f)

	feedDone := make(chan struct{})
	go func() {
		defer close(feedDone)
		defer close(inCh)
		for _, p := range parts {
			select {
			case inCh <- p:
			case <-feedCtx.Done():
				return
			}
		}
	}()

	// The writer drains outCh (every DataFormatWriter loops
	// `for part := range parts`), so the executor's tool goroutines can
	// make progress and close outCh — no deadlock. Should a writer return
	// early without draining (e.g. it rejects the input before consuming
	// all parts), drain the remainder here so a tool goroutine blocked on
	// an `outCh <- p` send can still finish; otherwise the executor's
	// errgroup Wait() — and thus this function — would hang.
	// When tracing, relay the executor output through a tap that records a
	// writer enter/exit event per Part before the writer consumes it. The
	// relay owns draining outCh and closes its own channel when outCh closes,
	// so the no-trace path (writerIn == outCh) is byte-for-byte unchanged.
	writerIn := outCh
	if r.cfg.Recorder != nil {
		tapCh := make(chan *model.Part, cap(outCh))
		go func() {
			defer close(tapCh)
			for p := range outCh {
				if p != nil && p.Resource != nil {
					id := p.Resource.ResourceID()
					r.cfg.Recorder.Record(TraceEnter, "writer", id, nil)
					r.cfg.Recorder.Record(TraceExit, "writer", id, nil)
				}
				tapCh <- p
			}
		}()
		writerIn = tapCh
	}

	writeErr := writer.Write(ctx, writerIn)
	if writeErr != nil {
		for range writerIn { //nolint:revive // intentional drain to unblock tools
		}
	}
	waitErr := wait()
	// The executor has finished; cancel and join the feeder so it never
	// leaks (it may still be parked on an inCh send if the tools stopped
	// early on a tool error before reading every part).
	feedCancel()
	<-feedDone
	if waitErr != nil {
		return failTmp("execute flow: %w", waitErr)
	}
	if writeErr != nil {
		return failTmp("write %q: %w", filepath.Base(outputPath), writeErr)
	}

	// Close the writer first (lets writers that emit on Close, like KLF,
	// finish writing into the buffer), then flush the buffer to the file,
	// then close the file, then rename into place. Any error removes the
	// temp file so outputPath is never left partial.
	if cerr := writer.Close(); cerr != nil {
		return failTmp("close writer %q: %w", filepath.Base(outputPath), cerr)
	}
	if ferr := bw.Flush(); ferr != nil {
		return failTmp("flush %q: %w", filepath.Base(outputPath), ferr)
	}
	if ferr := tmpFile.Close(); ferr != nil {
		_ = os.Remove(tmpPath)
		if skeletonStore != nil {
			skeletonStore.Close()
		}
		return fmt.Errorf("close %q: %w", filepath.Base(outputPath), ferr)
	}
	if rerr := os.Rename(tmpPath, outputPath); rerr != nil {
		_ = os.Remove(tmpPath)
		if skeletonStore != nil {
			skeletonStore.Close()
		}
		return fmt.Errorf("finalize %q: %w", filepath.Base(outputPath), rerr)
	}

	if skeletonStore != nil {
		skeletonStore.Close()
	}

	return nil
}

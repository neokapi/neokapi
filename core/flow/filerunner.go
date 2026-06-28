package flow

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/safeio"
	"github.com/neokapi/neokapi/core/structure"
	"github.com/neokapi/neokapi/core/tool"
)

// budgetedSource wraps a source stream with the shared safeio byte budget and a
// closer, so every reader fed by the file-run path is bounded uniformly
// (core/safeio) regardless of whether the format reader self-wraps. It is the
// streaming twin of the old "os.ReadFile then bytes.Reader" — the file's bytes
// flow through on demand instead of being buffered whole up front.
type budgetedSource struct {
	io.Reader
	closer io.Closer
}

func (b *budgetedSource) Close() error {
	if b.closer != nil {
		return b.closer.Close()
	}
	return nil
}

// openBudgetedFile opens path as a streaming, budget-bounded io.ReadCloser. The
// reader pulls bytes lazily, so a streaming format never holds the whole file in
// memory; a whole-document format still io.ReadAll-s it, but only once (no
// separate up-front os.ReadFile buffer).
func openBudgetedFile(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &budgetedSource{Reader: safeio.DefaultBudget().Reader(f), closer: f}, nil
}

// budgetedBytes wraps an in-memory source with the same byte budget, for the
// buffered path where a writer needs the original bytes (OpenXML/AsciiDoc) and
// the source is read once and shared with the reader.
func budgetedBytes(content []byte) io.ReadCloser {
	return &budgetedSource{Reader: safeio.DefaultBudget().Reader(bytes.NewReader(content))}
}

// structuralExportWriters are the document writers that render the canonical
// table structure (GroupStart "table"/"table-row" + table-cell/header roles).
// On a cross-format export to one of these, spreadsheet cell geometry is
// normalized into table groups so a grid renders as a real table (see
// structure.SpreadsheetGridToTables). Catalog/list writers are excluded so
// their flat extraction is unchanged.
var structuralExportWriters = map[string]bool{
	"markdown": true,
	"html":     true,
	"asciidoc": true,
	"doclang":  true,
}

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

	// PartCache, when non-nil, is the project's document cache: the runner
	// consults it before parsing a source so a file another operation (status,
	// a prior run) already parsed under the same config is replayed from the
	// cache instead of re-read. It is the L1 companion to Store's L2 overlay
	// cache — together they are the project's one internal model. nil (ad-hoc,
	// no project) parses directly. See PartCacheKey for the config key.
	PartCache PartCache

	// PartCacheKey fingerprints the parse configuration the caller applied
	// (format config map + source locale) so the document cache never serves a
	// document parsed under different config. The runner combines it with the
	// per-file detected format. Empty when PartCache is nil.
	PartCacheKey string
}

// PartCache is the file runner's optional document cache: a hit returns a file's
// full Part stream from a prior parse under the same config, so the reader never
// runs. Implemented by the CLI over the project's `.kapi/cache` (rebuildable from
// the files), it is the seam that makes kapi parse each source once in project
// mode. The configKey the runner passes already folds in the detected format and
// the caller's PartCacheKey, so implementations key on (path, configKey) plus
// their own staleness check (content hash) over the file.
type PartCache interface {
	// GetDocument returns the cached Part stream — and the reconstruction skeleton
	// bytes + origin format when one was captured — for a file under configKey,
	// when it is fresh relative to the file on disk. ok=false → the runner parses.
	// The process-only path stores no skeleton (nil); the file-writing path stores
	// the source skeleton so a writer can reconstruct byte-exact output on a hit.
	GetDocument(path, configKey string) (parts []*model.Part, skeleton []byte, originFormat string, ok bool)
	// PutDocument records a freshly-parsed Part stream (and optional skeleton) for
	// a file under configKey.
	PutDocument(path, configKey string, parts []*model.Part, skeleton []byte, originFormat string)
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
		var err error
		// Content-aware: an extension claimed by several formats (.xliff 1.x/2.x,
		// .xml, …) is disambiguated by the file head, not extension alone.
		fmtName, err = reg.DetectFile(inputPath, nil)
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

// RunFileProcessOnly detects the format, creates and configures a reader, and
// runs a process-only pipeline (no writer) against the configured Store —
// committing `targets/<locale>` overlays and emitting no output file
// (AD-026 §3). It is the process-only twin of RunFile: same detection and
// reader-configuration path, but no sink. Requires a persistent Store.
func (r *FileRunner) RunFileProcessOnly(ctx context.Context, flowName string, tools []tool.Tool, inputPath, targetLang string) error {
	reg := r.cfg.FormatReg

	var fmtName registry.FormatID
	if r.cfg.DetectFormat != nil {
		fmtName = r.cfg.DetectFormat(inputPath)
	}
	if fmtName == "" {
		var err error
		// Content-aware: an extension claimed by several formats (.xliff 1.x/2.x,
		// .xml, …) is disambiguated by the file head, not extension alone.
		fmtName, err = reg.DetectFile(inputPath, nil)
		if err != nil {
			return fmt.Errorf("detect format for %q: %w", filepath.Base(inputPath), err)
		}
	}

	reader, err := reg.NewReader(fmtName)
	if err != nil {
		return fmt.Errorf("no reader for %q: %w", fmtName, err)
	}

	if r.cfg.ConfigureReader != nil {
		if err := r.cfg.ConfigureReader(reader, fmtName); err != nil {
			reader.Close()
			return fmt.Errorf("configure reader for %q: %w", fmtName, err)
		}
	}

	return r.RunFileToStore(ctx, flowName, tools, inputPath, targetLang, reader)
}

// openReader opens reader over a budget-bounded streaming source for inputPath.
// The reader pulls bytes on demand (phase 1: no eager whole-file os.ReadFile);
// the caller is responsible for closing the reader, which closes the source.
func (r *FileRunner) openReader(ctx context.Context, reader format.DataFormatReader, source io.ReadCloser, inputPath, targetLang string) error {
	doc := &model.RawDocument{
		URI:          inputPath,
		SourceLocale: r.cfg.SourceLocale,
		TargetLocale: model.LocaleID(targetLang),
		Encoding:     r.cfg.Encoding,
		Reader:       source,
	}
	if err := reader.Open(ctx, doc); err != nil {
		reader.Close()
		return fmt.Errorf("open %q: %w", filepath.Base(inputPath), err)
	}
	return nil
}

// readParts opens the reader over the given source and drains every Part into a
// slice, recording reader-stage trace events when a Recorder is configured. It
// closes the reader before returning (for daemon-backed plugin formats this
// lets the daemon enter its idle state). This is the buffered read path used by
// non-streaming readers and the structural-grid cross-format case; streaming
// readers feed the executor directly via feedReader instead.
func (r *FileRunner) readParts(ctx context.Context, reader format.DataFormatReader, source io.ReadCloser, inputPath, targetLang string) (parts []*model.Part, err error) {
	if err := r.openReader(ctx, reader, source, inputPath, targetLang); err != nil {
		return nil, err
	}

	for result := range reader.Read(ctx) {
		if result.Error != nil {
			reader.Close()
			return nil, fmt.Errorf("read %q: %w", filepath.Base(inputPath), result.Error)
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
	return parts, nil
}

// cachedReadParts is readParts with the document cache in front: on a cache hit
// it returns the file's Part stream from a prior parse (under the same config)
// without opening the reader; on a miss it parses, stores the result, and
// returns it. Used by the process-only project path, where there is no writer or
// skeleton round-trip — only the parts feeding the executor matter, so replaying
// a cached parse is exact. It opens the source file itself (readParts' caller
// did) so the open is skipped entirely on a hit.
func (r *FileRunner) cachedReadParts(ctx context.Context, reader format.DataFormatReader, inputPath, targetLang string) ([]*model.Part, error) {
	key := r.partCacheKey(reader.Name(), "run")
	if r.cfg.PartCache != nil {
		if parts, _, _, ok := r.cfg.PartCache.GetDocument(inputPath, key); ok {
			reader.Close() // never opened — release the unused reader
			// Replay reader-stage trace events so --trace is identical to a parse.
			if r.cfg.Recorder != nil {
				for _, p := range parts {
					if p != nil && p.Resource != nil {
						r.cfg.Recorder.SnapshotPart(p, "reader", "initial")
						r.cfg.Recorder.Record(TraceExit, "reader", p.Resource.ResourceID(), nil)
					}
				}
			}
			return parts, nil
		}
	}

	source, err := openBudgetedFile(inputPath)
	if err != nil {
		reader.Close()
		return nil, fmt.Errorf("open %q: %w", filepath.Base(inputPath), err)
	}
	parts, err := r.readParts(ctx, reader, source, inputPath, targetLang)
	if err != nil {
		return nil, err
	}
	if r.cfg.PartCache != nil {
		r.cfg.PartCache.PutDocument(inputPath, key, parts, nil, reader.Name())
	}
	return parts, nil
}

// cacheableWriter reports whether a same-format writer reconstructs output from
// the content model (+ skeleton bytes) alone, so the file-writing path may
// replay it from the document cache. Writers that re-read the original source —
// by path (SourcePathSetter) or as raw bytes (OriginalContentSetter), i.e. the
// packaged/binary formats — are excluded; they stay on the live read path.
func cacheableWriter(w format.DataFormatWriter) bool {
	if _, ok := w.(format.SourcePathSetter); ok {
		return false
	}
	if _, ok := w.(format.OriginalContentSetter); ok {
		return false
	}
	return true
}

// cachedFileWrite serves the same-format file-writing path through the document
// cache. On a hit it replays the cached Part stream — and, for a skeleton-backed
// writer, the cached skeleton bytes — straight to the writer (the reader never
// runs). On a miss it parses once with a snapshottable skeleton store, captures
// the parts + skeleton bytes into the cache, then writes from that snapshot — the
// same from-bytes reconstruction a hit uses, so the first and subsequent runs are
// byte-identical (and identical to the uncached buffered path). It always handles
// the run (the caller gated on cacheableWriter).
func (r *FileRunner) cachedFileWrite(ctx context.Context, flowName string, tools []tool.Tool, inputPath, outputPath, targetLang string, reader format.DataFormatReader, writer format.DataFormatWriter) error {
	key := r.partCacheKey(reader.Name(), "write")
	consumer, isConsumer := writer.(format.SkeletonStoreConsumer)

	// wireSkeleton attaches a fresh from-bytes skeleton store to the writer when
	// the writer consumes one and we captured skeleton bytes. Returns the store
	// (or nil) so runPipelineToWriter can close it.
	wireSkeleton := func(skel []byte) *format.SkeletonStore {
		if !isConsumer || len(skel) == 0 {
			return nil
		}
		store := format.NewSkeletonStoreFromBytes(skel)
		consumer.SetSkeletonStore(store)
		return store
	}

	// Hit: replay parts (+ skeleton) to the writer without opening the reader.
	if parts, skel, _, ok := r.cfg.PartCache.GetDocument(inputPath, key); ok {
		reader.Close()
		store := wireSkeleton(skel)
		return r.runPipelineToWriter(ctx, flowName, tools, sliceFeeder(parts), outputPath, targetLang, writer, store, "", nil)
	}

	// Miss: parse buffered with a snapshottable (file-backed) skeleton store wired
	// to the reader, so a skeleton-backed format captures its reconstruction bytes.
	var captureStore *format.SkeletonStore
	if emitter, ok := reader.(format.SkeletonStoreEmitter); ok && isConsumer {
		if s, serr := format.NewSkeletonStore(); serr == nil {
			captureStore = s
			captureStore.SetOriginFormat(reader.Name())
			emitter.SetSkeletonStore(captureStore)
		}
	}

	source, oerr := openBudgetedFile(inputPath)
	if oerr != nil {
		reader.Close()
		if captureStore != nil {
			captureStore.Close()
		}
		return fmt.Errorf("open %q: %w", filepath.Base(inputPath), oerr)
	}
	parts, perr := r.readParts(ctx, reader, source, inputPath, targetLang) // closes reader, fills captureStore
	if perr != nil {
		if captureStore != nil {
			captureStore.Close()
		}
		return perr
	}

	var skelBytes []byte
	if captureStore != nil {
		if b, berr := captureStore.Bytes(); berr == nil {
			skelBytes = b
		}
		captureStore.Close()
	}
	r.cfg.PartCache.PutDocument(inputPath, key, parts, skelBytes, reader.Name())

	store := wireSkeleton(skelBytes)
	return r.runPipelineToWriter(ctx, flowName, tools, sliceFeeder(parts), outputPath, targetLang, writer, store, "", nil)
}

// partCacheKey is the document-cache key for a parse: the per-file detected
// format, a kind namespace, and the caller's config fingerprint (format config +
// source locale). The kind keeps the process-only entries ("run", parts only,
// no skeleton) separate from the file-writing entries ("write", parts + skeleton)
// so a skeleton-less process-only entry is never served to a writer that needs
// the skeleton. Returns "" when no cache is configured (the key is then unused).
func (r *FileRunner) partCacheKey(formatName, kind string) string {
	if r.cfg.PartCache == nil {
		return ""
	}
	return formatName + "|" + kind + "|" + r.cfg.PartCacheKey
}

// feedReader streams the reader's Parts directly into inCh (phase 2/3): the
// reader runs concurrently with the executor and writer, so neither the whole
// input nor the whole Part stream is buffered. It records reader-stage trace
// events inline, closes inCh and (for a streaming skeleton) its write side when
// the reader's channel drains, and closes the reader. A read error is returned
// via errOut. Used only for StreamingReader formats, which are in-process and
// pure-Go, so overlapping the read with the write is safe (no daemon "one
// Process stream at a time" constraint).
func (r *FileRunner) feedReader(ctx context.Context, feedCtx context.Context, reader format.DataFormatReader, skel *format.SkeletonStore, inCh chan<- *model.Part, errOut *error) {
	defer func() {
		close(inCh)
		if skel != nil {
			skel.CloseWrite()
		}
		reader.Close()
	}()
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			*errOut = fmt.Errorf("read: %w", result.Error)
			return
		}
		if result.Part == nil {
			continue
		}
		if r.cfg.Recorder != nil && result.Part.Resource != nil {
			id := result.Part.Resource.ResourceID()
			r.cfg.Recorder.SnapshotPart(result.Part, "reader", "initial")
			r.cfg.Recorder.Record(TraceExit, "reader", id, nil)
		}
		select {
		case inCh <- result.Part:
		case <-feedCtx.Done():
			return
		}
	}
}

// newExecutor builds the executor, wiring the configured block store when set.
// Shared by the file-writing and process-only paths so both run the tool chain
// against the same store (SessionTools commit overlays on a persistent store).
func (r *FileRunner) newExecutor() *DefaultExecutor {
	var execOpts []ExecutorOption
	if r.cfg.Store != nil {
		execOpts = append(execOpts, WithBlockStore(r.cfg.Store))
	}
	return NewExecutor(execOpts...)
}

// RunFileToStore reads and parses the input via the reader, runs the tool chain
// against the configured persistent Store so SessionTools commit
// `targets/<locale>` overlays, commits the session, and writes NO output file
// (AD-026 §3 — a process-only run). It requires a non-nil persistent Store: the
// whole point is to land the work as overlays for a later `merge` / `export`.
//
// The caller configures the reader (presets, project defaults) before calling,
// exactly as for RunFileWithReaderWriter. The reader is closed by this method.
func (r *FileRunner) RunFileToStore(ctx context.Context, flowName string, tools []tool.Tool, inputPath, targetLang string, reader format.DataFormatReader) error {
	if r.cfg.Store == nil {
		reader.Close()
		return errors.New("process-only run requires a persistent block store; none configured")
	}
	if !r.cfg.Store.Capabilities().Persistent {
		reader.Close()
		return errors.New("process-only run requires a persistent block store; the configured store is ephemeral")
	}

	parts, err := r.cachedReadParts(ctx, reader, inputPath, targetLang)
	if err != nil {
		return err
	}

	fb := NewFlow(flowName)
	for _, t := range tools {
		fb.AddTool(t)
	}
	// Append the implicit commit-targets step so channel-based translate tools
	// (recycle and other capability-typed Produce BaseTools that set the
	// target on the block but don't implement SessionTool) have their work
	// persisted as targets/<locale> overlays for a later merge. Bespoke
	// SessionTools already wrote their overlay; this step idempotently re-affirms
	// the same key from the block's final target text.
	fb.AddTool(newCommitTargetsTool(model.LocaleID(targetLang)))
	f, err := fb.Build()
	if err != nil {
		return fmt.Errorf("build flow: %w", err)
	}

	executor := r.newExecutor()

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

	// Drain (and discard) the executor output — there is no sink. Draining is
	// required so the executor's tool goroutines can make progress and so
	// ExecuteWithChannels' wait() commits the session. When tracing, emit a
	// writer enter/exit per Part so the trace shape matches a file run.
	for p := range outCh {
		if r.cfg.Recorder != nil && p != nil && p.Resource != nil {
			id := p.Resource.ResourceID()
			r.cfg.Recorder.Record(TraceEnter, "writer", id, nil)
			r.cfg.Recorder.Record(TraceExit, "writer", id, nil)
		}
	}

	waitErr := wait()
	feedCancel()
	<-feedDone
	if waitErr != nil {
		return fmt.Errorf("execute flow: %w", waitErr)
	}
	return nil
}

// RunFileWithReaderWriter runs the pipeline with pre-created reader and writer.
// The caller is responsible for configuring the reader (presets, project
// defaults) before calling. This is the primary integration point for CLI
// and MCP which need to apply format presets and project config.
func (r *FileRunner) RunFileWithReaderWriter(ctx context.Context, flowName string, tools []tool.Tool, inputPath, outputPath, targetLang string, reader format.DataFormatReader, writer format.DataFormatWriter) error {
	sameFormat := reader.Name() == writer.Name()

	// Document-cache fast path (project mode): for a same-format round-trip whose
	// writer reconstructs from the content model + skeleton (not the raw source
	// bytes), a prior parse of this source under the same config replays straight
	// to the writer — parts from the cache, skeleton from the cached bytes — so the
	// reader never runs. The raw-source-coupled binary/markup writers (openxml,
	// odf, epub, idml, html, asciidoc) reconstruct from the original file and stay
	// on the live path below.
	if r.cfg.PartCache != nil && sameFormat && cacheableWriter(writer) {
		return r.cachedFileWrite(ctx, flowName, tools, inputPath, outputPath, targetLang, reader, writer)
	}

	// Wire skeleton store if both support it AND reader and writer are the
	// SAME format. A skeleton holds opaque, format-specific bytes (e.g. the
	// WordprocessingML XML fragments an openxml reader captures); feeding it to
	// a different writer would dump that foreign markup verbatim. For a
	// cross-format conversion (e.g. report.docx → report.md) we deliberately
	// leave the skeleton unwired so the writer reconstructs output from the
	// content model + the structural layer (SemanticRole/role-driven semantic
	// export, WS6) rather than the source's byte skeleton.
	//
	// When both the reader and writer declare streaming capability, the skeleton
	// is a concurrent (channel-backed) store so a streaming round-trip stays
	// bounded-memory; otherwise the file-backed store (the writer buffers the
	// block map). Output is byte-identical either way.
	var skeletonStore *format.SkeletonStore
	if sameFormat {
		if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
			if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
				if isStreamingPair(reader, writer) {
					skeletonStore = format.NewStreamingSkeletonStore()
				} else if store, storeErr := format.NewSkeletonStore(); storeErr == nil {
					skeletonStore = store
				}
				if skeletonStore != nil {
					skeletonStore.SetOriginFormat(reader.Name())
					emitter.SetSkeletonStore(skeletonStore)
					consumer.SetSkeletonStore(skeletonStore)
				}
			}
		}
	}

	// Decide how the writer gets the original source (same-format only; a
	// cross-format writer reconstructs from the content model). A SourcePathSetter
	// reads the file itself by path; an OriginalContentSetter-only writer
	// (OpenXML, AsciiDoc) needs the raw bytes, which we read once and share with
	// the reader (those formats are non-streaming whole-document parsers anyway).
	srcPath := ""
	var preReadContent []byte
	if sameFormat {
		if _, ok := writer.(format.SourcePathSetter); ok && filepath.IsAbs(inputPath) {
			srcPath = inputPath
		} else if _, ok := writer.(format.OriginalContentSetter); ok {
			content, rerr := os.ReadFile(inputPath)
			if rerr != nil {
				reader.Close()
				if skeletonStore != nil {
					skeletonStore.Close()
				}
				return fmt.Errorf("read %q: %w", filepath.Base(inputPath), rerr)
			}
			preReadContent = content
		}
	}

	// Streaming path: a StreamingReader feeds the executor directly, concurrently
	// with the writer, so neither the input nor the Part stream is buffered. This
	// never co-occurs with the structural-grid case (spreadsheet sources are not
	// StreamingReaders) nor with preReadContent (OpenXML/AsciiDoc are not
	// StreamingReaders), so those buffered-only concerns are excluded by
	// construction.
	if _, ok := reader.(format.StreamingReader); ok && preReadContent == nil {
		source, oerr := openBudgetedFile(inputPath)
		if oerr != nil {
			reader.Close()
			if skeletonStore != nil {
				skeletonStore.Close()
			}
			return fmt.Errorf("open %q: %w", filepath.Base(inputPath), oerr)
		}
		if err := r.openReader(ctx, reader, source, inputPath, targetLang); err != nil {
			if skeletonStore != nil {
				skeletonStore.Close()
			}
			return err
		}
		feed := func(feedCtx context.Context, inCh chan<- *model.Part, errOut *error) {
			r.feedReader(ctx, feedCtx, reader, skeletonStore, inCh, errOut)
		}
		return r.runPipelineToWriter(ctx, flowName, tools, feed, outputPath, targetLang, writer, skeletonStore, srcPath, preReadContent)
	}

	// Buffered path (non-streaming readers): read every Part up front, exactly as
	// before. The source still streams into the reader (phase 1) rather than being
	// pre-buffered with a separate os.ReadFile.
	var source io.ReadCloser
	if preReadContent != nil {
		source = budgetedBytes(preReadContent)
	} else {
		s, oerr := openBudgetedFile(inputPath)
		if oerr != nil {
			reader.Close()
			if skeletonStore != nil {
				skeletonStore.Close()
			}
			return fmt.Errorf("open %q: %w", filepath.Base(inputPath), oerr)
		}
		source = s
	}
	parts, err := r.readParts(ctx, reader, source, inputPath, targetLang)
	if err != nil {
		if skeletonStore != nil {
			skeletonStore.Close()
		}
		return err
	}

	if !sameFormat && structuralExportWriters[writer.Name()] {
		// Cross-format export to a structural document writer: synthesize table
		// structure from spreadsheet cell geometry so a worksheet renders as a
		// real table rather than a flat list of cell values. A no-op when the
		// stream carries no cell-grid blocks.
		counter := 0
		parts = structure.SpreadsheetGridToTables(parts, &counter)
	}
	return r.runPipelineToWriter(ctx, flowName, tools, sliceFeeder(parts), outputPath, targetLang, writer, skeletonStore, srcPath, preReadContent)
}

// sliceFeeder returns a feeder that streams a pre-read Part slice into the
// executor and closes the input channel — the buffered (non-streaming) feed.
func sliceFeeder(parts []*model.Part) func(context.Context, chan<- *model.Part, *error) {
	return func(feedCtx context.Context, inCh chan<- *model.Part, _ *error) {
		defer close(inCh)
		for _, p := range parts {
			select {
			case inCh <- p:
			case <-feedCtx.Done():
				return
			}
		}
	}
}

// isStreamingPair reports whether both the reader and writer declare streaming
// capability, so the file-run path can wire a concurrent skeleton store and run
// a bounded-memory round-trip.
func isStreamingPair(reader format.DataFormatReader, writer format.DataFormatWriter) bool {
	_, ro := reader.(format.StreamingReader)
	_, wo := writer.(format.StreamingWriter)
	return ro && wo
}

// RunSkeletonReconstruct runs the tool chain when the raw source is absent but
// a round-trip skeleton is present — the skeleton-only .klz handoff case
// (AD-025 §6). The source's blocks are rebuilt from the skeleton's block refs
// (so their identities match the cached overlays and the merge hydrate step),
// the tool chain runs against the configured Store, and a writer of the given
// format reconstructs byte-exact output from the skeleton. For a transform the
// caller points outputPath at a throwaway file (the persisted work is the
// overlays); for a merge it is the localized destination.
func (r *FileRunner) RunSkeletonReconstruct(ctx context.Context, flowName string, tools []tool.Tool, formatID registry.FormatID, skelBytes []byte, outputPath, targetLang string) error {
	writer, err := r.cfg.FormatReg.NewWriter(formatID)
	if err != nil {
		return fmt.Errorf("no writer for %q: %w", formatID, err)
	}
	consumer, ok := writer.(format.SkeletonStoreConsumer)
	if !ok {
		return fmt.Errorf("format %q cannot reconstruct from a skeleton (no skeleton consumer)", formatID)
	}
	if r.cfg.ConfigureWriter != nil {
		r.cfg.ConfigureWriter(writer)
	}

	parts, err := partsFromSkeleton(skelBytes)
	if err != nil {
		return err
	}

	// A fresh read-mode store drives the writer; partsFromSkeleton consumed its
	// own copy enumerating the block refs.
	skeletonStore := format.NewSkeletonStoreFromBytes(skelBytes)
	consumer.SetSkeletonStore(skeletonStore)

	return r.runPipelineToWriter(ctx, flowName, tools, sliceFeeder(parts), outputPath, targetLang, writer, skeletonStore, "", nil)
}

// partsFromSkeleton rebuilds the translatable blocks a skeleton references,
// one Part per distinct SkeletonRef (layer refs are skipped — embedded layers
// are not reconstructed from overlays). The blocks carry only their identity
// (ID); their source text is unknown (it lived in the dropped raw source), so
// callers rely on cached target overlays to supply content.
func partsFromSkeleton(skelBytes []byte) ([]*model.Part, error) {
	store := format.NewSkeletonStoreFromBytes(skelBytes)
	defer store.Close()
	var parts []*model.Part
	seen := make(map[string]bool)
	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reconstruct from skeleton: %w", err)
		}
		if entry.Type != format.SkeletonRef {
			continue
		}
		id := string(entry.Data)
		if id == "" || strings.HasPrefix(id, "layer:") || seen[id] {
			continue
		}
		seen[id] = true
		parts = append(parts, &model.Part{Type: model.PartBlock, Resource: model.NewBlock(id, "")})
	}
	return parts, nil
}

// runExecuteWrite is the shared core of the write paths: it builds the flow,
// wires the writer's source (path/bytes) and locale, runs the executor while
// feed supplies Parts concurrently, and streams the result through writer to its
// already-configured output. It does NOT close the writer or the skeleton store
// and does NOT finalize the output — the caller owns those, so it can wrap the
// run in atomic temp-file/rename (runPipelineToWriter) or write straight to an
// in-memory/stream sink (RunStream). label names the destination for errors.
func (r *FileRunner) runExecuteWrite(ctx context.Context, flowName string, tools []tool.Tool, feed func(context.Context, chan<- *model.Part, *error), targetLang string, writer format.DataFormatWriter, sourcePath string, inputContent []byte, label string) error {
	fb := NewFlow(flowName)
	for _, t := range tools {
		fb.AddTool(t)
	}
	f, err := fb.Build()
	if err != nil {
		return fmt.Errorf("build flow: %w", err)
	}

	// Pass the source bytes/path to the writer ONLY for same-format
	// conversions (e.g. an HTML reader → HTML writer re-parse, or OpenXML's
	// skeleton-rebuild). The source is in the READER's format; handing it to a
	// different-format writer (e.g. DocLang → HTML) would make the writer
	// re-parse foreign bytes and echo the source markup. For a cross-format
	// conversion — or a skeleton-reconstructed run with no source — the caller
	// passes these empty so the writer reconstructs from the content model +
	// structural layer (role-driven semantic export, WS6) / the skeleton.
	if sps, ok := writer.(format.SourcePathSetter); ok && sourcePath != "" && filepath.IsAbs(sourcePath) {
		sps.SetSourcePath(sourcePath)
	} else if ocs, ok := writer.(format.OriginalContentSetter); ok && len(inputContent) > 0 {
		ocs.SetOriginalContent(inputContent)
	}

	writer.SetLocale(model.LocaleID(targetLang))

	// Single concurrent pipeline: feed Parts into the executor's input channel
	// from a goroutine while the writer drains the executor's output channel
	// directly. For the buffered feeder the reader has already been fully read
	// and closed (preserving the read-then-write ordering daemon-backed plugin
	// formats require). For the streaming feeder the reader runs concurrently
	// here — safe because only in-process StreamingReaders take that path, never
	// a daemon plugin — and a streaming skeleton store lets the writer consume
	// the skeleton interleaved rather than after a Flush.
	executor := r.newExecutor()

	// Derive a cancellable context for the feeder so a tool error (which
	// cancels the executor's own internal context and stops the tools)
	// can also unblock the feeder if it's parked on an inCh send. Without
	// this the feeder goroutine would leak. feedDone lets us join it.
	feedCtx, feedCancel := context.WithCancel(ctx)
	defer feedCancel()

	inCh, outCh, wait := executor.ExecuteWithChannels(ctx, f)

	var feedErr error
	feedDone := make(chan struct{})
	go func() {
		defer close(feedDone)
		// feed closes inCh itself (and, for a streaming feeder, the skeleton
		// store's write side and the reader) so the writer/skeleton see EOF.
		feed(feedCtx, inCh, &feedErr)
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
		return fmt.Errorf("execute flow: %w", waitErr)
	}
	if feedErr != nil {
		return fmt.Errorf("read %q: %w", label, feedErr)
	}
	if writeErr != nil {
		return fmt.Errorf("write %q: %w", label, writeErr)
	}
	return nil
}

// runPipelineToWriter executes the tool chain over the Parts supplied by feed
// and writes the result through writer, finalizing output atomically (temp file
// then rename) so a tool/writer error never leaves a partial destination. feed
// is responsible for closing inCh and reporting any read error via its *error
// argument; a buffered caller ranges a pre-read slice, a streaming caller drives
// the reader concurrently (feedReader). When skeletonStore is non-nil it is
// closed before returning. sourcePath/inputContent are handed to the writer only
// when non-empty (same-format runs); skeleton-reconstructed runs pass them empty.
func (r *FileRunner) runPipelineToWriter(ctx context.Context, flowName string, tools []tool.Tool, feed func(context.Context, chan<- *model.Part, *error), outputPath, targetLang string, writer format.DataFormatWriter, skeletonStore *format.SkeletonStore, sourcePath string, inputContent []byte) error {
	if skeletonStore != nil {
		defer skeletonStore.Close()
	}
	label := filepath.Base(outputPath)

	// Ensure output directory exists.
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
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
	// The executor and writer run concurrently — the writer drains the tool
	// output channel directly. Because output is produced incrementally, a
	// tool/writer error could leave a partial file at outputPath; the
	// temp-then-rename keeps the destination all-or-nothing, matching the
	// pre-S1 contract where a tool error produced no output file at all.
	tmpFile, err := os.CreateTemp(filepath.Dir(outputPath), ".kapi-out-*")
	if err != nil {
		return fmt.Errorf("set output: %w", err)
	}
	tmpPath := tmpFile.Name()
	failTmp := func(format string, args ...any) error {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf(format, args...)
	}

	bw := bufio.NewWriterSize(tmpFile, 64*1024)
	if err := writer.SetOutputWriter(bw); err != nil {
		return failTmp("set output: %w", err)
	}

	if err := r.runExecuteWrite(ctx, flowName, tools, feed, targetLang, writer, sourcePath, inputContent, label); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return err
	}

	// Close the writer first (lets writers that emit on Close, like KLF,
	// finish writing into the buffer), then flush the buffer to the file,
	// then close the file, then rename into place. Any error removes the
	// temp file so outputPath is never left partial.
	if cerr := writer.Close(); cerr != nil {
		return failTmp("close writer %q: %w", label, cerr)
	}
	if ferr := bw.Flush(); ferr != nil {
		return failTmp("flush %q: %w", label, ferr)
	}
	if ferr := tmpFile.Close(); ferr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close %q: %w", label, ferr)
	}
	if rerr := os.Rename(tmpPath, outputPath); rerr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("finalize %q: %w", label, rerr)
	}
	return nil
}

// RunStream runs the full read → process → write pipeline from an in-memory /
// streamed source to an io.Writer, with no temporary files for the document
// itself (AD-026 §6). It is the stream-based twin of RunFile: the container
// binding drives one archive entry through it (bytes in, bytes out) without
// staging the entry on disk, and for a streaming-capable inner format the entry
// is never even buffered whole. fmtID is the already-detected format of the
// content (a container knows each entry's format); reader and writer are created
// and configured exactly as RunFile does.
func (r *FileRunner) RunStream(ctx context.Context, flowName string, tools []tool.Tool, in io.Reader, srcURI string, fmtID registry.FormatID, out io.Writer, targetLang string) error {
	reg := r.cfg.FormatReg
	reader, err := reg.NewReader(fmtID)
	if err != nil {
		return fmt.Errorf("no reader for %q: %w", fmtID, err)
	}
	if r.cfg.ConfigureReader != nil {
		if err := r.cfg.ConfigureReader(reader, fmtID); err != nil {
			reader.Close()
			return fmt.Errorf("configure reader for %q: %w", fmtID, err)
		}
	}
	writer, err := reg.NewWriter(fmtID)
	if err != nil {
		reader.Close()
		return fmt.Errorf("no writer for %q: %w", fmtID, err)
	}
	if r.cfg.ConfigureWriter != nil {
		r.cfg.ConfigureWriter(writer)
	}

	// Same format in and out (a container round-trips each entry), so wire a
	// skeleton exactly as the file path does — streaming when both ends opt in.
	var skeletonStore *format.SkeletonStore
	if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
		if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
			if isStreamingPair(reader, writer) {
				skeletonStore = format.NewStreamingSkeletonStore()
			} else if store, serr := format.NewSkeletonStore(); serr == nil {
				skeletonStore = store
			}
			if skeletonStore != nil {
				skeletonStore.SetOriginFormat(reader.Name())
				emitter.SetSkeletonStore(skeletonStore)
				consumer.SetSkeletonStore(skeletonStore)
			}
		}
	}
	if skeletonStore != nil {
		defer skeletonStore.Close()
	}

	// A writer that needs the original bytes (OpenXML/AsciiDoc) is not a
	// StreamingReader's writer, so it only co-occurs with the buffered path;
	// materialise the source once and share it with the reader.
	var preReadContent []byte
	if _, ok := writer.(format.OriginalContentSetter); ok {
		if _, isSP := writer.(format.SourcePathSetter); !isSP {
			content, rerr := io.ReadAll(safeio.DefaultBudget().Reader(in))
			if rerr != nil {
				reader.Close()
				return fmt.Errorf("read %q: %w", srcURI, rerr)
			}
			preReadContent = content
		}
	}

	if err := writer.SetOutputWriter(out); err != nil {
		reader.Close()
		return fmt.Errorf("set output: %w", err)
	}

	label := filepath.Base(srcURI)
	if _, ok := reader.(format.StreamingReader); ok && preReadContent == nil {
		// Streaming inner format: feed the reader directly from the source
		// stream — the entry is never buffered whole.
		if err := r.openReader(ctx, reader, &budgetedSource{Reader: safeio.DefaultBudget().Reader(in)}, srcURI, targetLang); err != nil {
			return err
		}
		feed := func(feedCtx context.Context, inCh chan<- *model.Part, errOut *error) {
			r.feedReader(ctx, feedCtx, reader, skeletonStore, inCh, errOut)
		}
		if err := r.runExecuteWrite(ctx, flowName, tools, feed, targetLang, writer, "", preReadContent, label); err != nil {
			return err
		}
		return writer.Close()
	}

	// Buffered inner format.
	var source io.ReadCloser
	if preReadContent != nil {
		source = budgetedBytes(preReadContent)
	} else {
		source = &budgetedSource{Reader: safeio.DefaultBudget().Reader(in)}
	}
	parts, err := r.readParts(ctx, reader, source, srcURI, targetLang)
	if err != nil {
		return err
	}
	if err := r.runExecuteWrite(ctx, flowName, tools, sliceFeeder(parts), targetLang, writer, "", preReadContent, label); err != nil {
		return err
	}
	return writer.Close()
}

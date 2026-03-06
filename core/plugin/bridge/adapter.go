package bridge

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/shared"
)

// BridgeFormatReader implements format.DataFormatReader by delegating to
// a Java bridge subprocess running an Okapi filter. It acquires a bridge
// from the pool on Open and releases it on Close, ensuring exclusive
// access to the stateful JVM filter for the entire lifecycle.
type BridgeFormatReader struct {
	format.BaseFormatReader
	pool         *BridgePool
	cfg          BridgeConfig
	bridge       *JavaBridge // acquired from pool during Open
	filterClass  string
	filterParams map[string]any // optional filter parameters
	info         *InfoData
	content      []byte // raw document content
	sourcePath   string // absolute file path for direct disk access
}

var _ format.DataFormatReader = (*BridgeFormatReader)(nil)

// NewBridgeFormatReader creates a reader that acquires bridges from the pool.
// The cfg specifies which JAR to use when acquiring a bridge.
func NewBridgeFormatReader(pool *BridgePool, cfg BridgeConfig, filterClass string) *BridgeFormatReader {
	return &BridgeFormatReader{
		pool:        pool,
		cfg:         cfg,
		filterClass: filterClass,
	}
}

// SetFilterParams sets optional filter-specific parameters.
// These are passed to the Java bridge when opening the filter.
func (r *BridgeFormatReader) SetFilterParams(params map[string]any) {
	r.filterParams = params
}

// Signature returns the format detection signature from the Java filter.
// It acquires and releases a bridge just for the info query.
func (r *BridgeFormatReader) Signature() format.FormatSignature {
	info, err := r.fetchInfo()
	if err != nil {
		return format.FormatSignature{}
	}
	return format.FormatSignature{
		MIMETypes:  info.MimeTypes,
		Extensions: info.Extensions,
	}
}

// Open reads the document content and sends it to the Java bridge.
// It acquires a bridge from the pool, which is released by Close.
func (r *BridgeFormatReader) Open(_ context.Context, doc *model.RawDocument) error {
	b, err := r.pool.Acquire(r.cfg)
	if err != nil {
		return fmt.Errorf("acquiring bridge: %w", err)
	}

	r.bridge = b
	r.Doc = doc

	// Detect absolute file paths for direct disk access.
	// This enables relative URI resolution for auxiliary files (e.g. ITS standoff annotations).
	var content []byte
	var sourcePath string

	if filepath.IsAbs(doc.URI) {
		if _, serr := os.Stat(doc.URI); serr == nil {
			sourcePath = doc.URI
		}
	}

	// When source_path is available, Java reads from disk via content_ref.
	// Only read content bytes as fallback for non-file-based documents.
	if sourcePath == "" && doc.Reader != nil {
		content, err = io.ReadAll(doc.Reader)
		if err != nil {
			r.pool.Release(b)
			r.bridge = nil
			return fmt.Errorf("reading document content: %w", err)
		}
	}
	r.content = content
	r.sourcePath = sourcePath

	if err := r.bridge.Open(OpenParams{
		FilterClass:  r.filterClass,
		URI:          doc.URI,
		SourceLocale: string(doc.SourceLocale),
		TargetLocale: string(doc.TargetLocale),
		Encoding:     doc.Encoding,
		Content:      content,
		MimeType:     doc.MimeType,
		FilterParams: r.filterParams,
		SourcePath:   sourcePath,
	}); err != nil {
		r.pool.Release(b)
		r.bridge = nil
		return err
	}
	return nil
}

// Read sends a read command to the bridge and emits Parts on the returned channel.
func (r *BridgeFormatReader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult)
	go func() {
		defer close(ch)

		msgs, err := r.bridge.Read()
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("bridge read: %w", err)}
			return
		}

		for _, msg := range msgs {
			part := shared.ProtoToPart(msg)
			select {
			case ch <- model.PartResult{Part: part}:
			case <-ctx.Done():
				ch <- model.PartResult{Error: ctx.Err()}
				return
			}
		}
	}()
	return ch
}

// Close releases the filter resources in the Java bridge and returns
// the bridge to the pool.
func (r *BridgeFormatReader) Close() error {
	if r.bridge == nil {
		return nil
	}
	err := r.bridge.CloseFilter()
	r.pool.Release(r.bridge)
	r.bridge = nil
	return err
}

// fetchInfo caches and returns the filter's metadata.
// It acquires and releases a bridge from the pool for the info query.
func (r *BridgeFormatReader) fetchInfo() (*InfoData, error) {
	if r.info != nil {
		return r.info, nil
	}
	b, err := r.pool.Acquire(r.cfg)
	if err != nil {
		return nil, fmt.Errorf("acquiring bridge for info: %w", err)
	}
	defer r.pool.Release(b)

	info, err := b.Info(r.filterClass)
	if err != nil {
		return nil, err
	}
	r.info = info
	r.FormatName = info.Name
	r.FormatDisplayName = info.DisplayName
	return info, nil
}

// BridgeFormatWriter implements format.DataFormatWriter by delegating to
// a Java bridge subprocess running an Okapi filter writer.
type BridgeFormatWriter struct {
	format.BaseFormatWriter
	pool            *BridgePool
	cfg             BridgeConfig
	filterClass     string
	filterParams    map[string]any // optional filter parameters
	originalContent []byte         // original document needed by Okapi for skeleton
	sourcePath      string         // absolute file path for direct disk access
	outputPath      string         // captured from SetOutput for direct disk writing
}

var _ format.DataFormatWriter = (*BridgeFormatWriter)(nil)

// NewBridgeFormatWriter creates a writer that acquires bridges from the pool.
// The cfg specifies which JAR to use when acquiring a bridge.
func NewBridgeFormatWriter(pool *BridgePool, cfg BridgeConfig, filterClass string) *BridgeFormatWriter {
	return &BridgeFormatWriter{
		pool:        pool,
		cfg:         cfg,
		filterClass: filterClass,
	}
}

// SetOutput configures the output destination by file path.
// The path is captured for direct disk writing via output_ref.
func (w *BridgeFormatWriter) SetOutput(path string) error {
	w.outputPath = path
	return w.BaseFormatWriter.SetOutput(path)
}

// SetFilterParams sets optional filter-specific parameters.
// These are passed to the Java bridge when writing.
func (w *BridgeFormatWriter) SetFilterParams(params map[string]any) {
	w.filterParams = params
}

// SetOriginalContent sets the original document content, which Okapi needs
// as a skeleton for the filter writer.
func (w *BridgeFormatWriter) SetOriginalContent(content []byte) {
	w.originalContent = content
}

// SetSourcePath sets the absolute file path for direct disk access.
// When set, Java reads from this path instead of receiving content bytes.
func (w *BridgeFormatWriter) SetSourcePath(path string) {
	w.sourcePath = path
}

// Write streams parts from the channel directly to the Java bridge, which
// applies translations on-demand as it re-reads the original document skeleton.
// No parts are accumulated in memory — streaming keeps memory constant regardless
// of document size.
//
// For XLIFF filters, empty target-language attributes are stripped from the
// original content before sending to avoid duplicate attributes in Okapi's output.
func (w *BridgeFormatWriter) Write(ctx context.Context, parts <-chan *model.Part) error {
	b, err := w.pool.Acquire(w.cfg)
	if err != nil {
		return fmt.Errorf("acquiring bridge for write: %w", err)
	}
	defer w.pool.Release(b)

	originalContent := w.originalContent
	sourcePath := w.sourcePath

	// For XLIFF filters, strip empty target-language attributes to prevent
	// Okapi from producing duplicate attributes in the output XML.
	if isXLIFFFilter(w.filterClass) {
		if sourcePath != "" {
			// File-based path: create a temp file with stripping applied.
			stripped, cleanup, serr := stripEmptyTargetLanguageFile(sourcePath)
			if serr != nil {
				return fmt.Errorf("preprocessing XLIFF source: %w", serr)
			}
			if cleanup != nil {
				defer cleanup()
			}
			sourcePath = stripped
		} else if len(originalContent) > 0 {
			// Byte-based path: strip from in-memory content.
			originalContent = stripEmptyTargetLanguage(originalContent)
		}
	}

	result, err := b.WriteStream(ctx, WriteStreamParams{
		FilterClass:     w.filterClass,
		Locale:          string(w.Locale),
		Encoding:        w.Encoding,
		OriginalContent: originalContent,
		FilterParams:    w.filterParams,
		SourcePath:      sourcePath,
		OutputPath:      w.outputPath,
	}, parts)
	if err != nil {
		return fmt.Errorf("bridge write: %w", err)
	}

	// When output_ref was used, Java wrote directly to disk — no bytes to copy.
	// When inline bytes are returned, copy them to the output writer.
	if result.OutputPath == "" && w.Output != nil && len(result.Output) > 0 {
		if _, err := io.Copy(w.Output, bytes.NewReader(result.Output)); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
	}

	return nil
}

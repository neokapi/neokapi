package bridge

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// BridgeFormatReader implements format.DataFormatReader by delegating to
// a Java bridge subprocess running an Okapi filter via the Process RPC.
// It acquires a bridge from the registry for the duration of a read operation.
type BridgeFormatReader struct {
	format.BaseFormatReader
	registry     *BridgeRegistry
	cfg          BridgeConfig
	filterClass  string
	sig          format.FormatSignature // pre-populated from schema metadata
	filterParams map[string]any         // optional filter parameters
	content      []byte                 // raw document content (fallback when no file path)
	sourcePath   string                 // absolute file path for direct disk access
}

var _ format.DataFormatReader = (*BridgeFormatReader)(nil)

// NewBridgeFormatReader creates a reader that acquires bridges from the registry.
// The sig is pre-populated from schema metadata so no JVM query is needed.
func NewBridgeFormatReader(registry *BridgeRegistry, cfg BridgeConfig, filterClass string, sig format.FormatSignature) *BridgeFormatReader {
	return &BridgeFormatReader{
		registry:    registry,
		cfg:         cfg,
		filterClass: filterClass,
		sig:         sig,
	}
}

// SetFilterParams sets optional filter-specific parameters.
func (r *BridgeFormatReader) SetFilterParams(params map[string]any) {
	r.filterParams = params
}

// Signature returns the format detection signature from schema metadata.
func (r *BridgeFormatReader) Signature() format.FormatSignature {
	return r.sig
}

// Open stores document metadata for the subsequent Read call.
// It does NOT start the JVM — that happens lazily on Read.
func (r *BridgeFormatReader) Open(_ context.Context, doc *model.RawDocument) error {
	r.Doc = doc

	if filepath.IsAbs(doc.URI) {
		if _, err := os.Stat(doc.URI); err == nil {
			r.sourcePath = doc.URI
		}
	}

	if r.sourcePath == "" && doc.Reader != nil {
		var err error
		r.content, err = io.ReadAll(doc.Reader)
		if err != nil {
			return fmt.Errorf("reading document content: %w", err)
		}
	}

	return nil
}

// Read starts the Process RPC in read-only mode (no output config) and
// streams parts from the Java bridge to the returned channel.
func (r *BridgeFormatReader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult)
	go func() {
		defer close(ch)

		bridge, release, err := r.registry.Acquire(r.cfg)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("acquiring bridge: %w", err)}
			return
		}
		defer release()

		_, err = bridge.Process(ctx, ProcessParams{
			FilterClass:  r.filterClass,
			SourceLocale: string(r.Doc.SourceLocale),
			TargetLocale: string(r.Doc.TargetLocale),
			Encoding:     r.Doc.Encoding,
			MimeType:     r.Doc.MimeType,
			FilterParams: r.filterParams,
			Content:      r.content,
			SourcePath:   r.sourcePath,
		}, func(parts <-chan *model.Part, done <-chan struct{}) <-chan *model.Part {
			for part := range parts {
				select {
				case ch <- model.PartResult{Part: part}:
				case <-ctx.Done():
					ch <- model.PartResult{Error: ctx.Err()}
					return nil
				}
			}
			return nil
		})
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("bridge read: %w", err)}
		}
	}()
	return ch
}

// Close is a no-op — the bridge is released when Read completes.
func (r *BridgeFormatReader) Close() error {
	return nil
}

// NewProcessor creates a BridgeProcessor sharing this reader's registry and config.
func (r *BridgeFormatReader) NewProcessor() *BridgeProcessor {
	return &BridgeProcessor{
		registry:     r.registry,
		cfg:          r.cfg,
		filterClass:  r.filterClass,
		filterParams: r.filterParams,
	}
}

// BridgeProcessor runs a single-pass Okapi pipeline where Go participates
// as a step. Java reads each event, sends the part to Go, receives the
// processed part back, applies translations, and writes — all in a single
// filter iteration. One read, one write, no document re-read.
type BridgeProcessor struct {
	registry     *BridgeRegistry
	cfg          BridgeConfig
	filterClass  string
	filterParams map[string]any
}

// NewBridgeProcessor creates a processor for the given filter configuration.
func NewBridgeProcessor(registry *BridgeRegistry, cfg BridgeConfig, filterClass string) *BridgeProcessor {
	return &BridgeProcessor{
		registry:    registry,
		cfg:         cfg,
		filterClass: filterClass,
	}
}

// SetFilterParams sets optional filter-specific parameters.
func (p *BridgeProcessor) SetFilterParams(params map[string]any) {
	p.filterParams = params
}

// ProcessExecuteParams configures a single-pass Process execution.
type ProcessExecuteParams struct {
	InputPath      string  // absolute file path (preferred)
	Content        []byte  // inline content (fallback)
	OutputPath     string  // output file path (Java writes to disk)
	SourceLocale   string
	TargetLocale   string
	OutputLocale   string
	Encoding       string
	MimeType       string
	SubscribeParts []int32 // Part types to stream to Go (empty = all)
}

// Execute runs the single-pass pipeline. The processFn receives parts from
// Java's read phase and returns processed parts. For each TEXT_UNIT, Java
// blocks until the processed part arrives, applies the translation, and
// writes the event immediately — all in one filter iteration.
//
// If processFn is nil, parts pass through unmodified (identity roundtrip).
func (p *BridgeProcessor) Execute(ctx context.Context, params ProcessExecuteParams,
	processFn func(parts <-chan *model.Part) <-chan *model.Part,
) (*ProcessResult, error) {

	bridge, release, err := p.registry.Acquire(p.cfg)
	if err != nil {
		return nil, fmt.Errorf("acquiring bridge: %w", err)
	}
	defer release()

	sourcePath := params.InputPath
	if sourcePath != "" && !filepath.IsAbs(sourcePath) {
		if abs, err := filepath.Abs(sourcePath); err == nil {
			sourcePath = abs
		}
	}
	content := params.Content

	// For XLIFF filters, strip empty target-language attributes.
	if isXLIFFFilter(p.filterClass) {
		if sourcePath != "" {
			stripped, cleanup, serr := stripEmptyTargetLanguageFile(sourcePath)
			if serr != nil {
				return nil, fmt.Errorf("preprocessing XLIFF source: %w", serr)
			}
			if cleanup != nil {
				defer cleanup()
			}
			sourcePath = stripped
		} else if len(content) > 0 {
			content = stripEmptyTargetLanguage(content)
		}
	}

	return bridge.Process(ctx, ProcessParams{
		FilterClass:    p.filterClass,
		SourceLocale:   params.SourceLocale,
		TargetLocale:   params.TargetLocale,
		Encoding:       params.Encoding,
		MimeType:       params.MimeType,
		FilterParams:   p.filterParams,
		Content:        content,
		SourcePath:     sourcePath,
		OutputPath:     absPath(params.OutputPath),
		OutputLocale:   params.OutputLocale,
		SubscribeParts: params.SubscribeParts,
	}, func(parts <-chan *model.Part, done <-chan struct{}) <-chan *model.Part {
		if processFn == nil {
			// Identity: pass parts through unmodified.
			out := make(chan *model.Part, 64)
			go func() {
				defer close(out)
				for p := range parts {
					out <- p
				}
			}()
			return out
		}
		return processFn(parts)
	})
}

// ExecuteWithWriter runs Execute and copies inline output bytes to w.
// When the Java side writes to a file path (OutputPath set), this is a no-op
// on the writer side.
func (p *BridgeProcessor) ExecuteWithWriter(ctx context.Context, params ProcessExecuteParams,
	processFn func(parts <-chan *model.Part) <-chan *model.Part,
	w io.Writer,
) error {
	result, err := p.Execute(ctx, params, processFn)
	if err != nil {
		return err
	}
	if result.OutputPath == "" && w != nil && len(result.Output) > 0 {
		if _, err := io.Copy(w, bytes.NewReader(result.Output)); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
	}
	return nil
}

// absPath resolves a path to absolute, returning it unchanged if empty or already absolute.
func absPath(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

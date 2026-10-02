package pluginhost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// daemonReader implements format.DataFormatReader by routing the
// document through a Mode-C daemon's BridgeService.Process RPC.
//
// One reader instance corresponds to one Open/Read/Close cycle. The
// reader acquires a DaemonClient from the pool on Read and lets the
// daemon stream parts back over the bidirectional Process stream. The
// reader signals "no output" by omitting OutputRef in the header — the
// daemon then enters read-only mode.
type daemonReader struct {
	format.BaseFormatReader

	pool       *DaemonPool
	plugin     *Plugin
	formatName string
	signature  format.FormatSignature

	// content / sourcePath are filled in by Open. We prefer sourcePath
	// (the daemon reads directly from disk) and only fall back to bytes
	// streamed inline.
	content    []byte
	sourcePath string
}

// newDaemonReader constructs a reader bound to a specific plugin.
// mapConfig is a generic string-map DataFormatConfig for plugin formats: the
// host applies a config map (CLI flags, desktop config, recipe options) onto it,
// and the daemon reader forwards the entries verbatim as ProcessHeader
// FilterParams. Without this, plugin-format config (e.g. PDF "geometry"/"glyphs",
// or any okapi-bridge filter parameter) never reached the daemon.
type mapConfig struct {
	format string
	params map[string]string
}

func (c *mapConfig) FormatName() string { return c.format }
func (c *mapConfig) Reset()             { c.params = map[string]string{} }
func (c *mapConfig) Validate() error    { return nil }
func (c *mapConfig) ApplyMap(values map[string]any) error {
	for k, v := range values {
		// A LIST is marshalled as JSON; everything else keeps fmt.Sprint.
		//
		// fmt.Sprint on a slice yields Go's debug rendering — `[desc caveats]` —
		// which no plugin can parse back, and which silently produced an
		// extraction that matched nothing when the sourcecode format's
		// nodePathPatterns first crossed this boundary. Scalars keep their
		// existing spelling on purpose: a bridge filter parameter reading
		// "true" or "42" must not start seeing a quoted JSON string.
		switch v.(type) {
		case []any, []string:
			b, err := json.Marshal(v)
			if err != nil {
				return fmt.Errorf("%s: encode %s: %w", c.format, k, err)
			}
			c.params[k] = string(b)
		default:
			c.params[k] = fmt.Sprint(v)
		}
	}
	return nil
}

func newDaemonReader(pool *DaemonPool, plugin *Plugin, formatName string, sig format.FormatSignature, displayName string) *daemonReader {
	r := &daemonReader{
		pool:       pool,
		plugin:     plugin,
		formatName: formatName,
		signature:  sig,
	}
	r.FormatName = formatName
	r.FormatDisplayName = displayName
	r.FormatExtensions = sig.Extensions
	r.FormatMimeType = ""
	if len(sig.MIMETypes) > 0 {
		r.FormatMimeType = sig.MIMETypes[0]
	}
	r.Cfg = &mapConfig{format: formatName, params: map[string]string{}}
	return r
}

// Signature returns the format detection signature.
func (r *daemonReader) Signature() format.FormatSignature {
	return r.signature
}

// Open captures the document for the upcoming Read call. It does not
// open a daemon connection — that happens lazily in Read.
func (r *daemonReader) Open(_ context.Context, doc *model.RawDocument) error {
	r.Doc = doc

	// Prefer a real on-disk path so the daemon can resolve companion files
	// (linked rules, standoff XML, etc.).
	if filepath.IsAbs(doc.URI) {
		if _, err := os.Stat(doc.URI); err == nil {
			r.sourcePath = doc.URI
		}
	}
	if r.sourcePath == "" && doc.Reader != nil {
		var err error
		r.content, err = io.ReadAll(doc.Reader)
		if err != nil {
			return fmt.Errorf("read document content: %w", err)
		}
	}
	return nil
}

// Read drives a Process RPC against the plugin's Mode-C daemon and
// streams parts on the returned channel. Read-only mode: header omits
// OutputRef, so the daemon completes after sending ReadDone (and a
// final ProcessComplete).
func (r *daemonReader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		emit := func(result model.PartResult) bool {
			select {
			case ch <- result:
				return true
			case <-ctx.Done():
				return false
			}
		}

		client, err := r.pool.Acquire(ctx, r.plugin)
		if err != nil {
			emit(model.PartResult{Error: fmt.Errorf("acquire daemon for plugin %q: %w", r.plugin.Name(), err)})
			return
		}
		bridgeClient := pb.NewBridgeServiceClient(client.Conn)

		stream, err := bridgeClient.Process(ctx)
		if err != nil {
			emit(model.PartResult{Error: fmt.Errorf("process: %w", err)})
			return
		}

		// The document's name travels however its bytes do, so a reader that
		// picks a syntax by extension has one even for inline content.
		header := &pb.ProcessHeader{
			FilterClass:  r.formatName,
			SourceLocale: string(r.Doc.SourceLocale),
			TargetLocale: string(r.Doc.TargetLocale),
			Encoding:     r.Doc.Encoding,
			MimeType:     r.Doc.MimeType,
			InputName:    r.Doc.URI,
		}
		// Forward applied config (e.g. PDF "geometry"/"glyphs") to the daemon.
		if cfg, ok := r.Cfg.(*mapConfig); ok && len(cfg.params) > 0 {
			header.FilterParams = cfg.params
		}
		if r.sourcePath != "" {
			header.Input = &pb.ContentRef{Location: &pb.ContentRef_Path{Path: r.sourcePath}}
		} else if len(r.content) > 0 {
			header.Input = &pb.ContentRef{Location: &pb.ContentRef_Inline{Inline: r.content}}
		}
		if err := stream.Send(&pb.ProcessRequest{Request: &pb.ProcessRequest_Header{Header: header}}); err != nil {
			emit(model.PartResult{Error: fmt.Errorf("send header: %w", err)})
			return
		}
		// Read-only mode: signal we have nothing more to send so the
		// daemon doesn't sit waiting on its receive side.
		_ = stream.CloseSend()

		for {
			resp, err := stream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					err = io.ErrUnexpectedEOF
				}
				emit(model.PartResult{Error: fmt.Errorf("recv before process completion: %w", err)})
				return
			}
			switch m := resp.Response.(type) {
			case *pb.ProcessResponse_Part:
				if !emit(model.PartResult{Part: protoconvert.ProtoToPart(m.Part)}) {
					return
				}
			case *pb.ProcessResponse_PartBatch:
				for _, p := range m.PartBatch.Parts {
					if !emit(model.PartResult{Part: protoconvert.ProtoToPart(p)}) {
						return
					}
				}
			case *pb.ProcessResponse_ContentBatch:
				for _, cb := range m.ContentBatch.Blocks {
					if !emit(model.PartResult{Part: protoconvert.ContentBlockToPart(cb)}) {
						return
					}
				}
			case *pb.ProcessResponse_ReadDone:
				// ReadDone ends the read phase; ProcessComplete ends the RPC.
			case *pb.ProcessResponse_Complete:
				if m.Complete.Error != "" {
					// In-band errors have no status code; expose Internal just as
					// transport errors expose their gRPC status to callers.
					emit(model.PartResult{Error: status.Errorf(codes.Internal, "daemon: %s", m.Complete.Error)})
				}
				return
			}
		}
	}()
	return ch
}

// Close is a no-op — the daemon stays in the pool, and the Process
// stream tears itself down after Read finishes.
func (r *daemonReader) Close() error { return nil }

// daemonWriter implements format.DataFormatWriter by routing parts
// through a Mode-C daemon's BridgeService.Process RPC in read-write
// mode (header carries OutputRef). It sends processed parts concurrently with
// receiving source parts, preserving transport backpressure without buffering
// the document's part stream. Inline source and output bytes remain whole
// document payloads in the protocol.
type daemonWriter struct {
	format.BaseFormatWriter

	pool       *DaemonPool
	plugin     *Plugin
	formatName string

	// Output destination: either a path or an io.Writer.
	outPath   string
	outWriter io.Writer

	// Original document content / path — set by SourcePathSetter or
	// OriginalContentSetter so the daemon can read the source while
	// applying targets.
	sourcePath  string
	sourceBytes []byte

	locale string
}

func newDaemonWriter(pool *DaemonPool, plugin *Plugin, formatName string) *daemonWriter {
	w := &daemonWriter{
		pool:       pool,
		plugin:     plugin,
		formatName: formatName,
	}
	w.FormatName = formatName
	return w
}

// SetOutput records the output path so Write can emit OutputRef.
func (w *daemonWriter) SetOutput(path string) error {
	w.outPath = absOrSelf(path)
	w.outWriter = nil
	return nil
}

// SetOutputWriter records an io.Writer; the daemon will return inline
// bytes which we then copy into w.
func (w *daemonWriter) SetOutputWriter(writer io.Writer) error {
	w.outWriter = writer
	w.outPath = ""
	return nil
}

// SetLocale records the target locale, used as the daemon's
// output_locale.
func (w *daemonWriter) SetLocale(locale model.LocaleID) {
	w.locale = string(locale)
}

// SetSourcePath honours the format.SourcePathSetter contract.
func (w *daemonWriter) SetSourcePath(path string) {
	if filepath.IsAbs(path) {
		w.sourcePath = path
	} else if abs, err := filepath.Abs(path); err == nil {
		w.sourcePath = abs
	} else {
		w.sourcePath = path
	}
}

// SetOriginalContent honours the format.OriginalContentSetter contract.
func (w *daemonWriter) SetOriginalContent(content []byte) {
	w.sourceBytes = content
}

// Write drains parts and routes them to the daemon. A read-write
// Process call needs the daemon to read the source first; we feed it
// either the source path (preferred) or the inline source bytes captured
// via SetOriginalContent. If neither is set, the daemon will fail —
// callers should always wire one of them before calling Write.
func (w *daemonWriter) Write(ctx context.Context, parts <-chan *model.Part) error {
	client, err := w.pool.Acquire(ctx, w.plugin)
	if err != nil {
		return fmt.Errorf("acquire daemon for plugin %q: %w", w.plugin.Name(), err)
	}
	bridgeClient := pb.NewBridgeServiceClient(client.Conn)

	// Own a cancellable context so an early return (recv or daemon error)
	// releases the send goroutine instead of leaving it parked on `parts`.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := bridgeClient.Process(ctx)
	if err != nil {
		return fmt.Errorf("process: %w", err)
	}

	header := &pb.ProcessHeader{
		FilterClass:  w.formatName,
		TargetLocale: w.locale,
		OutputLocale: w.locale,
	}
	if w.sourcePath != "" {
		header.Input = &pb.ContentRef{Location: &pb.ContentRef_Path{Path: w.sourcePath}}
	} else if len(w.sourceBytes) > 0 {
		header.Input = &pb.ContentRef{Location: &pb.ContentRef_Inline{Inline: w.sourceBytes}}
	} else {
		return errors.New("daemon writer: no source path or original content set; call SetSourcePath or SetOriginalContent before Write")
	}
	if w.outPath != "" {
		header.Output = &pb.OutputRef{Destination: &pb.OutputRef_Path{Path: w.outPath}}
	}
	// Inline mode (outPath == ""): omit Output from the header entirely.
	// The Java daemon checks writeEnabled = header.hasOutput() || !outputLocale.isEmpty().
	// Since OutputLocale is always set above, write mode stays active; the daemon
	// detects no OutputRef, falls back to a ByteArrayOutputStream, and returns
	// the bytes in ProcessComplete.output for us to copy into outWriter.
	// Sending OutputRef{path:""} caused java.io.FileNotFoundException on the daemon.
	if err := stream.Send(&pb.ProcessRequest{Request: &pb.ProcessRequest_Header{Header: header}}); err != nil {
		return fmt.Errorf("send header: %w", err)
	}

	// Concurrent send/recv: the daemon may produce ReadDone after
	// streaming the source's read-phase parts; we simply discard those
	// because the Write contract is "consume processed parts and emit
	// the document". The processed parts we send are the targets.
	sendErr := make(chan error, 1)
	sendDone := make(chan struct{})
	defer func() {
		cancel()
		// The sender may still be reading a Part while converting it. Return
		// ownership to the caller only after that goroutine has stopped.
		<-sendDone
	}()
	go func() {
		defer close(sendDone)
		defer func() { _ = stream.CloseSend() }()
		for {
			select {
			case part, ok := <-parts:
				if !ok {
					sendErr <- nil
					return
				}
				msg := protoconvert.PartToProto(part)
				if err := stream.Send(&pb.ProcessRequest{Request: &pb.ProcessRequest_Part{Part: msg}}); err != nil {
					sendErr <- fmt.Errorf("send part: %w", err)
					return
				}
			case <-ctx.Done():
				sendErr <- ctx.Err()
				return
			}
		}
	}()

	var output []byte
	var outputPath string
	for {
		resp, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return fmt.Errorf("recv before process completion: %w", err)
		}
		if c, ok := resp.Response.(*pb.ProcessResponse_Complete); ok {
			if c.Complete.Error != "" {
				return fmt.Errorf("daemon: %s", c.Complete.Error)
			}
			output = c.Complete.Output
			outputPath = c.Complete.OutputPath
			break
		}
		// Read-phase parts and ReadDone are ignored — we only want the
		// final ProcessComplete in writer mode.
	}

	// A daemon may complete before the host closes its input: okapi-bridge
	// sends ProcessComplete once its pipeline has written the document, which
	// can precede the last structural parts the host sends. Completion then
	// waits for the sender to finish the input, bounded by ctx. A stream that
	// ended without completion has already returned io.ErrUnexpectedEOF above.
	select {
	case err := <-sendErr:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	if outputPath == "" && w.outWriter != nil && len(output) > 0 {
		if _, err := io.Copy(w.outWriter, bytes.NewReader(output)); err != nil {
			return fmt.Errorf("copy output: %w", err)
		}
	}
	return nil
}

// absOrSelf returns the absolute version of p, or p unchanged on error.
func absOrSelf(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// Compile-time interface assertions.
var (
	_ format.DataFormatReader      = (*daemonReader)(nil)
	_ format.DataFormatWriter      = (*daemonWriter)(nil)
	_ format.SourcePathSetter      = (*daemonWriter)(nil)
	_ format.OriginalContentSetter = (*daemonWriter)(nil)
)

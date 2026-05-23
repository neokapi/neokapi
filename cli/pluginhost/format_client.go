package pluginhost

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
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
	ch := make(chan model.PartResult)
	go func() {
		defer close(ch)

		client, err := r.pool.Acquire(ctx, r.plugin)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("acquire daemon for plugin %q: %w", r.plugin.Name(), err)}
			return
		}
		bridgeClient := pb.NewBridgeServiceClient(client.Conn)

		stream, err := bridgeClient.Process(ctx)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("process: %w", err)}
			return
		}

		header := &pb.ProcessHeader{
			FilterClass:  r.formatName,
			SourceLocale: string(r.Doc.SourceLocale),
			TargetLocale: string(r.Doc.TargetLocale),
			Encoding:     r.Doc.Encoding,
			MimeType:     r.Doc.MimeType,
		}
		if r.sourcePath != "" {
			header.Input = &pb.ContentRef{Location: &pb.ContentRef_Path{Path: r.sourcePath}}
		} else if len(r.content) > 0 {
			header.Input = &pb.ContentRef{Location: &pb.ContentRef_Inline{Inline: r.content}}
		}
		if err := stream.Send(&pb.ProcessRequest{Request: &pb.ProcessRequest_Header{Header: header}}); err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("send header: %w", err)}
			return
		}
		// Read-only mode: signal we have nothing more to send so the
		// daemon doesn't sit waiting on its receive side.
		_ = stream.CloseSend()

		for {
			resp, err := stream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				ch <- model.PartResult{Error: fmt.Errorf("recv: %w", err)}
				return
			}
			switch m := resp.Response.(type) {
			case *pb.ProcessResponse_Part:
				select {
				case ch <- model.PartResult{Part: protoconvert.ProtoToPart(m.Part)}:
				case <-ctx.Done():
					ch <- model.PartResult{Error: ctx.Err()}
					return
				}
			case *pb.ProcessResponse_PartBatch:
				for _, p := range m.PartBatch.Parts {
					select {
					case ch <- model.PartResult{Part: protoconvert.ProtoToPart(p)}:
					case <-ctx.Done():
						ch <- model.PartResult{Error: ctx.Err()}
						return
					}
				}
			case *pb.ProcessResponse_ContentBatch:
				for _, cb := range m.ContentBatch.Blocks {
					select {
					case ch <- model.PartResult{Part: protoconvert.ContentBlockToPart(cb)}:
					case <-ctx.Done():
						ch <- model.PartResult{Error: ctx.Err()}
						return
					}
				}
			case *pb.ProcessResponse_ReadDone:
				// Continue: a ProcessComplete (or stream EOF) follows.
			case *pb.ProcessResponse_Complete:
				if m.Complete.Error != "" {
					ch <- model.PartResult{Error: fmt.Errorf("daemon: %s", m.Complete.Error)}
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
// mode (header carries OutputRef). The writer buffers the stream of
// parts in-memory for the duration of one document because the proto
// expects a single Process call to drive both phases.
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
	go func() {
		defer func() { _ = stream.CloseSend() }()
		for part := range parts {
			msg := protoconvert.PartToProto(part)
			if err := stream.Send(&pb.ProcessRequest{Request: &pb.ProcessRequest_Part{Part: msg}}); err != nil {
				sendErr <- fmt.Errorf("send part: %w", err)
				return
			}
		}
		sendErr <- nil
	}()

	var output []byte
	var outputPath string
	for {
		resp, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("recv: %w", err)
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

	if err := <-sendErr; err != nil {
		return err
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

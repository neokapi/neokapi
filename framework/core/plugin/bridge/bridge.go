package bridge

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"github.com/neokapi/neokapi/core/plugin/shared"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// JavaBridge manages a JVM subprocess that runs the Okapi bridge gRPC server.
// The subprocess prints its socket address to stdout on startup, and the Go
// side connects as a gRPC client.
//
// The JVM is stateful: Open sets the active filter, and Read/Close operate on
// it. Concurrent access is handled by BridgePool, which leases each bridge
// exclusively to one goroutine for the full Open→Read→Close lifecycle.
type JavaBridge struct {
	cfg     BridgeConfig
	cmd     *exec.Cmd
	conn    *grpc.ClientConn
	client  pb.BridgeServiceClient
	mu      sync.RWMutex
	logger  *log.Logger
	running bool
	healthy atomic.Bool // set after successful operations; checked by pool.Release to skip RPC health check
}

// NewJavaBridge creates a new bridge with the given config.
func NewJavaBridge(cfg BridgeConfig, logger *log.Logger) *JavaBridge {
	cfg = cfg.withDefaults()
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	return &JavaBridge{
		cfg:    cfg,
		logger: logger,
	}
}

// Start launches the JVM subprocess, reads the gRPC address from stdout,
// and establishes a gRPC client connection.
func (b *JavaBridge) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		return fmt.Errorf("bridge already running")
	}

	// External bridge mode: connect to a pre-started server.
	if b.cfg.Address != "" {
		conn, err := grpc.NewClient("passthrough:///"+b.cfg.Address,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64*1024*1024)),
		)
		if err != nil {
			return fmt.Errorf("connecting to bridge at %s: %w", b.cfg.Address, err)
		}
		b.conn = conn
		b.client = pb.NewBridgeServiceClient(conn)
		b.running = true
		b.logger.Printf("[bridge] connected to external bridge at %s", b.cfg.Address)
		return nil
	}

	b.cmd = exec.Command(b.cfg.Command, b.cfg.Args...)
	setPdeathsig(b.cmd)
	if len(b.cfg.Env) > 0 {
		b.cmd.Env = os.Environ()
		for k, v := range b.cfg.Env {
			b.cmd.Env = append(b.cmd.Env, k+"="+v)
		}
	}

	stdout, err := b.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating stdout pipe: %w", err)
	}

	// Stderr goes to logger.
	b.cmd.Stderr = &logWriter{logger: b.logger}

	if err := b.cmd.Start(); err != nil {
		return fmt.Errorf("starting JVM: %w", err)
	}
	processTracker.track(b.cmd.Process)

	// Read the gRPC address from the first line of stdout.
	addrCh := make(chan addrResult, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				addrCh <- addrResult{err: fmt.Errorf("reading address: %w", err)}
			} else {
				addrCh <- addrResult{err: fmt.Errorf("JVM closed stdout before sending address")}
			}
			return
		}
		addrCh <- addrResult{addr: strings.TrimSpace(scanner.Text())}
		// Drain remaining stdout to prevent blocking.
		for scanner.Scan() {
		}
	}()

	var addr string
	select {
	case result := <-addrCh:
		if result.err != nil {
			_ = b.cmd.Process.Kill()
			return result.err
		}
		addr = result.addr
	case <-time.After(b.cfg.StartupTimeout):
		_ = b.cmd.Process.Kill()
		return fmt.Errorf("JVM startup timed out after %s", b.cfg.StartupTimeout)
	}

	if addr == "" {
		_ = b.cmd.Process.Kill()
		return fmt.Errorf("JVM sent empty address")
	}

	b.logger.Printf("[bridge] connecting to %s", addr)

	// Establish gRPC connection (lazy — actually connects on first RPC).
	conn, err := grpc.NewClient("passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64*1024*1024)),
	)
	if err != nil {
		_ = b.cmd.Process.Kill()
		return fmt.Errorf("connecting to bridge gRPC: %w", err)
	}

	b.conn = conn
	b.client = pb.NewBridgeServiceClient(conn)
	b.running = true
	return nil
}

type addrResult struct {
	addr string
	err  error
}

// Stop gracefully shuts down the JVM subprocess.
func (b *JavaBridge) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.running {
		return nil
	}
	b.running = false

	// Send shutdown RPC only for subprocess-managed bridges.
	// External bridges (Address mode) must not be shut down — they're shared.
	if b.client != nil && b.cmd != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = b.client.Shutdown(ctx, &pb.ShutdownRequest{})
		cancel()
	}

	// Close gRPC connection.
	if b.conn != nil {
		_ = b.conn.Close()
	}

	if b.cmd == nil {
		return nil
	}

	processTracker.untrack(b.cmd.Process)

	// Wait for process to exit.
	done := make(chan error, 1)
	go func() { done <- b.cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = b.cmd.Process.Kill()
	}

	return nil
}

// Info queries filter metadata.
func (b *JavaBridge) Info(filterClass string) (*InfoData, error) {
	b.mu.RLock()
	if !b.running || b.client == nil {
		b.mu.RUnlock()
		return nil, fmt.Errorf("bridge not running")
	}
	client := b.client
	b.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.CommandTimeout)
	defer cancel()

	resp, err := client.Info(ctx, &pb.InfoRequest{FilterClass: filterClass})
	if err != nil {
		return nil, fmt.Errorf("info: %w", err)
	}
	return &InfoData{
		Name:        resp.Name,
		DisplayName: resp.DisplayName,
		MimeTypes:   resp.MimeTypes,
		Extensions:  resp.Extensions,
	}, nil
}

// Open opens a document for reading via the Java bridge.
func (b *JavaBridge) Open(params OpenParams) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.CommandTimeout)
	defer cancel()

	req := &pb.OpenRequest{
		FilterClass:  params.FilterClass,
		Uri:          params.URI,
		SourceLocale: params.SourceLocale,
		TargetLocale: params.TargetLocale,
		Encoding:     params.Encoding,
		Content:      params.Content,
		MimeType:     params.MimeType,
		FilterParams: encodeFilterParams(params.FilterParams),
		SourcePath:   params.SourcePath,
	}

	// Prefer content_ref when a source path is available to avoid byte transfer.
	if params.SourcePath != "" {
		req.ContentRef = &pb.ContentRef{
			Location: &pb.ContentRef_Path{Path: params.SourcePath},
		}
	}

	resp, err := b.client.Open(ctx, req)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	if resp.Error != "" {
		return fmt.Errorf("open: %s", resp.Error)
	}
	return nil
}

// Read streams all parts from an opened document.
// Uses 10x the command timeout since streaming large documents (e.g. 570K+
// parts for large XLSX files) can take much longer than unary RPCs.
func (b *JavaBridge) Read() ([]*pb.PartMessage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.healthy.Store(false)

	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.streamTimeout())
	defer cancel()

	stream, err := b.client.Read(ctx, &pb.ReadRequest{})
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	var parts []*pb.PartMessage
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read stream: %w", err)
		}
		parts = append(parts, msg)
	}
	b.healthy.Store(true)
	return parts, nil
}

// Write sends translated parts and receives the reconstructed document.
// Uses 10x the command timeout since streaming large documents can take
// much longer than unary RPCs.
func (b *JavaBridge) Write(params WriteParams) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.streamTimeout())
	defer cancel()

	stream, err := b.client.Write(ctx)
	if err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	header := &pb.WriteHeader{
		FilterClass:     params.FilterClass,
		Locale:          params.Locale,
		Encoding:        params.Encoding,
		OriginalContent: params.OriginalContent,
		FilterParams:    encodeFilterParams(params.FilterParams),
		SourcePath:      params.SourcePath,
	}

	// Prefer content_ref when a source path is available.
	if params.SourcePath != "" {
		header.OriginalContentRef = &pb.ContentRef{
			Location: &pb.ContentRef_Path{Path: params.SourcePath},
		}
	}

	// Send header first.
	if err := stream.Send(&pb.WriteChunk{
		Chunk: &pb.WriteChunk_Header{Header: header},
	}); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}

	// Send parts.
	for _, part := range params.Parts {
		if err := stream.Send(&pb.WriteChunk{
			Chunk: &pb.WriteChunk_Part{Part: part},
		}); err != nil {
			return nil, fmt.Errorf("write part: %w", err)
		}
	}

	// Close and receive response.
	resp, err := stream.CloseAndRecv()
	if err != nil {
		return nil, fmt.Errorf("write close: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("write: %s", resp.Error)
	}
	return resp.Output, nil
}

// WriteStream sends translated parts one-by-one from a channel to the Java bridge
// and receives the reconstructed document. Unlike Write, it never accumulates all
// parts in memory — parts are streamed directly from the pipeline channel to gRPC.
//
// When SourcePath is set, original_content_ref is populated so Java reads from
// disk instead of receiving bytes over gRPC. When OutputPath is set, output_ref
// tells Java to write directly to disk.
func (b *JavaBridge) WriteStream(ctx context.Context, params WriteStreamParams,
	parts <-chan *model.Part) (*WriteStreamResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.healthy.Store(false)

	ctx, cancel := context.WithTimeout(ctx, b.cfg.streamTimeout())
	defer cancel()

	stream, err := b.client.Write(ctx)
	if err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	header := &pb.WriteHeader{
		FilterClass:     params.FilterClass,
		Locale:          params.Locale,
		Encoding:        params.Encoding,
		OriginalContent: params.OriginalContent,
		FilterParams:    encodeFilterParams(params.FilterParams),
		SourcePath:      params.SourcePath,
		SourceLocale:    params.SourceLocale,
	}

	// Prefer content_ref when a source path is available.
	if params.SourcePath != "" {
		header.OriginalContentRef = &pb.ContentRef{
			Location: &pb.ContentRef_Path{Path: params.SourcePath},
		}
	}

	// When an output path is set, tell Java to write directly to disk.
	if params.OutputPath != "" {
		header.OutputRef = &pb.OutputRef{
			Destination: &pb.OutputRef_Path{Path: params.OutputPath},
		}
	}

	// Send header first.
	if err := stream.Send(&pb.WriteChunk{
		Chunk: &pb.WriteChunk_Header{Header: header},
	}); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}

	// Stream parts one-by-one from channel — no accumulation.
	for p := range parts {
		msg := shared.PartToProto(p)
		if err := stream.Send(&pb.WriteChunk{
			Chunk: &pb.WriteChunk_Part{Part: msg},
		}); err != nil {
			return nil, fmt.Errorf("write part: %w", err)
		}
	}

	// Close and receive response.
	resp, err := stream.CloseAndRecv()
	if err != nil {
		return nil, fmt.Errorf("write close: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("write: %s", resp.Error)
	}
	b.healthy.Store(true)
	return &WriteStreamResult{
		Output:     resp.Output,
		OutputPath: resp.OutputPath,
	}, nil
}

// CloseFilter releases the current filter resources in the JVM.
func (b *JavaBridge) CloseFilter() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.healthy.Store(false)

	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.CommandTimeout)
	defer cancel()

	resp, err := b.client.Close(ctx, &pb.CloseRequest{})
	if err != nil {
		return fmt.Errorf("close: %w", err)
	}
	if resp.Error != "" {
		return fmt.Errorf("close: %s", resp.Error)
	}
	b.healthy.Store(true)
	return nil
}

// IsHealthy checks if the bridge's gRPC connection is still alive by
// performing a short-timeout Info RPC with an empty filter class. Returns
// false if the call fails or times out due to connectivity issues,
// indicating the bridge should be discarded. A NOT_FOUND error is expected
// and indicates the bridge is healthy. Returns true if the bridge has no
// client (e.g. mock/test bridges).
func (b *JavaBridge) IsHealthy() bool {
	b.mu.RLock()
	running := b.running
	client := b.client
	b.mu.RUnlock()

	if !running {
		return false
	}

	// No client means this is a mock/test bridge — skip the health check.
	if client == nil {
		return true
	}

	// Fast path: if the last operation succeeded, the bridge is healthy.
	// This avoids a full gRPC round-trip on every pool.Release().
	if b.healthy.Load() {
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Use Info with an empty class as a lightweight health check.
	// This avoids triggering full filter discovery on the Java side.
	// A NOT_FOUND error means the bridge is alive but the class doesn't exist — that's fine.
	_, err := client.Info(ctx, &pb.InfoRequest{})
	if err == nil {
		return true
	}
	// gRPC status errors (NOT_FOUND, INVALID_ARGUMENT, etc.) mean the server is alive.
	if st, ok := status.FromError(err); ok {
		return st.Code() != codes.Unavailable && st.Code() != codes.DeadlineExceeded
	}
	return false
}

// ListFilters returns all available Okapi filters.
func (b *JavaBridge) ListFilters() (*ListFiltersData, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.CommandTimeout)
	defer cancel()

	resp, err := b.client.ListFilters(ctx, &pb.ListFiltersRequest{})
	if err != nil {
		return nil, fmt.Errorf("list_filters: %w", err)
	}

	var filters []FilterEntry
	for _, f := range resp.Filters {
		filters = append(filters, FilterEntry{
			FilterClass: f.FilterClass,
			FilterID:    f.FilterId,
			Name:        f.Name,
			DisplayName: f.DisplayName,
			MimeTypes:   f.MimeTypes,
			Extensions:  f.Extensions,
		})
	}
	return &ListFiltersData{Filters: filters}, nil
}

// RoundTripParams configures a complete read→process→write cycle.
type RoundTripParams struct {
	FilterClass  string
	URI          string
	SourceLocale string
	TargetLocale string
	Encoding     string
	MimeType     string
	FilterParams map[string]any
	Content      []byte // Inline content bytes (sent via gRPC, Java writes to temp file)
	SourcePath   string // Input file path (Java reads from disk)
	OutputPath   string // Output file path (Java writes to disk)
	OutputLocale string // Locale for the output document
}

// RoundTripResult holds the output from a RoundTrip call.
type RoundTripResult struct {
	Output     []byte
	OutputPath string
}

// RoundTrip performs a complete read→process→write cycle on a single bridge
// instance using bidirectional streaming. The processFn is called for each
// part read from the document; it should return the (possibly modified) parts
// to send back for writing.
//
// This eliminates the need for separate Open/Read + Write calls and uses only
// one JVM bridge for the entire operation.
func (b *JavaBridge) RoundTrip(ctx context.Context, params RoundTripParams,
	processFn func(parts <-chan *model.Part) <-chan *model.Part) (*RoundTripResult, error) {
	// Use RLock to allow concurrent RoundTrip calls on the same bridge.
	// gRPC connections are thread-safe and support concurrent streams.
	// RWMutex ensures Stop() (write lock) waits for all active RoundTrips.
	b.mu.RLock()
	if !b.running {
		b.mu.RUnlock()
		return nil, fmt.Errorf("bridge not running")
	}
	client := b.client
	b.mu.RUnlock()

	ctx, cancel := context.WithTimeout(ctx, b.cfg.streamTimeout())
	defer cancel()

	b.healthy.Store(false)

	stream, err := client.RoundTrip(ctx)
	if err != nil {
		return nil, fmt.Errorf("roundtrip: %w", err)
	}

	// 1. Send header.
	header := &pb.RoundTripHeader{
		FilterClass:  params.FilterClass,
		Uri:          params.URI,
		SourceLocale: params.SourceLocale,
		TargetLocale: params.TargetLocale,
		Encoding:     params.Encoding,
		MimeType:     params.MimeType,
		FilterParams: encodeFilterParams(params.FilterParams),
		OutputLocale: params.OutputLocale,
	}
	if params.Content != nil {
		header.ContentRef = &pb.ContentRef{
			Location: &pb.ContentRef_Inline{Inline: params.Content},
		}
	} else if params.SourcePath != "" {
		header.ContentRef = &pb.ContentRef{
			Location: &pb.ContentRef_Path{Path: params.SourcePath},
		}
	}
	if params.OutputPath != "" {
		header.OutputRef = &pb.OutputRef{
			Destination: &pb.OutputRef_Path{Path: params.OutputPath},
		}
	}

	if err := stream.Send(&pb.RoundTripRequest{
		Request: &pb.RoundTripRequest_Header{Header: header},
	}); err != nil {
		return nil, fmt.Errorf("roundtrip send header: %w", err)
	}

	// 2. Receive parts from Java (read phase) and feed to processFn.
	readParts := make(chan *model.Part, 64)
	readErr := make(chan error, 1)
	go func() {
		defer close(readParts)
		for {
			resp, err := stream.Recv()
			if err != nil {
				readErr <- fmt.Errorf("roundtrip recv: %w", err)
				return
			}
			switch r := resp.Response.(type) {
			case *pb.RoundTripResponse_Part:
				part := shared.ProtoToPart(r.Part)
				select {
				case readParts <- part:
				case <-ctx.Done():
					readErr <- ctx.Err()
					return
				}
			case *pb.RoundTripResponse_ReadDone:
				readErr <- nil
				return
			case *pb.RoundTripResponse_Complete:
				// Server sent early completion (error case).
				if r.Complete.Error != "" {
					readErr <- fmt.Errorf("roundtrip: %s", r.Complete.Error)
				} else {
					readErr <- nil
				}
				return
			}
		}
	}()

	// 3. Process parts through the tool chain.
	processedParts := processFn(readParts)

	// Wait for read phase to finish.
	if err := <-readErr; err != nil {
		return nil, err
	}

	// 4. Send processed parts back to Java.
	for part := range processedParts {
		msg := shared.PartToProto(part)
		if err := stream.Send(&pb.RoundTripRequest{
			Request: &pb.RoundTripRequest_ProcessedPart{
				ProcessedPart: &pb.RoundTripProcessed{Part: msg},
			},
		}); err != nil {
			return nil, fmt.Errorf("roundtrip send processed: %w", err)
		}
	}

	// 5. Signal flush (all processed parts sent).
	if err := stream.Send(&pb.RoundTripRequest{
		Request: &pb.RoundTripRequest_Flush{Flush: &pb.RoundTripFlush{}},
	}); err != nil {
		return nil, fmt.Errorf("roundtrip send flush: %w", err)
	}

	// Close our send side.
	if err := stream.CloseSend(); err != nil {
		return nil, fmt.Errorf("roundtrip close send: %w", err)
	}

	// 6. Receive completion from Java.
	for {
		resp, err := stream.Recv()
		if err != nil {
			return nil, fmt.Errorf("roundtrip recv complete: %w", err)
		}
		if c, ok := resp.Response.(*pb.RoundTripResponse_Complete); ok {
			if c.Complete.Error != "" {
				return nil, fmt.Errorf("roundtrip: %s", c.Complete.Error)
			}
			b.healthy.Store(true)
			return &RoundTripResult{
				Output:     c.Complete.Output,
				OutputPath: c.Complete.OutputPath,
			}, nil
		}
	}
}

// logWriter adapts a *log.Logger to io.Writer for stderr capture.
type logWriter struct {
	logger *log.Logger
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.logger.Printf("[bridge-jvm] %s", string(p))
	return len(p), nil
}

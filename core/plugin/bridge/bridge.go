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
	"time"

	"github.com/gokapi/gokapi/core/model"
	pb "github.com/gokapi/gokapi/core/plugin/proto/v2"
	"github.com/gokapi/gokapi/core/plugin/shared"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
	mu      sync.Mutex
	logger  *log.Logger
	running bool
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

	b.cmd = exec.Command(b.cfg.Command, b.cfg.Args...)
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

	// Send shutdown RPC.
	if b.client != nil {
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
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.CommandTimeout)
	defer cancel()

	resp, err := b.client.Info(ctx, &pb.InfoRequest{FilterClass: filterClass})
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

	resp, err := b.client.Open(ctx, &pb.OpenRequest{
		FilterClass:  params.FilterClass,
		Uri:          params.URI,
		SourceLocale: params.SourceLocale,
		TargetLocale: params.TargetLocale,
		Encoding:     params.Encoding,
		Content:      params.Content,
		MimeType:     params.MimeType,
		FilterParams: encodeFilterParams(params.FilterParams),
		SourcePath:   params.SourcePath,
	})
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

	// Send header first.
	if err := stream.Send(&pb.WriteChunk{
		Chunk: &pb.WriteChunk_Header{
			Header: &pb.WriteHeader{
				FilterClass:     params.FilterClass,
				Locale:          params.Locale,
				Encoding:        params.Encoding,
				OriginalContent: params.OriginalContent,
				FilterParams:    encodeFilterParams(params.FilterParams),
				SourcePath:      params.SourcePath,
			},
		},
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
func (b *JavaBridge) WriteStream(ctx context.Context, params WriteStreamParams,
	parts <-chan *model.Part) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, b.cfg.streamTimeout())
	defer cancel()

	stream, err := b.client.Write(ctx)
	if err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	// Send header first.
	if err := stream.Send(&pb.WriteChunk{
		Chunk: &pb.WriteChunk_Header{Header: &pb.WriteHeader{
			FilterClass:     params.FilterClass,
			Locale:          params.Locale,
			Encoding:        params.Encoding,
			OriginalContent: params.OriginalContent,
			FilterParams:    encodeFilterParams(params.FilterParams),
			SourcePath:      params.SourcePath,
		}},
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
	return resp.Output, nil
}

// CloseFilter releases the current filter resources in the JVM.
func (b *JavaBridge) CloseFilter() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.CommandTimeout)
	defer cancel()

	resp, err := b.client.Close(ctx, &pb.CloseRequest{})
	if err != nil {
		return fmt.Errorf("close: %w", err)
	}
	if resp.Error != "" {
		return fmt.Errorf("close: %s", resp.Error)
	}
	return nil
}

// IsHealthy checks if the bridge's gRPC connection is still alive by
// performing a short-timeout ListFilters RPC. Returns false if the call
// fails or times out, indicating the bridge should be discarded.
// Returns true if the bridge has no client (e.g. mock/test bridges).
func (b *JavaBridge) IsHealthy() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.running {
		return false
	}

	// No client means this is a mock/test bridge — skip the health check.
	if b.client == nil {
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := b.client.ListFilters(ctx, &pb.ListFiltersRequest{})
	return err == nil
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
			Name:        f.Name,
			DisplayName: f.DisplayName,
			MimeTypes:   f.MimeTypes,
			Extensions:  f.Extensions,
		})
	}
	return &ListFiltersData{Filters: filters}, nil
}

// logWriter adapts a *log.Logger to io.Writer for stderr capture.
type logWriter struct {
	logger *log.Logger
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.logger.Printf("[bridge-jvm] %s", string(p))
	return len(p), nil
}

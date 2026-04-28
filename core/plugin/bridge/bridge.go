package bridge

import (
	"bufio"
	"context"
	"errors"
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
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// sendBatchSize is the max number of ContentBlocks per ContentBlockBatch message.
const sendBatchSize = 1024

// sendContentBatch sends a ContentBlockBatch message on the stream.
func sendContentBatch(stream pb.BridgeService_ProcessClient, blocks []*pb.ContentBlock) error {
	if err := stream.Send(&pb.ProcessRequest{
		Request: &pb.ProcessRequest_ContentBatch{
			ContentBatch: &pb.ContentBlockBatch{Blocks: blocks},
		},
	}); err != nil {
		return fmt.Errorf("process send content batch: %w", err)
	}
	return nil
}

// JavaBridge manages a JVM subprocess that runs the Okapi bridge gRPC server.
// The subprocess prints its socket address to stdout on startup, and the Go
// side connects as a gRPC client.
//
// On Linux and macOS, the bridge uses Unix domain sockets for IPC, which
// bypasses the TCP/IP stack for lower latency and higher throughput. On
// Windows, the bridge falls back to TCP on localhost.
//
// The JVM supports concurrent Process streams via gRPC — each stream creates
// its own filter instance in Java. Concurrency is controlled by BridgeRegistry's
// semaphores, not by locks on the bridge itself.
type JavaBridge struct {
	cfg        BridgeConfig
	cmd        *exec.Cmd
	conn       *grpc.ClientConn
	client     pb.BridgeServiceClient
	mu         sync.RWMutex
	logger     *log.Logger
	running    bool
	healthy    atomic.Bool
	addr       string // gRPC address (set after Start)
	socketPath string // Unix socket path (non-empty when using UDS)
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
		return errors.New("bridge already running")
	}

	// External bridge mode: connect to a pre-started server.
	if b.cfg.Address != "" {
		conn, err := grpc.NewClient(grpcTarget(b.cfg.Address),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64*1024*1024)),
			grpc.WithWriteBufferSize(256*1024),
			grpc.WithReadBufferSize(256*1024),
			grpc.WithInitialWindowSize(4*1024*1024),
			grpc.WithInitialConnWindowSize(8*1024*1024),
		)
		if err != nil {
			return fmt.Errorf("connecting to bridge at %s: %w", b.cfg.Address, err)
		}
		b.conn = conn
		b.client = pb.NewBridgeServiceClient(conn)
		b.running = true
		b.addr = b.cfg.Address
		b.logger.Printf("[bridge] connected to external bridge at %s", b.cfg.Address)
		return nil
	}

	b.cmd = exec.Command(b.cfg.Command, b.cfg.Args...) //nolint:noctx // long-lived subprocess managed by bridge lifecycle, not a single request
	setPdeathsig(b.cmd)

	// Generate a Unix socket path for IPC (empty on Windows → JVM uses TCP).
	sockPath := generateSocketPath()
	b.socketPath = sockPath
	if sockPath != "" {
		b.logger.Printf("[bridge] using Unix socket: %s", sockPath)
	}

	// Build subprocess environment.
	b.cmd.Env = os.Environ()
	if sockPath != "" {
		b.cmd.Env = append(b.cmd.Env, bridgeSocketEnvVar+"="+sockPath)
	}
	for k, v := range b.cfg.Env {
		b.cmd.Env = append(b.cmd.Env, k+"="+v)
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
				addrCh <- addrResult{err: errors.New("jvm closed stdout before sending address")}
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
		return fmt.Errorf("jvm startup timed out after %s", b.cfg.StartupTimeout)
	}

	if addr == "" {
		_ = b.cmd.Process.Kill()
		return errors.New("jvm sent empty address")
	}

	b.logger.Printf("[bridge] connecting to %s", addr)

	// Establish gRPC connection (lazy — actually connects on first RPC).
	// If the address is a Unix socket path (starts with /), use unix: scheme.
	conn, err := grpc.NewClient(grpcTarget(addr),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64*1024*1024)),
		grpc.WithWriteBufferSize(256*1024),
		grpc.WithReadBufferSize(256*1024),
		grpc.WithInitialWindowSize(4*1024*1024),
		grpc.WithInitialConnWindowSize(8*1024*1024),
	)
	if err != nil {
		_ = b.cmd.Process.Kill()
		return fmt.Errorf("connecting to bridge gRPC: %w", err)
	}

	b.conn = conn
	b.client = pb.NewBridgeServiceClient(conn)
	b.running = true
	b.addr = addr
	return nil
}

// Address returns the gRPC address of this bridge (set after Start).
func (b *JavaBridge) Address() string {
	return b.addr
}

// Disconnect closes the gRPC connection without sending Shutdown.
// Used in daemon mode to disconnect from a JVM that should keep running.
func (b *JavaBridge) Disconnect() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.running = false
	if b.conn != nil {
		_ = b.conn.Close()
	}
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
	// Use a short timeout — the JVM may already be exiting, and a stale UDS
	// connection can hang for the full timeout.
	if b.client != nil && b.cmd != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
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
	case <-time.After(3 * time.Second):
		_ = b.cmd.Process.Kill()
	}

	// Clean up Unix socket file.
	cleanupSocket(b.socketPath)
	b.socketPath = ""

	return nil
}

// ProcessParams configures a Process RPC call.
type ProcessParams struct {
	FilterClass    string
	SourceLocale   string
	TargetLocale   string
	Encoding       string
	MimeType       string
	FilterParams   map[string]any
	Content        []byte  // Inline content bytes (sent via gRPC, Java writes to temp file)
	SourcePath     string  // Input file path (Java reads from disk)
	OutputPath     string  // Output file path (Java writes to disk)
	OutputLocale   string  // Locale for the output document
	SubscribeParts []int32 // Part types to stream to Go (empty = all)
}

// ProcessResult holds the output from a Process call.
type ProcessResult struct {
	Output     []byte
	OutputPath string
}

// Process performs a complete document processing cycle using bidirectional
// streaming. The processFn is called with the parts read from the document
// and a done channel that closes when all read-phase parts have been received.
// It should return a channel of processed parts to send back to Java for writing.
//
// For read-only mode (no output config), processFn can simply drain the parts
// channel and the done channel will close when reading is complete.
func (b *JavaBridge) Process(ctx context.Context, params ProcessParams,
	processFn func(parts <-chan *model.Part, done <-chan struct{}) <-chan *model.Part,
) (*ProcessResult, error) {
	// Use RLock to allow concurrent Process calls on the same bridge.
	b.mu.RLock()
	if !b.running {
		b.mu.RUnlock()
		return nil, errors.New("bridge not running")
	}
	client := b.client
	b.mu.RUnlock()

	ctx, cancel := context.WithTimeout(ctx, b.cfg.streamTimeout())
	defer cancel()

	b.healthy.Store(false)

	stream, err := client.Process(ctx)
	if err != nil {
		return nil, fmt.Errorf("process: %w", err)
	}

	// 1. Send header.
	header := &pb.ProcessHeader{
		FilterClass:    params.FilterClass,
		SourceLocale:   params.SourceLocale,
		TargetLocale:   params.TargetLocale,
		Encoding:       params.Encoding,
		MimeType:       params.MimeType,
		FilterParams:   encodeFilterParams(params.FilterParams),
		OutputLocale:   params.OutputLocale,
		SubscribeParts: params.SubscribeParts,
	}
	// Prefer file path over inline bytes — file paths allow Java to resolve
	// relative references to companion files (ITS linked rules, XLIFF standoff, etc.).
	if params.SourcePath != "" {
		header.Input = &pb.ContentRef{
			Location: &pb.ContentRef_Path{Path: params.SourcePath},
		}
	} else if params.Content != nil {
		header.Input = &pb.ContentRef{
			Location: &pb.ContentRef_Inline{Inline: params.Content},
		}
	}
	if params.OutputPath != "" {
		header.Output = &pb.OutputRef{
			Destination: &pb.OutputRef_Path{Path: params.OutputPath},
		}
	}

	if err := stream.Send(&pb.ProcessRequest{
		Request: &pb.ProcessRequest_Header{Header: header},
	}); err != nil {
		return nil, fmt.Errorf("process send header: %w", err)
	}

	// 2. Receive parts from Java (read phase) and feed to processFn.
	readParts := make(chan *model.Part, 4096)
	done := make(chan struct{})
	recvResult := make(chan *ProcessResult, 1)
	recvErr := make(chan error, 1)

	go func() {
		// Phase 1: receive read-phase parts until ReadDone.
		for {
			resp, err := stream.Recv()
			if err != nil {
				close(readParts)
				recvErr <- fmt.Errorf("process recv: %w", err)
				return
			}
			switch r := resp.Response.(type) {
			case *pb.ProcessResponse_Part:
				select {
				case readParts <- protoconvert.ProtoToPart(r.Part):
				case <-ctx.Done():
					close(readParts)
					recvErr <- ctx.Err()
					return
				}
			case *pb.ProcessResponse_PartBatch:
				for _, msg := range r.PartBatch.Parts {
					part := protoconvert.ProtoToPart(msg)
					select {
					case readParts <- part:
					case <-ctx.Done():
						close(readParts)
						recvErr <- ctx.Err()
						return
					}
				}
			case *pb.ProcessResponse_ContentBatch:
				for _, cb := range r.ContentBatch.Blocks {
					part := protoconvert.ContentBlockToPart(cb)
					select {
					case readParts <- part:
					case <-ctx.Done():
						close(readParts)
						recvErr <- ctx.Err()
						return
					}
				}
			case *pb.ProcessResponse_ReadDone:
				// Close readParts so processFn can finish draining.
				close(readParts)
				close(done)
				goto waitComplete
			case *pb.ProcessResponse_Complete:
				// Early completion (error or read-only mode).
				close(readParts)
				close(done)
				if r.Complete.Error != "" {
					recvErr <- fmt.Errorf("process: %s", r.Complete.Error)
				} else {
					recvResult <- &ProcessResult{
						Output:     r.Complete.Output,
						OutputPath: r.Complete.OutputPath,
					}
				}
				return
			}
		}
	waitComplete:
		// Phase 2: wait for ProcessComplete after read phase.
		for {
			resp, err := stream.Recv()
			if err != nil {
				recvErr <- fmt.Errorf("process recv complete: %w", err)
				return
			}
			if r, ok := resp.Response.(*pb.ProcessResponse_Complete); ok {
				if r.Complete.Error != "" {
					recvErr <- fmt.Errorf("process: %s", r.Complete.Error)
				} else {
					recvResult <- &ProcessResult{
						Output:     r.Complete.Output,
						OutputPath: r.Complete.OutputPath,
					}
				}
				return
			}
		}
	}()

	// 3. Process parts through the tool chain.
	processedParts := processFn(readParts, done)

	// 4. Send processed parts back to Java concurrently. This MUST run in a
	// goroutine because for single-pass pipelines, Java blocks on each TEXT_UNIT
	// waiting for the translation — so sending and receiving must be concurrent.
	//
	// Block parts are batched into ContentBlockBatch messages to amortize
	// gRPC per-message overhead. The batch is flushed when:
	//   - it reaches sendBatchSize (1024),
	//   - a non-block part arrives (must be sent before blocks),
	//   - the processedParts channel has no more parts ready (drain-flush).
	//
	// The drain-flush is critical: Java's writer thread blocks on each TEXT_UNIT
	// waiting for the translation. If Go holds translations in an unflushed
	// batch, the writer stalls → ReadDone is delayed → the send goroutine
	// never sees the channel close → deadlock. Flushing when the channel is
	// empty ensures translations flow back promptly.
	sendErr := make(chan error, 1)
	go func() {
		if processedParts != nil {
			var batch []*pb.ContentBlock
			for part := range processedParts {
				if part.Type == model.PartBlock {
					batch = append(batch, protoconvert.PartToContentBlock(part))
					if len(batch) >= sendBatchSize {
						if err := sendContentBatch(stream, batch); err != nil {
							sendErr <- err
							return
						}
						batch = nil
						continue
					}
					// Drain: keep reading while parts are immediately available.
					drained := false
					for !drained {
						select {
						case p, ok := <-processedParts:
							if !ok {
								drained = true
								break
							}
							if p.Type == model.PartBlock {
								batch = append(batch, protoconvert.PartToContentBlock(p))
								if len(batch) >= sendBatchSize {
									if err := sendContentBatch(stream, batch); err != nil {
										sendErr <- err
										return
									}
									batch = nil
								}
							} else {
								// Non-block: flush batch, send part.
								if len(batch) > 0 {
									if err := sendContentBatch(stream, batch); err != nil {
										sendErr <- err
										return
									}
									batch = nil
								}
								msg := protoconvert.PartToProto(p)
								if err := stream.Send(&pb.ProcessRequest{
									Request: &pb.ProcessRequest_Part{Part: msg},
								}); err != nil {
									sendErr <- fmt.Errorf("process send part: %w", err)
									return
								}
							}
						default:
							// Channel empty — flush pending batch so Java can proceed.
							drained = true
						}
					}
					if len(batch) > 0 {
						if err := sendContentBatch(stream, batch); err != nil {
							sendErr <- err
							return
						}
						batch = nil
					}
					continue
				}
				// Non-block part: send immediately.
				msg := protoconvert.PartToProto(part)
				if err := stream.Send(&pb.ProcessRequest{
					Request: &pb.ProcessRequest_Part{Part: msg},
				}); err != nil {
					sendErr <- fmt.Errorf("process send part: %w", err)
					return
				}
			}
			// Flush remaining batch (channel closed).
			if len(batch) > 0 {
				if err := sendContentBatch(stream, batch); err != nil {
					sendErr <- err
					return
				}
			}
		}
		if err := stream.CloseSend(); err != nil {
			sendErr <- fmt.Errorf("process close send: %w", err)
			return
		}
		sendErr <- nil
	}()

	// 5. Wait for completion (recv goroutine gets ProcessComplete).
	select {
	case result := <-recvResult:
		// Wait for send goroutine to finish.
		if err := <-sendErr; err != nil {
			return nil, err
		}
		b.healthy.Store(true)
		return result, nil
	case err := <-recvErr:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// IsHealthy checks if the bridge is still usable. It uses the healthy flag
// (set after each successful operation) and falls back to checking the gRPC
// connection state. Returns true if the bridge has no client (e.g. mock/test
// bridges).
func (b *JavaBridge) IsHealthy() bool {
	b.mu.RLock()
	running := b.running
	conn := b.conn
	b.mu.RUnlock()

	if !running {
		return false
	}
	if b.healthy.Load() {
		return true
	}
	if conn == nil {
		return true // mock/test bridge
	}
	state := conn.GetState()
	return state != connectivity.Shutdown
}

// logWriter adapts a *log.Logger to io.Writer for stderr capture.
type logWriter struct {
	logger *log.Logger
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.logger.Printf("[bridge-jvm] %s", string(p))
	return len(p), nil
}

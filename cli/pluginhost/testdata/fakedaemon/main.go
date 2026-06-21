// Command fakedaemon is a minimal Mode-C plugin used by daemon_test.go
// and format_factory_test.go.
//
// It binds a Unix socket, prints a JSON handshake on stdout, and serves
// a gRPC server. By default no service methods are registered (the pool
// only needs a successful TCP-level dial + RPC connection to consider it
// ready). When FAKE_DAEMON_BRIDGE=1, the daemon registers a minimal
// BridgeService implementation that handles the Process RPC — used by
// format-factory tests to drive a Mode-C reader/writer end to end
// without a real Java daemon.
//
// Behavior is controlled via env vars:
//
//	FAKE_DAEMON_NAME       Plugin name embedded in handshake (default "fake")
//	FAKE_DAEMON_VERSION    Version embedded in handshake (default "0.0.1")
//	FAKE_DAEMON_NO_HANDSHAKE  If "1", do not print the handshake (forces a
//	                           startup-timeout error in the pool)
//	FAKE_DAEMON_CRASH_AFTER  Duration string (e.g. "200ms"); the daemon
//	                           exits with status 1 after this delay.
//	                           Mimics a crashed plugin.
//	FAKE_DAEMON_BRIDGE     If "1", register the BridgeService stub. The
//	                       stub emits one BlockMessage per Process call
//	                       and echoes back any client-sent parts.
//	FAKE_DAEMON_SPAWN_LOG  Path to a file. On startup the daemon appends
//	                       its PID + newline (O_APPEND, locked-by-OS).
//	                       Tests count spawns by line-counting the file.
//	FAKE_DAEMON_STARTUP_DELAY  Duration string (e.g. "200ms"); sleep
//	                       before printing the handshake. Used to widen
//	                       the race window for concurrent-spawn tests.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"google.golang.org/grpc"
)

func main() {
	name := envOr("FAKE_DAEMON_NAME", "fake")
	version := envOr("FAKE_DAEMON_VERSION", "0.0.1")

	// Record this spawn so tests can count actual JVM-equivalent starts
	// (independent of how many clients the pool returns to callers).
	if path := os.Getenv("FAKE_DAEMON_SPAWN_LOG"); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()
		}
	}

	// Optional startup delay — widens the race window for tests that
	// validate concurrent-spawn coordination.
	if d := os.Getenv("FAKE_DAEMON_STARTUP_DELAY"); d != "" {
		if dur, err := time.ParseDuration(d); err == nil {
			time.Sleep(dur)
		}
	}

	// Pick a unique socket path under TMPDIR.
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("kapi-%s-%d.sock", name, os.Getpid()))
	_ = os.Remove(socket)

	lis, err := net.Listen("unix", socket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fakedaemon: listen %s: %v\n", socket, err)
		os.Exit(1)
	}
	if err := os.Chmod(socket, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "fakedaemon: chmod %s: %v\n", socket, err)
	}

	server := grpc.NewServer()
	if os.Getenv("FAKE_DAEMON_BRIDGE") == "1" {
		pb.RegisterBridgeServiceServer(server, &fakeBridge{})
	}
	go func() {
		if err := server.Serve(lis); err != nil {
			fmt.Fprintf(os.Stderr, "fakedaemon: serve: %v\n", err)
		}
	}()

	// Print handshake unless explicitly suppressed. When suppressed, we
	// also do NOT emit any subsequent stdout lines — the daemon just
	// hangs, which is what the pool's startup-timeout test expects.
	if os.Getenv("FAKE_DAEMON_NO_HANDSHAKE") == "1" {
		// Block forever (until SIGTERM).
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		<-sigCh
		_ = lis.Close()
		_ = os.Remove(socket)
		return
	}
	hs := map[string]any{
		"socket":  socket,
		"version": version,
		"pid":     os.Getpid(),
	}
	enc, _ := json.Marshal(hs)
	fmt.Println(string(enc))
	// Subsequent log lines on stdout are forwarded by the pool.
	fmt.Println("fakedaemon ready")

	// Crash after delay, if requested.
	if d := os.Getenv("FAKE_DAEMON_CRASH_AFTER"); d != "" {
		if dur, err := time.ParseDuration(d); err == nil {
			go func() {
				time.Sleep(dur)
				server.Stop()
				_ = lis.Close()
				_ = os.Remove(socket)
				os.Exit(1)
			}()
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh

	server.GracefulStop()
	_ = lis.Close()
	_ = os.Remove(socket)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// fakeBridge is a minimal BridgeService implementation used by the
// format-factory tests. The Process RPC accepts a header, emits one
// BlockMessage with the filter_class as its source text, then signals
// ReadDone. If the header sets an OutputRef, fakeBridge waits for the
// client to stream parts back and replies with ProcessComplete carrying
// any inline output bytes set via FAKE_DAEMON_OUTPUT.
type fakeBridge struct {
	pb.UnimplementedBridgeServiceServer
}

func (f *fakeBridge) Process(stream pb.BridgeService_ProcessServer) error {
	first, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("recv header: %w", err)
	}
	header, ok := first.Request.(*pb.ProcessRequest_Header)
	if !ok {
		return errors.New("expected header as first request")
	}

	// Emit a single BlockMessage so the Reader observes a non-empty
	// stream. The content carries the filter_class so tests can assert
	// on it.
	block := &pb.BlockMessage{
		Id:           "block-1",
		Type:         "p",
		MimeType:     "text/plain",
		Translatable: true,
		Source: []*pb.SegmentMessage{{
			Id: "s1",
			Runs: []*pb.RunMessage{{
				Kind: &pb.RunMessage_Text{Text: &pb.TextRunMessage{Text: header.Header.GetFilterClass()}},
			}},
		}},
	}
	if err := stream.Send(&pb.ProcessResponse{
		Response: &pb.ProcessResponse_Part{Part: &pb.PartMessage{
			PartType: 5, // PartBlock
			Block:    block,
		}},
	}); err != nil {
		return fmt.Errorf("send part: %w", err)
	}

	if err := stream.Send(&pb.ProcessResponse{
		Response: &pb.ProcessResponse_ReadDone{ReadDone: &pb.ProcessReadDone{}},
	}); err != nil {
		return fmt.Errorf("send read done: %w", err)
	}

	// Read-only mode: complete immediately when the client closes its
	// send side. Read-write mode: drain client parts first so we exercise
	// the bidirectional path.
	//
	// Mirror the Java BridgeServiceImpl write-enabled check:
	//   writeEnabled = header.hasOutput() || !header.getOutputLocale().isEmpty()
	// Previously write mode was detected solely via OutputRef presence, but
	// issue #636 fixed the Go client to omit OutputRef in inline-write mode
	// (sending OutputRef{path:""} caused FileNotFoundException on the real
	// Java daemon). Now write mode is also enabled when output_locale is set.
	hasOutput := header.Header.Output != nil || header.Header.OutputLocale != ""
	for {
		req, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("recv part: %w", err)
		}
		_ = req // discarded; we only need to drain
	}

	output := []byte(os.Getenv("FAKE_DAEMON_OUTPUT"))
	complete := &pb.ProcessComplete{}
	if hasOutput {
		complete.Output = output
		// If the OutputRef pointed at a path, leave OutputPath empty so
		// the client treats it as inline output. Production daemons
		// would write the file; the fake doesn't, because tests don't
		// assert on file contents.
	}
	return stream.Send(&pb.ProcessResponse{
		Response: &pb.ProcessResponse_Complete{Complete: complete},
	})
}

// Segment is a deterministic test segmenter: it returns an interior boundary
// (rune offset) just after each '.' in the text, so the host's daemonSegmenter
// projection can be asserted. When params carry "fail"="1" it returns an
// in-band error, exercising the error path.
func (f *fakeBridge) Segment(_ context.Context, req *pb.SegmentRequest) (*pb.SegmentResponse, error) {
	if req.GetParams()["fail"] == "1" {
		return &pb.SegmentResponse{Error: "forced failure for engine " + req.GetEngine()}, nil
	}
	var bounds []int32
	for i, r := range []rune(req.GetText()) {
		if r == '.' {
			if next := i + 1; next > 0 {
				bounds = append(bounds, int32(next))
			}
		}
	}
	return &pb.SegmentResponse{Boundaries: bounds}, nil
}

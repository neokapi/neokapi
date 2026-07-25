// Command probedaemon is the conformance suite's own Mode-C fixture: a
// deliberately minimal plugin that speaks the whole protocol, plus env-var
// switches that make it violate one rule at a time so the suite's negative
// paths are exercised against a real subprocess rather than a mock.
//
// It implements every standard verb:
//
//	probedaemon version   → prints the version, exit 0
//	probedaemon doctor    → exit 0
//	probedaemon daemon    → binds a socket, prints the handshake, serves gRPC
//	probedaemon <other>   → exit 2
//
// Fault-injection switches (each defaults to off):
//
//	PROBE_VERSION           version string to print and announce (default 1.0.0)
//	PROBE_NO_HANDSHAKE=1    never print the handshake line
//	PROBE_BAD_HANDSHAKE=1   print a non-JSON first line
//	PROBE_NO_SOCKET_FIELD=1 print valid JSON without a "socket" field
//	PROBE_RELATIVE_SOCKET=1 announce a relative socket path
//	PROBE_NO_SERVICE=1      serve gRPC without registering BridgeService
//	PROBE_LOOSE_SOCKET=1    chmod the socket 0666 instead of 0600
//	PROBE_NO_SHUTDOWN=1     leave the Shutdown RPC unimplemented
//	PROBE_IGNORE_SIGTERM=1  ignore SIGTERM
//	PROBE_LEAK_SOCKET=1     exit without unlinking the socket
//	PROBE_SEGMENT_ERROR=1   answer Segment with an error string
//	PROBE_BLOCKS=n          blocks the Process RPC returns (default: input lines)
//	PROBE_UNKNOWN_VERB_OK=1 exit 0 on an unrecognised verb
//	PROBE_ANY_COMMAND_OK=1  exit 0 for any `command <name>`
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
	"strconv"
	"strings"
	"syscall"

	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	contentv1 "github.com/neokapi/neokapi/core/proto/content/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func on(name string) bool { return os.Getenv(name) == "1" }

func version() string {
	if v := os.Getenv("PROBE_VERSION"); v != "" {
		return v
	}
	return "1.0.0"
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "probedaemon: usage: probedaemon {version|doctor|command|daemon}")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println(version())
	case "doctor":
		fmt.Println("probedaemon: ok")
	case "command":
		runCommand(os.Args[2:])
	case "daemon":
		runDaemon()
	default:
		if on("PROBE_UNKNOWN_VERB_OK") {
			return
		}
		fmt.Fprintf(os.Stderr, "probedaemon: unknown verb %q\n", os.Args[1])
		os.Exit(2)
	}
}

func runCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "probedaemon: command requires a name")
		os.Exit(2)
	}
	if on("PROBE_ANY_COMMAND_OK") {
		fmt.Println("ok")
		return
	}
	switch args[0] {
	case "echo":
		fmt.Println(strings.Join(args[1:], " "))
	default:
		fmt.Fprintf(os.Stderr, "probedaemon: unknown command %q\n", args[0])
		os.Exit(2)
	}
}

func runDaemon() {
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("kapi-probedaemon-%d.sock", os.Getpid()))
	_ = os.Remove(socket)

	lis, err := net.Listen("unix", socket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "probedaemon: listen %s: %v\n", socket, err)
		os.Exit(1)
	}
	// Go's UnixListener unlinks the socket on Close by default, so leaking it
	// has to be opted into at the listener level, not just by skipping Remove.
	if on("PROBE_LEAK_SOCKET") {
		if ul, ok := lis.(*net.UnixListener); ok {
			ul.SetUnlinkOnClose(false)
		}
	}
	mode := os.FileMode(0o600)
	if on("PROBE_LOOSE_SOCKET") {
		mode = 0o666
	}
	if err := os.Chmod(socket, mode); err != nil {
		fmt.Fprintf(os.Stderr, "probedaemon: chmod %s: %v\n", socket, err)
	}

	srv := grpc.NewServer()
	stopped := make(chan struct{})
	if !on("PROBE_NO_SERVICE") {
		pb.RegisterBridgeServiceServer(srv, &bridge{stop: stopped})
	}
	go func() {
		if err := srv.Serve(lis); err != nil {
			fmt.Fprintf(os.Stderr, "probedaemon: serve: %v\n", err)
		}
	}()

	cleanup := func() {
		srv.Stop()
		_ = lis.Close()
		if !on("PROBE_LEAK_SOCKET") {
			_ = os.Remove(socket)
		}
	}

	switch {
	case on("PROBE_NO_HANDSHAKE"):
		// Print nothing; the host's startup timeout must fire.
	case on("PROBE_BAD_HANDSHAKE"):
		fmt.Println("probedaemon starting up, please wait")
	case on("PROBE_NO_SOCKET_FIELD"):
		fmt.Println(`{"version":"` + version() + `"}`)
	case on("PROBE_RELATIVE_SOCKET"):
		fmt.Println(`{"socket":"` + filepath.Base(socket) + `","version":"` + version() + `"}`)
	default:
		enc, _ := json.Marshal(map[string]any{
			"socket": socket, "version": version(), "pid": os.Getpid(),
		})
		fmt.Println(string(enc))
		// Post-handshake stdout is log output, which the host forwards.
		fmt.Println("probedaemon ready")
	}

	sigCh := make(chan os.Signal, 1)
	if !on("PROBE_IGNORE_SIGTERM") {
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	} else {
		signal.Ignore(syscall.SIGTERM)
	}
	select {
	case <-sigCh:
	case <-stopped:
	}
	cleanup()
}

// bridge is a minimal BridgeService: enough to prove registration, to read a
// staged document, and to segment text.
type bridge struct {
	pb.UnimplementedBridgeServiceServer
	stop chan struct{}
}

// Process reads the header, opens the referenced document, and returns one
// block per non-blank line (or PROBE_BLOCKS blocks when set).
func (b *bridge) Process(stream pb.BridgeService_ProcessServer) error {
	req, err := stream.Recv()
	if err != nil {
		// An empty stream is how the suite probes for service registration.
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	header, ok := req.Request.(*pb.ProcessRequest_Header)
	if !ok {
		return status.Error(codes.InvalidArgument, "first message must be a ProcessHeader")
	}
	h := header.Header

	texts, err := blockTexts(h)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	blocks := make([]*contentv1.ContentBlock, 0, len(texts))
	for i, t := range texts {
		blocks = append(blocks, &contentv1.ContentBlock{
			Id:           fmt.Sprintf("b%d", i+1),
			Type:         "paragraph",
			Translatable: true,
			Source: []*contentv1.SegmentMessage{{
				Runs: []*contentv1.RunMessage{{
					Kind: &contentv1.RunMessage_Text{Text: &contentv1.TextRunMessage{Text: t}},
				}},
			}},
		})
	}
	if len(blocks) > 0 {
		if err := stream.Send(&pb.ProcessResponse{
			Response: &pb.ProcessResponse_ContentBatch{
				ContentBatch: &pb.ContentBlockBatch{Blocks: blocks},
			},
		}); err != nil {
			return err
		}
	}
	if err := stream.Send(&pb.ProcessResponse{
		Response: &pb.ProcessResponse_ReadDone{ReadDone: &pb.ProcessReadDone{}},
	}); err != nil {
		return err
	}
	return stream.Send(&pb.ProcessResponse{
		Response: &pb.ProcessResponse_Complete{Complete: &pb.ProcessComplete{}},
	})
}

// blockTexts resolves the header's input into one string per block.
func blockTexts(h *pb.ProcessHeader) ([]string, error) {
	if n := os.Getenv("PROBE_BLOCKS"); n != "" {
		count, err := strconv.Atoi(n)
		if err != nil {
			return nil, fmt.Errorf("PROBE_BLOCKS=%q is not a number", n)
		}
		out := make([]string, 0, count)
		for i := range count {
			out = append(out, fmt.Sprintf("synthetic block %d", i+1))
		}
		return out, nil
	}
	var raw []byte
	switch loc := h.GetInput().GetLocation().(type) {
	case *contentv1.ContentRef_Path:
		b, err := os.ReadFile(loc.Path)
		if err != nil {
			return nil, fmt.Errorf("read input %s: %w", loc.Path, err)
		}
		raw = b
	case *contentv1.ContentRef_Inline:
		raw = loc.Inline
	default:
		return nil, errors.New("header carries no input reference")
	}
	var out []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		if t := strings.TrimSpace(line); t != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

// Segment splits on ". " so the suite sees real interior boundaries.
func (b *bridge) Segment(_ context.Context, req *pb.SegmentRequest) (*pb.SegmentResponse, error) {
	if on("PROBE_SEGMENT_ERROR") {
		return &pb.SegmentResponse{Error: "probedaemon: segmentation disabled by PROBE_SEGMENT_ERROR"}, nil
	}
	var boundaries []int32
	runes := []rune(req.GetText())
	for i := range len(runes) - 1 {
		if runes[i] == '.' && runes[i+1] == ' ' {
			boundaries = append(boundaries, int32(i+2)) //nolint:gosec // bounded by the sample text length
		}
	}
	return &pb.SegmentResponse{Boundaries: boundaries}, nil
}

// Shutdown stops the daemon, unless the fixture is asked to omit it.
func (b *bridge) Shutdown(_ context.Context, _ *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	if on("PROBE_NO_SHUTDOWN") {
		return nil, status.Error(codes.Unimplemented, "Shutdown is not implemented")
	}
	close(b.stop)
	return &pb.ShutdownResponse{}, nil
}

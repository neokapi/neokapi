// Command kapi-sat is the SaT (Segment any Text / wtpsplit) segmenter plugin
// for kapi. It runs SaT ONNX models in-process — the heavy onnxruntime + XLM-R
// tokenizer native dependencies live here, not in the portable kapi binary.
//
// It is a Mode-C (daemon-over-socket) plugin: it serves the shared
// BridgeService over a Unix socket and answers the Segment RPC, exactly as the
// host's generic daemonSegmenter drives any plugin-declared segmenter
// (capabilities.segmenters in manifest.json). There is no kapi-sat-specific code
// in the host — the engine is wired purely from manifest metadata.
//
// Subcommands:
//
//	kapi-sat daemon    serve BridgeService over a Unix socket (the pool's entry)
//	kapi-sat version   print the plugin version
//	kapi-sat doctor    self-check: construct the engine and list supported models
//	                   (the standard self-check that `kapi plugins doctor` runs)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"google.golang.org/grpc"

	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/plugins/sat/internal/model"
	"github.com/neokapi/neokapi/plugins/sat/internal/sat"
)

func main() {
	sub := "daemon"
	if len(os.Args) >= 2 {
		sub = os.Args[1]
	}
	switch sub {
	case "daemon", "serve": // "serve" kept as an alias for manual runs
		os.Exit(runDaemon())
	case "version":
		fmt.Println(version.Version)
	case "doctor":
		os.Exit(runDoctor())
	default:
		fmt.Fprintf(os.Stderr, "kapi-sat: unknown subcommand %q (want daemon|version|doctor)\n", sub)
		os.Exit(2)
	}
}

// runDaemon binds a Unix socket, advertises it via the stdio handshake, and
// serves BridgeService until SIGTERM/SIGINT or a Shutdown RPC. The engine is
// created lazily-failing: a non-ONNX build still serves (so the handshake and
// capability probing work) and reports the build limitation per Segment call.
func runDaemon() int {
	logf := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "kapi-sat: "+format+"\n", args...)
	}
	engine, err := sat.NewEngine(logf)
	if err != nil {
		logf("engine init failed: %v", err)
		// Continue with a nil engine: the daemon still serves; Segment errors.
	}
	defer func() {
		if engine != nil {
			_ = engine.Close()
		}
	}()

	dir, err := os.MkdirTemp("", "kapi-sat-")
	if err != nil {
		logf("temp dir: %v", err)
		return 1
	}
	sock := filepath.Join(dir, "kapi-sat.sock")
	lis, err := net.Listen("unix", sock)
	if err != nil {
		logf("listen %s: %v", sock, err)
		return 1
	}

	srv := grpc.NewServer()
	pb.RegisterBridgeServiceServer(srv, &server{engine: engine, initErr: err, stop: srv.GracefulStop})

	// Handshake: one JSON line on stdout, then keep stdout open for logs.
	hs, _ := json.Marshal(map[string]string{"socket": sock, "version": version.Version})
	fmt.Println(string(hs))
	_ = os.Stdout.Sync()

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigc
		srv.GracefulStop()
	}()

	serveErr := srv.Serve(lis)
	_ = os.Remove(sock)
	_ = os.Remove(dir)
	if serveErr != nil {
		logf("serve: %v", serveErr)
		return 1
	}
	return 0
}

// server implements BridgeService. Only Segment (and Shutdown) are supported;
// Process / ProcessStep fall through to UnimplementedBridgeServiceServer, which
// returns codes.Unimplemented — kapi-sat is a segmenter, not a format/step plugin.
type server struct {
	pb.UnimplementedBridgeServiceServer
	engine  sat.Engine
	initErr error
	stop    func()
}

func (s *server) Shutdown(_ context.Context, _ *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	if s.stop != nil {
		go s.stop()
	}
	return &pb.ShutdownResponse{}, nil
}

// Segment runs the SaT model over the request text and returns the interior
// sentence-break offsets (rune indices). The model name and boundary threshold
// ride in params (satModel / threshold), matching the engine's segment schema.
func (s *server) Segment(_ context.Context, req *pb.SegmentRequest) (*pb.SegmentResponse, error) {
	if s.engine == nil {
		return &pb.SegmentResponse{Error: fmt.Sprintf("engine unavailable: %v", s.initErr)}, nil
	}
	params := req.GetParams()
	modelName := params["satModel"]
	var threshold float64
	if t := params["threshold"]; t != "" {
		threshold, _ = strconv.ParseFloat(t, 64)
	}
	bounds, err := s.engine.Segment(req.GetText(), modelName, req.GetLocale(), threshold)
	if err != nil {
		return &pb.SegmentResponse{Error: err.Error()}, nil
	}
	out := make([]int32, len(bounds))
	for i, b := range bounds {
		out[i] = int32(b)
	}
	return &pb.SegmentResponse{Boundaries: out}, nil
}

// runDoctor is the standard self-check (run by `kapi plugins doctor`): it
// confirms the in-process engine constructs and prints the supported models,
// independent of the daemon path.
func runDoctor() int {
	engine, err := sat.NewEngine(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kapi-sat: engine init failed: %v\n", err)
		return 1
	}
	defer func() { _ = engine.Close() }()

	fmt.Printf("kapi-sat %s — SaT segmenter ready\n", version.Version)
	fmt.Println("supported models:")
	for _, m := range model.Registry {
		def := ""
		if m.Default {
			def = " (default)"
		}
		fmt.Printf("  - %s%s\n", m.Name, def)
	}
	return 0
}

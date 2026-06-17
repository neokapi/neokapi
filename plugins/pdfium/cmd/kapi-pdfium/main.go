// kapi-pdfium is a Mode-C (daemon-over-socket) kapi plugin that provides a
// high-fidelity PDF reader backed by Google's PDFium (go-pdfium, cgo). It runs
// as a subprocess so a malformed-PDF crash is contained to the plugin, never
// the host. It speaks the same BridgeService.Process protocol okapi-bridge uses,
// so the host drives it with no new client code; the geometry/annotations ride
// the wire via the shared payload registry.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"google.golang.org/grpc"

	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/plugins/pdfium/internal/pdfreader"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		if err := serve(); err != nil {
			fmt.Fprintln(os.Stderr, "kapi-pdfium:", err)
			os.Exit(1)
		}
		return
	}
	// Bare invocation: a self-check the manifest can surface (mirrors kapi-sat).
	fmt.Printf("kapi-pdfium %s — PDFium-backed PDF reader (run `kapi-pdfium serve` for the daemon)\n", version.Version)
}

// serve binds a Unix socket, advertises it via the stdio handshake, and serves
// BridgeService until SIGTERM/SIGINT or a Shutdown RPC.
func serve() error {
	dir, err := os.MkdirTemp("", "kapi-pdfium-")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	sock := filepath.Join(dir, "kapi-pdfium.sock")
	lis, err := net.Listen("unix", sock)
	if err != nil {
		return fmt.Errorf("listen %s: %w", sock, err)
	}

	srv := grpc.NewServer()
	pb.RegisterBridgeServiceServer(srv, &server{stop: srv.GracefulStop})

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

	err = srv.Serve(lis)
	_ = os.Remove(sock)
	_ = os.Remove(dir)
	return err
}

// server implements BridgeService. Only read-mode Process is supported; tools
// and read-write are not (kapi-pdfium is a reader-only format provider).
type server struct {
	pb.UnimplementedBridgeServiceServer
	stop func()
}

func (s *server) Shutdown(_ context.Context, _ *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	if s.stop != nil {
		go s.stop()
	}
	return &pb.ShutdownResponse{}, nil
}

// Process handles a single document. Read-only mode: the header carries the
// input (path or inline bytes) and no output; we stream Parts, then ReadDone,
// then Complete.
func (s *server) Process(stream pb.BridgeService_ProcessServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}
	header := req.GetHeader()
	if header == nil {
		return complete(stream, "first message must be a header")
	}

	data, uri, err := readInput(header.GetInput())
	if err != nil {
		return complete(stream, err.Error())
	}

	geometry := header.GetFilterParams()["geometry"] == "true"
	parts, err := pdfreader.ReadParts(data, model.LocaleID(header.GetSourceLocale()), uri, pdfreader.Options{Geometry: geometry})
	if err != nil {
		return complete(stream, err.Error())
	}

	for _, p := range parts {
		if err := stream.Send(&pb.ProcessResponse{Response: &pb.ProcessResponse_Part{Part: protoconvert.PartToProto(p)}}); err != nil {
			return err
		}
	}
	if err := stream.Send(&pb.ProcessResponse{Response: &pb.ProcessResponse_ReadDone{ReadDone: &pb.ProcessReadDone{}}}); err != nil {
		return err
	}
	return complete(stream, "")
}

func complete(stream pb.BridgeService_ProcessServer, errMsg string) error {
	return stream.Send(&pb.ProcessResponse{Response: &pb.ProcessResponse_Complete{Complete: &pb.ProcessComplete{Error: errMsg}}})
}

// readInput resolves the document bytes from a ContentRef (path preferred,
// inline fallback) and returns a URI label for the layer name.
func readInput(in *pb.ContentRef) ([]byte, string, error) {
	if in == nil {
		return nil, "", fmt.Errorf("no input in header")
	}
	if path := in.GetPath(); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", fmt.Errorf("read %s: %w", path, err)
		}
		return data, filepath.Base(path), nil
	}
	if inline := in.GetInline(); len(inline) > 0 {
		return inline, "document.pdf", nil
	}
	return nil, "", fmt.Errorf("empty input")
}

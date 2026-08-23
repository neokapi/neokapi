// kapi-sourcecode is a Mode-C (daemon-over-socket) kapi plugin that reads the
// prose out of source files using tree-sitter grammars.
//
// It runs as a subprocess for the same reason kapi-pdfium does: the grammars
// are C, and a parser fault on a malformed file is contained to the plugin
// rather than taking the host down with it. It speaks the BridgeService.Process
// protocol okapi-bridge and kapi-pdfium already use, so the host drives it with
// no new client code.
//
// The format is READ-ONLY and says so in the manifest. There is no writer, and
// a program is never written back — see internal/proseread for why that is a
// decision rather than an omission.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"google.golang.org/grpc"

	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/plugins/sourcecode/internal/proseread"
)

// selfcheckRuby is a cask in miniature: two prose sites among identifiers, so
// `doctor` proves the grammar loads AND that the tree still separates them.
// A self-check that only proved the parser starts would pass on a build whose
// node kinds had drifted.
const selfcheckRuby = `# a comment
cask "widget" do
  desc "A workbench for content"
  app "Widget.app"
end
`

func main() {
	sub := ""
	if len(os.Args) > 1 {
		sub = os.Args[1]
	}
	switch sub {
	case "daemon", "serve":
		if err := serve(); err != nil {
			fmt.Fprintln(os.Stderr, "kapi-sourcecode:", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println(version.Version)
	case "doctor":
		os.Exit(runDoctor())
	default:
		fmt.Fprintf(os.Stderr, "kapi-sourcecode %s\nusage: kapi-sourcecode daemon | kapi-sourcecode version | kapi-sourcecode doctor\n", version.Version)
		os.Exit(2)
	}
}

// runDoctor confirms the cgo grammars load and that extraction still picks the
// prose out of the structure. `kapi plugins doctor` runs this.
func runDoctor() int {
	parts, err := proseread.ReadParts([]byte(selfcheckRuby), model.LocaleID("en"), "selfcheck.rb",
		proseread.Options{NodePaths: []string{"desc"}})
	if err != nil {
		fmt.Fprintf(os.Stderr, "kapi-sourcecode: self-check failed: %v\n", err)
		return 1
	}
	var got []string
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && p.Type == model.PartBlock {
			got = append(got, b.SourceText())
		}
	}
	if len(got) != 1 || got[0] != "A workbench for content" {
		fmt.Fprintf(os.Stderr, "kapi-sourcecode: self-check extracted %v, want the desc only\n", got)
		return 1
	}
	fmt.Printf("kapi-sourcecode %s: grammars ok (%s)\n", version.Version, strings.Join(proseread.Grammars(), ", "))
	return 0
}

func serve() error {
	dir, err := os.MkdirTemp("", "kapi-sourcecode-")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	sock := filepath.Join(dir, "kapi-sourcecode.sock")
	lis, err := net.Listen("unix", sock)
	if err != nil {
		return fmt.Errorf("listen %s: %w", sock, err)
	}

	srv := grpc.NewServer()
	pb.RegisterBridgeServiceServer(srv, &server{stop: srv.GracefulStop})

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

// server implements BridgeService. Read-mode Process only: this plugin provides
// a reader-only format, so there is nothing for the write path to do.
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

	parts, err := proseread.ReadParts(data, model.LocaleID(header.GetSourceLocale()), uri, optionsFrom(header.GetFilterParams()))
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

// optionsFrom decodes the recipe's format config, which arrives as flat strings.
// nodePathPatterns is comma-separated; empty means "extract everything the
// grammar exposes", which is the reader's own default.
func optionsFrom(params map[string]string) proseread.Options {
	opts := proseread.Options{Comments: params["comments"] == "true"}
	if raw := strings.TrimSpace(params["nodePathPatterns"]); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				opts.NodePaths = append(opts.NodePaths, p)
			}
		}
	}
	return opts
}

func complete(stream pb.BridgeService_ProcessServer, errMsg string) error {
	return stream.Send(&pb.ProcessResponse{Response: &pb.ProcessResponse_Complete{Complete: &pb.ProcessComplete{Error: errMsg}}})
}

// readInput resolves the document bytes from a ContentRef (path preferred,
// inline fallback) and returns a URI label — which also chooses the grammar, so
// it has to carry the real extension.
func readInput(in *pb.ContentRef) ([]byte, string, error) {
	if in == nil {
		return nil, "", fmt.Errorf("no input in header")
	}
	if path := in.GetPath(); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", fmt.Errorf("read %s: %w", path, err)
		}
		return data, path, nil
	}
	if inline := in.GetInline(); len(inline) > 0 {
		// The URI is the only name inline content carries, and the grammar is
		// chosen by extension — so unlike a single-format reader, this one
		// cannot fall back to a fixed filename. Refusing beats guessing at a
		// language.
		if uri := in.GetUri(); uri != "" {
			return inline, uri, nil
		}
		return nil, "", fmt.Errorf("inline input needs a uri to choose a grammar")
	}
	return nil, "", fmt.Errorf("empty input")
}

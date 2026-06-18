// Command kapi-vision is the document-vision plugin for kapi. It runs OCR (and,
// in later phases, ML layout and table-structure) ONNX models in-process — the
// heavy onnxruntime native dependency lives here, not in the portable kapi
// binary — and speaks the binary-framed visionproto protocol on stdin/stdout
// (length-prefixed JSON header + raw image frame; see
// github.com/neokapi/neokapi/plugins/vision/visionproto).
//
// The host-side `vision` engine spawns this binary in `serve` mode and drives it
// with a visionproto client. The process stays alive across requests, loading
// models lazily on first use.
//
// Subcommands:
//
//	kapi-vision serve     start the stdin/stdout protocol loop (default)
//	kapi-vision version   print the plugin version
//	kapi-vision command   manifest Mode-A entry; "command vision" self-checks
package main

import (
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/plugins/vision/internal/models"
	"github.com/neokapi/neokapi/plugins/vision/internal/ocr"
	"github.com/neokapi/neokapi/plugins/vision/visionproto"
)

func main() {
	sub := "serve"
	if len(os.Args) >= 2 {
		sub = os.Args[1]
	}
	switch sub {
	case "serve":
		os.Exit(runServe())
	case "version":
		fmt.Println(version.Version)
	case "command":
		os.Exit(runCommand(os.Args[2:]))
	default:
		fmt.Fprintf(os.Stderr, "kapi-vision: unknown subcommand %q (want serve|version|command)\n", sub)
		os.Exit(2)
	}
}

// runServe runs the protocol loop. As with kapi-sat, the engine is
// lazily-failing: a binary built without ONNX still answers ping/info (so a host
// can probe capability) and reports the build limitation per OCR request.
func runServe() int {
	logf := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "kapi-vision: "+format+"\n", args...)
	}
	engine, err := ocr.NewEngine(logf)
	if err != nil {
		logf("engine init failed: %v", err)
	}
	defer func() {
		if engine != nil {
			_ = engine.Close()
		}
	}()

	if err := visionproto.Serve(os.Stdin, os.Stdout, serveHandler(engine, err)); err != nil {
		logf("serve loop error: %v", err)
		return 1
	}
	return 0
}

// serveHandler builds the protocol handler over the engine. initErr is the
// engine-construction error (nil engine), reported per OCR request so ping/info
// still answer for capability probing. Extracted for testing.
func serveHandler(engine ocr.Engine, initErr error) visionproto.Handler {
	return func(req visionproto.Request, _ []byte) visionproto.Response {
		switch req.Op {
		case visionproto.OpPing:
			return visionproto.Response{Version: version.Version}
		case visionproto.OpInfo:
			return visionproto.Response{Version: version.Version, Models: modelInfos(engine)}
		case visionproto.OpOCR:
			if engine == nil {
				return visionproto.Response{Error: fmt.Sprintf("engine unavailable: %v", initErr)}
			}
			res, oerr := engine.OCR(req.Path, req.Lang, req.Model)
			if oerr != nil {
				return visionproto.Response{Error: oerr.Error()}
			}
			return visionproto.Response{OCR: res}
		case "":
			return visionproto.Response{Error: "missing op"}
		default:
			return visionproto.Response{Error: fmt.Sprintf("unknown op %q", req.Op)}
		}
	}
}

// modelInfos reports the supported model assets and whether the engine has them
// resident.
func modelInfos(engine ocr.Engine) []visionproto.ModelInfo {
	loaded := engine != nil && engine.Loaded()
	out := make([]visionproto.ModelInfo, 0, len(models.Registry))
	for _, a := range models.Registry {
		if a.Key == "dict" {
			continue
		}
		out = append(out, visionproto.ModelInfo{Name: a.File, Default: a.Key == "rec", Loaded: loaded})
	}
	return out
}

// runCommand implements the manifest Mode-A `command` entry: an in-process
// self-check that confirms the engine constructs and lists the model assets.
func runCommand(args []string) int {
	if len(args) == 0 || args[0] != "vision" {
		fmt.Fprintln(os.Stderr, "kapi-vision: usage: kapi-vision command vision")
		return 2
	}
	engine, err := ocr.NewEngine(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kapi-vision: engine init failed: %v\n", err)
		return 1
	}
	defer func() { _ = engine.Close() }()

	fmt.Printf("kapi-vision %s — document vision (OCR) ready\n", version.Version)
	fmt.Println("model assets:")
	for _, a := range models.Registry {
		fmt.Printf("  - %s (%s)\n", a.File, a.Key)
	}
	return 0
}

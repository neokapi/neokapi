// Command kapi-check is the in-process ML checker plugin. It runs a small,
// open, multilingual embedding model behind the onnxruntime, and speaks the
// line-delimited JSON checkproto on stdin/stdout (`kapi-check serve`) so the
// host can score voice/style similarity without inheriting the native build.
//
// The model is acquired explicitly — `kapi-check pull` — not by a surprise
// download mid-check; serve/embed/similarity fail with guidance when it is
// absent. The default build links no native libraries (engine_stub.go) so the
// binary and the pure-Go protocol/model/vec tests build everywhere; the real
// engine is selected with `-tags onnx`.
package main

import (
	"fmt"
	"os"

	"github.com/neokapi/neokapi/plugins/check/checkproto"
	"github.com/neokapi/neokapi/plugins/check/internal/embed"
	"github.com/neokapi/neokapi/plugins/check/internal/model"
	"github.com/neokapi/neokapi/plugins/check/internal/vec"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "pull":
		cmdPull(os.Args[2:])
	case "info":
		cmdInfo()
	case "serve":
		cmdServe()
	case "embed":
		cmdEmbed(os.Args[2:])
	case "similarity":
		cmdSimilarity(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "kapi-check: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `kapi-check `+version+` — in-process ML checker (sentence-embedding similarity)

Usage:
  kapi-check pull [model]         download the model files (explicit, cached)
  kapi-check info                 list models and whether they are installed
  kapi-check serve                run the checkproto stdin/stdout loop (host-driven)
  kapi-check embed <text>         print the embedding vector length (self-check)
  kapi-check similarity <text> <ref>...   print cosine(text, ref) for each ref

The model is downloaded only by pull; the other commands fail with guidance
when it is absent. Build with -tags onnx (plus onnxruntime + libtokenizers) for
the real backend; the default build is a stub.
`)
}

func cmdPull(args []string) {
	name := model.DefaultModelName()
	if len(args) > 0 {
		name = args[0]
	}
	dl := &model.Downloader{Logf: func(f string, a ...any) { fmt.Fprintf(os.Stderr, "· "+f+"\n", a...) }}
	paths, err := dl.Ensure(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kapi-check: pull %s: %v\n", name, err)
		os.Exit(1)
	}
	fmt.Printf("installed %s\n  onnx:      %s\n  tokenizer: %s\n", name, paths.ONNX, paths.Tokenizer)
}

func cmdInfo() {
	for _, s := range model.Registry {
		state := "not installed (run `kapi-check pull " + s.Name + "`)"
		if model.Present(s.Name) {
			state = "installed"
		}
		def := ""
		if s.Default {
			def = " (default)"
		}
		fmt.Printf("%s%s — %s\n  %s, %dd, %s\n", s.Name, def, s.Repo, s.ONNXFile, s.Dim, state)
	}
}

func newEngine() embed.Engine {
	eng, err := embed.NewEngine(func(f string, a ...any) { fmt.Fprintf(os.Stderr, "· "+f+"\n", a...) })
	if err != nil {
		fmt.Fprintf(os.Stderr, "kapi-check: %v\n", err)
		os.Exit(1)
	}
	return eng
}

func cmdServe() {
	eng := newEngine()
	defer func() { _ = eng.Close() }()
	if err := checkproto.Serve(os.Stdin, os.Stdout, handler(eng)); err != nil {
		fmt.Fprintf(os.Stderr, "kapi-check: serve: %v\n", err)
		os.Exit(1)
	}
}

func handler(eng embed.Engine) checkproto.Handler {
	return func(req checkproto.Request) checkproto.Response {
		switch req.Op {
		case checkproto.OpPing:
			return checkproto.Response{OK: true, Version: version}
		case checkproto.OpInfo:
			return checkproto.Response{Models: modelInfos(eng), Version: version}
		case checkproto.OpEmbed:
			v, err := eng.Embed(req.Text, req.Model)
			if err != nil {
				return checkproto.Response{Error: err.Error()}
			}
			return checkproto.Response{Embedding: v}
		case checkproto.OpSimilarity:
			tv, err := eng.Embed(req.Text, req.Model)
			if err != nil {
				return checkproto.Response{Error: err.Error()}
			}
			scores := make([]float64, len(req.Refs))
			for i, ref := range req.Refs {
				rv, err := eng.Embed(ref, req.Model)
				if err != nil {
					return checkproto.Response{Error: err.Error()}
				}
				scores[i] = vec.Cosine(tv, rv)
			}
			return checkproto.Response{Scores: scores}
		default:
			return checkproto.Response{Error: fmt.Sprintf("unknown op %q", req.Op)}
		}
	}
}

func modelInfos(eng embed.Engine) []checkproto.ModelInfo {
	out := make([]checkproto.ModelInfo, 0, len(model.Registry))
	for _, s := range model.Registry {
		out = append(out, checkproto.ModelInfo{Name: s.Name, Loaded: eng.Loaded(s.Name), Default: s.Default})
	}
	return out
}

func cmdEmbed(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: kapi-check embed <text>")
		os.Exit(2)
	}
	eng := newEngine()
	defer func() { _ = eng.Close() }()
	v, err := eng.Embed(args[0], "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "kapi-check: embed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("embedding: %d dims (first 4: %.4f %.4f %.4f %.4f)\n", len(v), v[0], v[1], v[2], v[3])
}

func cmdSimilarity(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: kapi-check similarity <text> <ref>...")
		os.Exit(2)
	}
	eng := newEngine()
	defer func() { _ = eng.Close() }()
	tv, err := eng.Embed(args[0], "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "kapi-check: %v\n", err)
		os.Exit(1)
	}
	for _, ref := range args[1:] {
		rv, err := eng.Embed(ref, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "kapi-check: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("%.4f  %s\n", vec.Cosine(tv, rv), ref)
	}
}

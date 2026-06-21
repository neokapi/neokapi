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
	var err error
	switch os.Args[1] {
	case "version":
		fmt.Println(version)
	case "doctor":
		os.Exit(runDoctor())
	case "pull":
		err = cmdPull(os.Args[2:])
	case "info":
		cmdInfo()
	case "serve":
		err = cmdServe()
	case "embed":
		err = cmdEmbed(os.Args[2:])
	case "similarity":
		err = cmdSimilarity(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "kapi-check: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "kapi-check: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `kapi-check `+version+` — in-process ML checker (sentence-embedding similarity)

Usage:
  kapi-check version              print the plugin version
  kapi-check doctor               self-check: construct the engine, list models
  kapi-check info                 list models and whether they are installed
  kapi-check serve                run the checkproto stdin/stdout loop (host-driven)
  kapi-check embed <text>         print the embedding vector length (self-check)
  kapi-check similarity <text> <ref>...   print cosine(text, ref) for each ref

Model acquisition is host-owned: run "kapi models pull check" (kapi downloads,
verifies the pinned digest, and caches). serve/embed/similarity fail with
guidance when the model is absent. Build with -tags onnx (plus onnxruntime +
libtokenizers) for the real backend; the default build is a stub.
`)
}

// cmdPull is retained only to redirect: model acquisition is now host-owned, so
// kapi (not the plugin) downloads, verifies against pinned digests, and caches
// the model. See `kapi models pull check`.
func cmdPull([]string) error {
	return fmt.Errorf("`kapi-check pull` has moved — run `kapi models pull check` " +
		"(the host downloads, verifies against the manifest's pinned digest, and caches the model)")
}

func cmdInfo() {
	for _, s := range model.Registry {
		state := "not installed (run `kapi models pull check`)"
		if model.Present(s.Name) {
			state = "installed"
		}
		def := ""
		if s.Default {
			def = " (default)"
		}
		fmt.Printf("%s%s — %dd, %s\n", s.Name, def, s.Dim, state)
	}
}

// runDoctor is the standard self-check: it confirms the embedding engine
// constructs and lists the supported models with their installed state. It fails
// only on a real defect (engine construction) — a model not yet pulled is a
// readiness note, not a broken install. `kapi plugins doctor` runs this.
func runDoctor() int {
	eng, err := newEngine()
	if err != nil {
		fmt.Fprintf(os.Stderr, "kapi-check: engine init failed: %v\n", err)
		return 1
	}
	defer func() { _ = eng.Close() }()

	fmt.Printf("kapi-check %s — ML checker (sentence-embedding similarity) ready\n", version)
	fmt.Println("models:")
	for _, s := range model.Registry {
		state := "not installed (run `kapi models pull check`)"
		if model.Present(s.Name) {
			state = "installed"
		}
		def := ""
		if s.Default {
			def = " (default)"
		}
		fmt.Printf("  - %s%s — %s\n", s.Name, def, state)
	}
	return 0
}

func newEngine() (embed.Engine, error) {
	return embed.NewEngine(func(f string, a ...any) { fmt.Fprintf(os.Stderr, "· "+f+"\n", a...) })
}

func cmdServe() error {
	eng, err := newEngine()
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()
	if err := checkproto.Serve(os.Stdin, os.Stdout, handler(eng)); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
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

func cmdEmbed(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: kapi-check embed <text>")
		os.Exit(2)
	}
	eng, err := newEngine()
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()
	v, err := eng.Embed(args[0], "")
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	fmt.Printf("embedding: %d dims (first 4: %.4f %.4f %.4f %.4f)\n", len(v), v[0], v[1], v[2], v[3])
	return nil
}

func cmdSimilarity(args []string) error {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: kapi-check similarity <text> <ref>...")
		os.Exit(2)
	}
	eng, err := newEngine()
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()
	tv, err := eng.Embed(args[0], "")
	if err != nil {
		return err
	}
	for _, ref := range args[1:] {
		rv, err := eng.Embed(ref, "")
		if err != nil {
			return err
		}
		fmt.Printf("%.4f  %s\n", vec.Cosine(tv, rv), ref)
	}
	return nil
}

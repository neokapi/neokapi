// Command kapi-sat is the SaT (Segment any Text / wtpsplit) segmenter plugin
// for kapi. It runs SaT ONNX models in-process — the heavy onnxruntime + XLM-R
// tokenizer native dependencies live here, not in the portable kapi binary —
// and speaks a tiny line-delimited JSON protocol on stdin/stdout (see
// github.com/neokapi/neokapi/plugins/sat/satproto).
//
// The host-side `sat` segment engine spawns this binary in `serve` mode and
// drives it with satproto.Client. The process stays alive across many
// requests, loading each model lazily on first use and caching it.
//
// Subcommands:
//
//	kapi-sat serve     start the stdin/stdout protocol loop (default)
//	kapi-sat version   print the plugin version
//	kapi-sat doctor    self-check: construct the engine and list supported models
//	                   (the standard self-check that `kapi plugins doctor` runs)
package main

import (
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/plugins/sat/internal/model"
	"github.com/neokapi/neokapi/plugins/sat/internal/sat"
	"github.com/neokapi/neokapi/plugins/sat/satproto"
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
	case "doctor":
		os.Exit(runDoctor())
	default:
		fmt.Fprintf(os.Stderr, "kapi-sat: unknown subcommand %q (want serve|version|doctor)\n", sub)
		os.Exit(2)
	}
}

// runServe runs the protocol loop. The engine is created lazily-failing: if the
// binary was built without ONNX support, NewEngine still succeeds (returning a
// stub) and segment requests report the build limitation per-request, so ping
// and info still work for host capability probing.
func runServe() int {
	logf := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "kapi-sat: "+format+"\n", args...)
	}
	engine, err := sat.NewEngine(logf)
	if err != nil {
		logf("engine init failed: %v", err)
		// Continue with a nil engine: ping/info still answer; segment errors.
	}
	defer func() {
		if engine != nil {
			_ = engine.Close()
		}
	}()

	h := func(req satproto.Request) satproto.Response {
		switch req.Op {
		case satproto.OpPing:
			return satproto.Response{OK: true, Version: version.Version}
		case satproto.OpInfo:
			return satproto.Response{Version: version.Version, Models: modelInfos(engine)}
		case satproto.OpSegment:
			if engine == nil {
				return satproto.Response{Error: fmt.Sprintf("engine unavailable: %v", err)}
			}
			bounds, serr := engine.Segment(req.Text, req.Model, req.Lang, req.Threshold)
			if serr != nil {
				return satproto.Response{Error: serr.Error()}
			}
			if bounds == nil {
				bounds = []int{}
			}
			return satproto.Response{Boundaries: bounds}
		case "":
			return satproto.Response{Error: "missing op"}
		default:
			return satproto.Response{Error: fmt.Sprintf("unknown op %q", req.Op)}
		}
	}

	if err := satproto.Serve(os.Stdin, os.Stdout, h); err != nil {
		logf("serve loop error: %v", err)
		return 1
	}
	return 0
}

// modelInfos reports the supported models and whether each is currently loaded.
func modelInfos(engine sat.Engine) []satproto.ModelInfo {
	out := make([]satproto.ModelInfo, 0, len(model.Registry))
	for _, s := range model.Registry {
		mi := satproto.ModelInfo{Name: s.Name, Default: s.Default}
		if engine != nil {
			mi.Loaded = engine.Loaded(s.Name)
		}
		out = append(out, mi)
	}
	return out
}

// runDoctor is the standard self-check: it confirms the in-process engine
// constructs and prints the supported models, independent of the protocol path.
// `kapi plugins doctor` runs this.
func runDoctor() int {
	engine, err := sat.NewEngine(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kapi-sat: engine init failed: %v\n", err)
		return 1
	}
	defer func() { _ = engine.Close() }()

	fmt.Printf("kapi-sat %s — SaT segmenter ready\n", version.Version)
	fmt.Println("supported models:")
	for _, s := range model.Registry {
		def := ""
		if s.Default {
			def = " (default)"
		}
		fmt.Printf("  - %s%s\n", s.Name, def)
	}
	return 0
}

// Command kapi-llm is the local-LLM plugin for kapi. It runs Gemma 4 ONNX
// models in-process — the heavy onnxruntime + tokenizer native dependencies live
// here, not in the portable kapi binary — and speaks a line-delimited JSON
// protocol on stdin/stdout (see github.com/neokapi/neokapi/plugins/llm/llmproto).
//
// The host-side Gemma provider spawns this binary in `serve` mode and drives it
// with llmproto.Client. The process stays alive across many requests, loading
// the model lazily on first use and caching it.
//
// Subcommands:
//
//	kapi-llm serve     start the stdin/stdout protocol loop (default)
//	kapi-llm version   print the plugin version
//	kapi-llm doctor    self-check: construct the engine and list supported models
//	                   (the standard self-check that `kapi plugins doctor` runs)
package main

import (
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/plugins/llm/internal/llm"
	"github.com/neokapi/neokapi/plugins/llm/internal/model"
	"github.com/neokapi/neokapi/plugins/llm/llmproto"
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
		fmt.Fprintf(os.Stderr, "kapi-llm: unknown subcommand %q (want serve|version|doctor)\n", sub)
		os.Exit(2)
	}
}

// runServe runs the protocol loop. The engine is created lazily-failing: if the
// binary was built without ONNX support, NewEngine still succeeds (returning a
// stub) and generate requests report the build limitation per-request, so ping
// and info still work for host capability probing.
func runServe() int {
	logf := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "kapi-llm: "+format+"\n", args...)
	}
	engine, err := llm.NewEngine(logf)
	if err != nil {
		logf("engine init failed: %v", err)
		// Continue with a nil engine: ping/info still answer; generate errors.
	}
	defer func() {
		if engine != nil {
			_ = engine.Close()
		}
	}()

	h := func(req llmproto.Request) llmproto.Response {
		switch req.Op {
		case llmproto.OpPing:
			return llmproto.Response{OK: true, Version: version.Version}
		case llmproto.OpInfo:
			return llmproto.Response{
				Version:    version.Version,
				Models:     modelInfos(engine),
				Modalities: modalities(engine),
			}
		case llmproto.OpGenerate:
			if engine == nil {
				return llmproto.Response{Error: fmt.Sprintf("engine unavailable: %v", err)}
			}
			res, gerr := engine.Generate(toParams(req))
			if gerr != nil {
				return llmproto.Response{Error: gerr.Error()}
			}
			return llmproto.Response{
				Text:         res.Text,
				InputTokens:  res.InputTokens,
				OutputTokens: res.OutputTokens,
			}
		case "":
			return llmproto.Response{Error: "missing op"}
		default:
			return llmproto.Response{Error: fmt.Sprintf("unknown op %q", req.Op)}
		}
	}

	if err := llmproto.Serve(os.Stdin, os.Stdout, h); err != nil {
		logf("serve loop error: %v", err)
		return 1
	}
	return 0
}

// toParams converts a wire request into engine generation params.
func toParams(req llmproto.Request) llm.GenerateParams {
	msgs := make([]llm.Message, len(req.Messages))
	for i, m := range req.Messages {
		em := llm.Message{Role: m.Role, Text: m.Text}
		for _, md := range m.Media {
			em.Media = append(em.Media, llm.Media{Kind: string(md.Kind), Path: md.Path, MIME: md.MIME})
		}
		msgs[i] = em
	}
	return llm.GenerateParams{
		Messages:    msgs,
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Schema:      req.Schema,
	}
}

// modelInfos reports the supported models and whether each is currently loaded.
func modelInfos(engine llm.Engine) []llmproto.ModelInfo {
	out := make([]llmproto.ModelInfo, 0, len(model.Registry))
	for _, s := range model.Registry {
		mi := llmproto.ModelInfo{Name: s.Name, Default: s.Default}
		if engine != nil {
			mi.Loaded = engine.Loaded(s.Name)
		}
		out = append(out, mi)
	}
	return out
}

// modalities reports the engine's accepted non-text input modalities.
func modalities(engine llm.Engine) []string {
	if engine == nil {
		return nil
	}
	return engine.Modalities()
}

// runDoctor is the standard self-check: it confirms the in-process engine
// constructs and prints the supported models. `kapi plugins doctor` runs this.
func runDoctor() int {
	engine, err := llm.NewEngine(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kapi-llm: engine init failed: %v\n", err)
		return 1
	}
	defer func() { _ = engine.Close() }()

	fmt.Printf("kapi-llm %s — Gemma 4 local LLM ready\n", version.Version)
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

// Package model maps the Gemma 4 ONNX model's files to their on-disk paths.
//
// The plugin no longer downloads anything: model acquisition is owned by the
// HOST (kapi), which reads the model assets declared in the plugin's
// manifest.json, fetches + verifies + caches them (cli.EnsureModel), and passes
// the staged directory to this plugin via $KAPI_LLM_MODEL_DIR. This package
// resolves the names the engine needs (embed/decoder graphs, tokenizer, configs)
// to files within that directory, and verifies they are present. Keeping the
// download out of the plugin means one uniform, integrity-pinned, progress-bar'd
// acquisition path for every kapi model asset.
//
// # Variant
//
// The plugin uses the q4 variant — 4-bit-quantized weights with float32 tensor
// I/O. The Go onnxruntime binding has no native float16 tensor type, so the
// fp16/q4f16 graphs would require custom-typed tensors throughout; q4 keeps the
// whole pipeline in float32 with no precision-relevant difference for
// generation.
//
// Gemma 4 ships as a transformers.js-style SPLIT export: the component graphs
// each have one or more external-data siblings (`*.onnx_data`). Every file is
// staged flat under its basename in the model directory so the external-data
// references inside the .onnx files resolve when onnxruntime memory-maps them.
package model

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
)

// File names one file the model is composed of, identified by its path within
// the upstream repo. Only the basename matters on disk; the host owns the URL
// and digest (declared in the plugin manifest's `models` section).
type File struct {
	// RepoPath is the path within the upstream repo (e.g.
	// "onnx/decoder_model_merged_q4.onnx" or "tokenizer.json"); its basename is
	// the on-disk name the host stages the file under.
	RepoPath string
}

// Base returns the on-disk basename the file is stored under.
func (f File) Base() string { return path.Base(f.RepoPath) }

// Spec describes one model: the role each file plays and the chat-template
// family. All files are staged flat in the model directory under their basename,
// so the .onnx external-data references (which name siblings by basename)
// resolve.
type Spec struct {
	// Name is the model identifier the protocol uses (e.g. "gemma-4-e2b").
	Name string
	// Family selects the chat-template + special-token handling the engine uses
	// for this model (e.g. "gemma", "chatml" for Qwen, "llama"). The decode loop,
	// KV cache, and sampling are family-agnostic; only prompt assembly differs.
	// Empty defaults to "gemma".
	Family string

	// The four component graphs. Audio/Vision may be empty for a text-only
	// model; embed + decoder are always required.
	Embed   File
	Decoder File
	Vision  File
	Audio   File
	// Data holds the external-data siblings (`*.onnx_data`) that must accompany
	// the component graphs.
	Data []File

	// Tokenizer + processor/config JSON.
	Tokenizer          File
	Config             File
	GenerationConfig   File
	PreprocessorConfig File
	ProcessorConfig    File

	// Default marks the plugin's default model.
	Default bool
}

// allFiles returns every file the spec needs, skipping empty optional ones.
func (s Spec) allFiles() []File {
	out := make([]File, 0, 8+len(s.Data))
	for _, f := range []File{s.Embed, s.Decoder, s.Vision, s.Audio} {
		if f.RepoPath != "" {
			out = append(out, f)
		}
	}
	out = append(out, s.Data...)
	for _, f := range []File{s.Tokenizer, s.Config, s.GenerationConfig, s.PreprocessorConfig, s.ProcessorConfig} {
		if f.RepoPath != "" {
			out = append(out, f)
		}
	}
	return out
}

// Registry is the set of models the plugin supports. The host's manifest
// `models` section is the source of truth for what to download (URLs + pinned
// digests); this registry only maps a model name to the role each staged file
// plays, and must stay in lockstep with the manifest's file basenames.
var Registry = []Spec{
	{
		Name:    "gemma-4-e2b",
		Family:  "gemma",
		Default: true,
		// v0.1.x is text-only: only the embed + decoder graphs are used, so a
		// text user never needs the (not-yet-validated) vision/audio encoders.
		Embed:   File{RepoPath: "onnx/embed_tokens_q4.onnx"},
		Decoder: File{RepoPath: "onnx/decoder_model_merged_q4.onnx"},
		Data: []File{
			{RepoPath: "onnx/embed_tokens_q4.onnx_data"},
			{RepoPath: "onnx/decoder_model_merged_q4.onnx_data"},
		},
		Tokenizer:        File{RepoPath: "tokenizer.json"},
		Config:           File{RepoPath: "config.json"},
		GenerationConfig: File{RepoPath: "generation_config.json"},
	},
}

// DefaultModelName returns the registry's default model name.
func DefaultModelName() string {
	for _, s := range Registry {
		if s.Default {
			return s.Name
		}
	}
	if len(Registry) > 0 {
		return Registry[0].Name
	}
	return ""
}

// Lookup returns the Spec for the named model, or false if unknown. An empty
// name resolves to the default model.
func Lookup(name string) (Spec, bool) {
	if name == "" {
		name = DefaultModelName()
	}
	for _, s := range Registry {
		if s.Name == name {
			return s, true
		}
	}
	return Spec{}, false
}

// Paths holds the resolved on-disk locations of a model's files.
type Paths struct {
	// Dir is the model's directory (all files live here, flat).
	Dir string
	// Component graph paths. Vision/Audio are "" for a text-only model.
	Embed   string
	Decoder string
	Vision  string
	Audio   string
	// Tokenizer + config paths.
	Tokenizer          string
	Config             string
	GenerationConfig   string
	PreprocessorConfig string
	ProcessorConfig    string
}

// ResolveInDir maps the named model's files to their on-disk paths within dir —
// where the host has already staged and SHA-256 verified them — and returns
// them. It performs no network access: if a required file is missing the caller
// must (re)provision the model through the host (`kapi models pull llm`), not
// fetch it here. dir is typically $KAPI_LLM_MODEL_DIR.
func ResolveInDir(name, dir string) (Paths, error) {
	spec, ok := Lookup(name)
	if !ok {
		return Paths{}, fmt.Errorf("model: unknown model %q", name)
	}
	if dir == "" {
		return Paths{}, errors.New("model: no model directory provided " +
			"(set KAPI_LLM_MODEL_DIR, or run the plugin through kapi, which stages the model)")
	}
	for _, f := range spec.allFiles() {
		p := filepath.Join(dir, f.Base())
		if fi, err := os.Stat(p); err != nil || fi.Size() == 0 {
			return Paths{}, fmt.Errorf("model: %s: missing or empty %q in %s "+
				"(run `kapi models pull llm` to (re)stage it)", name, f.Base(), dir)
		}
	}
	at := func(f File) string {
		if f.RepoPath == "" {
			return ""
		}
		return filepath.Join(dir, f.Base())
	}
	return Paths{
		Dir:                dir,
		Embed:              at(spec.Embed),
		Decoder:            at(spec.Decoder),
		Vision:             at(spec.Vision),
		Audio:              at(spec.Audio),
		Tokenizer:          at(spec.Tokenizer),
		Config:             at(spec.Config),
		GenerationConfig:   at(spec.GenerationConfig),
		PreprocessorConfig: at(spec.PreprocessorConfig),
		ProcessorConfig:    at(spec.ProcessorConfig),
	}, nil
}

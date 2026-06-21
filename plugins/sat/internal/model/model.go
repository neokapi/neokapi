// Package model maps the SaT ONNX model files and the XLM-RoBERTa tokenizer to
// their on-disk paths.
//
// The plugin no longer downloads anything: model acquisition is owned by the
// HOST (kapi), which reads the model assets declared in the plugin manifest,
// fetches + verifies + caches them (cli.EnsureModel), and passes the plugin's
// model-cache root to this daemon via $KAPI_SAT_MODELS_ROOT. SaT's model is
// runtime-selectable (sat-3l-sm / sat-12l-sm), so the engine resolves each model
// under <root>/<id>/<version>/. The host pre-stages the default; a non-default
// model is staged on demand via `kapi models pull sat/<id>`.
//
// This package is pure Go (os + path) with no cgo dependency, so it builds and
// unit-tests without the ONNX runtime or tokenizer native libraries present.
package model

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// onnxFile / tokenizerFile are the canonical on-disk basenames every SaT model
// is staged under (all SaT *-sm models share the xlm-roberta-base tokenizer).
const (
	onnxFile      = "model.onnx"
	tokenizerFile = "tokenizer.json"
)

// Spec identifies one SaT model and the cache version it is staged under. The
// host manifest's `models` section is the source of truth for what to download
// (URLs + pinned digests); this registry only maps a model name to its cache
// version, and must stay in lockstep with the manifest's model id + version.
type Spec struct {
	// Name is the model id the recipe's satModel param selects and the cache
	// key (e.g. "sat-3l-sm").
	Name string
	// Version must match the manifest model asset's version, so the engine
	// resolves the same cache directory the host staged.
	Version string
	// Default marks the model used when none is selected.
	Default bool
}

// Registry is the set of models the plugin supports. Keep ids + versions in
// lockstep with plugins/sat/manifest.json `models`.
var Registry = []Spec{
	{Name: "sat-3l-sm", Version: "1", Default: true},
	{Name: "sat-12l-sm", Version: "1"},
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
	// ONNX is the absolute path to the ONNX model file.
	ONNX string
	// Tokenizer is the absolute path to tokenizer.json.
	Tokenizer string
}

// ResolveInDir maps the named model's files to their on-disk paths within the
// plugin's model-cache root (root = $KAPI_SAT_MODELS_ROOT, i.e. the host's
// <cache>/models/sat directory). Each model lives at <root>/<id>/<version>/.
// It performs no network access: a missing model must be (re)provisioned via the
// host (`kapi models pull sat/<id>`).
func ResolveInDir(name, root string) (Paths, error) {
	spec, ok := Lookup(name)
	if !ok {
		return Paths{}, fmt.Errorf("model: unknown model %q", name)
	}
	if root == "" {
		return Paths{}, errors.New("model: no model root provided " +
			"(set KAPI_SAT_MODELS_ROOT, or run the plugin through kapi, which stages the model)")
	}
	dir := filepath.Join(root, spec.Name, spec.Version)
	p := Paths{
		Dir:       dir,
		ONNX:      filepath.Join(dir, onnxFile),
		Tokenizer: filepath.Join(dir, tokenizerFile),
	}
	for _, f := range []string{p.ONNX, p.Tokenizer} {
		if fi, err := os.Stat(f); err != nil || fi.Size() == 0 {
			return Paths{}, fmt.Errorf("model: %s: missing or empty %q in %s "+
				"(run `kapi models pull sat/%s` to (re)stage it)", name, filepath.Base(f), dir, spec.Name)
		}
	}
	return p, nil
}

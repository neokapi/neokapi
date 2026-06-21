// Package model maps the kapi-check sentence-embedding model files to their
// on-disk paths.
//
// The plugin no longer downloads anything: model acquisition is owned by the
// HOST (kapi). The model is declared in the plugin manifest and fetched +
// verified + cached by `kapi models pull check` (cli.EnsureModel) — replacing
// the old bespoke `kapi-check pull`. This package resolves the staged files; its
// cache root mirrors the host's convention ($XDG_CACHE_HOME/kapi/models/check)
// so the plugin finds exactly what the host staged, and honors
// $KAPI_CHECK_MODELS_ROOT when a host provider sets it.
//
// Pure Go (os + path); builds and unit-tests without the ONNX runtime or
// tokenizer native libraries present.
package model

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	onnxFile      = "model.onnx"
	tokenizerFile = "tokenizer.json"
)

// Spec identifies one checker model and the cache version it is staged under.
// The host manifest's `models` section is the source of truth for what to
// download (URLs + pinned digests); this registry maps a model name to its cache
// version + embedding dimension and must stay in lockstep with the manifest.
type Spec struct {
	// Name is the model id (e.g. "e5-small-int8") and the cache key.
	Name string
	// Version must match the manifest model asset's version.
	Version string
	// Dim is the embedding dimension (for sanity checks / info).
	Dim int
	// Default marks the model used when none is named.
	Default bool
}

// Registry is the set of models the plugin supports. Keep id + version in
// lockstep with plugins/check/manifest.json `models`.
var Registry = []Spec{
	{Name: "e5-small-int8", Version: "1", Dim: 384, Default: true},
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
	Dir       string
	ONNX      string
	Tokenizer string
	Dim       int
}

// ModelsRoot returns the plugin's model-cache root, mirroring the host's
// $XDG_CACHE_HOME/kapi/models/check convention so the plugin resolves exactly
// what `kapi models pull check` stages. Precedence: $KAPI_CHECK_MODELS_ROOT (set
// by a host provider, if any), then $KAPI_MODELS_CACHE/check, then
// $XDG_CACHE_HOME/kapi/models/check, then ~/.cache/kapi/models/check.
func ModelsRoot() (string, error) {
	if v := os.Getenv("KAPI_CHECK_MODELS_ROOT"); v != "" {
		return v, nil
	}
	if v := os.Getenv("KAPI_MODELS_CACHE"); v != "" {
		return filepath.Join(v, "check"), nil
	}
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return filepath.Join(v, "kapi", "models", "check"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("model: resolve cache root: %w", err)
	}
	return filepath.Join(home, ".cache", "kapi", "models", "check"), nil
}

// ResolveInDir maps the named model's files to their on-disk paths within root
// (<root>/<id>/<version>/), where the host has staged + verified them. It does
// no network access: a missing model must be (re)provisioned via the host
// (`kapi models pull check`).
func ResolveInDir(name, root string) (Paths, error) {
	spec, ok := Lookup(name)
	if !ok {
		return Paths{}, fmt.Errorf("model: unknown model %q", name)
	}
	if root == "" {
		return Paths{}, errors.New("model: no model root provided (run `kapi models pull check`)")
	}
	dir := filepath.Join(root, spec.Name, spec.Version)
	p := Paths{
		Dir:       dir,
		ONNX:      filepath.Join(dir, onnxFile),
		Tokenizer: filepath.Join(dir, tokenizerFile),
		Dim:       spec.Dim,
	}
	for _, f := range []string{p.ONNX, p.Tokenizer} {
		if fi, err := os.Stat(f); err != nil || fi.Size() == 0 {
			return Paths{}, fmt.Errorf("model: %s: missing or empty %q in %s "+
				"(run `kapi models pull check` to (re)stage it)", name, filepath.Base(f), dir)
		}
	}
	return p, nil
}

// Present reports whether the named model is fully staged in the cache.
func Present(name string) bool {
	root, err := ModelsRoot()
	if err != nil {
		return false
	}
	_, err = ResolveInDir(name, root)
	return err == nil
}

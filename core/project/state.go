package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// StateManifest is the content of `.kapi/manifest.yaml` — kapi's
// bookkeeping about the project's current block state. Users don't
// hand-edit this; kapi tools maintain it.
//
// It is safe to delete and regenerate from the block store; nothing
// authoritative lives here that isn't also in the store.
type StateManifest struct {
	SchemaVersion int                        `yaml:"schemaVersion" json:"schemaVersion"`
	Kind          string                     `yaml:"kind" json:"kind"`
	Generator     StateGenerator             `yaml:"generator" json:"generator"`
	Project       StateProjectRef            `yaml:"project" json:"project"`
	Blocks        map[string]StateBlockStats `yaml:"blocks,omitempty" json:"blocks,omitempty"`
	UpdatedAt     string                     `yaml:"updatedAt,omitempty" json:"updatedAt,omitempty"`
}

// StateGenerator identifies which kapi tool last wrote the state.
type StateGenerator struct {
	ID      string `yaml:"id" json:"id"`
	Version string `yaml:"version" json:"version"`
}

// StateProjectRef references the owning recipe. Path is relative to
// the state dir so projects can move around without invalidation.
type StateProjectRef struct {
	ID   string `yaml:"id" json:"id"`
	Path string `yaml:"path" json:"path"`
}

// StateBlockStats summarises a collection's block state.
type StateBlockStats struct {
	Count   int                     `yaml:"count" json:"count"`
	SHA256  string                  `yaml:"sha256,omitempty" json:"sha256,omitempty"`
	Sources []StateBlockSourceStats `yaml:"sources,omitempty" json:"sources,omitempty"`
}

// StateBlockSourceStats captures per-source-file fingerprinting for
// staleness detection (re-extract only files whose content changed).
type StateBlockSourceStats struct {
	Path   string `yaml:"path" json:"path"`
	SHA256 string `yaml:"sha256" json:"sha256"`
	Blocks int    `yaml:"blocks" json:"blocks"`
}

// StateManifestKind is the literal kind string written to disk.
const StateManifestKind = "kapi-state"

// StateManifestFilename is the fixed filename inside `.kapi/`.
const StateManifestFilename = "manifest.yaml"

// LoadState reads `.kapi/manifest.yaml`. Returns (nil, nil) when the
// file is absent — fresh projects don't have one until the first
// flow runs. Returns an error only on malformed YAML or IO failure.
func LoadState(layout Layout) (*StateManifest, error) {
	p := filepath.Join(layout.StateDir, StateManifestFilename)
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("project: read state manifest: %w", err)
	}
	var m StateManifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("project: decode state manifest: %w", err)
	}
	if m.Kind != "" && m.Kind != StateManifestKind {
		return nil, fmt.Errorf("project: state manifest kind %q (expected %q)", m.Kind, StateManifestKind)
	}
	return &m, nil
}

// SaveState writes the manifest to `.kapi/manifest.yaml`. Creates
// `.kapi/` if absent. Stamps UpdatedAt with current UTC time.
func SaveState(layout Layout, m *StateManifest) error {
	if m == nil {
		return errors.New("project: nil state manifest")
	}
	if m.SchemaVersion == 0 {
		m.SchemaVersion = 1
	}
	if m.Kind == "" {
		m.Kind = StateManifestKind
	}
	m.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := os.MkdirAll(layout.StateDir, 0o755); err != nil {
		return fmt.Errorf("project: ensure state dir: %w", err)
	}
	b, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("project: encode state manifest: %w", err)
	}
	p := filepath.Join(layout.StateDir, StateManifestFilename)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return fmt.Errorf("project: write state manifest: %w", err)
	}
	return nil
}

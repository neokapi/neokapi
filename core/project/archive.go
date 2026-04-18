package project

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ArchiveManifest is the content of `manifest.yaml` at the root of a
// `.klz` archive — the merged form of Recipe + State used when the
// project is serialized to a portable snapshot.
//
// Inside a `.klz` there's no author-boundary to preserve (nobody
// edits a sealed archive), so recipe fields and state bookkeeping
// live in one file.
type ArchiveManifest struct {
	SchemaVersion int             `yaml:"schemaVersion" json:"schemaVersion"`
	Kind          string          `yaml:"kind" json:"kind"`
	Project       ArchiveRecipe   `yaml:"project" json:"project"`
	State         ArchiveState    `yaml:"state,omitempty" json:"state,omitempty"`
}

// ArchiveRecipe is the portable form of a `KapiProject`. Uses a
// plain map for flows/defaults to keep the wire format stable
// independent of the live KapiProject struct layout — the archive
// format needs to survive incremental changes to the runtime struct.
type ArchiveRecipe struct {
	Version         string              `yaml:"version,omitempty" json:"version,omitempty"`
	ID              string              `yaml:"id,omitempty" json:"id,omitempty"`
	Name            string              `yaml:"name,omitempty" json:"name,omitempty"`
	SourceLocale    string              `yaml:"sourceLocale,omitempty" json:"sourceLocale,omitempty"`
	TargetLocales   []string            `yaml:"targetLocales,omitempty" json:"targetLocales,omitempty"`
	// Raw is the full recipe as a YAML value tree. Kept opaque so
	// archive reads round-trip everything the recipe might carry
	// (collections, flows, presets, plugin specs) without this file
	// needing to know the full schema.
	Raw map[string]any `yaml:",inline" json:"raw,omitempty"`
}

// ArchiveState mirrors the runtime StateManifest.
type ArchiveState struct {
	Generator StateGenerator             `yaml:"generator" json:"generator"`
	Blocks    map[string]StateBlockStats `yaml:"blocks,omitempty" json:"blocks,omitempty"`
	SnapshotAt string                    `yaml:"snapshotAt,omitempty" json:"snapshotAt,omitempty"`
}

// ArchiveManifestKind is the literal kind string written to disk.
const ArchiveManifestKind = "kapi-archive"

// ArchiveManifestFilename is the canonical filename at the zip root.
const ArchiveManifestFilename = "manifest.yaml"

// DecodeArchiveManifest parses the bytes from a `.klz`'s root
// `manifest.yaml`.
func DecodeArchiveManifest(b []byte) (*ArchiveManifest, error) {
	var m ArchiveManifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("project: decode archive manifest: %w", err)
	}
	if m.Kind != "" && m.Kind != ArchiveManifestKind {
		return nil, fmt.Errorf("project: archive manifest kind %q (expected %q)", m.Kind, ArchiveManifestKind)
	}
	if m.SchemaVersion == 0 {
		m.SchemaVersion = 1
	}
	return &m, nil
}

// EncodeArchiveManifest serializes for writing at the zip root.
func EncodeArchiveManifest(m *ArchiveManifest) ([]byte, error) {
	if m == nil {
		return nil, errors.New("project: nil archive manifest")
	}
	if m.SchemaVersion == 0 {
		m.SchemaVersion = 1
	}
	if m.Kind == "" {
		m.Kind = ArchiveManifestKind
	}
	return yaml.Marshal(m)
}

// Package project provides the .kapi project file format.
//
// A .kapi file is a self-contained YAML document that captures a localization
// workflow recipe: languages, content patterns, flows, tool configs, and plugin
// requirements. Users can save .kapi files anywhere, have multiple per directory,
// and share them via git or email.
//
// The .kapi file contains no credentials (those come from the OS keychain or
// environment variables) and no state (no sync cursors or caches).
package project

import (
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/flow"

	"gopkg.in/yaml.v3"
)

// CurrentVersion is the schema version for .kapi files.
const CurrentVersion = "v1"

// KapiProject is the root type for a .kapi project file.
type KapiProject struct {
	Version         string                     `yaml:"version" json:"version"`
	Name            string                     `yaml:"name" json:"name"`
	BasePath        string                     `yaml:"base_path,omitempty" json:"base_path,omitempty"`
	SourceLanguage  string                     `yaml:"source_language,omitempty" json:"source_language,omitempty"`
	TargetLanguages []string                   `yaml:"target_languages,omitempty" json:"target_languages,omitempty"`
	Content         []ContentEntry             `yaml:"content,omitempty" json:"content,omitempty"`
	Preset          string                     `yaml:"preset,omitempty" json:"preset,omitempty"`
	Plugins         []string                   `yaml:"plugins,omitempty" json:"plugins,omitempty"`
	Flows           map[string]*flow.StepsSpec `yaml:"flows,omitempty" json:"flows,omitempty"`
	Defaults        Defaults                   `yaml:"defaults,omitempty" json:"defaults,omitempty"`
}

// ContentEntry maps local files to format and target path patterns.
type ContentEntry struct {
	Path   string `yaml:"path" json:"path"`
	Format string `yaml:"format,omitempty" json:"format,omitempty"`
	Target string `yaml:"target,omitempty" json:"target,omitempty"`
}

// Defaults holds project-wide processing defaults.
type Defaults struct {
	Concurrency    int    `yaml:"concurrency,omitempty" json:"concurrency,omitempty"`
	ParallelBlocks int    `yaml:"parallel_blocks,omitempty" json:"parallel_blocks,omitempty"`
	Encoding       string `yaml:"encoding,omitempty" json:"encoding,omitempty"`
}

// Load reads a .kapi project file from the given path.
func Load(path string) (*KapiProject, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read project file: %w", err)
	}

	var proj KapiProject
	if err := yaml.Unmarshal(data, &proj); err != nil {
		return nil, fmt.Errorf("parse project file: %w", err)
	}

	if err := proj.Validate(); err != nil {
		return nil, fmt.Errorf("invalid project file: %w", err)
	}

	return &proj, nil
}

// Save writes a .kapi project file to the given path.
func Save(path string, proj *KapiProject) error {
	if proj.Version == "" {
		proj.Version = CurrentVersion
	}

	data, err := yaml.Marshal(proj)
	if err != nil {
		return fmt.Errorf("marshal project: %w", err)
	}

	// Atomic write: temp file + rename to avoid corruption on crash.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write project file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename project file: %w", err)
	}

	return nil
}

// Validate checks that the project file is well-formed.
func (p *KapiProject) Validate() error {
	if p.Version == "" {
		return fmt.Errorf("version is required")
	}
	if p.Version != CurrentVersion {
		return fmt.Errorf("unsupported version %q (expected %q)", p.Version, CurrentVersion)
	}
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	for i, c := range p.Content {
		if c.Path == "" {
			return fmt.Errorf("content[%d]: path is required", i)
		}
	}
	for name, spec := range p.Flows {
		if len(spec.Steps) == 0 {
			return fmt.Errorf("flow %q: at least one step is required", name)
		}
		for j, step := range spec.Steps {
			if step.Tool == "" && len(step.Parallel) == 0 {
				return fmt.Errorf("flow %q step[%d]: tool is required", name, j)
			}
		}
	}
	return nil
}

// GetFlow returns the StepsSpec for a named flow, or nil if not found.
func (p *KapiProject) GetFlow(name string) *flow.StepsSpec {
	if p.Flows == nil {
		return nil
	}
	return p.Flows[name]
}

// FlowNames returns the names of all flows defined in the project.
func (p *KapiProject) FlowNames() []string {
	names := make([]string, 0, len(p.Flows))
	for name := range p.Flows {
		names = append(names, name)
	}
	return names
}

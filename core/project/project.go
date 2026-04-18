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
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"

	"gopkg.in/yaml.v3"
)

// CurrentVersion is the schema version for .kapi files.
const CurrentVersion = "v1"

// KapiProject is the root type for a .kapi project file.
type KapiProject struct {
	Version  string                     `yaml:"version" json:"version"`
	Name     string                     `yaml:"name,omitempty" json:"name"`
	Plugins  map[string]PluginSpec      `yaml:"plugins,omitempty" json:"plugins,omitempty"`
	Defaults Defaults                   `yaml:"defaults,omitempty" json:"defaults,omitempty"`
	Content  []ContentCollection        `yaml:"content,omitempty" json:"content,omitempty"`
	Preset   string                     `yaml:"preset,omitempty" json:"preset,omitempty"`
	Flows    map[string]*flow.StepsSpec `yaml:"flows,omitempty" json:"flows,omitempty"`
}

// Defaults holds project-wide processing defaults.
type Defaults struct {
	SourceLanguage  model.LocaleID            `yaml:"source_language,omitempty" json:"source_language,omitempty"`
	TargetLanguages []model.LocaleID          `yaml:"target_languages,omitempty" json:"target_languages,omitempty"`
	LocaleFormat    string                    `yaml:"locale_format,omitempty" json:"locale_format,omitempty"` // "bcp-47" (default) or "posix"
	Concurrency     int                       `yaml:"concurrency,omitempty" json:"concurrency,omitempty"`
	ParallelBlocks  int                       `yaml:"parallel_blocks,omitempty" json:"parallel_blocks,omitempty"`
	Encoding        string                    `yaml:"encoding,omitempty" json:"encoding,omitempty"`
	Formats         map[string]FormatDefaults `yaml:"formats,omitempty" json:"formats,omitempty"`
}

// FormatDefaults holds project-level default settings for a specific format.
type FormatDefaults struct {
	Preset   string         `yaml:"preset,omitempty" json:"preset,omitempty"`
	Config   map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
	Priority int            `yaml:"priority,omitempty" json:"priority,omitempty"`
}

// PluginSpec describes a plugin dependency with version constraints and settings.
// Supports short form (bare string → version range) and long form (struct).
//
// Short form: okapi: "^1.47.0"
// Long form:
//
//	okapi:
//	  version: "^0.38.0"
//	  framework_version: "^1.47.0"
//	  format_priority: 200
type PluginSpec struct {
	Version          string `yaml:"version,omitempty" json:"version,omitempty"`
	FrameworkVersion string `yaml:"framework_version,omitempty" json:"framework_version,omitempty"`
	FormatPriority   int    `yaml:"format_priority,omitempty" json:"format_priority,omitempty"`
}

// UnmarshalYAML implements custom YAML unmarshaling for PluginSpec.
// A scalar string is treated as the version range (short form).
// A mapping is decoded as the full struct (long form).
func (s *PluginSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		s.Version = node.Value
		return nil
	}
	type pluginSpecAlias PluginSpec
	var alias pluginSpecAlias
	if err := node.Decode(&alias); err != nil {
		return err
	}
	*s = PluginSpec(alias)
	return nil
}

// ContentCollection is either a bare content entry or a named collection of items.
//
// Bare entry (has path, no items):
//
//   - path: "src/**/*"
//     target: "output/{lang}/**/*"
//
// Collection (has name and items):
//
//   - name: Marketing
//     target_languages: [fr-FR]
//     items:
//   - path: "marketing/*.html"
//     format: okf_html
type ContentCollection struct {
	// Collection fields (long form).
	Name            string           `yaml:"name,omitempty" json:"name,omitempty"`
	SourceLanguage  model.LocaleID   `yaml:"source_language,omitempty" json:"source_language,omitempty"`
	TargetLanguages []model.LocaleID `yaml:"target_languages,omitempty" json:"target_languages,omitempty"`
	Items           []ContentItem    `yaml:"items,omitempty" json:"items,omitempty"`

	// Archive is the project-relative path to the .klz that carries
	// this collection's extracted content + translations. When set,
	// `kapi status` and `kapi sync` know where to read/write. Omit
	// for file-based flows that never materialise a .klz.
	//
	// Custom extraction (e.g. JSX → klf blocks) is expressed via a
	// `format: { name: exec, config: { command } }` on the item,
	// not via a separate field. See core/formats/exec.
	Archive string `yaml:"archive,omitempty" json:"archive,omitempty"`

	// Bare entry fields (short form — promoted from ContentItem).
	Path   string      `yaml:"path,omitempty" json:"path,omitempty"`
	Format *FormatSpec `yaml:"format,omitempty" json:"format,omitempty"`
	Target string      `yaml:"target,omitempty" json:"target,omitempty"`
}

// IsBareEntry reports whether this is a bare entry (has path, no items).
func (c *ContentCollection) IsBareEntry() bool {
	return c.Path != "" && len(c.Items) == 0
}

// EffectiveItems returns the items for this collection. For bare entries, it
// wraps the promoted fields as a single-item slice.
func (c *ContentCollection) EffectiveItems() []ContentItem {
	if c.IsBareEntry() {
		return []ContentItem{{
			Path:   c.Path,
			Format: c.Format,
			Target: c.Target,
		}}
	}
	return c.Items
}

// ContentItem is a single content pattern within a collection.
type ContentItem struct {
	Path            string           `yaml:"path" json:"path"`
	Format          *FormatSpec      `yaml:"format,omitempty" json:"format,omitempty"`
	Target          string           `yaml:"target,omitempty" json:"target,omitempty"`
	SourceLanguage  model.LocaleID   `yaml:"source_language,omitempty" json:"source_language,omitempty"`
	TargetLanguages []model.LocaleID `yaml:"target_languages,omitempty" json:"target_languages,omitempty"`
}

// ResolvedSourceLanguage returns the source language for this item, falling
// back through collection and project defaults.
func (item *ContentItem) ResolvedSourceLanguage(coll *ContentCollection, defaults Defaults) model.LocaleID {
	if item.SourceLanguage != "" {
		return item.SourceLanguage
	}
	if coll != nil && coll.SourceLanguage != "" {
		return coll.SourceLanguage
	}
	return defaults.SourceLanguage
}

// ResolvedTargetLanguages returns the target languages for this item, falling
// back through collection and project defaults.
func (item *ContentItem) ResolvedTargetLanguages(coll *ContentCollection, defaults Defaults) []model.LocaleID {
	if len(item.TargetLanguages) > 0 {
		return item.TargetLanguages
	}
	if coll != nil && len(coll.TargetLanguages) > 0 {
		return coll.TargetLanguages
	}
	return defaults.TargetLanguages
}

// FormatSpec is the per-item format override.
// Supports short form (scalar string → just the name) and long form (mapping).
//
// Short form: format: okf_html
// Long form:
//
//	format:
//	  name: okf_html
//	  preset: lenient
//	  config: {escapeGT: false}
type FormatSpec struct {
	Name   string         `yaml:"name" json:"name"`
	Preset string         `yaml:"preset,omitempty" json:"preset,omitempty"`
	Config map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

// UnmarshalYAML implements custom YAML unmarshaling for FormatSpec.
// A scalar string is treated as the format name (short form).
// A mapping is decoded as the full struct (long form).
func (f *FormatSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		f.Name = node.Value
		return nil
	}
	type formatSpecAlias FormatSpec
	var alias formatSpecAlias
	if err := node.Decode(&alias); err != nil {
		return err
	}
	*f = FormatSpec(alias)
	return nil
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

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(proj); err != nil {
		return fmt.Errorf("marshal project: %w", err)
	}

	// Atomic write: temp file + rename to avoid corruption on crash.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
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
		return errors.New("version is required")
	}
	if p.Version != CurrentVersion {
		return fmt.Errorf("unsupported version %q (expected %q)", p.Version, CurrentVersion)
	}
	for i, c := range p.Content {
		if c.IsBareEntry() {
			if c.Path == "" {
				return fmt.Errorf("content[%d]: path is required for bare entries", i)
			}
			if len(c.Items) > 0 {
				return fmt.Errorf("content[%d]: bare entry cannot have items", i)
			}
		} else {
			// Collection form.
			if c.Path != "" {
				return fmt.Errorf("content[%d]: collection %q cannot have a path (use items)", i, c.Name)
			}
			if len(c.Items) == 0 {
				return fmt.Errorf("content[%d]: collection %q must have at least one item", i, c.Name)
			}
			for j, item := range c.Items {
				if item.Path == "" {
					return fmt.Errorf("content[%d].items[%d]: path is required", i, j)
				}
			}
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

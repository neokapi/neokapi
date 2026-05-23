// Package project provides the .kapi project file format.
//
// A .kapi file is a self-contained YAML document that captures a localization
// workflow recipe: languages, content patterns, flows, tool configs, and plugin
// requirements. Users can save .kapi files anywhere, have multiple per directory,
// and share them via git or email.
//
// The .kapi file contains no credentials (those come from the OS keychain or
// environment variables) and no state (no sync cursors or caches).
//
// # Extension mechanism
//
// Platform layers attach their own typed schema by reading and
// writing through the `Extras` field on KapiProject, Defaults, and ContentItem.
// Unknown top-level YAML keys are captured as `yaml.Node` values; platforms
// decode them into their own types and re-encode on save. The framework knows
// nothing about platform-specific extensions and round-trips them verbatim.
package project

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"

	"gopkg.in/yaml.v3"
)

// sortMissingRequires sorts a slice of MissingRequirement by plugin name
// for deterministic output.
func sortMissingRequires(s []MissingRequirement) {
	sort.SliceStable(s, func(i, j int) bool {
		return s[i].Plugin < s[j].Plugin
	})
}

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

	// Requires lists plugin dependencies as a map of plugin name → version
	// constraint. Validation fails if any named plugin (or extension group
	// of the same name) has no registered extension in the loading process.
	// A recipe with `requires: { bowrain: "^1.0" }` will refuse to load in a
	// binary that has not registered the bowrain extension.
	//
	// Version constraints follow semver (`^1.0`, `>=1.47.0`, `~1.4.2`,
	// `1.4.0` exact-match, `*` any). The map form is mandatory — a bare-list
	// form (`requires: [bowrain]`) is rejected with an actionable error.
	Requires RequiresMap `yaml:"requires,omitempty" json:"requires,omitempty"`

	// Extras captures any top-level YAML keys the framework does not know
	// about. Platform layers decode their own typed schema
	// from here at load time and re-encode on save. Round-tripping a recipe
	// through the framework alone preserves these keys verbatim.
	Extras map[string]yaml.Node `yaml:",inline" json:"-"`
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

	// Exclude is a list of glob patterns skipped during content scanning.
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`

	// Merge governs kapi merge behavior (AD-017).
	Merge MergeDefaults `yaml:"merge,omitempty" json:"merge,omitempty"`

	// TM governs TM pre-fill on kapi extract and TM write-back on kapi merge (AD-017).
	TM TMDefaults `yaml:"tm,omitempty" json:"tm,omitempty"`

	// Segmentation governs the opt-in sentence-level segmentation overlay
	// applied on extract (AD-017).
	Segmentation SegmentationDefaults `yaml:"segmentation,omitempty" json:"segmentation,omitempty"`

	// Redaction governs replacing sensitive content with protected
	// placeholders before processing and restoring it afterwards. nil means
	// no redaction.
	Redaction *RedactionSpec `yaml:"redaction,omitempty" json:"redaction,omitempty"`

	// Extras captures unknown keys under `defaults:`. Platform layers decode
	// their own defaults from this map.
	Extras map[string]yaml.Node `yaml:",inline" json:"-"`
}

// RedactionSpec configures content redaction. The sensitive term list itself
// lives in a dedicated rules file (so it can be gitignored) referenced by
// Rules; this spec only points at it and selects detection backends. Declared
// under defaults: project-wide and overridable per content item.
type RedactionSpec struct {
	// Enabled turns redaction on for extract/merge.
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	// Rules is the path to a redaction rules YAML file.
	Rules string `yaml:"rules,omitempty" json:"rules,omitempty"`
	// Detectors selects detection backends: "rules" and/or "entities".
	Detectors []string `yaml:"detectors,omitempty" json:"detectors,omitempty"`
	// Placeholder overrides the visible stand-in template, e.g.
	// "[REDACTED:{category}]".
	Placeholder string `yaml:"placeholder,omitempty" json:"placeholder,omitempty"`
}

// validate checks the spec's detector names. Detector identifiers mirror the
// redact tool's constants ("rules", "entities"); project keeps them as
// literals to avoid depending on the tools package.
func (r *RedactionSpec) validate() error {
	if r == nil {
		return nil
	}
	for _, d := range r.Detectors {
		switch d {
		case "rules", "entities":
		default:
			return fmt.Errorf("redaction: unknown detector %q (want \"rules\" or \"entities\")", d)
		}
	}
	return nil
}

// FormatDefaults holds project-level default settings for a specific format.
type FormatDefaults struct {
	Preset   string         `yaml:"preset,omitempty" json:"preset,omitempty"`
	Config   map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
	Priority int            `yaml:"priority,omitempty" json:"priority,omitempty"`
}

// Conflict policy values for Defaults.Merge.ConflictPolicy (AD-017).
const (
	ConflictPolicyTranslatorWins = "translator-wins"
	ConflictPolicyExistingWins   = "existing-wins"
	ConflictPolicyNewestWins     = "newest-wins"
)

// DefaultFuzzyThreshold is the TM fuzzy-match cutoff (percent) applied when
// the recipe does not specify one (AD-017).
const DefaultFuzzyThreshold = 75

// MergeDefaults governs kapi merge behavior (AD-017).
type MergeDefaults struct {
	// ConflictPolicy governs how merge applies a translator's target when
	// an existing on-disk target or TM TU already has a translation. Valid
	// values: "translator-wins" (default), "existing-wins", "newest-wins".
	ConflictPolicy string `yaml:"conflict_policy,omitempty" json:"conflict_policy,omitempty"`
}

// TMDefaults governs TM pre-fill on extract and TM write-back on merge (AD-017).
type TMDefaults struct {
	// FuzzyThreshold is the minimum fuzzy match score (0..100) to pre-fill
	// the target on extract. Defaults to DefaultFuzzyThreshold when zero.
	FuzzyThreshold int `yaml:"fuzzy_threshold,omitempty" json:"fuzzy_threshold,omitempty"`

	// Read lists additional read-only TM files consulted during pre-fill on
	// extract. Writes always go to the project TM, never to these.
	Read []string `yaml:"read,omitempty" json:"read,omitempty"`
}

// SegmentationDefaults governs the opt-in SRX segmentation overlay (AD-017).
type SegmentationDefaults struct {
	// Source toggles sentence-level segmentation of source text on extract.
	Source bool `yaml:"source,omitempty" json:"source,omitempty"`

	// SRX optionally points at an SRX rules file. When empty, built-in
	// default rules are used.
	SRX string `yaml:"srx,omitempty" json:"srx,omitempty"`
}

// ResolvedConflictPolicy returns the effective conflict policy, applying the
// default when the recipe does not set one.
func (m MergeDefaults) ResolvedConflictPolicy() string {
	if m.ConflictPolicy == "" {
		return ConflictPolicyTranslatorWins
	}
	return m.ConflictPolicy
}

// ResolvedFuzzyThreshold returns the effective TM fuzzy threshold, applying
// the default when the recipe does not set one.
func (t TMDefaults) ResolvedFuzzyThreshold() int {
	if t.FuzzyThreshold == 0 {
		return DefaultFuzzyThreshold
	}
	return t.FuzzyThreshold
}

func (m MergeDefaults) validate() error {
	switch m.ConflictPolicy {
	case "", ConflictPolicyTranslatorWins, ConflictPolicyExistingWins, ConflictPolicyNewestWins:
		return nil
	default:
		return fmt.Errorf("defaults.merge.conflict_policy: unknown value %q (expected one of %q, %q, %q)",
			m.ConflictPolicy,
			ConflictPolicyTranslatorWins,
			ConflictPolicyExistingWins,
			ConflictPolicyNewestWins)
	}
}

func (t TMDefaults) validate() error {
	if t.FuzzyThreshold < 0 || t.FuzzyThreshold > 100 {
		return fmt.Errorf("defaults.tm.fuzzy_threshold: %d out of range (0..100)", t.FuzzyThreshold)
	}
	return nil
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

	// Bare entry fields (short form — promoted from ContentItem).
	Path   string      `yaml:"path,omitempty" json:"path,omitempty"`
	Format *FormatSpec `yaml:"format,omitempty" json:"format,omitempty"`
	Target string      `yaml:"target,omitempty" json:"target,omitempty"`

	// Extras captures keys the framework does not know about, both for
	// bare entries and for named-collection wrappers. Platform layers
	// decode their per-collection / per-bare-entry extensions from here.
	Extras map[string]yaml.Node `yaml:",inline" json:"-"`
}

// IsBareEntry reports whether this is a bare entry (has path, no items).
func (c *ContentCollection) IsBareEntry() bool {
	return c.Path != "" && len(c.Items) == 0
}

// EffectiveItems returns the items for this collection. For bare entries, it
// wraps the promoted fields as a single-item slice (carrying the bare
// entry's Extras through, so platform-specific per-item fields survive).
func (c *ContentCollection) EffectiveItems() []ContentItem {
	if c.IsBareEntry() {
		return []ContentItem{{
			Path:   c.Path,
			Format: c.Format,
			Target: c.Target,
			Extras: c.Extras,
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

	// Redaction overrides the project-wide redaction spec for this item.
	// nil means inherit defaults.
	Redaction *RedactionSpec `yaml:"redaction,omitempty" json:"redaction,omitempty"`

	// Extras captures unknown keys at the per-item level. Platform layers
	// decode their per-item fields from here.
	Extras map[string]yaml.Node `yaml:",inline" json:"-"`
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

// FindProject discovers the project layout by walking up from start and
// loads the recipe. Returns the parsed KapiProject and its on-disk Layout.
//
// Pass an empty string to start from the current working directory.
// When the start path is itself a `.kapi` recipe file, that exact recipe
// is loaded directly.
func FindProject(start string) (*KapiProject, Layout, error) {
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, Layout{}, fmt.Errorf("project: get cwd: %w", err)
		}
		start = cwd
	}
	var layout Layout
	if info, err := os.Stat(start); err == nil && !info.IsDir() {
		l, err := LayoutFor(start)
		if err != nil {
			return nil, Layout{}, err
		}
		layout = l
	} else {
		l, err := ResolveLayout(start)
		if err != nil {
			return nil, Layout{}, err
		}
		layout = l
	}
	proj, err := Load(layout.RecipePath)
	if err != nil {
		return nil, Layout{}, err
	}
	return proj, layout, nil
}

// LoadOptions tunes Load behavior.
//
// The zero value matches the historical Load semantics (full validation
// including the requires-extension check). Setting SkipRequiresCheck lets
// higher layers (the CLI) intercept missing-plugin failures and offer an
// interactive auto-install before re-validating.
type LoadOptions struct {
	// SkipRequiresCheck disables the "every requires.<plugin> must have
	// a registered extension" check during Validate. The rest of
	// validation still runs (version, content shape, flow shape, extras
	// schema, version-constraint syntax). Callers that set this should
	// run ValidateRequires explicitly once they've taken any
	// remediation actions (e.g. auto-installing the missing plugin).
	SkipRequiresCheck bool
}

// Load reads a .kapi project file from the given path. It is a
// backwards-compatible wrapper around LoadWithOptions that runs full
// validation.
func Load(path string) (*KapiProject, error) {
	return LoadWithOptions(path, LoadOptions{})
}

// LoadWithOptions reads a .kapi project file with the given options.
//
// When opts.SkipRequiresCheck is set, missing extension groups named in
// the recipe's requires: block do NOT fail Validate. The caller is
// expected to call ValidateRequires once it's had a chance to install
// missing plugins.
func LoadWithOptions(path string, opts LoadOptions) (*KapiProject, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read project file: %w", err)
	}

	var proj KapiProject
	if err := yaml.Unmarshal(data, &proj); err != nil {
		return nil, fmt.Errorf("parse project file: %w", err)
	}

	if err := proj.validate(opts); err != nil {
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

// Validate checks that the project file is well-formed. Equivalent to
// validate(LoadOptions{}) — i.e. it runs the requires-extension check.
func (p *KapiProject) Validate() error {
	return p.validate(LoadOptions{})
}

// validate is the option-driven implementation behind Validate /
// LoadWithOptions. When opts.SkipRequiresCheck is set, it still
// validates the syntax of every requires constraint but does not fail
// when an extension group is missing.
func (p *KapiProject) validate(opts LoadOptions) error {
	if p.Version == "" {
		return errors.New("version is required")
	}
	if p.Version != CurrentVersion {
		return fmt.Errorf("unsupported version %q (expected %q)", p.Version, CurrentVersion)
	}
	if err := p.Defaults.Merge.validate(); err != nil {
		return err
	}
	if err := p.Defaults.TM.validate(); err != nil {
		return err
	}
	if err := p.Defaults.Redaction.validate(); err != nil {
		return err
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
				if err := item.Redaction.validate(); err != nil {
					return fmt.Errorf("content[%d].items[%d]: %w", i, j, err)
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
	if err := p.validateRequiresSyntax(); err != nil {
		return err
	}
	if !opts.SkipRequiresCheck {
		if err := p.validateRequiresExtensionsRegistered(); err != nil {
			return err
		}
	}
	if err := validateExtras(ScopeProject, "", p.Extras); err != nil {
		return err
	}
	if err := validateExtras(ScopeDefaults, "defaults.", p.Defaults.Extras); err != nil {
		return err
	}
	for i, c := range p.Content {
		prefix := fmt.Sprintf("content[%d].", i)
		if err := validateExtras(ScopeCollection, prefix, c.Extras); err != nil {
			return err
		}
		for j, item := range c.Items {
			ip := fmt.Sprintf("content[%d].items[%d].", i, j)
			if err := validateExtras(ScopeItem, ip, item.Extras); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateRequiresSyntax checks every Requires entry for non-empty
// plugin name and well-formed version constraint. It does not check
// whether the extension is registered with the framework.
func (p *KapiProject) validateRequiresSyntax() error {
	for name, constraint := range p.Requires {
		if name == "" {
			return errors.New("requires: plugin name cannot be empty")
		}
		if !validVersionConstraint(constraint) {
			return fmt.Errorf("requires.%s: invalid version constraint %q (use semver: ^1.0, >=1.4.0, 1.4.0, ~1.4.2, or *)", name, constraint)
		}
	}
	return nil
}

// validateRequiresExtensionsRegistered checks that every plugin named in
// Requires has at least one Extension registered with the framework.
func (p *KapiProject) validateRequiresExtensionsRegistered() error {
	for name, constraint := range p.Requires {
		if !HasExtensionGroup(name) {
			return fmt.Errorf("recipe requires plugin %q (%s) but no matching extension is registered (install with `kapi plugin install %s`)", name, constraint, name)
		}
	}
	return nil
}

// MissingRequires returns the subset of Requires entries for which no
// Extension group is currently registered, in deterministic (sorted)
// order. Higher layers can use this to drive an interactive
// auto-install prompt.
func (p *KapiProject) MissingRequires() []MissingRequirement {
	var missing []MissingRequirement
	for name, constraint := range p.Requires {
		if !HasExtensionGroup(name) {
			missing = append(missing, MissingRequirement{
				Plugin:     name,
				Constraint: constraint,
			})
		}
	}
	// Deterministic order so the prompt UI is stable.
	sortMissingRequires(missing)
	return missing
}

// ValidateRequires re-runs the requires-extension check on its own.
// Useful after LoadWithOptions(SkipRequiresCheck: true) and any
// remediation (e.g. auto-installing the missing plugin) has been
// performed.
func (p *KapiProject) ValidateRequires() error {
	if err := p.validateRequiresSyntax(); err != nil {
		return err
	}
	return p.validateRequiresExtensionsRegistered()
}

// MissingRequirement names one declared dependency for which no
// matching extension group is currently registered.
type MissingRequirement struct {
	// Plugin is the plugin name as it appears in requires:.
	Plugin string
	// Constraint is the version constraint declared in requires:.
	Constraint string
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

// IteratedItem pairs a ContentItem with the parent collection it came
// from, so callers can resolve fall-through fields (source/target language,
// collection name) without duplicating logic.
type IteratedItem struct {
	Collection *ContentCollection
	Item       ContentItem
}

// IterateContent yields every ContentItem in the project, walking both bare
// entries and named collections. The Collection pointer is non-nil for
// items that came from a named collection so callers can read its Name.
func (p *KapiProject) IterateContent() []IteratedItem {
	var out []IteratedItem
	for i := range p.Content {
		coll := &p.Content[i]
		if coll.IsBareEntry() {
			out = append(out, IteratedItem{
				Collection: coll,
				Item: ContentItem{
					Path:   coll.Path,
					Format: coll.Format,
					Target: coll.Target,
					Extras: coll.Extras,
				},
			})
			continue
		}
		for _, item := range coll.Items {
			out = append(out, IteratedItem{Collection: coll, Item: item})
		}
	}
	return out
}

// SetExtra encodes value as a YAML node and stores it under key in the
// project's Extras map. Used by platform layers to persist
// their typed extensions through Save.
func (p *KapiProject) SetExtra(key string, value any) error {
	if p.Extras == nil {
		p.Extras = map[string]yaml.Node{}
	}
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return fmt.Errorf("encode extra %q: %w", key, err)
	}
	p.Extras[key] = node
	return nil
}

// GetExtra decodes the value stored under key in Extras into target. Returns
// (false, nil) if the key is not present, (true, nil) on success.
func (p *KapiProject) GetExtra(key string, target any) (bool, error) {
	node, ok := p.Extras[key]
	if !ok {
		return false, nil
	}
	if err := node.Decode(target); err != nil {
		return true, fmt.Errorf("decode extra %q: %w", key, err)
	}
	return true, nil
}

// DeleteExtra removes a key from Extras. No-op if the key is absent.
func (p *KapiProject) DeleteExtra(key string) {
	delete(p.Extras, key)
}

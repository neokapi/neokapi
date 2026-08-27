// Package project provides the .kapi project file format.
//
// A .kapi file is a self-contained YAML document that captures a content
// workflow recipe: the content to process, the languages and variants, the
// flows and tool configs to run over it, and the plugin requirements. The same
// recipe shape drives a monolingual voice and terminology check over
// source content and a multilingual translate-and-QA round-trip alike. Users
// can save .kapi files anywhere, have multiple per directory, and share them
// via git or email.
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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/yamledit"

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

// retiredProjectKeys are top-level keys the recipe does not have, mapped to
// what a recipe should say instead. Left unnamed, such a key would be captured
// as an unknown extension and silently carry nothing.
var retiredProjectKeys = map[string]string{
	"content":     "collections",
	"coordinates": "profiles (each profile declares its channels)",
}

// KapiProject is the root type for a .kapi project file.
type KapiProject struct {
	Version     string                     `yaml:"version" json:"version"`
	Name        string                     `yaml:"name,omitempty" json:"name"`
	Plugins     map[string]PluginSpec      `yaml:"plugins,omitempty" json:"plugins,omitempty"`
	Defaults    Defaults                   `yaml:"defaults,omitempty" json:"defaults,omitzero"`
	Collections []Collection               `yaml:"collections,omitempty" json:"collections,omitempty"`
	Preset      string                     `yaml:"preset,omitempty" json:"preset,omitempty"`
	Flows       map[string]*flow.StepsSpec `yaml:"flows,omitempty" json:"flows,omitempty"`

	// Profiles binds governance to a product, keyed by the product's name. A
	// project is not always one voice: a repository holding both a framework
	// and the platform built on it carries two, and one project-wide binding
	// governs the wrong one half the time. Each profile declares the channels
	// its product ships on, and a collection names the point its content sits
	// at with one `channel:` reference (Collection.Channel). Empty means the
	// whole project sits at one point, under defaults.voice /
	// defaults.terms_source. See profiles.go.
	Profiles map[string]Profile `yaml:"profiles,omitempty" json:"profiles,omitempty"`

	// Ship gates decide when localized content is shippable, as coverage
	// thresholds over the lifecycle ladder (see core/gate). Three optional,
	// additive forms:
	//   ShipGate  — a single catch-all gate ({translated: 100, reviewed: 100}).
	//   ShipGates — a when/gate rule list; most-specific rule wins.
	//   Gates     — a named registry referenced by a rule's `gate: <name>`.
	// BuildShipGates resolves these into an evaluatable gate.RuleSet.
	ShipGate  gate.Gate            `yaml:"ship_gate,omitempty" json:"ship_gate,omitempty"`
	ShipGates []ShipGateRule       `yaml:"ship_gates,omitempty" json:"ship_gates,omitempty"`
	Gates     map[string]gate.Gate `yaml:"gates,omitempty" json:"gates,omitempty"`

	// Verified gates decide when localized content is "human-verified": the
	// second gate, evaluated exactly like the ship gate but against a bar that
	// implies a person reviewed or signed off the work (e.g. {reviewed: 100}).
	// A locale that clears its ship gate but not its verified gate ships flagged
	// AI in a language picker; a verified locale carries no badge. The two gates
	// are independent — being verified is not a prerequisite for shipping. Same
	// additive forms and precedence as the ship gate, resolving a rule's
	// `gate: <name>` reference against the same Gates registry:
	//   VerifiedGate  — a single catch-all gate.
	//   VerifiedGates — a when/gate rule list; most-specific rule wins.
	// With NO verified gate configured, nothing is verified: BuildVerifiedGates
	// returns an empty RuleSet, so every locale reads unverified (the honest
	// default — a project opts in to "verified" by declaring the bar).
	VerifiedGate  gate.Gate      `yaml:"verified_gate,omitempty" json:"verified_gate,omitempty"`
	VerifiedGates []ShipGateRule `yaml:"verified_gates,omitempty" json:"verified_gates,omitempty"`

	// SourceGate is the source-readiness bar: a single coverage gate over the
	// source authoring ladder (authored → checked → approved), e.g.
	// {checked: 100}. It is the source-side counterpart of ShipGate — it gates
	// the author's own content, not the translations. BuildSourceGate
	// resolves it; evaluated by `kapi check --ship` (never an ordinary
	// build).
	SourceGate gate.Gate `yaml:"source_gate,omitempty" json:"source_gate,omitempty"`

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
	SourceLanguage  model.LocaleID   `yaml:"source_language,omitempty" json:"source_language,omitempty"`
	TargetLanguages []model.LocaleID `yaml:"target_languages,omitempty" json:"target_languages,omitempty"`

	// Flow names the project's default flow — the one `kapi run` (with no flow
	// argument) executes to bring the project's content up to date across every
	// target language. It is a built-in composed flow name or a key in `flows:`.
	// Empty means there is no default; `kapi run` then requires an explicit flow.
	Flow string `yaml:"flow,omitempty" json:"flow,omitempty"`

	// Materialize governs whether the convergence loop (`kapi up`) owns
	// delivery of the localized target files (#1078 C2/C3).
	//
	// "on-converge" makes the run responsible for them, under the ship gate:
	// its passes draft into a run-local tree and only a locale whose gated
	// scopes are all shippable has its files written to the collection's
	// `target:` path. "manual" (the default) leaves delivery to an explicit
	// `kapi merge` (or `up --materialize`) and claims no gate — its passes
	// write where the recipe points as they produce each unit.
	Materialize string `yaml:"materialize,omitempty" json:"materialize,omitempty"`

	// Jobs is how many target languages one convergence pass (`kapi up`)
	// runs concurrently. 0 (the default) leaves it to the runner's default;
	// `up --jobs` overrides per run.
	Jobs int `yaml:"jobs,omitempty" json:"jobs,omitempty"`

	// SourceGate is the source-first convergence gate: the SourceStatus a
	// source block must reach before its translations are produced. Source-first
	// convergence settles the source (terminology + voice + source-QA) and gates
	// the fan-out on it, so an unsettled, non-compliant, un-term-checked source is
	// never translated into N locales only to be redone when it changes
	// (strategy 2026-07-dogfood doc 07 / roadmap epic 019).
	//
	// Values:
	//   ""         — unset; the runner applies the default gate (`checked`).
	//   "authored" — the presence baseline (any non-empty source qualifies).
	//   "checked"  — the DEFAULT: source cleared its automated terminology,
	//                voice, and source-QA checks (no human bottleneck).
	//   "approved" — a human/agent signed off the source (voice-critical or
	//                regulated projects).
	//   "none"     — the deliberate opt-out: no gate, raw MT / fan-out on push
	//                exactly as before source-first. You have to choose it.
	//
	// It is the level-based, per-project counterpart of the coverage-bar
	// SourceGate on KapiProject (which `kapi check --ship` evaluates); this one
	// governs the convergence fan-out.
	SourceGate string `yaml:"source_gate,omitempty" json:"source_gate,omitempty"`

	LocaleFormat   string                    `yaml:"locale_format,omitempty" json:"locale_format,omitempty"` // "bcp-47" (default) or "posix"
	Concurrency    int                       `yaml:"concurrency,omitempty" json:"concurrency,omitempty"`
	ParallelBlocks int                       `yaml:"parallel_blocks,omitempty" json:"parallel_blocks,omitempty"`
	Encoding       string                    `yaml:"encoding,omitempty" json:"encoding,omitempty"`
	Formats        map[string]FormatDefaults `yaml:"formats,omitempty" json:"formats,omitempty"`

	// Exclude is a list of glob patterns skipped during content scanning.
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`

	// Merge governs kapi merge behavior (AD-017).
	Merge MergeDefaults `yaml:"merge,omitempty" json:"merge,omitzero"`

	// content memory governs content memory pre-fill on kapi extract and content memory write-back on kapi merge (AD-017).
	Memory MemoryDefaults `yaml:"memory,omitempty" json:"memory,omitzero"`

	// Segmentation governs the opt-in sentence-level segmentation overlay
	// applied on extract (AD-017).
	Segmentation SegmentationDefaults `yaml:"segmentation,omitempty" json:"segmentation,omitzero"`

	// Annotations governs which of a block's stand-off annotations a writer
	// draws into the document as inline marks. Zero leaves each format's own
	// declaration standing.
	Annotations AnnotationDefaults `yaml:"annotations,omitempty" json:"annotations,omitzero"`

	// Redaction governs replacing sensitive content with protected
	// placeholders before processing and restoring it afterwards. nil means
	// no redaction.
	Redaction *RedactionSpec `yaml:"redaction,omitempty" json:"redaction,omitempty"`

	// Voice binds a voice profile as standing project context. When set,
	// project-scoped commands (voice check/rewrite/guide and project
	// translation flows) honor it with no profile flag. nil means no bound
	// voice profile.
	//
	// This is the framework binding under `defaults:`. It is distinct from
	// bowrain's top-level `brand_voice` extension (decoded from Extras),
	// which is a platform-level policy with collection scoping.
	Voice *VoiceBinding `yaml:"voice,omitempty" json:"voice,omitempty"`

	// Coordinates is the project's default point in the context space: the axes
	// every collection sits at unless it says otherwise.
	//
	// It exists because most axes default. A project has one brand far more
	// often than one per collection, so stating it per entry is noise that
	// drifts — the same reason Voice above is a default with a per-collection
	// override rather than a field repeated on every entry.
	//
	// The structural axes (product, channel) are NOT written here: they are
	// derived from a collection's `channel:`, and a default that could shadow
	// them would let a recipe contradict its own point. Declared axes only.
	Coordinates map[string]string `yaml:"coordinates,omitempty" json:"coordinates,omitempty"`

	// TermsSource binds the committed, git-tracked native source artifact
	// (a .terms.json document) the project terms store is compiled from. This is the
	// authored, reviewable form: `kapi apply` edits the .terms.json here and then
	// re-imports it into the gitignored terms tables inside `.kapi/work/store.db`,
	// so the store is written by exactly one path and `git diff` is the review
	// surface. The path resolves relative to the project root. Empty means no
	// bound source (whatever the store already holds is the only artifact).
	TermsSource string `yaml:"terms_source,omitempty" json:"terms_source,omitempty"`

	// MemorySource binds the committed, git-tracked native source artifact (a
	// .memory.json document) the project content memory is compiled from, the content memory
	// analogue of TermsSource. `kapi apply` edits the .memory.json here and
	// re-imports it into the project store. The path resolves relative to the
	// project root. Empty means no bound content memory source.
	MemorySource string `yaml:"memory_source,omitempty" json:"memory_source,omitempty"`

	// Tools holds project-level tool presets: per-tool config defaults applied
	// wherever the tool runs in a project flow. A flow step's own config
	// overrides the preset per key (step wins), so a project can pin, say,
	// redaction rules or a pseudo-translate prefix once while an individual
	// flow refines them. Resolution happens at tool construction and feeds the
	// same merged config to data-flow and placement validation, so a preset
	// that enables redact's entity detection makes the entity port required
	// exactly as an inline config would.
	Tools map[string]map[string]any `yaml:"tools,omitempty" json:"tools,omitempty"`

	// Locales holds per-target-language overrides, keyed by locale. Each entry's
	// tool presets merge on top of the project-wide Tools presets (and under a
	// flow step's own config) whenever a flow runs for that target locale — so a
	// project can declare an advanced feature once per locale ("de needs
	// redaction, others don't") without forking the flow. Convergence and any
	// targeted run honor it; locales with no entry use the project defaults.
	Locales map[string]LocaleDefaults `yaml:"locales,omitempty" json:"locales,omitempty"`

	// Extras captures unknown keys under `defaults:`. Platform layers decode
	// their own defaults from this map.
	Extras map[string]yaml.Node `yaml:",inline" json:"-"`
}

// LocaleDefaults holds per-target-language overrides applied when a flow runs
// for that locale. Tool presets are the lever: they merge on top of the
// project-wide defaults.tools (per-locale wins) and under a flow step's own
// config (the step still wins), the same precedence as the project presets.
type LocaleDefaults struct {
	Tools map[string]map[string]any `yaml:"tools,omitempty" json:"tools,omitempty"`
}

// VoiceBinding binds a voice profile — to the project under `defaults.voice`,
// or to a region of the context space under a profile's `voice:`. Exactly one
// source is expected: a standalone profile YAML (ProfileFile, resolved relative
// to the project root), a profile in the local voice store (Profile), or a
// built-in starter pack (Pack).
//
// The short form is the profile file itself — `voice: context/kapi-voice.yaml`
// — which is what a recipe writes when the profile is a file in the project,
// as it usually is.
type VoiceBinding struct {
	// ProfileFile is the path to a standalone profile YAML, resolved
	// relative to the project root.
	ProfileFile string `yaml:"profile_file,omitempty" json:"profile_file,omitempty"`
	// Profile names a profile in the local voice store.
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
	// Pack names a built-in starter pack.
	Pack string `yaml:"pack,omitempty" json:"pack,omitempty"`
}

// UnmarshalYAML accepts both forms: a scalar is the profile file, a mapping is
// the full binding.
func (b *VoiceBinding) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		b.ProfileFile = node.Value
		return nil
	}
	type voiceBindingAlias VoiceBinding
	var alias voiceBindingAlias
	if err := node.Decode(&alias); err != nil {
		return err
	}
	*b = VoiceBinding(alias)
	return nil
}

// MarshalYAML writes back the form the binding was authored in, so saving a
// recipe does not expand a plain profile path into a mapping.
func (b VoiceBinding) MarshalYAML() (any, error) {
	if b.Profile == "" && b.Pack == "" {
		return b.ProfileFile, nil
	}
	type voiceBindingAlias VoiceBinding
	return voiceBindingAlias(b), nil
}

// validate checks that exactly one voice source is set. field names the recipe
// key being validated (`defaults.voice`, or a profile's own `profiles[i].voice`),
// so the message points at the binding at fault.
func (b *VoiceBinding) validate(field string) error {
	if b == nil {
		return nil
	}
	count := 0
	for _, v := range []string{b.ProfileFile, b.Profile, b.Pack} {
		if v != "" {
			count++
		}
	}
	if count == 0 {
		return fmt.Errorf("%s: specify one of profile_file, profile, or pack", field)
	}
	if count > 1 {
		return fmt.Errorf("%s: profile_file, profile, and pack are mutually exclusive", field)
	}
	return nil
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

// Materialize policy values for Defaults.Materialize (#1078 C2/C3).
const (
	// MaterializeManual: the run does not own delivery — bringing the
	// localized files up to date is an explicit `kapi merge` (or `kapi up
	// --materialize`). The default, and the policy that claims no gate.
	MaterializeManual = "manual"
	// MaterializeOnConverge: the run owns delivery under the ship gate. Its
	// passes draft, and the localized files are written at the end for every
	// locale whose gated scopes are all shippable — a parked locale's files
	// are absent, not merely unblessed.
	MaterializeOnConverge = "on-converge"
)

// ResolvedMaterialize returns the effective materialize policy, applying the
// default (manual) when the recipe does not set one.
func (d Defaults) ResolvedMaterialize() string {
	if d.Materialize == "" {
		return MaterializeManual
	}
	return d.Materialize
}

// validateMaterialize checks Defaults.Materialize.
func (d Defaults) validateMaterialize() error {
	switch d.Materialize {
	case "", MaterializeManual, MaterializeOnConverge:
		return nil
	default:
		return fmt.Errorf("defaults.materialize: unknown value %q (expected %q or %q)",
			d.Materialize, MaterializeOnConverge, MaterializeManual)
	}
}

// DefaultFuzzyThreshold is the content memory fuzzy-match cutoff (percent) applied when
// the recipe does not specify one (AD-017).
const DefaultFuzzyThreshold = 75

// AnnotationDefaults narrows which annotation types a writer draws into the
// document as inline marks: a located term as an XLIFF <mrk>, as an HTML
// <span>.
//
// A format declares the ceiling. Its writer names the types it knows how to
// draw (format.InlineAnnotationWriter, recorded on
// registry.FormatInfo.InlineAnnotations), and a recipe can only narrow that
// set. Two things follow, both wanted: a format that gains the capability
// starts projecting without anyone editing a recipe, and a project that wants
// a clean export with no marks in it can still say so.
type AnnotationDefaults struct {
	// Write names the annotation types to draw, out of what the format
	// declares it can carry. Empty means the declaration stands: every type
	// the writer can draw, it draws.
	//
	// Naming a type the format cannot carry asks for nothing rather than
	// failing. The recipe is one document describing many formats, and a
	// project that wants terms marked wherever they can be should not have to
	// enumerate which of its outputs happen to support it.
	Write []string `yaml:"write,omitempty" json:"write,omitempty"`
}

// EffectiveInlineAnnotations is the annotation types a writer draws for this
// project: what the format declared it can carry, narrowed by what the recipe
// asked for, in the format's declared order.
func (a AnnotationDefaults) EffectiveInlineAnnotations(declared []string) []string {
	if len(a.Write) == 0 || len(declared) == 0 {
		return declared
	}
	want := make(map[string]bool, len(a.Write))
	for _, t := range a.Write {
		want[t] = true
	}
	out := make([]string, 0, len(declared))
	for _, t := range declared {
		if want[t] {
			out = append(out, t)
		}
	}
	return out
}

// MergeDefaults governs kapi merge behavior (AD-017).
type MergeDefaults struct {
	// ConflictPolicy governs how merge applies a translator's target when
	// an existing on-disk target or content memory TU already has a translation. Valid
	// values: "translator-wins" (default), "existing-wins", "newest-wins".
	ConflictPolicy string `yaml:"conflict_policy,omitempty" json:"conflict_policy,omitempty"`
}

// MemoryDefaults governs content memory pre-fill on extract and content memory write-back on merge (AD-017).
type MemoryDefaults struct {
	// FuzzyThreshold is the minimum fuzzy match score (0..100) to pre-fill
	// the target on extract. Defaults to DefaultFuzzyThreshold when zero.
	FuzzyThreshold int `yaml:"fuzzy_threshold,omitempty" json:"fuzzy_threshold,omitempty"`
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

// ResolvedFuzzyThreshold returns the effective content memory fuzzy threshold, applying
// the default when the recipe does not set one.
func (t MemoryDefaults) ResolvedFuzzyThreshold() int {
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

func (t MemoryDefaults) validate() error {
	if t.FuzzyThreshold < 0 || t.FuzzyThreshold > 100 {
		return fmt.Errorf("defaults.memory.fuzzy_threshold: %d out of range (0..100)", t.FuzzyThreshold)
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

// Collection is either a bare content entry or a named collection of content
// items.
//
// Bare entry (has path, no content):
//
//   - path: "src/**/*"
//     target: "output/{lang}/**/*"
//
// Named collection (the ordinary form):
//
//   - name: bowrain-docs
//     channel: bowrain/docs
//     base: bowrain/web/docs
//     content:
//   - path: "docs/**/*.mdx"
//     target: "i18n/{lang}/{path}.mdx"
type Collection struct {
	Name            string           `yaml:"name,omitempty" json:"name,omitempty"`
	SourceLanguage  model.LocaleID   `yaml:"source_language,omitempty" json:"source_language,omitempty"`
	TargetLanguages []model.LocaleID `yaml:"target_languages,omitempty" json:"target_languages,omitempty"`
	Content         []ContentItem    `yaml:"content,omitempty" json:"content,omitempty"`

	// Base is the directory this collection lives in: every content path,
	// target and item base below is written relative to it, so a collection
	// reads as the tree it governs rather than as a repeated prefix. Empty
	// means the paths are project-relative.
	Base string `yaml:"base,omitempty" json:"base,omitempty"`

	// Channel places this collection's content at a point in the project's
	// context space: `profile/channel`, or a bare `channel` when exactly one
	// profile declares it. The point selects the profile whose governance —
	// voice, terms — a run carries over this content, and the channel then
	// selects the matching override inside that voice. Empty means the
	// project's default point governs it. Named collections only: a point is
	// resolved by collection name, so an unnamed entry has nothing to resolve.
	Channel string `yaml:"channel,omitempty" json:"channel,omitempty"`

	// Coordinates places this collection on the DECLARED axes — the ones a
	// project names for itself, brand among them. It overlays
	// `defaults.coordinates`, per axis, so a collection moves on the one axis
	// it differs on and inherits the rest.
	//
	// The structural axes come from `channel:` above and are not written here.
	Coordinates map[string]string `yaml:"coordinates,omitempty" json:"coordinates,omitempty"`

	// SourceOnly declares that this collection has no target language and is
	// never translated: a run reads it, checks it, and writes nothing back.
	// Package descriptions and installer strings are the usual case.
	//
	// It is an assertion, not a mode. Naming no target already made a
	// collection source-only, so this flag adds no behaviour — what it adds is
	// the difference between meaning it and forgetting. Validate rejects a
	// collection that sets it and also carries a target, so the declaration and
	// the items cannot drift into disagreeing.
	SourceOnly bool `yaml:"source_only,omitempty" json:"source_only,omitempty"`

	// Bare entry fields (short form — promoted from ContentItem).
	Path   string      `yaml:"path,omitempty" json:"path,omitempty"`
	Format *FormatSpec `yaml:"format,omitempty" json:"format,omitempty"`
	Target string      `yaml:"target,omitempty" json:"target,omitempty"`

	// Extras captures keys the framework does not know about, both for
	// bare entries and for named-collection wrappers. Platform layers
	// decode their per-collection / per-bare-entry extensions from here.
	Extras map[string]yaml.Node `yaml:",inline" json:"-"`
}

// IsBareEntry reports whether this is a bare entry (has path, no content).
func (c *Collection) IsBareEntry() bool {
	return c.Path != "" && len(c.Content) == 0
}

// validateSourceOnly rejects a collection that declares source_only and then
// contradicts it.
//
// Without the flag, a collection with no target and a collection whose target
// was forgotten are the same recipe, so the mistake reads as a decision and the
// content is quietly never translated. The flag is only worth having if it is
// checked: an unchecked one drifts away from the items under it and becomes a
// second, wrong answer to the question the items already answer.
//
// Both spellings of a target are covered — the collection's own
// target_languages, and each item's target path — because either alone would
// leave a way to declare the contradiction.
func (c *Collection) validateSourceOnly(i int) error {
	if !c.SourceOnly {
		return nil
	}
	where := fmt.Sprintf("collections[%d]", i)
	if c.Name != "" {
		where = fmt.Sprintf("%s (%q)", where, c.Name)
	}
	if len(c.TargetLanguages) > 0 {
		return fmt.Errorf("%s: source_only is set, so target_languages must be empty (found %v)", where, c.TargetLanguages)
	}
	if c.Target != "" {
		return fmt.Errorf("%s: source_only is set, so the entry cannot have a target (found %q)", where, c.Target)
	}
	for j, item := range c.Content {
		if item.Target != "" {
			return fmt.Errorf("%s: source_only is set, so content[%d] cannot have a target (found %q)", where, j, item.Target)
		}
		if len(item.TargetLanguages) > 0 {
			return fmt.Errorf("%s: source_only is set, so content[%d] cannot have target_languages (found %v)", where, j, item.TargetLanguages)
		}
	}
	return nil
}

// EffectiveItems returns the collection's items with Base folded in: every
// path, target and item base joined onto the collection's base, so every
// consumer downstream sees project-relative paths and never has to know a base
// was declared. An item that declares no base of its own keeps none — the
// target tokens then relativize against the joined pattern's own fixed prefix,
// which is what makes `base:` a location and not a second relativization root.
//
// For bare entries it wraps the promoted fields as a single-item slice
// (carrying the bare entry's Extras through, so platform-specific per-item
// fields survive).
//
// The collection's languages fold in the same way its base does: an item that
// declares none of its own carries the collection's, so a consumer holding an
// effective item needs no second lookup to know which languages apply to it.
// Every host path resolves an item's languages with no collection in hand
// (`ResolvedTargetLanguages(nil, defaults)`), so without this a collection-level
// `target_languages:` was invisible to coverage, checks, the plan and the
// convergence fan-out alike — declared in the recipe and silently unused.
func (c *Collection) EffectiveItems() []ContentItem {
	var items []ContentItem
	if c.IsBareEntry() {
		items = []ContentItem{{
			Path:   c.Path,
			Format: c.Format,
			Target: c.Target,
			Extras: c.Extras,
		}}
	} else {
		items = make([]ContentItem, len(c.Content))
		copy(items, c.Content)
	}
	for i := range items {
		if items[i].SourceLanguage == "" {
			items[i].SourceLanguage = c.SourceLanguage
		}
		if len(items[i].TargetLanguages) == 0 {
			items[i].TargetLanguages = c.TargetLanguages
		}
	}
	if c.Base == "" {
		return items
	}
	for i := range items {
		items[i].Path = JoinBase(c.Base, items[i].Path)
		items[i].Target = JoinBase(c.Base, items[i].Target)
		items[i].Base = JoinBase(c.Base, items[i].Base)
	}
	return items
}

// JoinBase prefixes a collection-relative path with the collection's base.
// An empty path stays empty — an item with no target has no target under a
// base either — and an absolute path is left alone so the escape check
// downstream still sees it.
func JoinBase(base, p string) string {
	if base == "" || p == "" {
		return p
	}
	if strings.HasPrefix(p, "/") {
		return p
	}
	return strings.TrimSuffix(filepath.ToSlash(base), "/") + "/" + p
}

// ContentItem is a single content pattern within a collection.
type ContentItem struct {
	Path            string           `yaml:"path" json:"path"`
	Format          *FormatSpec      `yaml:"format,omitempty" json:"format,omitempty"`
	Target          string           `yaml:"target,omitempty" json:"target,omitempty"`
	Base            string           `yaml:"base,omitempty" json:"base,omitempty"`
	SourceLanguage  model.LocaleID   `yaml:"source_language,omitempty" json:"source_language,omitempty"`
	TargetLanguages []model.LocaleID `yaml:"target_languages,omitempty" json:"target_languages,omitempty"`

	// Channel overrides the collection's `channel:` for the files this item
	// matches — the one place a single file, or a sub-pattern, sits at a
	// different point in the context space than the rest of its collection. It
	// takes the same qualified `profile/channel` form and the same validator as
	// a collection's channel; empty inherits the collection's. A run resolves it
	// per file through ResolveGovernanceForPath.
	Channel string `yaml:"channel,omitempty" json:"channel,omitempty"`

	// Redaction overrides the project-wide redaction spec for this item.
	// nil means inherit defaults.
	Redaction *RedactionSpec `yaml:"redaction,omitempty" json:"redaction,omitempty"`

	// Extras captures unknown keys at the per-item level. Platform layers
	// decode their per-item fields from here.
	Extras map[string]yaml.Node `yaml:",inline" json:"-"`
}

// ResolvedSourceLanguage returns the source language for this item, falling
// back through collection and project defaults.
func (item *ContentItem) ResolvedSourceLanguage(coll *Collection, defaults Defaults) model.LocaleID {
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
func (item *ContentItem) ResolvedTargetLanguages(coll *Collection, defaults Defaults) []model.LocaleID {
	if len(item.TargetLanguages) > 0 {
		return item.TargetLanguages
	}
	if coll != nil && len(coll.TargetLanguages) > 0 {
		return coll.TargetLanguages
	}
	return defaults.TargetLanguages
}

// DeclaresTargetLanguages reports whether the recipe names a target language
// anywhere its content could be written to one: in `defaults.target_languages`,
// or on a collection or item that also declares a target template. False means
// the project is MONOLINGUAL — there is no per-language work to plan, price or
// run, so a caller must not ask for an AI provider or quote a translation cost.
//
// It reads the recipe only: no glob expands, nothing is stat-ed. A caller that
// needs the locales themselves resolves the content instead.
func (p *KapiProject) DeclaresTargetLanguages() bool {
	if len(p.Defaults.TargetLanguages) > 0 {
		return true
	}
	for i := range p.Collections {
		for _, item := range p.Collections[i].EffectiveItems() {
			if item.Target != "" && len(item.TargetLanguages) > 0 {
				return true
			}
		}
	}
	return false
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
	// A key that is nearly a field is preserved like any other unknown one, so
	// nothing above this line notices it. See keywarnings.go and #2223.
	reportKeyWarnings(path, &proj)

	return &proj, nil
}

// Save writes a recipe to the given path, keeping the comments and key order of
// whatever recipe is already there and writing nothing at all when the
// serialization is unchanged (core/yamledit).
//
// A recipe is committed, human-authored source — `kapi init` scaffolds it with a
// commented tutorial — so a verb that touches one binding must not take the
// explanation with it. And a save that writes nothing new is worse than untidy
// here: the recipe sits at the repo root, which the erasure gate classifies as
// foreign, so a convergence run that rewrote it — identical content and all — is
// refused by name.
func Save(path string, proj *KapiProject) error {
	if proj.Version == "" {
		proj.Version = CurrentVersion
	}
	if _, err := yamledit.WriteFile(path, proj, 0o644); err != nil {
		return fmt.Errorf("write project file: %w", err)
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
	for _, key := range sortedKeys(p.Extras) {
		if replacement, retired := retiredProjectKeys[key]; retired {
			return fmt.Errorf("%s: is no longer a recipe key — use %s", key, replacement)
		}
	}
	if err := p.Defaults.Merge.validate(); err != nil {
		return err
	}
	if err := p.Defaults.validateMaterialize(); err != nil {
		return err
	}
	if err := p.Defaults.Memory.validate(); err != nil {
		return err
	}
	if err := p.Defaults.Redaction.validate(); err != nil {
		return err
	}
	if err := p.Defaults.Voice.validate("defaults.voice"); err != nil {
		return err
	}
	if err := p.validateContextSpace(); err != nil {
		return err
	}
	for i, c := range p.Collections {
		if err := c.validateSourceOnly(i); err != nil {
			return err
		}
		if c.IsBareEntry() {
			if c.Path == "" {
				return fmt.Errorf("collections[%d]: path is required for bare entries", i)
			}
		} else {
			if c.Path != "" {
				return fmt.Errorf("collections[%d]: collection %q cannot have a path (use content)", i, c.Name)
			}
			if len(c.Content) == 0 {
				return fmt.Errorf("collections[%d]: collection %q must have at least one content item", i, c.Name)
			}
			for j, item := range c.Content {
				if item.Path == "" {
					return fmt.Errorf("collections[%d].content[%d]: path is required", i, j)
				}
				if err := item.Redaction.validate(); err != nil {
					return fmt.Errorf("collections[%d].content[%d]: %w", i, j, err)
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
	for i, c := range p.Collections {
		prefix := fmt.Sprintf("collections[%d].", i)
		if err := validateExtras(ScopeCollection, prefix, c.Extras); err != nil {
			return err
		}
		for j, item := range c.Content {
			ip := fmt.Sprintf("collections[%d].content[%d].", i, j)
			if err := validateExtras(ScopeItem, ip, item.Extras); err != nil {
				return err
			}
		}
	}
	return nil
}

// sortedKeys returns a map's keys in deterministic order, so validation reports
// the same message on every load.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
func (p *KapiProject) Flow(name string) *flow.StepsSpec {
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
	Collection *Collection
	Item       ContentItem
}

// IterateContent yields every ContentItem in the project with its collection's
// base folded in, walking both bare entries and named collections. The
// Collection pointer is non-nil so callers can read its Name.
func (p *KapiProject) IterateContent() []IteratedItem {
	var out []IteratedItem
	for i := range p.Collections {
		coll := &p.Collections[i]
		for _, item := range coll.EffectiveItems() {
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

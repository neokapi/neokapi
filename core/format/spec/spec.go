// Package spec defines the format-specification model used to describe
// what a neokapi DataFormat does, exercised through executable examples.
//
// A Spec is the canonical authored description of a format: its config
// surface, the features it supports, and concrete examples that demonstrate
// each feature. The same Spec is consumed by the native test runner
// (verifies the Go reader honors the spec) and by the parity bridge runner
// (verifies upstream Okapi agrees) and by drift detection (verifies the
// spec stays in sync with both implementations).
//
// Specs live next to their format: core/formats/<name>/spec.yaml.
package spec

// Spec is the canonical specification for one neokapi format.
//
// The same Spec drives:
//   - native verification (spec_test.go in the format package)
//   - parity verification (cli/parity/spec, build-tagged "parity")
//   - drift detection against the bridge schema and Okapi @Test list
//   - documentation rendering on the docs site
type Spec struct {
	// Format is the okapi-bridge filter id (e.g. "okf_openxml") so the
	// spec aligns with the bridge manifest and parity report keys.
	Format string `yaml:"format"`

	// Kind classifies the filter's role. Empty (the default) is treated
	// as KindTopLevel — a standalone filter the bridge dispatches by
	// MIME type or extension. KindSubfilter marks filters that are only
	// invoked through a parent filter's content (e.g. ICU MessageFormat
	// inside a Java Properties value, or HTML embedded in a Markdown
	// block). Subfilters have no top-level bridge JSON Schema, so the
	// contract-audit drift check for config keys is skipped; they are
	// also not exercised via the parity bridge daemon (no standalone
	// dispatch path), so the parity runner records a `subfilter` skip.
	// The native runner still runs every example because the native Go
	// reader can be constructed and fed inline input directly.
	Kind Kind `yaml:"kind,omitempty"`

	// MimeType is the primary mime type. Variants may override.
	MimeType string `yaml:"mime_type"`

	// Description is human-readable prose describing what this format
	// is for. Rendered on the docs site.
	Description string `yaml:"description,omitempty"`

	// Variants enumerates sub-formats sharing one filter (e.g. openxml
	// → docx/xlsx/pptx). Empty when the format is monolithic. Features
	// and config keys reference variants via AppliesTo.
	Variants []Variant `yaml:"variants,omitempty"`

	// Config enumerates every configuration key the format accepts.
	// Each key corresponds 1:1 to a key recognised by Config.ApplyMap
	// on the native side; OkapiParam carries the upstream Java
	// parameter name for drift checking against the bridge schema.
	Config []ConfigKey `yaml:"config,omitempty"`

	// Features are the named behaviors this format implements. Each
	// feature carries one or more Examples that demonstrate it
	// concretely.
	Features []Feature `yaml:"features"`

	// dir is the source directory of the loaded spec file (set by Load),
	// used to resolve Example.InputFile relative paths. Unexported so
	// the YAML unmarshaller leaves it alone.
	dir string
}

// Variant is one sub-format inside a multi-variant filter (docx, xlsx,
// pptx for openxml; opendocument-text vs spreadsheet for odf, etc.).
type Variant struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Extension   string `yaml:"extension"`
	MimeType    string `yaml:"mime_type"`
	Description string `yaml:"description,omitempty"`
}

// ConfigKey is one configuration option recognised by the format.
type ConfigKey struct {
	Key         string   `yaml:"key"`
	Type        string   `yaml:"type"` // "boolean" | "string" | "string_list" | "int" | "string_map"
	Default     any      `yaml:"default"`
	Description string   `yaml:"description"`
	AppliesTo   []string `yaml:"applies_to,omitempty"` // variant ids; empty = all
	OkapiParam  string   `yaml:"okapi_param,omitempty"`
}

// Feature is one named behavior of the format.
type Feature struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	AppliesTo   []string `yaml:"applies_to,omitempty"` // variant ids; empty = all

	// Config is applied to every Example in this feature unless the
	// example overrides individual keys. Keyed by ConfigKey.Key.
	Config map[string]any `yaml:"config,omitempty"`

	// Examples are the concrete demonstrations of this feature.
	Examples []Example `yaml:"examples"`

	// OkapiRefs lists upstream Okapi @Test methods that exercise this
	// feature, in "ClassName#methodName" form. Drift detection asserts
	// each ref still exists in the pinned Okapi version.
	OkapiRefs []string `yaml:"okapi_refs,omitempty"`

	// NativeRefs lists existing Go test functions that exercise this
	// feature, in "package.TestFuncName" form. Used by docs to link to
	// authoritative implementations.
	NativeRefs []string `yaml:"native_refs,omitempty"`

	// SpecRefs cites the format specification(s) this feature asserts
	// against. When the bridge diverges from native, the upstream
	// conversation is "your filter violates <cited spec>" rather than
	// "your filter behaves differently from ours" — much harder to push
	// back on. Free-text lines like "CommonMark §6.7" or
	// "YAML 1.2 §8.1.1.1" or "Java SE Properties spec (Properties.load)".
	// Empty when the feature is a behavioral convention with no formal
	// spec authority (e.g. plaintext line-ending handling).
	SpecRefs []string `yaml:"spec_refs,omitempty"`
}

// Example is one concrete input + assertion pair under a Feature.
//
// Exactly one of InputFile, InputXML, or InputBytes must be set.
// Variant selects the sub-format ParseType for multi-variant filters
// (required when Spec.Variants is non-empty and the input shape is
// ambiguous, e.g. a raw XML snippet).
//
// Fixture sourcing rule (binary formats — idml, archive, openxml, rtf,
// pdf, mif, icml, epub, txml, transtable, vignette, etc.):
//   - PREFER `input_file: okapi:okapi/filters/<name>/src/test/resources/<file>`
//     so both the bridge and the native reader exercise real upstream
//     test fixtures. Synthetic minimal fixtures (`gen-*-fixtures`
//     scripts emitting `testdata/*.idml` etc.) routinely omit attributes
//     that real authoring tools always emit; the bridge null-derefs and
//     the divergence reads as a "bridge bug" (e.g. #482) when in fact
//     the fixture is malformed.
//   - Synthetic fixtures are acceptable only when no upstream file
//     covers the feature. In that case, note the justification in the
//     spec.yaml header comment.
//
// Text-based formats (json, yaml, properties, po, html, markdown, xml,
// xliff, …) may continue to use `input_xml` / `input_bytes` inline —
// the failure mode above doesn't apply to handwritten text snippets.
type Example struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Variant     string `yaml:"variant,omitempty"`

	// Input shape — pick one. For binary formats, prefer
	// `input_file: okapi:...` over synthetic `testdata/*` fixtures
	// (see Example doc comment).
	InputFile  string `yaml:"input_file,omitempty"`  // path relative to spec.yaml dir, or `okapi:<path-under-okapi-testdata>`
	InputXML   string `yaml:"input_xml,omitempty"`   // inline XML/text snippet
	InputBytes []byte `yaml:"input_bytes,omitempty"` // base64-encoded in YAML

	// BridgeOnly marks examples whose input shape isn't consumable by
	// the native reader (typically raw-XML snippets when the native
	// reader requires a full document package). The native runner
	// skips such examples; the parity bridge runner still uses them.
	BridgeOnly bool `yaml:"bridge_only,omitempty"`

	// ExpectedFail documents a known divergence between the spec
	// contract and one of the implementations. When set, assertion
	// failures are logged + recorded but don't fail the test.
	// Surfaces in the dashboard as "expected_fail" with this reason.
	// If the assertion unexpectedly PASSES, the test logs a warning
	// (the divergence has been fixed — remove this tag).
	ExpectedFail string `yaml:"expected_fail,omitempty"`

	// DivergenceKind optionally attributes WHICH side is at fault for an
	// ExpectedFail / parity_warn divergence, so the dashboard can colour
	// the example by severity (only "native-bug" is alarming; everything
	// else is correct-by-design or an upstream/transport issue). When
	// empty, contract-audit infers the kind heuristically from the
	// ExpectedFail reason text; an explicit value here always wins over
	// the heuristic. Recognised values:
	//
	//   native-bug     — the neokapi reader is wrong (should be ~0).
	//   bridge-gap     — okapi-bridge can't receive neokapi's config/rules
	//                    over gRPC; native is correct.
	//   okapi-bug      — upstream Okapi is wrong; native is correct.
	//   scope-diff     — the Okapi filter has a different feature scope
	//                    (e.g. Trados-tagged RTF only); native is correct.
	//   default-diff   — Okapi's default config differs; same semantic
	//                    config → same result; native is correct.
	//   missing-filter — the bridge doesn't ship the okf_ filter; native
	//                    is correct.
	//   fixture        — a test-infra / synthetic-fixture artefact.
	//   contract       — a parse-error-by-design (no blocks).
	DivergenceKind string `yaml:"divergence_kind,omitempty"`

	// ParityStrict promotes bridge↔native bytewise mismatch to a
	// hard failure, even when both sides individually satisfy the
	// spec assertions. Default false — divergent representations
	// (e.g. inline-formatting markers) record as parity_warn instead.
	ParityStrict bool `yaml:"parity_strict,omitempty"`

	// Config overrides the feature's Config map for this example.
	Config map[string]any `yaml:"config,omitempty"`

	// Origin documents where the example's input came from. Helps
	// reviewers (and upstream maintainers when a bridge bug gets filed)
	// trace the provenance of any failure. Three canonical forms:
	//   - "authored: <reason>" — input crafted by the spec author to
	//     exercise a specific spec contract (most common; pair with the
	//     feature's SpecRefs to point at the spec section being exercised).
	//   - "okapi-fixture: <ref>" — input copied from an Okapi @Test
	//     fixture (cite the test class + method).
	//   - "real-world: <source>" — input excerpted from a real-world
	//     document (e.g. "Excalidraw locales/en.json"). Useful when
	//     real fixtures expose subtle layout the synthetic ones miss.
	// Free-text after the kind tag is welcome; the form is for grep
	// not for parsing.
	Origin string `yaml:"origin,omitempty"`

	// Assertions evaluated against the read parts. Inlined so spec
	// authors write assertion fields directly under the example.
	Assertions `yaml:",inline"`
}

// Assertions captures the checks evaluated against the read parts.
// Each field is optional; nil means "do not check".
type Assertions struct {
	BlockCount       *int     `yaml:"block_count,omitempty"`
	BlockCountMin    *int     `yaml:"block_count_min,omitempty"`
	BlockCountMax    *int     `yaml:"block_count_max,omitempty"`
	FirstBlockText   *string  `yaml:"first_block_text,omitempty"`
	BlockTexts       []string `yaml:"block_texts,omitempty"`
	HasBlockWithText []string `yaml:"has_block_with_text,omitempty"`
	NoBlockWithText  []string `yaml:"no_block_with_text,omitempty"`
}

// Kind classifies a Spec's dispatch role.
type Kind string

const (
	// KindTopLevel is the default — the filter is dispatched by the
	// bridge based on MIME type / extension, has its own composite
	// JSON Schema, and is exercised standalone by the parity runner.
	KindTopLevel Kind = "top_level"

	// KindSubfilter marks filters that are only invoked from inside a
	// parent filter's content (e.g. ICU MessageFormat inside a Java
	// Properties value). Subfilters skip the parity bridge runner and
	// the bridge schema drift check; their behavior is exercised
	// through their parents' specs and through the native runner.
	KindSubfilter Kind = "subfilter"
)

// IsSubfilter reports whether the spec marks a layer-only filter.
// Treats the empty string as top-level so legacy specs without an
// explicit kind: continue to behave as before.
func (s *Spec) IsSubfilter() bool {
	return s != nil && s.Kind == KindSubfilter
}

// IntPtr / StrPtr help spec authors writing examples programmatically.
//
//go:fix inline
func IntPtr(v int) *int { return new(v) }

//go:fix inline
func StrPtr(v string) *string { return new(v) }

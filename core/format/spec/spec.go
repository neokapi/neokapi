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

import "fmt"

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

	// BridgeFilterClass overrides the FilterClass the parity bridge runner
	// dispatches to when it differs from Format. Config-preset formats use
	// the manifest id as Format (e.g. "okf_dita") for the dashboard join,
	// but dispatch to the base filter on the bridge (e.g. "okf_xmlstream")
	// plus BridgeConfigID. Empty = dispatch to Format. This replaces the
	// hand-maintained FormatSpec.BridgeFilterClass parity-table field —
	// spec.yaml is now the single source of truth (#852). Use BridgeClass()
	// to read the effective value.
	BridgeFilterClass string `yaml:"bridge_filter_class,omitempty"`

	// BridgeConfigID names a built-in Okapi filter configuration the parity
	// bridge loads before opening (e.g. "okf_xmlstream-dita"). The native
	// side configures the equivalent preset via its reader; the bridge
	// applies the same named config so the comparison is head-to-head.
	// Empty = filter defaults. Replaces FormatSpec.ConfigID (#852).
	BridgeConfigID string `yaml:"bridge_config_id,omitempty"`

	// Tikal wires the third parity reference corner: when set and a tikal
	// launcher is reachable, the parity harness runs `tikal -x` + `tikal -m`
	// against the same input and compares the merged bytes against the
	// native round-trip output. Replaces FormatSpec.TikalExt / TikalConfig
	// (#852). Nil disables the tikal corner for the format.
	Tikal *TikalSpec `yaml:"tikal,omitempty"`

	// Parity carries spec/feature-level parity-runner skip directives that
	// are implementation/transport facts rather than spec contract (a bridge
	// crash, a binary-corpus gap, a documented bridge↔native divergence).
	// Replaces the hand-maintained FormatSpec.Skip / SkipRoundTrip /
	// SkipTikal parity-table fields (#852). Nil = no parity skips. The
	// always-on native runner and the per-example expectations mechanisms
	// (bridge_only / expected_fail / parity_strict, §7) are unaffected by
	// this block — it only steers the legacy filter-level parity runner.
	Parity *ParitySpec `yaml:"parity,omitempty"`

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

// TikalSpec configures the tikal parity reference corner (Spec.Tikal).
type TikalSpec struct {
	// Ext is the file extension tikal receives (e.g. ".properties"). It is
	// the gating field — a TikalSpec is meaningless without it.
	Ext string `yaml:"ext"`
	// Config is the optional `-fc` filter-configuration id passed to tikal
	// (e.g. "okf_properties"). Empty lets tikal pick by extension.
	Config string `yaml:"config,omitempty"`
}

// ParitySpec carries filter-level parity-runner skip directives (Spec.Parity).
// These are transport/implementation facts, never spec contract — the corpus
// stays implementation-agnostic (format-spec-cases.md §7). The reasons are
// recorded so the parity dashboard surfaces the gap.
type ParitySpec struct {
	// Skip, when non-empty, skips the whole filter in the legacy
	// filter-level parity runner with this reason (e.g. a binary-corpus gap
	// or a bridge crash).
	Skip string `yaml:"skip,omitempty"`
	// SkipRoundTrip skips just the read→write round-trip pass with this
	// reason, leaving read parity intact.
	SkipRoundTrip string `yaml:"skip_roundtrip,omitempty"`
	// SkipTikal skips just the tikal pass with this reason.
	SkipTikal string `yaml:"skip_tikal,omitempty"`
}

// BridgeClass returns the FilterClass the parity bridge runner should
// dispatch to: BridgeFilterClass when set (config-preset formats dispatch to a
// base filter), else Format.
func (s *Spec) BridgeClass() string {
	if s == nil {
		return ""
	}
	if s.BridgeFilterClass != "" {
		return s.BridgeFilterClass
	}
	return s.Format
}

// validateParity checks the optional parity-runner config (Tikal, BridgeConfigID)
// the spec.yaml may carry (#852). Called from Validate(); a no-op for the
// (majority of) specs that declare no parity block.
func (s *Spec) validateParity() error {
	if s.Tikal != nil && s.Tikal.Ext == "" {
		return fmt.Errorf("tikal: ext is required when a tikal block is present")
	}
	if s.BridgeConfigID != "" && s.BridgeFilterClass == "" {
		return fmt.Errorf("bridge_config_id %q requires bridge_filter_class (the base filter the config applies to)", s.BridgeConfigID)
	}
	return nil
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

	// ID is the stable, 4–6 char alphanumeric case identifier
	// (format-spec-cases.md §2). Optional and never reused after
	// deletion so results stay comparable across corpus revisions.
	// Legacy examples omit it and are addressed by Name; CaseID()
	// returns ID when set, else falls back to Name.
	ID string `yaml:"id,omitempty"`

	// Class is the validity class (§3): "valid" (default — the input
	// parses and extraction produces the asserted model), "invalid"
	// (the input must be rejected cleanly, one fault per case), or
	// "operation" (an in-out pair). Empty means "valid". Use
	// CaseClass() to read the effective value.
	Class string `yaml:"class,omitempty"`

	// Cite is the machine-checkable citation (§6) anchoring the case to
	// a pinned spec section. Required for valid/invalid cases of
	// spec-backed formats; behavioral conventions may omit it with a
	// tags: [convention] marker.
	Cite *Citation `yaml:"cite,omitempty"`

	// Tags are free-form selectors (§2).
	Tags []string `yaml:"tags,omitempty"`

	// Since / Until bound the format versions the case applies to (§9);
	// omit when version-independent.
	Since string `yaml:"since,omitempty"`
	Until string `yaml:"until,omitempty"`

	// Expected carries the multi-view expected model (§5):
	// blocks (the §4 event dump), extracted (today's source-text
	// Assertions), roundtrip (writer output), and error (invalid-class
	// rejection). When nil, the inline top-level Assertions below remain
	// the (extracted) view — full YAML back-compat for the ~41 existing
	// specs. Use ExtractedAssertions() to read the effective extracted
	// view regardless of which form a case uses.
	Expected *Expected `yaml:"expected,omitempty"`

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
	//
	// The structured `origin: {kind: okapi-fixture, ...}` form
	// (format-spec-cases.md §9) is the target shape for the harvested
	// fixtures the legacy parity table still carries inline in Go
	// (cli/parity/formats/fixtures_*_generated.go). Folding those bulk
	// fixtures into spec.yaml as informational cases is deferred (#852
	// took the lighter touch); the field is recognised here so the fold
	// is a data move, not a grammar change.
	Origin string `yaml:"origin,omitempty"`

	// Informational marks an exploratory case: it is recorded and reported
	// to the parity dashboard but its assertion failures do not fail CI.
	// Auto-generated / harvested fixtures (origin: okapi-fixture) carry it;
	// hand-curated cases leave it false so they act as strict gates. Mirrors
	// the legacy FormatInput.Informational parity-table field, surfaced in
	// the grammar so harvested fixtures can fold into spec.yaml (#852).
	Informational bool `yaml:"informational,omitempty"`

	// Assertions evaluated against the read parts. Inlined so spec
	// authors write assertion fields directly under the example.
	Assertions `yaml:",inline"`
}

// Case validity classes (format-spec-cases.md §3).
const (
	// ClassValid is the default: the input parses and extraction
	// produces the asserted model.
	ClassValid = "valid"
	// ClassInvalid: the input must be rejected cleanly (no panic,
	// bounded resources), one fault per case, named after the fault.
	ClassInvalid = "invalid"
	// ClassOperation: an in-out pair (input + named operation → output).
	ClassOperation = "operation"
)

// Round-trip modes for Expected.Roundtrip.Mode (§5).
const (
	// RoundtripByteExact asserts the writer output equals the input bytes.
	RoundtripByteExact = "byte_exact"
	// RoundtripIdempotent asserts read→write→read→write reaches a fixpoint:
	// the second write equals the first.
	RoundtripIdempotent = "idempotent"
	// RoundtripNormalized asserts the writer output equals a committed
	// normalized fixture (Expected.Roundtrip.OutputFile).
	RoundtripNormalized = "normalized"
)

// Expected is the multi-view expected model for a case (§5). Every view is
// optional; an absent view is simply not checked. The legacy inline
// Assertions on Example remain the extracted view when Expected is nil.
type Expected struct {
	// Blocks is the canonical block-event dump (§4) the read parts must
	// match. Either an inline JSONL string (recognised by containing a
	// `{`) or a sibling file reference like "cases/<id>.events.jsonl"
	// resolved relative to the spec dir.
	Blocks string `yaml:"blocks,omitempty"`

	// Extracted is today's source-text Assertions vocabulary, unchanged
	// (§5). When set it is the extracted view; when nil the runner falls
	// back to Example's inline Assertions.
	Extracted *Assertions `yaml:"extracted,omitempty"`

	// Roundtrip asserts writer output (§5). Requires the runner be wired
	// with a NewWriter factory.
	Roundtrip *Roundtrip `yaml:"roundtrip,omitempty"`

	// Error is the rejection assertion for class: invalid cases (§3, §5):
	// the reader must surface a clean error. Never auto-updated (§8).
	Error *ErrorExpect `yaml:"error,omitempty"`

	// ValidBy names an external validator the writer output must pass
	// (§5). Recorded on the case; enforcement is the acceptance harness's
	// concern, not this in-process runner.
	ValidBy string `yaml:"valid_by,omitempty"`
}

// Roundtrip describes a writer-output expectation (§5).
type Roundtrip struct {
	// Mode is byte_exact | idempotent | normalized.
	Mode string `yaml:"mode"`
	// OutputContains optionally asserts substrings present in the writer
	// output, independent of Mode — a lightweight signal when an exact
	// fixture is overkill.
	OutputContains []string `yaml:"output_contains,omitempty"`
	// OutputFile names the committed normalized fixture for mode:
	// normalized (spec-relative path).
	OutputFile string `yaml:"output_file,omitempty"`
}

// ErrorExpect is the clean-rejection assertion for class: invalid cases.
type ErrorExpect struct {
	// Category labels the fault (e.g. "syntax", "schema", "encoding").
	// Readers do not emit categories today, so the category is recorded
	// (and validated for presence) but not matched against reader output;
	// MessageContains carries the runtime check.
	Category string `yaml:"category"`
	// MessageContains, when set, must be a substring of the reader's
	// error message.
	MessageContains string `yaml:"message_contains,omitempty"`
}

// Citation is a machine-checkable spec citation (§6).
type Citation struct {
	Spec        string `yaml:"spec,omitempty"`
	Version     string `yaml:"version,omitempty"`
	URL         string `yaml:"url,omitempty"`
	Clause      string `yaml:"clause,omitempty"`
	Heading     string `yaml:"heading,omitempty"`
	Quote       string `yaml:"quote,omitempty"`
	QuoteSHA256 string `yaml:"quote_sha256,omitempty"`
}

// CaseID returns the case's stable identifier: Example.ID when set, else the
// human Name. Used by the runner and accept-mode to address a case.
func (e *Example) CaseID() string {
	if e.ID != "" {
		return e.ID
	}
	return e.Name
}

// CaseClass returns the effective validity class, defaulting empty to
// ClassValid.
func (e *Example) CaseClass() string {
	if e.Class == "" {
		return ClassValid
	}
	return e.Class
}

// ExtractedAssertions returns the effective extracted-view assertions:
// Expected.Extracted when present, else the inline top-level Assertions. This
// is what keeps every legacy example's seven-field assertions reachable
// unchanged as the extracted view.
func (e *Example) ExtractedAssertions() Assertions {
	if e.Expected != nil && e.Expected.Extracted != nil {
		return *e.Expected.Extracted
	}
	return e.Assertions
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

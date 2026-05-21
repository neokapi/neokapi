//go:build parity

// Package formats holds the per-filter parity tests that run every
// Okapi filter present in the okapi-bridge v2 manifest against its
// neokapi Go counterpart (when one exists) or as a bridge-only
// stability snapshot (when no Go port exists).
//
// Each entry in `formatSpecs` declares one filter:
//
//	ID            okf_<name> — the manifest id and the FilterClass
//	              sent to BridgeService.Process.
//	MimeType      mime hint passed to both bridge and native readers.
//	Inputs        list of named sample inputs (small inline strings
//	              or testdata paths).
//	NewReader     constructs the in-process Go reader. Nil means
//	              bridge-only — the test asserts the bridge runs and
//	              produces a non-empty stream against `Inputs`, but
//	              does not compare against a native implementation.
//	Skip          if non-empty, the test skips with this reason
//	              (used for filters that need binary corpus we don't
//	              ship in the repo — see SKIP_BINARY).
package formats

import (
	"github.com/neokapi/neokapi/core/format"
	csvfmt "github.com/neokapi/neokapi/core/formats/csv"
	doxygenfmt "github.com/neokapi/neokapi/core/formats/doxygen"
	dtdfmt "github.com/neokapi/neokapi/core/formats/dtd"
	fixedwidthfmt "github.com/neokapi/neokapi/core/formats/fixedwidth"
	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	icmlfmt "github.com/neokapi/neokapi/core/formats/icml"
	idmlfmt "github.com/neokapi/neokapi/core/formats/idml"
	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	markdownfmt "github.com/neokapi/neokapi/core/formats/markdown"
	miffmt "github.com/neokapi/neokapi/core/formats/mif"
	mosestextfmt "github.com/neokapi/neokapi/core/formats/mosestext"
	odffmt "github.com/neokapi/neokapi/core/formats/odf"
	openxmlfmt "github.com/neokapi/neokapi/core/formats/openxml"
	paraplaintextfmt "github.com/neokapi/neokapi/core/formats/paraplaintext"
	pdffmt "github.com/neokapi/neokapi/core/formats/pdf"
	phpcontentfmt "github.com/neokapi/neokapi/core/formats/phpcontent"
	plaintextfmt "github.com/neokapi/neokapi/core/formats/plaintext"
	pofmt "github.com/neokapi/neokapi/core/formats/po"
	propertiesfmt "github.com/neokapi/neokapi/core/formats/properties"
	regexfmt "github.com/neokapi/neokapi/core/formats/regex"
	rtffmt "github.com/neokapi/neokapi/core/formats/rtf"
	splicedlinesfmt "github.com/neokapi/neokapi/core/formats/splicedlines"
	srtfmt "github.com/neokapi/neokapi/core/formats/srt"
	texfmt "github.com/neokapi/neokapi/core/formats/tex"
	tmxfmt "github.com/neokapi/neokapi/core/formats/tmx"
	transtablefmt "github.com/neokapi/neokapi/core/formats/transtable"
	tsfmt "github.com/neokapi/neokapi/core/formats/ts"
	ttmlfmt "github.com/neokapi/neokapi/core/formats/ttml"
	ttxfmt "github.com/neokapi/neokapi/core/formats/ttx"
	txmlfmt "github.com/neokapi/neokapi/core/formats/txml"
	vignettefmt "github.com/neokapi/neokapi/core/formats/vignette"
	vttfmt "github.com/neokapi/neokapi/core/formats/vtt"
	wikifmt "github.com/neokapi/neokapi/core/formats/wiki"
	xlifffmt "github.com/neokapi/neokapi/core/formats/xliff"
	xliff2fmt "github.com/neokapi/neokapi/core/formats/xliff2"
	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
	yamlfmt "github.com/neokapi/neokapi/core/formats/yaml"
)

// SKIP_BINARY is the standard skip reason for filters whose only
// realistic input is a binary container (DOCX, IDML, MIF, ICML, PDF,
// archive zips, SDL packages, …). We do not commit binary corpus to
// the neokapi repo per the constraint on parity test data; these
// rows still get a row in the report so the dashboard surfaces the
// gap. Resolution path: ship a tiny corpus inside okapi-bridge's
// plugin tarball (under testdata/) and adapt the spec to read from
// there.
const SKIP_BINARY = "binary corpus not in repo (rely on okapi-bridge testdata/ when available)"

// SKIP_DIVERGENCE_453 marks filters whose Go port and Okapi filter
// agree the file is parseable but disagree on which segments are
// translatable. Each row has a per-filter line in #453 explaining
// the gap. Flip Skip back to "" once aligned.
const SKIP_DIVERGENCE_453 = "documented divergence — see #453"

// SKIP_BRIDGE_BUG_452 marks rows where the bridge daemon errors out
// on a valid input. The first one filed is okf_ttml (NPE in Jericho).
const SKIP_BRIDGE_BUG_452 = "bridge crash — see #452"

// SKIP_BRIDGE_CONFIG marks the okf_xml/okf_xmlstream config-preset formats
// (DITA, DocBook, ResX) whose native side is wired (xml reader + the Go
// config preset) but whose bridge side can't yet load the equivalent named
// Okapi config: the okapi-bridge okf_xml/okf_xmlstream schema exposes no
// configId/rules parameter, so a head-to-head comparison would run the
// bridge with default rules against the native preset (a false divergence).
// Head-to-head enables once okapi-bridge gains config-by-name support (#613).
const SKIP_BRIDGE_CONFIG = "native xml config preset wired; bridge config-by-name not yet supported — see okapi-bridge #613"

// FormatInput is one named sample input.
//
// OkapiTest is optional. When set ("ClassName#methodName", short class
// form like "HtmlSnippetsTest#testEscapes"), the harness reports each
// fixture run under format-fixture parity rows keyed on the Java test
// id. The contract-audit dashboard then joins per-test bridge/native
// status against Surefire's own row for that test, giving true 3-way
// per-test granularity instead of one filter-level badge.
//
// Informational marks a fixture as exploratory: comparison failures
// are logged and reported to the parity dashboard, but they don't
// fail the Go test (so CI stays green on auto-generated fixtures
// that surface known divergences). Hand-curated fixtures leave it
// false to act as strict regression gates.
type FormatInput struct {
	Name          string
	Content       []byte
	OkapiTest     string
	Informational bool
}

// FormatSpec describes one parity test row.
//
// NewWriter is optional. When set (and neither Skip nor SkipRoundTrip
// fires), the harness drives an additional round-trip pass: input →
// reader → writer on each side and compares the two output byte
// streams. The round-trip outcome is reported separately under
// Kind="format-roundtrip" so the contract-audit dashboard can surface
// read parity and round-trip parity as distinct badges.
//
// SkipRoundTrip skips just the round-trip pass with the given reason,
// while leaving read parity intact. Use it to document a known writer
// divergence without breaking CI.
//
// TikalExt and TikalConfig wire the third reference corner: when a
// tikal launcher is reachable and TikalExt is set, the harness also
// runs `tikal -x` + `tikal -m` against the same input and compares
// the merged bytes against the native round-trip output. Tikal
// outcomes report under Kind="format-tikal" so a tikal-vs-native
// divergence is visible without polluting bridge-vs-native rows.
type FormatSpec struct {
	ID            string
	MimeType      string
	Inputs        []FormatInput
	NewReader     func() format.DataFormatReader
	NewWriter     func() format.DataFormatWriter
	Skip          string
	SkipRoundTrip string
	SkipTikal     string
	TikalExt      string // file extension passed to tikal (e.g. ".properties"); empty disables tikal.
	TikalConfig   string // optional -fc filter id (e.g. "okf_properties").

	// Params holds the configuration applied to both sides of the
	// comparison. Native filters receive these via the existing
	// DataFormatConfig.ApplyMap path (typed Go config stays the
	// in-memory representation); the bridge receives the same keys
	// stringified into FilterParams. Empty means each side runs with
	// its own defaults.
	//
	// Use the flat names from the bridge schema's
	// `x-okapi-flatten-path` annotations — they line up with the
	// camelCase keys native ApplyMap implementations recognise.
	Params map[string]any
}

func ttext(s string) []byte { return []byte(s) }

// mergeInputs concatenates curated fixtures with one or more
// auto-generated batches. Lets each FormatSpec list its hand-curated
// fixtures inline and append the scanner's output without writing the
// same `append(append(...))` chain by hand.
func mergeInputs(curated []FormatInput, generated ...[]FormatInput) []FormatInput {
	out := make([]FormatInput, 0, len(curated))
	out = append(out, curated...)
	for _, g := range generated {
		out = append(out, g...)
	}
	return out
}

// formatSpecs lists every okf_* filter declared in the okapi-bridge
// v2 manifest at framework_version 1.48.0. Pinned by intent: when a
// future release adds, removes, or renames a filter, update this
// table (and the parity dashboard regenerates).
//
// IMPORTANT for contributors: when adding a row, choose the smallest
// possible Inputs that still exercise the format's main read path. The
// goal is regression safety, not exhaustive coverage — long fixtures
// inflate CI time and obscure failure messages.
var formatSpecs = []FormatSpec{
	// ── Text / structured ────────────────────────────────────────────
	{
		ID:        "okf_html",
		MimeType:  "text/html",
		NewReader: func() format.DataFormatReader { return htmlfmt.NewReader() },
		Inputs: mergeInputs(
			[]FormatInput{
				{Name: "minimal", Content: ttext(`<html><body><p>Hello world.</p></body></html>`)},
				{Name: "inline-codes", Content: ttext(`<html><body><p>Click <a href="/x">here</a> to continue.</p></body></html>`)},
				{Name: "two-paragraphs", Content: ttext(`<html><body><p>First.</p><p>Second.</p></body></html>`)},
			},
			// Auto-extracted by scripts/okapi-test-scan.
			GeneratedHtmlSnippetsTestInputs,
			GeneratedHtmlEventTestInputs,
			GeneratedHtmlConfigurationSupportTestInputs,
			GeneratedSkipEncodingDeclarationTestInputs,
		),
	},
	{
		ID:        "okf_html5",
		MimeType:  "text/html",
		NewReader: func() format.DataFormatReader { return htmlfmt.NewReader() },
		Inputs: []FormatInput{
			{Name: "minimal", Content: ttext(`<!DOCTYPE html><html><body><p>Hello world.</p></body></html>`)},
		},
	},
	{
		ID:        "okf_json",
		MimeType:  "application/json",
		NewReader: func() format.DataFormatReader { return jsonfmt.NewReader() },
		Inputs: mergeInputs(
			[]FormatInput{
				{Name: "flat", Content: ttext(`{"greeting":"Hello world."}`)},
				{Name: "nested", Content: ttext(`{"messages":{"hello":"Hi","bye":"Bye"}}`)},
			},
			GeneratedJSONFilterTestInputs,
			GeneratedJsonSnippetParserTestInputs,
		),
	},
	{
		ID:        "okf_yaml",
		MimeType:  "text/x-yaml",
		NewReader: func() format.DataFormatReader { return yamlfmt.NewReader() },
		Inputs: mergeInputs(
			[]FormatInput{
				{Name: "flat", Content: ttext("greeting: Hello world.\nfarewell: Goodbye.\n")},
			},
			GeneratedYmlFilterTestInputs,
			GeneratedYamlFilterTestInputs,
			GeneratedYamlParserTestInputs,
		),
	},
	{
		ID:        "okf_xml",
		MimeType:  "text/xml",
		NewReader: func() format.DataFormatReader { return xmlfmt.NewReader() },
		Inputs: []FormatInput{
			{Name: "minimal", Content: ttext(`<?xml version="1.0"?><root><msg>Hello world.</msg></root>`)},
		},
	},
	{
		ID:       "okf_xmlstream",
		MimeType: "text/xml",
		// xmlstream uses the xml format with a streaming flag; the
		// neokapi xml reader handles both modes.
		NewReader: func() format.DataFormatReader { return xmlfmt.NewReader() },
		Inputs: []FormatInput{
			{Name: "dita-like", Content: ttext(`<?xml version="1.0"?><topic><title>Hi</title><body>Hello.</body></topic>`)},
		},
	},
	// okf_xml / okf_xmlstream config-preset formats. The native side runs the
	// xml reader with the Go config preset (DitaConfig/DocBookConfig/ResXConfig
	// in core/formats/xml/presets.go); head-to-head is gated on okapi-bridge
	// config-by-name support (SKIP_BRIDGE_CONFIG). Native behaviour is
	// regression-tested in core/formats/xml/presets_test.go against the
	// upstream gold XLIFF.
	{
		ID:       "okf_xmlstream-dita",
		MimeType: "text/xml",
		NewReader: func() format.DataFormatReader {
			r := xmlfmt.NewReader()
			_ = r.SetConfig(xmlfmt.DitaConfig())
			return r
		},
		Inputs: []FormatInput{
			{Name: "dita", Content: ttext(`<?xml version="1.0"?><concept id="c"><title>Hi</title><conbody><p>Hello.</p></conbody></concept>`)},
		},
		Skip: SKIP_BRIDGE_CONFIG,
	},
	{
		ID:       "okf_xml-docbook",
		MimeType: "text/xml",
		NewReader: func() format.DataFormatReader {
			r := xmlfmt.NewReader()
			_ = r.SetConfig(xmlfmt.DocBookConfig())
			return r
		},
		Inputs: []FormatInput{
			{Name: "docbook", Content: ttext(`<?xml version="1.0"?><article><para>Hello <emphasis>world</emphasis>.</para></article>`)},
		},
		Skip: SKIP_BRIDGE_CONFIG,
	},
	{
		ID:       "okf_xml-resx",
		MimeType: "text/xml",
		NewReader: func() format.DataFormatReader {
			r := xmlfmt.NewReader()
			_ = r.SetConfig(xmlfmt.ResXConfig())
			return r
		},
		Inputs: []FormatInput{
			{Name: "resx", Content: ttext(`<?xml version="1.0"?><root><data name="greeting"><value>Hello world.</value></data></root>`)},
		},
		Skip: SKIP_BRIDGE_CONFIG,
	},
	{
		ID:        "okf_dtd",
		MimeType:  "application/xml+dtd",
		NewReader: func() format.DataFormatReader { return dtdfmt.NewReader() },
		Inputs: mergeInputs(
			[]FormatInput{
				{Name: "minimal", Content: ttext(`<!ENTITY greeting "Hello world.">`)},
			},
			GeneratedDTDFilterTestInputs,
		),
	},
	{
		ID:          "okf_properties",
		MimeType:    "text/x-properties",
		NewReader:   func() format.DataFormatReader { return propertiesfmt.NewReader() },
		NewWriter:   func() format.DataFormatWriter { return propertiesfmt.NewWriter() },
		TikalExt:    ".properties",
		TikalConfig: "okf_properties",
		// extraComments=true exercises the typed-Params chain end-to-end:
		// the native side receives it via DataFormatConfig.ApplyMap,
		// the bridge receives it via StringifyParams → FilterParams.
		// Both sides should recognise `;` and `//` as comment markers
		// in addition to the standard `#`/`!`.
		Params: map[string]any{
			"extraComments": true,
		},
		Inputs: mergeInputs(
			[]FormatInput{
				{Name: "flat", Content: ttext("greeting=Hello world.\nfarewell=Goodbye.\n")},
				{Name: "semi-comments", Content: ttext("# standard\n; semi-comment\ngreeting=Hello world.\n")},
			},
			GeneratedPropertiesFilterTestInputs,
		),
	},
	{
		ID:          "okf_po",
		MimeType:    "application/x-gettext",
		NewReader:   func() format.DataFormatReader { return pofmt.NewReader() },
		NewWriter:   func() format.DataFormatWriter { return pofmt.NewWriter() },
		TikalExt:    ".po",
		TikalConfig: "okf_po",
		// Bridge writes msgstr "Hello world." (auto-fills target with
		// source); native preserves msgstr "" (empty target). Tikal
		// (the canonical Okapi CLI) produces the same auto-fill output
		// as the bridge — confirming this is an Okapi semantic, not
		// bridge plumbing. Recorded as a documented divergence rather
		// than a bug; needs a recipe-level decision before either side
		// changes.
		SkipRoundTrip: "writer fills empty target with source on bridge side; native preserves empty target",
		SkipTikal:     "same divergence as bridge: tikal also auto-fills empty msgstr with source",
		Inputs: mergeInputs(
			[]FormatInput{
				{Name: "single", Content: ttext(`msgid ""
msgstr ""
"Content-Type: text/plain; charset=UTF-8\n"

msgid "Hello world."
msgstr ""
`)},
			},
			GeneratedPOFilterTestInputs,
			GeneratedPOWriterTestInputs,
		),
	},
	{
		ID:        "okf_phpcontent",
		MimeType:  "application/x-php",
		NewReader: func() format.DataFormatReader { return phpcontentfmt.NewReader() },
		Skip:      SKIP_DIVERGENCE_453,
	},
	{
		ID:          "okf_plaintext",
		MimeType:    "text/plain",
		NewReader:   func() format.DataFormatReader { return plaintextfmt.NewReader() },
		NewWriter:   func() format.DataFormatWriter { return plaintextfmt.NewWriter() },
		TikalExt:    ".txt",
		TikalConfig: "okf_plaintext",
		Inputs: []FormatInput{
			{Name: "two-lines", Content: ttext("Hello world.\nGoodbye.\n")},
		},
	},
	{
		ID:       "okf_baseplaintext",
		MimeType: "text/plain",
		// baseplaintext is the parent class; the plaintext reader
		// covers it.
		NewReader: func() format.DataFormatReader { return plaintextfmt.NewReader() },
		Inputs: []FormatInput{
			{Name: "single-line", Content: ttext("Hello world.\n")},
		},
	},
	{
		ID:        "okf_paraplaintext",
		MimeType:  "text/plain",
		NewReader: func() format.DataFormatReader { return paraplaintextfmt.NewReader() },
		Inputs: []FormatInput{
			{Name: "two-paragraphs", Content: ttext("First paragraph.\n\nSecond paragraph.\n")},
		},
	},
	{
		ID:        "okf_splicedlines",
		MimeType:  "text/plain",
		NewReader: func() format.DataFormatReader { return splicedlinesfmt.NewReader() },
		Inputs: []FormatInput{
			{Name: "two-lines", Content: ttext("Line one.\nLine two.\n")},
		},
	},
	{
		ID:        "okf_regex",
		MimeType:  "text/x-regex",
		NewReader: func() format.DataFormatReader { return regexfmt.NewReader() },
		Inputs: mergeInputs(
			[]FormatInput{
				{Name: "key-value", Content: ttext("greeting = Hello world.\nfarewell = Goodbye.\n")},
			},
			GeneratedRegexFilterTestInputs,
		),
	},
	{
		ID:        "okf_regexplaintext",
		MimeType:  "text/plain",
		NewReader: func() format.DataFormatReader { return regexfmt.NewReader() },
		Skip:      SKIP_DIVERGENCE_453,
	},
	{
		ID:        "okf_doxygen",
		MimeType:  "text/x-doxygen-txt",
		NewReader: func() format.DataFormatReader { return doxygenfmt.NewReader() },
		Skip:      SKIP_DIVERGENCE_453,
	},
	{
		ID:        "okf_markdown",
		MimeType:  "text/markdown",
		NewReader: func() format.DataFormatReader { return markdownfmt.NewReader() },
		Inputs: mergeInputs(
			[]FormatInput{
				{Name: "minimal", Content: ttext("# Hello\n\nThis is a paragraph.\n")},
			},
			GeneratedMarkdownFilterTestInputs,
			GeneratedMarkdownWriterTestInputs,
		),
	},
	{
		ID:        "okf_wiki",
		MimeType:  "text/x-wiki-txt",
		NewReader: func() format.DataFormatReader { return wikifmt.NewReader() },
		Inputs: mergeInputs(
			[]FormatInput{
				{Name: "minimal", Content: ttext("== Hello ==\n\nThis is a paragraph.\n")},
			},
			GeneratedWikiFilterTestInputs,
			GeneratedWikiWriterTestInputs,
		),
	},
	{
		ID:        "okf_tex",
		MimeType:  "text/x-tex-text",
		NewReader: func() format.DataFormatReader { return texfmt.NewReader() },
		Skip:      SKIP_DIVERGENCE_453,
	},
	{
		ID:        "okf_mosestext",
		MimeType:  "text/x-mosestext",
		NewReader: func() format.DataFormatReader { return mosestextfmt.NewReader() },
		Inputs: []FormatInput{
			{Name: "two-lines", Content: ttext("Hello world.\nGoodbye.\n")},
		},
	},
	{
		ID:        "okf_transtable",
		MimeType:  "text/x-transtable",
		NewReader: func() format.DataFormatReader { return transtablefmt.NewReader() },
		Skip:      SKIP_DIVERGENCE_453,
	},
	{
		ID:        "okf_commaseparatedvalues",
		MimeType:  "text/csv",
		NewReader: func() format.DataFormatReader { return csvfmt.NewReader() },
		Skip:      SKIP_DIVERGENCE_453,
	},
	{
		ID:        "okf_tabseparatedvalues",
		MimeType:  "text/csv",
		NewReader: func() format.DataFormatReader { return csvfmt.NewReader() },
		Skip:      SKIP_DIVERGENCE_453,
	},
	{
		ID:       "okf_basetable",
		MimeType: "text/csv",
		// basetable is the abstract parent for csv/fixedwidth/tsv —
		// csv covers it for parity.
		NewReader: func() format.DataFormatReader { return csvfmt.NewReader() },
		Skip:      SKIP_DIVERGENCE_453,
	},
	{
		ID:        "okf_fixedwidthcolumns",
		MimeType:  "text/csv",
		NewReader: func() format.DataFormatReader { return fixedwidthfmt.NewReader() },
		Skip:      SKIP_DIVERGENCE_453,
	},
	{
		ID:        "okf_table",
		MimeType:  "text/csv",
		NewReader: func() format.DataFormatReader { return csvfmt.NewReader() },
		Skip:      SKIP_DIVERGENCE_453,
	},

	// ── XLIFF family ─────────────────────────────────────────────────
	{
		ID:        "okf_xliff",
		MimeType:  "application/x-xliff+xml",
		NewReader: func() format.DataFormatReader { return xlifffmt.NewReader() },
		Inputs: mergeInputs(
			[]FormatInput{
				{Name: "single-tu", Content: ttext(`<?xml version="1.0"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="hello.txt">
    <body>
      <trans-unit id="1"><source>Hello world.</source></trans-unit>
    </body>
  </file>
</xliff>`)},
			},
			GeneratedXLIFFFilterTestInputs,
			GeneratedXLIFFFilterXtmPropTestInputs,
		),
	},
	{
		ID:        "okf_xliff2",
		MimeType:  "application/xliff+xml",
		NewReader: func() format.DataFormatReader { return xliff2fmt.NewReader() },
		Inputs: []FormatInput{
			{Name: "single-tu", Content: ttext(`<?xml version="1.0"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <segment><source>Hello world.</source></segment>
    </unit>
  </file>
</xliff>`)},
		},
	},
	{
		ID:        "okf_tmx",
		MimeType:  "application/x-tmx+xml",
		NewReader: func() format.DataFormatReader { return tmxfmt.NewReader() },
		Inputs: mergeInputs(
			[]FormatInput{
				{Name: "single-tu", Content: ttext(`<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="manual" creationtoolversion="1" segtype="sentence" o-tmf="x" adminlang="en" srclang="en" datatype="plaintext"/>
  <body>
    <tu><tuv xml:lang="en"><seg>Hello world.</seg></tuv></tu>
  </body>
</tmx>`)},
			},
			GeneratedTmxFilterTestInputs,
		),
	},
	{
		ID:        "okf_ttx",
		MimeType:  "application/x-ttx+xml",
		NewReader: func() format.DataFormatReader { return ttxfmt.NewReader() },
		Skip:      SKIP_DIVERGENCE_453,
	},
	{
		ID:        "okf_txml",
		MimeType:  "text/xml",
		NewReader: func() format.DataFormatReader { return txmlfmt.NewReader() },
		Skip:      SKIP_DIVERGENCE_453,
	},
	{
		ID:        "okf_ttml",
		MimeType:  "application/ttml+xml",
		NewReader: func() format.DataFormatReader { return ttmlfmt.NewReader() },
		Skip:      SKIP_BRIDGE_BUG_452,
	},
	{
		ID:        "okf_ts",
		MimeType:  "application/x-ts",
		NewReader: func() format.DataFormatReader { return tsfmt.NewReader() },
		Inputs: mergeInputs(
			[]FormatInput{
				{Name: "minimal", Content: ttext(`<?xml version="1.0"?>
<!DOCTYPE TS>
<TS version="2.1" language="fr">
  <context>
    <name>main</name>
    <message><source>Hello world.</source><translation type="unfinished"></translation></message>
  </context>
</TS>`)},
			},
			GeneratedTsFilterTestInputs,
		),
	},
	{
		ID:        "okf_vtt",
		MimeType:  "text/vtt",
		NewReader: func() format.DataFormatReader { return vttfmt.NewReader() },
		Inputs: []FormatInput{
			{Name: "minimal", Content: ttext("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHello world.\n")},
		},
	},
	{
		ID:        "okf_vignette",
		MimeType:  "text/xml",
		NewReader: func() format.DataFormatReader { return vignettefmt.NewReader() },
		Skip:      SKIP_DIVERGENCE_453,
	},

	// ── Office / archive (binary, snapshotted as bridge-only) ────────
	{
		ID:       "okf_idml",
		MimeType: "application/vnd.adobe.indesign-idml-package",
		Skip:     SKIP_BINARY,
	},
	{
		ID:       "okf_icml",
		MimeType: "application/x-icml+xml",
		// Need binary ICML; treat as skipped until corpus is shipped.
		Skip: SKIP_BINARY,
	},
	{
		ID:       "okf_openxml",
		MimeType: "text/xml",
		Skip:     SKIP_BINARY,
	},
	{
		ID:       "okf_odf",
		MimeType: "text/x-odf",
		Skip:     SKIP_BINARY,
	},
	{
		ID:       "okf_openoffice",
		MimeType: "application/x-openoffice",
		Skip:     SKIP_BINARY,
	},
	{
		ID:       "okf_archive",
		MimeType: "application/x-archive",
		Skip:     SKIP_BINARY,
	},
	{
		ID:       "okf_mif",
		MimeType: "application/vnd.mif",
		Skip:     SKIP_BINARY,
	},
	{
		ID:       "okf_pdf",
		MimeType: "application/pdf",
		Skip:     SKIP_BINARY,
	},
	{
		ID:       "okf_rtf",
		MimeType: "application/rtf",
		Skip:     SKIP_BINARY,
	},
	{
		ID:       "okf_sdlpackage",
		MimeType: "application/x-sdlpackage",
		Skip:     SKIP_BINARY,
	},

	// ── Bridge-only or specialized (no native Go port) ───────────────
	{
		ID:       "okf_pensieve",
		MimeType: "application/x-pensieve-tm",
		Skip:     SKIP_BINARY,
	},
	{
		ID:        "okf_multiparsers",
		MimeType:  "text/csv",
		NewReader: nil, // bridge-only
		Inputs: []FormatInput{
			{Name: "minimal", Content: ttext("hello,Hello world.\n")},
		},
	},
	{
		ID:       "okf_rainbowkit",
		MimeType: "application/x-rainbowkit",
		Skip:     SKIP_BINARY,
	},
	{
		ID:       "okf_xini",
		MimeType: "text/x-xini",
		Skip:     SKIP_BINARY,
	},
	{
		ID:       "okf_xinirainbowkit",
		MimeType: "text/x-xini",
		Skip:     SKIP_BINARY,
	},
	{
		ID:       "okf_transifex",
		MimeType: "application/x-transifex",
		Skip:     SKIP_BINARY,
	},
}

// SRT (bridge filter is okf_regex tuned to .srt; native srt has its
// own reader). The bridge does not expose okf_srt as a separate
// filter, so we don't add a parity row here — the okf_regex row
// above covers the bridge side; the native srt port is exercised
// by core/formats/srt unit tests.
//
// The imports below keep the binary-format readers reachable from the
// parity package so that landing testdata in okapi-bridge can flip
// their Skip strings to head-to-head with a one-line edit.
var _ = []func() format.DataFormatReader{
	func() format.DataFormatReader { return srtfmt.NewReader() },
	func() format.DataFormatReader { return openxmlfmt.NewReader() },
	func() format.DataFormatReader { return idmlfmt.NewReader() },
	func() format.DataFormatReader { return icmlfmt.NewReader() },
	func() format.DataFormatReader { return miffmt.NewReader() },
	func() format.DataFormatReader { return pdffmt.NewReader() },
	func() format.DataFormatReader { return rtffmt.NewReader() },
	func() format.DataFormatReader { return odffmt.NewReader() },
}

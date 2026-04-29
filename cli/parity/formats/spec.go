//go:build parity

// Package formats holds the per-filter parity tests that run every
// Okapi filter present in the okapi-bridge v2 manifest against its
// neokapi Go counterpart (when one exists) or as a bridge-only
// stability snapshot (when no Go port exists).
//
// Each entry in `formatSpecs` declares one filter:
//
//	  ID            okf_<name> — the manifest id and the FilterClass
//	                sent to BridgeService.Process.
//	  MimeType      mime hint passed to both bridge and native readers.
//	  Inputs        list of named sample inputs (small inline strings
//	                or testdata paths).
//	  NewReader     constructs the in-process Go reader. Nil means
//	                bridge-only — the test asserts the bridge runs and
//	                produces a non-empty stream against `Inputs`, but
//	                does not compare against a native implementation.
//	  Skip          if non-empty, the test skips with this reason
//	                (used for filters that need binary corpus we don't
//	                ship in the repo — see SKIP_BINARY).
package formats

import (
	"github.com/neokapi/neokapi/core/format"
	archivefmt "github.com/neokapi/neokapi/core/formats/archive"
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

// FormatInput is one named sample input.
type FormatInput struct {
	Name    string
	Content []byte
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
type FormatSpec struct {
	ID            string
	MimeType      string
	Inputs        []FormatInput
	NewReader     func() format.DataFormatReader
	NewWriter     func() format.DataFormatWriter
	Skip          string
	SkipRoundTrip string
	FilterArgs    map[string]string
}

func ttext(s string) []byte { return []byte(s) }

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
		Inputs: []FormatInput{
			{"minimal", ttext(`<html><body><p>Hello world.</p></body></html>`)},
			{"inline-codes", ttext(`<html><body><p>Click <a href="/x">here</a> to continue.</p></body></html>`)},
			{"two-paragraphs", ttext(`<html><body><p>First.</p><p>Second.</p></body></html>`)},
		},
	},
	{
		ID:        "okf_html5",
		MimeType:  "text/html",
		NewReader: func() format.DataFormatReader { return htmlfmt.NewReader() },
		Inputs: []FormatInput{
			{"minimal", ttext(`<!DOCTYPE html><html><body><p>Hello world.</p></body></html>`)},
		},
	},
	{
		ID:        "okf_json",
		MimeType:  "application/json",
		NewReader: func() format.DataFormatReader { return jsonfmt.NewReader() },
		Inputs: []FormatInput{
			{"flat", ttext(`{"greeting":"Hello world."}`)},
			{"nested", ttext(`{"messages":{"hello":"Hi","bye":"Bye"}}`)},
		},
	},
	{
		ID:        "okf_yaml",
		MimeType:  "text/x-yaml",
		NewReader: func() format.DataFormatReader { return yamlfmt.NewReader() },
		Inputs: []FormatInput{
			{"flat", ttext("greeting: Hello world.\nfarewell: Goodbye.\n")},
		},
	},
	{
		ID:        "okf_xml",
		MimeType:  "text/xml",
		NewReader: func() format.DataFormatReader { return xmlfmt.NewReader() },
		Inputs: []FormatInput{
			{"minimal", ttext(`<?xml version="1.0"?><root><msg>Hello world.</msg></root>`)},
		},
	},
	{
		ID:       "okf_xmlstream",
		MimeType: "text/xml",
		// xmlstream uses the xml format with a streaming flag; the
		// neokapi xml reader handles both modes.
		NewReader: func() format.DataFormatReader { return xmlfmt.NewReader() },
		Inputs: []FormatInput{
			{"dita-like", ttext(`<?xml version="1.0"?><topic><title>Hi</title><body>Hello.</body></topic>`)},
		},
	},
	{
		ID:        "okf_dtd",
		MimeType:  "application/xml+dtd",
		NewReader: func() format.DataFormatReader { return dtdfmt.NewReader() },
		Inputs: []FormatInput{
			{"minimal", ttext(`<!ENTITY greeting "Hello world.">`)},
		},
	},
	{
		ID:        "okf_properties",
		MimeType:  "text/x-properties",
		NewReader: func() format.DataFormatReader { return propertiesfmt.NewReader() },
		NewWriter: func() format.DataFormatWriter { return propertiesfmt.NewWriter() },
		Inputs: []FormatInput{
			{"flat", ttext("greeting=Hello world.\nfarewell=Goodbye.\n")},
		},
	},
	{
		ID:        "okf_po",
		MimeType:  "application/x-gettext",
		NewReader: func() format.DataFormatReader { return pofmt.NewReader() },
		NewWriter: func() format.DataFormatWriter { return pofmt.NewWriter() },
		// Bridge writes msgstr "Hello world." (auto-fills target with
		// source); native preserves msgstr "" (empty target). Tracked
		// as a writer-side default-handling divergence rather than a
		// neokapi bug — needs a recipe-level decision before either
		// side changes.
		SkipRoundTrip: "writer fills empty target with source on bridge side; native preserves empty target",
		Inputs: []FormatInput{
			{"single", ttext(`msgid ""
msgstr ""
"Content-Type: text/plain; charset=UTF-8\n"

msgid "Hello world."
msgstr ""
`)},
		},
	},
	{
		ID:        "okf_phpcontent",
		MimeType:  "application/x-php",
		NewReader: func() format.DataFormatReader { return phpcontentfmt.NewReader() },
		Skip:      SKIP_DIVERGENCE_453,
	},
	{
		ID:        "okf_plaintext",
		MimeType:  "text/plain",
		NewReader: func() format.DataFormatReader { return plaintextfmt.NewReader() },
		NewWriter: func() format.DataFormatWriter { return plaintextfmt.NewWriter() },
		Inputs: []FormatInput{
			{"two-lines", ttext("Hello world.\nGoodbye.\n")},
		},
	},
	{
		ID:       "okf_baseplaintext",
		MimeType: "text/plain",
		// baseplaintext is the parent class; the plaintext reader
		// covers it.
		NewReader: func() format.DataFormatReader { return plaintextfmt.NewReader() },
		Inputs: []FormatInput{
			{"single-line", ttext("Hello world.\n")},
		},
	},
	{
		ID:        "okf_paraplaintext",
		MimeType:  "text/plain",
		NewReader: func() format.DataFormatReader { return paraplaintextfmt.NewReader() },
		Inputs: []FormatInput{
			{"two-paragraphs", ttext("First paragraph.\n\nSecond paragraph.\n")},
		},
	},
	{
		ID:        "okf_splicedlines",
		MimeType:  "text/plain",
		NewReader: func() format.DataFormatReader { return splicedlinesfmt.NewReader() },
		Inputs: []FormatInput{
			{"two-lines", ttext("Line one.\nLine two.\n")},
		},
	},
	{
		ID:        "okf_regex",
		MimeType:  "text/x-regex",
		NewReader: func() format.DataFormatReader { return regexfmt.NewReader() },
		Inputs: []FormatInput{
			{"key-value", ttext("greeting = Hello world.\nfarewell = Goodbye.\n")},
		},
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
		Inputs: []FormatInput{
			{"minimal", ttext("# Hello\n\nThis is a paragraph.\n")},
		},
	},
	{
		ID:        "okf_wiki",
		MimeType:  "text/x-wiki-txt",
		NewReader: func() format.DataFormatReader { return wikifmt.NewReader() },
		Inputs: []FormatInput{
			{"minimal", ttext("== Hello ==\n\nThis is a paragraph.\n")},
		},
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
			{"two-lines", ttext("Hello world.\nGoodbye.\n")},
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
		Inputs: []FormatInput{
			{"single-tu", ttext(`<?xml version="1.0"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="hello.txt">
    <body>
      <trans-unit id="1"><source>Hello world.</source></trans-unit>
    </body>
  </file>
</xliff>`)},
		},
	},
	{
		ID:        "okf_xliff2",
		MimeType:  "application/xliff+xml",
		NewReader: func() format.DataFormatReader { return xliff2fmt.NewReader() },
		Inputs: []FormatInput{
			{"single-tu", ttext(`<?xml version="1.0"?>
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
		Inputs: []FormatInput{
			{"single-tu", ttext(`<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtool="manual" creationtoolversion="1" segtype="sentence" o-tmf="x" adminlang="en" srclang="en" datatype="plaintext"/>
  <body>
    <tu><tuv xml:lang="en"><seg>Hello world.</seg></tuv></tu>
  </body>
</tmx>`)},
		},
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
		Inputs: []FormatInput{
			{"minimal", ttext(`<?xml version="1.0"?>
<!DOCTYPE TS>
<TS version="2.1" language="fr">
  <context>
    <name>main</name>
    <message><source>Hello world.</source><translation type="unfinished"></translation></message>
  </context>
</TS>`)},
		},
	},
	{
		ID:        "okf_vtt",
		MimeType:  "text/vtt",
		NewReader: func() format.DataFormatReader { return vttfmt.NewReader() },
		Inputs: []FormatInput{
			{"minimal", ttext("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHello world.\n")},
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
			{"minimal", ttext("hello,Hello world.\n")},
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
	func() format.DataFormatReader { return archivefmt.NewReader() },
	func() format.DataFormatReader { return openxmlfmt.NewReader() },
	func() format.DataFormatReader { return idmlfmt.NewReader() },
	func() format.DataFormatReader { return icmlfmt.NewReader() },
	func() format.DataFormatReader { return miffmt.NewReader() },
	func() format.DataFormatReader { return pdffmt.NewReader() },
	func() format.DataFormatReader { return rtffmt.NewReader() },
	func() format.DataFormatReader { return odffmt.NewReader() },
}

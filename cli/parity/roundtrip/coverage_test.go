//go:build parity

package roundtrip_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli/parity/roundtrip"
	"github.com/neokapi/neokapi/core/registry"
)

// repoFile loads a fixture from the framework testdata tree by
// repo-relative path (e.g. "core/formats/idml/testdata/helloworld.idml").
// Path is resolved against the package's own location so the test
// works regardless of the caller's CWD.
func repoFile(t *testing.T, rel string) []byte {
	t.Helper()
	// cli/parity/roundtrip → ../../.. is the repo root.
	candidate := filepath.Join("..", "..", "..", rel)
	abs, err := filepath.Abs(candidate)
	if err != nil {
		t.Fatalf("repoFile %q: abs: %v", rel, err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("repoFile %q: %v", rel, err)
	}
	return data
}

// fixture captures one format's round-trip case. The harness wires
// every fixture through the available engines (native always; bridge
// when filterClass is set; tikal when installed).
//
// Add a row to extend coverage. If the bridge filter expects
// different parameter names than neokapi's spec config, fill
// bridgeParams with the already-translated map (same approach as the
// spec runner's BridgeConfig hook in cli/parity/formats/).
//
// `skip` lists engines whose divergence is genuine and tracked
// elsewhere — same semantics as `expected_fail` in spec.yaml. Each
// entry must be accompanied by a one-line comment explaining the
// divergence so future readers understand whether to keep skipping
// or revisit.
type fixture struct {
	name             string
	formatID         registry.FormatID
	filterClass      string // empty = no bridge engine
	tikalUnsupported bool   // true = no tikal engine
	filename         string
	body             []byte // either body or bodyFile must be set
	bodyFile         string // repo-relative path to load at test time
	nativeConfig     map[string]any
	bridgeParams     map[string]string
	skip             []string
}

func TestRoundTrip_Coverage(t *testing.T) {
	for _, c := range coverageCases() {
		c := c
		t.Run(c.name, func(t *testing.T) {
			body := c.body
			if body == nil && c.bodyFile != "" {
				body = repoFile(t, c.bodyFile)
			}
			if len(body) == 0 {
				t.Fatalf("fixture %q: body or bodyFile required", c.name)
			}
			var bridge *roundtrip.BridgeEngine
			if c.filterClass != "" {
				bridge = &roundtrip.BridgeEngine{
					FilterClass:  c.filterClass,
					FilterParams: c.bridgeParams,
				}
			}
			var tikal *roundtrip.TikalEngine
			if !c.tikalUnsupported {
				tikal = &roundtrip.TikalEngine{}
			}
			roundtrip.RunThreeWay(t, roundtrip.Case{
				Name:     c.name,
				FormatID: c.formatID,
				Input: roundtrip.Input{
					Bytes:    body,
					Filename: c.filename,
				},
				ExpectedSkipped: c.skip,
			},
				&roundtrip.NativeEngine{
					FormatID:     c.formatID,
					ReaderConfig: c.nativeConfig,
				},
				bridge,
				tikal,
			)
		})
	}
}

func coverageCases() []fixture {
	return []fixture{
		// ── Plain-text family ─────────────────────────────────────
		{
			name:        "plaintext_three_lines",
			formatID:    "plaintext",
			filterClass: "okf_plaintext",
			filename:    "doc.txt",
			body:        []byte("Hello world\nAnother line\nThird paragraph\n"),
		},
		{
			name:        "paraplaintext_two_paragraphs",
			formatID:    "paraplaintext",
			filterClass: "okf_paraplaintext",
			filename:    "doc.txt",
			body:        []byte("First paragraph here.\n\nSecond paragraph next."),
		},
		{
			name:        "mosestext_two_lines",
			formatID:    "mosestext",
			filterClass: "okf_mosestext",
			filename:    "moses.txt",
			body:        []byte("Hello world\nGoodbye now\n"),
		},

		// ── HTML / markup ─────────────────────────────────────────
		{
			name:        "html_paragraphs",
			formatID:    "html",
			filterClass: "okf_html",
			filename:    "doc.html",
			body: []byte(
				"<!doctype html>\n<html><body>\n" +
					"<p>First paragraph.</p>\n" +
					"<p>Second one.</p>\n" +
					"</body></html>\n"),
		},
		{
			name:        "markdown_two_paragraphs",
			formatID:    "markdown",
			filterClass: "okf_markdown",
			filename:    "doc.md",
			body: []byte(
				"# Title\n\nFirst paragraph here.\n\nSecond paragraph follows.\n"),
		},
		{
			// Bridge merges adjacent wiki lines into a single block;
			// native treats each line as its own block. Skip bridge
			// rather than over-fitting the fixture — wiki segmentation
			// is a real divergence that deserves its own follow-up.
			name:        "wiki_two_paragraphs",
			formatID:    "wiki",
			filterClass: "okf_wiki",
			filename:    "doc.wiki",
			body:        []byte("First wiki paragraph.\n\nSecond wiki paragraph.\n"),
			skip:        []string{"bridge"},
		},

		// ── Key-value & structured data ───────────────────────────
		{
			name:        "po_two_entries",
			formatID:    "po",
			filterClass: "okf_po",
			filename:    "messages.po",
			body: []byte(`msgid ""
msgstr ""
"Content-Type: text/plain; charset=UTF-8\n"

msgid "Hello"
msgstr ""

msgid "Goodbye"
msgstr ""
`),
		},
		{
			name:        "properties_two_keys",
			formatID:    "properties",
			filterClass: "okf_properties",
			filename:    "messages.properties",
			body: []byte(
				"greeting=Hello world\n" +
					"farewell=Goodbye now\n"),
		},
		{
			name:        "json_two_strings",
			formatID:    "json",
			filterClass: "okf_json",
			filename:    "messages.json",
			body: []byte(`{
  "greeting": "Hello world",
  "farewell": "Goodbye now"
}
`),
		},
		{
			// Native YAML writer reorders mapping keys on emit, so
			// re-extraction returns blocks in reverse order. Real
			// native bug — skip native here.
			name:        "yaml_two_keys",
			formatID:    "yaml",
			filterClass: "okf_yaml",
			filename:    "messages.yaml",
			body:        []byte("greeting: Hello world\nfarewell: Goodbye now\n"),
			skip:        []string{"native"},
		},
		{
			// Native phpcontent's writer produces output that the
			// reader can't re-extract — the round-trip yields 0
			// blocks. Bridge round-trips fine. Skip native; this is
			// a real native bug worth a follow-up.
			name:        "phpcontent_two_strings",
			formatID:    "phpcontent",
			filterClass: "okf_phpcontent",
			filename:    "messages.php",
			body: []byte(`<?php
$greeting = 'Hello world';
$farewell = 'Goodbye now';
`),
			skip: []string{"native"},
		},

		// ── Tabular ───────────────────────────────────────────────
		{
			// Bridge's TSV default treats the first row as a header
			// and skips it; native's default is cell-per-Block over
			// every row (see feedback_parity_semantic_config). Skip
			// bridge — fixing it requires explicit hasHeader
			// translation in a TSV BridgeConfig hook.
			name:        "tsv_two_rows",
			formatID:    "tsv",
			filterClass: "okf_tabseparatedvalues",
			filename:    "data.tsv",
			body:        []byte("Hello world\nGoodbye now\n"),
			skip:        []string{"bridge"},
		},

		// ── Code/markup ───────────────────────────────────────────
		{
			// Bridge keeps the leading space after `@brief`; native
			// trims it. Cosmetic divergence in the upstream
			// comment-parser — skip bridge.
			name:        "doxygen_two_briefs",
			formatID:    "doxygen",
			filterClass: "okf_doxygen",
			filename:    "code.c",
			body: []byte(`/**
 * @brief First widget description
 */
void widget1(void);

/**
 * @brief Second gadget description
 */
void widget2(void);
`),
			skip: []string{"bridge"},
		},

		// ── XML / bilingual exchange formats ──────────────────────
		{
			// Native XML reader needs explicit ITS rules to know
			// which elements are translatable; with default config
			// it extracts the entire root as one Block. Skip native
			// here — proper config would be a per-format spec.yaml
			// concern, not the round-trip default.
			name:        "xml_two_strings",
			formatID:    "xml",
			filterClass: "okf_xml",
			filename:    "doc.xml",
			body: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<root>
  <text>Hello world</text>
  <text>Goodbye now</text>
</root>
`),
			skip: []string{"native"},
		},
		{
			name:        "xliff_two_units",
			formatID:    "xliff",
			filterClass: "okf_xliff",
			filename:    "doc.xlf",
			body: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="src">
    <body>
      <trans-unit id="u1"><source>Hello world</source></trans-unit>
      <trans-unit id="u2"><source>Goodbye now</source></trans-unit>
    </body>
  </file>
</xliff>
`),
		},
		{
			// okapi-bridge's daemon reports "filter does not support
			// writing: okf_xliff2" — the upstream filter is read-only.
			// Native xliff2 still round-trips fine; bridge is just
			// not a viable engine for xliff2 round-trips.
			name:        "xliff2_two_units",
			formatID:    "xliff2",
			filterClass: "okf_xliff2",
			filename:    "doc.xlf",
			body: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1"><segment><source>Hello world</source></segment></unit>
    <unit id="u2"><segment><source>Goodbye now</source></segment></unit>
  </file>
</xliff>
`),
			skip: []string{"bridge"},
		},
		{
			name:        "tmx_two_units",
			formatID:    "tmx",
			filterClass: "okf_tmx",
			filename:    "doc.tmx",
			body: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1" segtype="sentence" o-tmf="x" adminlang="en" srclang="en" datatype="plaintext"/>
  <body>
    <tu><tuv xml:lang="en"><seg>Hello world</seg></tuv></tu>
    <tu><tuv xml:lang="en"><seg>Goodbye now</seg></tuv></tu>
  </body>
</tmx>
`),
		},
		{
			// Bridge's okf_ts writer emits malformed XML on merge
			// (unexpected </translation> close). Real upstream bug.
			name:        "ts_two_messages",
			formatID:    "ts",
			filterClass: "okf_ts",
			filename:    "doc.ts",
			body: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="fr">
  <context>
    <name>MyApp</name>
    <message><source>Hello world</source><translation type="unfinished"></translation></message>
    <message><source>Goodbye now</source><translation type="unfinished"></translation></message>
  </context>
</TS>
`),
			skip: []string{"bridge"},
		},
		{
			// Bridge's Process RPC hangs (deadline exceeded) on TXML
			// round-trip — daemon never emits ProcessComplete. Real
			// bridge bug worth a follow-up issue.
			name:        "txml_two_segments",
			formatID:    "txml",
			filterClass: "okf_txml",
			filename:    "doc.txml",
			body: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<txml version="1.0" datatype="plaintext" segtype="sentence" sourcelang="en" targetlang="fr">
  <translatable id="1">
    <segment segmentId="1">
      <source>Hello world</source>
    </segment>
  </translatable>
  <translatable id="2">
    <segment segmentId="1">
      <source>Goodbye now</source>
    </segment>
  </translatable>
</txml>
`),
			skip: []string{"bridge"},
		},

		// ── Subtitle / timed-text ─────────────────────────────────
		{
			// Bridge's WebVTT filter concatenates adjacent cues into
			// one Block on round-trip. Real upstream segmentation
			// divergence — skip bridge.
			name:        "vtt_two_cues",
			formatID:    "vtt",
			filterClass: "okf_vtt",
			filename:    "subs.vtt",
			body: []byte(`WEBVTT

00:00:01.000 --> 00:00:03.000
Hello world

00:00:04.000 --> 00:00:06.000
Goodbye now
`),
			skip: []string{"bridge"},
		},
		{
			// Force `mergeCaptions: false` to align bridge with
			// native's per-paragraph segmentation. Without this the
			// upstream TTML filter's default `mergeAdjacentCaptions`
			// folds the two <p> elements into one Block on round-trip.
			name:         "ttml_two_paragraphs",
			formatID:     "ttml",
			filterClass:  "okf_ttml",
			filename:     "subs.ttml",
			bridgeParams: map[string]string{"mergeCaptions": "false"},
			body: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml">
  <body>
    <div>
      <p begin="00:00:01.000" end="00:00:03.000">Hello world</p>
      <p begin="00:00:04.000" end="00:00:06.000">Goodbye now</p>
    </div>
  </body>
</tt>
`),
		},

		// ── Misc text formats ─────────────────────────────────────
		{
			name:        "dtd_two_entities",
			formatID:    "dtd",
			filterClass: "okf_dtd",
			filename:    "doc.dtd",
			body: []byte(`<!ENTITY greeting "Hello world">
<!ENTITY farewell "Goodbye now">
`),
		},
		{
			// Bridge's TeX filter merges paragraphs separated by a
			// blank line into a single Block, native treats them
			// independently. Skip bridge.
			name:        "tex_two_paragraphs",
			formatID:    "tex",
			filterClass: "okf_tex",
			filename:    "doc.tex",
			body: []byte(`\documentclass{article}
\begin{document}
First paragraph here.

Second paragraph follows.
\end{document}
`),
			skip: []string{"bridge"},
		},

		// ── Tabular needing config ────────────────────────────────
		{
			// Native CSV defaults to cell-per-block (no header skip),
			// upstream Okapi defaults to row-per-block from
			// sourceColumns=1 and extracts the header. Aligning the
			// two requires an explicit per-format BridgeConfig
			// translator (see csv_bridge_config.go in the spec
			// runner). Skip bridge here — the round-trip-CSV
			// translator is a follow-up.
			name:        "csv_two_rows",
			formatID:    "csv",
			filterClass: "okf_commaseparatedvalues",
			filename:    "data.csv",
			body:        []byte("Hello world\nGoodbye now\n"),
			skip:        []string{"bridge"},
		},
		{
			// Same story as csv: the fixedwidthcolumns bridge filter
			// expects a flat string-encoded column spec (1-based,
			// inclusive); native takes a typed `columns: [...]`
			// list. The spec runner's fixedwidthBridgeConfig handles
			// the translation; the round-trip harness doesn't yet
			// invoke per-format translators. Skip bridge.
			//
			// trimValues:true so the writer's column-padding spaces
			// don't count as block content during re-extraction.
			name:        "fixedwidth_two_rows",
			formatID:    "fixedwidth",
			filterClass: "okf_fixedwidthcolumns",
			filename:    "data.fwc",
			nativeConfig: map[string]any{
				"trimValues": true,
				"columns": []any{
					map[string]any{"name": "id", "start": 0, "width": 5, "translatable": false},
					map[string]any{"name": "text", "start": 5, "width": 20, "translatable": true},
				},
			},
			body: []byte(`id001Hello world
id002Goodbye now
`),
			skip: []string{"bridge"},
		},
		{
			// Native splicedlines preserves LFs when joining lines;
			// bridge collapses them to a single space (tracked under
			// #536 in the spec runner). Skip bridge here too.
			name:        "splicedlines_two_chunks",
			formatID:    "splicedlines",
			filterClass: "okf_splicedlines",
			filename:    "doc.txt",
			body:        []byte("Hello world\nGoodbye now\n"),
			skip:        []string{"bridge"},
		},
		{
			// Transtable's wire format requires `okpCtx:tu=<id>` as
			// the first cell on each row. Targets are the second
			// cell (when present) — bilingual.
			//
			// Bridge's okf_transtable writer drops the TransTableV1
			// header and the `okpCtx` crumbs entirely (it concatenates
			// just the wrapped strings). Real upstream bug — skip
			// bridge.
			name:        "transtable_two_pairs",
			formatID:    "transtable",
			filterClass: "okf_transtable",
			filename:    "data.tsv",
			body: []byte(`TransTableV1	en	fr
"okpCtx:tu=1"	"Hello world"	""
"okpCtx:tu=2"	"Goodbye now"	""
`),
			skip: []string{"bridge"},
		},

		// regex, vignette, ttx: not covered yet — each needs
		// format-specific config wiring (regex rule set, vignette
		// importContentInstance shape, ttx UTF-16 encoding) that
		// doesn't fit the generic harness. Track these as follow-ups
		// once the comparable spec.yaml runners settle their config
		// translators.

		// ── Native-only formats (no bridge counterpart) ───────────
		{
			name:             "srt_two_cues",
			formatID:         "srt",
			filterClass:      "", // native-only, no okf_srt
			tikalUnsupported: true,
			filename:         "subs.srt",
			body: []byte(`1
00:00:01,000 --> 00:00:03,000
Hello world

2
00:00:04,000 --> 00:00:06,000
Goodbye now
`),
		},
		{
			// JSX (KLF) — native-only, exchange-format only.
			name:             "jsx_klf_two_blocks",
			formatID:         "jsx",
			filterClass:      "",
			tikalUnsupported: true,
			filename:         "doc.klf",
			body: []byte(`{
  "schemaVersion": "1.0",
  "kind": "kapi-localization-format",
  "generator": {"id": "test", "version": "1.0"},
  "project": {"id": "p1", "sourceLocale": "en"},
  "documents": [{
    "id": "doc1",
    "documentType": "jsx",
    "path": "test.tsx",
    "blocks": [
      {"id": "b1", "translatable": true, "type": "jsx:string", "source": [{"text": "Hello world"}]},
      {"id": "b2", "translatable": true, "type": "jsx:string", "source": [{"text": "Goodbye now"}]}
    ]
  }]
}`),
		},
		{
			name:             "versifiedtext_two_verses",
			formatID:         "versifiedtext",
			filterClass:      "",
			tikalUnsupported: true,
			filename:         "scripture.vrs",
			body: []byte(`book.chapter.1|Hello world
book.chapter.2|Goodbye now
`),
		},
		{
			// MessageFormat has no upstream okf_messageformat filter.
			// Native is line-oriented — one ICU MessageFormat string
			// per line.
			name:             "messageformat_two_lines",
			formatID:         "messageformat",
			filterClass:      "",
			tikalUnsupported: true,
			filename:         "messages.mf",
			body:             []byte("Hello world\nGoodbye now\n"),
		},

		// ── Binary / compound formats (load fixtures from disk) ──
		{
			// Native idml writer doesn't merge the pseudo-translated
			// target back into the .idml XML — output still carries
			// the original source. Real native bug.
			//
			// Bridge daemon throws an NPE on idml round-trip in the
			// upstream filter ("Cannot invoke
			// StartElement.getAttributeByName(...).getValue()").
			// Real upstream bug.
			name:        "idml_helloworld",
			formatID:    "idml",
			filterClass: "okf_idml",
			filename:    "helloworld.idml",
			bodyFile:    "core/formats/idml/testdata/helloworld.idml",
			skip:        []string{"native", "bridge"},
		},
		{
			name:        "icml_minimal",
			formatID:    "icml",
			filterClass: "okf_icml",
			filename:    "minimal.icml",
			bodyFile:    "core/formats/icml/testdata/minimal.icml",
		},
		{
			// Native openxml writer skips the target text on merge —
			// output docx contains the original English. Real native
			// bug. Bridge passes.
			name:        "openxml_simple_docx",
			formatID:    "openxml",
			filterClass: "okf_openxml",
			filename:    "simple.docx",
			bodyFile:    "core/formats/openxml/testdata/simple.docx",
			skip:        []string{"native"},
		},
		// odf is not covered yet — the framework has no
		// core/formats/odf/testdata/*.odt fixture committed. Add a
		// row when an .odt/.ods/.odp/.odg lands.
		{
			// Native rtf writer emits "?" sentinels around target
			// text rather than properly inserting it. Real native
			// bug. Bridge's okf_rtf is read-only — daemon returns
			// empty output on round-trip. Both real divergences;
			// skip both engines (epub-style native-only would also
			// fail since the native writer is broken).
			name:        "rtf_simple",
			formatID:    "rtf",
			filterClass: "okf_rtf",
			filename:    "simple.rtf",
			bodyFile:    "core/formats/rtf/testdata/simple.rtf",
			skip:        []string{"native", "bridge"},
		},
		{
			// Native mif writer produces output the reader can't
			// re-extract (0 blocks). Bridge picks up only 1 of 3
			// blocks. Both real bugs.
			name:        "mif_simple",
			formatID:    "mif",
			filterClass: "okf_mif",
			filename:    "simple.mif",
			bodyFile:    "core/formats/mif/testdata/simple.mif",
			skip:        []string{"native", "bridge"},
		},
		{
			// EPUB has no okf_epub bridge filter — it's handled by
			// okf_archive on the bridge side. Native epub round-trip
			// only.
			name:             "epub_minimal",
			formatID:         "epub",
			filterClass:      "",
			tikalUnsupported: true,
			filename:         "minimal.epub",
			bodyFile:         "core/formats/epub/testdata/minimal.epub",
		},
	}
}

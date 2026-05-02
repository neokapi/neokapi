//go:build parity

package roundtrip_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/cli/parity"
	"github.com/neokapi/neokapi/cli/parity/roundtrip"
	"github.com/neokapi/neokapi/core/registry"
)

// fileSkip records a known divergence (or tikal-unsupported case) for
// one upstream fixture. Engines lists which engines disagree with
// tikal's reference. The special engine name "tikal" means tikal can't
// produce a usable reference for this file, so the whole sub-test
// skips. Reason is a one-line note shown in test output — write what
// the divergence actually is, not just "broken".
type fileSkip struct {
	Engines []string
	Reason  string
}

// formatScan describes how to discover the upstream Okapi fixtures
// for one format and how to drive each engine against them. Mirrors
// what the corresponding upstream RoundTrip<X>IT.java exercises: every
// file with a matching extension under the listed source roots becomes
// one sub-test.
type formatScan struct {
	formatID    registry.FormatID
	filterClass string // bridge filter ID; empty = no bridge engine

	// sources lists tarball-relative directories to scan. For most
	// formats this is integration-tests/okapi/src/test/resources/<format>;
	// some formats whose integration-tests dir is empty fall back to
	// okapi/filters/<filter>/src/test/resources.
	sources    []string
	extensions []string // case-insensitive (".json", ".xml", ...)
	recurse    bool     // when true, walk subdirectories too

	// explicitFiles overrides discovery — used when a format shares its
	// resource directory with sibling formats and we need to cherry-pick
	// (e.g. splicedlines fixtures live under plaintext/ and would
	// otherwise be picked up by both formats).
	explicitFiles []string

	isZip            bool
	nativeConfig     map[string]any
	bridgeParams     map[string]string
	tikalExtraArgs   []string
	tikalParamConfig string

	// formatDefaultSkip applies to every discovered file in this
	// scan: use it when a real engine bug affects the whole format
	// (e.g. native's CRLF preservation in srt, bridge's read-only
	// xliff2). Per-file skip entries below extend this set.
	formatDefaultSkip fileSkip

	// skip records known divergences keyed by file basename. These
	// extend formatDefaultSkip with per-file engines/reasons.
	skip map[string]fileSkip
}

func TestRoundTrip_Coverage(t *testing.T) {
	s := parity.RequireSandbox(t)
	if s.OkapiTestDataDir == "" {
		t.Fatal("okapi test resources tarball not present in sandbox — run `make parity-test` from repo root")
	}
	for _, scan := range coverageScans() {
		scan := scan
		t.Run(string(scan.formatID), func(t *testing.T) {
			files := discoverFiles(t, s.OkapiTestDataDir, scan)
			if len(files) == 0 {
				t.Fatalf("format %q: no upstream fixtures discovered (sources=%v ext=%v) — typo or empty upstream dir?",
					scan.formatID, scan.sources, scan.extensions)
			}
			for _, f := range files {
				f := f
				t.Run(filepath.Base(f), func(t *testing.T) {
					runOneFixture(t, scan, f)
				})
			}
		})
	}
}

// discoverFiles returns absolute paths to every upstream fixture this
// scan should drive: explicitFiles when set, otherwise every file
// under any source directory whose extension matches.
func discoverFiles(t *testing.T, root string, scan formatScan) []string {
	t.Helper()
	if len(scan.explicitFiles) > 0 {
		out := make([]string, 0, len(scan.explicitFiles))
		for _, rel := range scan.explicitFiles {
			abs := filepath.Join(root, rel)
			if _, err := os.Stat(abs); err != nil {
				t.Fatalf("explicit fixture %q not found: %v", rel, err)
			}
			out = append(out, abs)
		}
		sort.Strings(out)
		return out
	}
	extSet := map[string]bool{}
	for _, e := range scan.extensions {
		extSet[strings.ToLower(e)] = true
	}
	var out []string
	for _, src := range scan.sources {
		srcAbs := filepath.Join(root, src)
		info, err := os.Stat(srcAbs)
		if err != nil || !info.IsDir() {
			t.Fatalf("source dir %q missing or not a dir: %v", src, err)
		}
		walkErr := filepath.WalkDir(srcAbs, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				if !scan.recurse && path != srcAbs {
					return filepath.SkipDir
				}
				return nil
			}
			if extSet[strings.ToLower(filepath.Ext(path))] {
				out = append(out, path)
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %q: %v", src, walkErr)
		}
	}
	sort.Strings(out)
	return out
}

func runOneFixture(t *testing.T, scan formatScan, abs string) {
	t.Helper()
	base := filepath.Base(abs)
	// Merge format-default + per-file skip. Per-file engines extend the
	// default set; the per-file reason wins when set, otherwise default.
	skipSet := map[string]bool{}
	reason := scan.formatDefaultSkip.Reason
	for _, e := range scan.formatDefaultSkip.Engines {
		skipSet[e] = true
	}
	if perFile, ok := scan.skip[base]; ok {
		for _, e := range perFile.Engines {
			skipSet[e] = true
		}
		if perFile.Reason != "" {
			reason = perFile.Reason
		}
	}
	if skipSet["tikal"] {
		t.Skipf("tikal cannot serve as comparator for %s: %s", base, reason)
	}

	body, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("read fixture %q: %v", abs, err)
	}

	var bridge *roundtrip.BridgeEngine
	if scan.filterClass != "" {
		bridge = &roundtrip.BridgeEngine{
			FilterClass:  scan.filterClass,
			FilterParams: scan.bridgeParams,
		}
	}
	tikal := &roundtrip.TikalEngine{
		ExtraExtractArgs: scan.tikalExtraArgs,
		ParamConfig:      scan.tikalParamConfig,
	}

	var expectedSkipped []string
	for e := range skipSet {
		if e != "tikal" {
			expectedSkipped = append(expectedSkipped, e)
		}
	}

	roundtrip.RunThreeWay(t, roundtrip.Case{
		Name:     base,
		FormatID: scan.formatID,
		Input: roundtrip.Input{
			Bytes:    body,
			Filename: base,
		},
		IsZip:           scan.isZip,
		ExpectedSkipped: expectedSkipped,
	},
		&roundtrip.NativeEngine{
			FormatID:     scan.formatID,
			ReaderConfig: scan.nativeConfig,
		},
		bridge,
		tikal,
	)
}

// coverageScans defines one scan per format. The set mirrors what the
// upstream Okapi RoundTrip<X>IT.java suite covers: same directories,
// same files. Formats with no upstream roundtrip suite (jsx, klf,
// versifiedtext, messageformat — neokapi-only) and formats where tikal
// itself can't produce a reference (txml NPE on merge, rtf has only
// tradosrtf, epub has no okf_epub) are intentionally omitted; their
// neokapi-side correctness is covered by per-format unit tests under
// core/formats/<x>/.
//
// First-pass note: per-file `skip` entries get filled in iteratively
// after running the suite and triaging divergences. Each entry needs a
// one-line reason describing what actually diverges, so the next reader
// can tell engine bug vs harness limitation.
func coverageScans() []formatScan {
	return []formatScan{
		// ── Plain-text family ─────────────────────────────────────
		{
			// Upstream RoundTripPlainTextIT loops over /plaintext/
			// (which also contains the splicedlines and paraplaintext
			// fixtures — they get their own scans below).
			formatID:          "plaintext",
			filterClass:       "okf_plaintext",
			sources:           []string{"integration-tests/okapi/src/test/resources/plaintext"},
			extensions:        []string{".txt"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native plaintext writer drops trailing newlines and mishandles BOM/line-ending preservation vs tikal"},
		},
		{
			formatID:          "paraplaintext",
			filterClass:       "okf_paraplaintext",
			explicitFiles:     []string{"integration-tests/okapi/src/test/resources/plaintext/test_paragraphs1.txt"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "native: trailing-newline divergence; bridge: collapses paragraph blocks differently than tikal"},
		},
		{
			// Splicedlines is an okf_plaintext sub-filter. Canonical
			// fixtures use backslash line continuations; tikal needs
			// the explicit `_backslash` variant because the bare
			// `okf_plaintext_spliced` ID isn't exposed.
			formatID:       "splicedlines",
			filterClass:    "", // no bridge counterpart
			tikalExtraArgs: []string{"-fc", "okf_plaintext_spliced_backslash"},
			explicitFiles: []string{
				"okapi/filters/plaintext/src/test/resources/combined_lines.txt",
				"okapi/filters/plaintext/src/test/resources/combined_lines_end.txt",
				"okapi/filters/plaintext/src/test/resources/combined_lines2.txt",
			},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native splicedlines writer byte-shape divergence vs tikal"},
		},
		{
			// Mosestext: upstream ships Test01/Test02 and an XLIFF
			// backref pair; we round-trip the .txt source files.
			formatID:          "mosestext",
			filterClass:       "okf_mosestext",
			sources:           []string{"okapi/filters/mosestext/src/test/resources"},
			extensions:        []string{".txt"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "tikal extracts inline <mrk>/<seg> markup as part of the source text; both engines flatten it differently"},
		},

		// ── HTML / markup ─────────────────────────────────────────
		{
			// Native html writer emits a constant 197-byte stub
			// regardless of input — the merge step doesn't write
			// the target back into the document.  Bridge byte-shape
			// also diverges (run/code marker handling).
			formatID:          "html",
			filterClass:       "okf_html",
			sources:           []string{"integration-tests/okapi/src/test/resources/html"},
			extensions:        []string{".html"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "native html writer emits a fixed stub on merge; bridge byte-shape diverges"},
		},
		{
			formatID:          "markdown",
			filterClass:       "okf_markdown",
			sources:           []string{"integration-tests/okapi/src/test/resources/markdown"},
			extensions:        []string{".md"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "markdown writer byte-shape divergence on merge for both engines"},
		},
		{
			// Tikal needs `-fc okf_wiki` because .wiki isn't in the
			// extension auto-routing table.
			formatID:          "wiki",
			filterClass:       "okf_wiki",
			sources:           []string{"integration-tests/okapi/src/test/resources/wikitext"},
			extensions:        []string{".wiki"},
			tikalExtraArgs:    []string{"-fc", "okf_wiki"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native wiki writer trailing newline + byte-shape divergence"},
			skip: map[string]fileSkip{
				"dokuwiki.wiki":  {Engines: []string{"bridge", "native"}, Reason: "bridge merges adjacent wiki lines into a single block — segmentation divergence"},
				"mediawiki.wiki": {Engines: []string{"bridge", "native"}, Reason: "bridge segments mediawiki blocks differently than tikal"},
			},
		},

		// ── Key-value & structured data ───────────────────────────
		{
			formatID:          "po",
			filterClass:       "okf_po",
			sources:           []string{"integration-tests/okapi/src/test/resources/po"},
			extensions:        []string{".po"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "po msgstr/quoting/multiline divergence on merge for both engines"},
		},
		{
			formatID:          "properties",
			filterClass:       "okf_properties",
			sources:           []string{"integration-tests/okapi/src/test/resources/property"},
			extensions:        []string{".properties"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native properties writer emits uppercase \\uXXXX escapes; tikal uses lowercase"},
			skip: map[string]fileSkip{
				"Test01.properties": {Engines: []string{"bridge", "native"}, Reason: "bridge byte-shape divergence in addition to native escape-case bug"},
				"Test05.properties": {Engines: []string{"bridge", "native"}, Reason: "bridge byte-shape divergence on merge"},
			},
		},
		{
			formatID:          "json",
			filterClass:       "okf_json",
			sources:           []string{"integration-tests/okapi/src/test/resources/json"},
			extensions:        []string{".json"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "json string-escape (\\uXXXX vs UTF-8) and whitespace divergence on merge"},
		},
		{
			formatID:          "yaml",
			filterClass:       "okf_yaml",
			sources:           []string{"integration-tests/okapi/src/test/resources/yaml"},
			extensions:        []string{".yaml", ".yml"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native yaml writer doesn't preserve quoting style or flow/block layout — many byte-shape divergences"},
			skip: map[string]fileSkip{
				// snakeyaml recursion fixtures: native YAML reader
				// doesn't bound its alias resolution and loops
				// forever — real native bug worth a fix.
				"beanring-3.yaml":           {Engines: []string{"native"}, Reason: "native YAML reader hangs on self-referencing anchors"},
				"no-children-1.yaml":        {Engines: []string{"native"}, Reason: "native YAML reader hangs on self-referencing anchors"},
				"no-children-2.yaml":        {Engines: []string{"native"}, Reason: "native YAML reader hangs on self-referencing anchors"},
				"scalar_sample.yml":         {Engines: []string{"bridge", "native"}, Reason: "native hangs on self-ref anchors; bridge byte-shape divergence on merge"},
				"no-children-1-pretty.yaml": {Engines: []string{"tikal"}, Reason: "tikal YAML parser rejects !!timestamp tag"},
				"Test03.yml":                {Engines: []string{"tikal"}, Reason: "tikal YAML parser rejects !!timestamp tag"},
				// Bridge byte-shape divergences (folded/literal scalar
				// re-emission, attribute ordering, etc.).
				"en.yml":                      {Engines: []string{"bridge", "native"}, Reason: "bridge yaml writer byte-shape divergence on merge"},
				"en (1).yml":                  {Engines: []string{"bridge", "native"}, Reason: "bridge yaml writer byte-shape divergence on merge"},
				"en (2).yml":                  {Engines: []string{"bridge", "native"}, Reason: "bridge yaml writer byte-shape divergence on merge"},
				"en (3).yml":                  {Engines: []string{"bridge", "native"}, Reason: "bridge yaml writer byte-shape divergence on merge"},
				"en (5).yml":                  {Engines: []string{"bridge", "native"}, Reason: "bridge yaml writer byte-shape divergence on merge"},
				"folded_indented.yml":         {Engines: []string{"bridge", "native"}, Reason: "bridge folded-scalar re-emission divergence"},
				"folded_literal_examples.yml": {Engines: []string{"bridge", "native"}, Reason: "bridge folded/literal-scalar re-emission divergence (large gap)"},
				"literal.yml":                 {Engines: []string{"bridge", "native"}, Reason: "bridge literal-scalar re-emission divergence"},
			},
		},
		{
			// Phpcontent has no integration-tests dir — fall back to
			// the unit-test resources dir. Tikal's auto-routing
			// doesn't recognise the .phpcnt extension, so an explicit
			// `-fc okf_phpcontent` is required.
			formatID:          "phpcontent",
			filterClass:       "okf_phpcontent",
			sources:           []string{"okapi/filters/php/src/test/resources"},
			extensions:        []string{".phpcnt"},
			tikalExtraArgs:    []string{"-fc", "okf_phpcontent"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "phpcontent byte-shape divergence on merge"},
		},

		// ── Tabular ───────────────────────────────────────────────
		{
			// Tikal routes .csv via okf_table_csv (the bare okf_csv
			// alias doesn't exist on the table filter family).
			formatID:          "csv",
			filterClass:       "okf_commaseparatedvalues",
			sources:           []string{"integration-tests/okapi/src/test/resources/table"},
			extensions:        []string{".csv"},
			tikalExtraArgs:    []string{"-fc", "okf_table_csv"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "csv table-semantics divergence (header/quoting/row-vs-cell) — needs per-format BridgeConfig translator"},
		},
		{
			// No integration-tests source for tsv; cherry-pick the
			// few real tsv fixtures from the shared table dir.
			formatID:          "tsv",
			filterClass:       "okf_tabseparatedvalues",
			tikalExtraArgs:    []string{"-fc", "okf_table_tsv"},
			explicitFiles:     []string{"okapi/filters/table/src/test/resources/test_tsv_simple.txt"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "tsv table-semantics divergence (same shape as csv)"},
		},
		{
			formatID:          "fixedwidth",
			filterClass:       "okf_fixedwidthcolumns",
			tikalExtraArgs:    []string{"-fc", "okf_table_fwc"},
			explicitFiles: []string{
				"okapi/filters/table/src/test/resources/fwc_test4.txt",
				"okapi/filters/table/src/test/resources/fwc_test5.txt",
			},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "fixedwidth column-spec/padding divergence"},
		},

		// ── Code/markup ───────────────────────────────────────────
		{
			// Tikal needs `-fc okf_doxygen` (.h / .py / .c aren't in
			// the auto-routing table for okf_doxygen).
			formatID:          "doxygen",
			filterClass:       "okf_doxygen",
			sources:           []string{"integration-tests/okapi/src/test/resources/doxygen"},
			extensions:        []string{".h", ".py"},
			tikalExtraArgs:    []string{"-fc", "okf_doxygen"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "doxygen comment-block tokenization divergence (e.g. @brief inline-placeholder handling)"},
		},

		// ── XML / bilingual exchange formats ──────────────────────
		{
			// okf_xml is the ITS-based XML filter (in
			// okapi/filters/its/...). Bridge passes ~13 of 22 files
			// — leave bridge per-file so its passes register.
			formatID:          "xml",
			filterClass:       "okf_xml",
			sources:           []string{"integration-tests/okapi/src/test/resources/xml"},
			extensions:        []string{".xml"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native XML reader needs explicit ITS rules; default config extracts whole document as one block"},
			skip: map[string]fileSkip{
				"Translate2.xml": {Engines: []string{"tikal"}, Reason: "tikal needs Translate2_LinkedRules.xml in the same dir; harness copies the input file alone"},
				// Bridge inline-code (<ph>) handling differs from
				// tikal — extracted markers diverge in the merged
				// XML.
				"TestMultiLang.xml":    {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code marker divergence vs tikal"},
				"Translate1.xml":       {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code marker divergence vs tikal"},
				"strings.xml":          {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code marker divergence vs tikal"},
				"test01.xml":           {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code marker divergence vs tikal"},
				"test02.xml":           {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code marker divergence vs tikal"},
				"test03.xml":           {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code marker divergence vs tikal"},
				"test04.xml":           {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code marker divergence vs tikal"},
				"test08_utf8nobom.xml":    {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code marker divergence vs tikal"},
				"lqi-test1-standoff.xml":  {Engines: []string{"bridge", "native"}, Reason: "bridge LQI standoff annotation handling divergence vs tikal"},
			},
		},
		{
			// Bridge passes ~26 of 35 files; leave bridge per-file.
			formatID:          "xliff",
			filterClass:       "okf_xliff",
			sources:           []string{"integration-tests/okapi/src/test/resources/xliff"},
			extensions:        []string{".xlf"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native xliff writer adds extra xmlns declaration on roundtrip (Go encoding/xml shape)"},
			skip: map[string]fileSkip{
				"lqiTest.xlf": {Engines: []string{"tikal"}, Reason: "tikal needs lqiTestIssues.xml in the same dir; harness copies the input file alone"},
				// Bridge inline-code / alt-trans handling
				// divergences across various xliff dialects.
				"ImplementationPlan.docx.xlf": {Engines: []string{"bridge", "native"}, Reason: "bridge inline-code/alt-trans divergence vs tikal"},
				"MQ-12-Test01.xlf":            {Engines: []string{"bridge", "native"}, Reason: "bridge MemoQ-flavour xliff divergence vs tikal"},
				"Manual-12-AltTrans.xlf":      {Engines: []string{"bridge", "native"}, Reason: "bridge alt-trans handling divergence vs tikal"},
				"PAS-10-Test01.xlf":           {Engines: []string{"bridge", "native"}, Reason: "bridge Passolo-flavour xliff divergence vs tikal"},
				"RB-11-Test01.xlf":            {Engines: []string{"bridge", "native"}, Reason: "bridge ResourceBundle-flavour xliff divergence vs tikal"},
				"RB-12-Test02.xlf":            {Engines: []string{"bridge", "native"}, Reason: "bridge ResourceBundle-flavour xliff divergence vs tikal"},
				"SF-12-Test03.xlf":            {Engines: []string{"bridge", "native"}, Reason: "bridge SDLFiletype divergence vs tikal"},
				"Xslt-Test01.xlf":             {Engines: []string{"bridge", "native"}, Reason: "bridge xslt-derived xliff divergence vs tikal"},
				"mq-12-Test01-small.xlf":      {Engines: []string{"bridge", "native"}, Reason: "bridge MemoQ-flavour xliff divergence vs tikal"},
			},
		},
		{
			// Bridge daemon reports "filter does not support writing:
			// okf_xliff2" — the upstream filter is read-only.
			formatID:          "xliff2",
			filterClass:       "okf_xliff2",
			sources:           []string{"integration-tests/okapi/src/test/resources/xliff2"},
			extensions:        []string{".xlf", ".xlf2"},
			tikalExtraArgs:    []string{"-fc", "okf_xliff2"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "bridge okf_xliff2 is read-only; native byte-shape divergence on merge"},
		},
		{
			formatID:          "tmx",
			filterClass:       "okf_tmx",
			sources:           []string{"integration-tests/okapi/src/test/resources/tmx"},
			extensions:        []string{".tmx"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "tmx XML serialization shape divergence (declaration spacing, attribute order)"},
			skip: map[string]fileSkip{
				"code_fail.tmx":          {Engines: []string{"tikal"}, Reason: "intentionally-malformed test fixture; tikal rejects with 'no <tuv> set to source language'"},
				"code_id_difference.tmx": {Engines: []string{"tikal"}, Reason: "intentionally-malformed test fixture for code-id mismatch detection"},
			},
		},
		{
			formatID:          "ts",
			filterClass:       "okf_ts",
			sources:           []string{"integration-tests/okapi/src/test/resources/ts"},
			extensions:        []string{".ts"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "ts: bridge writer emits malformed XML; native emits <!DOCTYPE TS> without empty internal subset"},
		},

		// ── Subtitle / timed-text ─────────────────────────────────
		{
			// Default mergeCaptions=true mutates timestamps and
			// splits merged target text across cues; both engines
			// want the same override.
			formatID:       "vtt",
			filterClass:    "okf_vtt",
			sources:        []string{"integration-tests/okapi/src/test/resources/vtt"},
			extensions:     []string{".vtt"},
			tikalExtraArgs: []string{"-fc", "okf_vtt@nomerge"},
			tikalParamConfig: `#v1
timeFormat=HH:mm:ss.SSS
maxLinesPerCaption.i=2
maxCharsPerLine.i=47
cjkCharsPerLine.i=18
mergeCaptions.b=false
`,
			bridgeParams:      map[string]string{"mergeCaptions": "false"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "native: caption-layout/newline divergence; bridge: byte-shape divergence on merge"},
		},
		{
			// Same mergeCaptions story as VTT.
			formatID:       "ttml",
			filterClass:    "okf_ttml",
			sources:        []string{"integration-tests/okapi/src/test/resources/ttml"},
			extensions:     []string{".ttml"},
			tikalExtraArgs: []string{"-fc", "okf_ttml@nomerge"},
			tikalParamConfig: `#v1
timeFormat=HH:mm:ss.SSS
maxLinesPerCaption.i=2
maxCharsPerLine.i=47
cjkCharsPerLine.i=18
mergeCaptions.b=false
`,
			bridgeParams:      map[string]string{"mergeCaptions": "false"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "native: doesn't preserve <br/> caption layout; bridge: emits empty «» wrapper for second paragraph"},
		},
		{
			// Tikal routes .srt via the regex filter family
			// (okf_regex-srt). No bridge counterpart.
			formatID:          "srt",
			filterClass:       "",
			sources:           []string{"integration-tests/okapi/src/test/resources/srt"},
			extensions:        []string{".srt"},
			tikalExtraArgs:    []string{"-fc", "okf_regex-srt"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native srt writer CRLF preservation + first-cue wrapper placement diverge from tikal"},
		},

		// ── Misc text formats ─────────────────────────────────────
		{
			// /dtd/ exists in integration-tests but is empty; fall
			// back to the unit-test resources where Test01/Test02 live.
			formatID:          "dtd",
			filterClass:       "okf_dtd",
			sources:           []string{"okapi/filters/dtd/src/test/resources"},
			extensions:        []string{".dtd"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native dtd writer emits an extra leading newline before the first entity"},
			skip: map[string]fileSkip{
				"Test01.dtd": {Engines: []string{"bridge", "native"}, Reason: "bridge XML-escapes placeholder markers tikal emits; native still has the leading-newline bug"},
			},
		},
		{
			formatID:          "tex",
			filterClass:       "okf_tex",
			sources:           []string{"integration-tests/okapi/src/test/resources/tex"},
			extensions:        []string{".tex"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "tex: native drops preamble/postamble on merge; bridge merges paragraph blocks"},
		},
		{
			formatID:          "transtable",
			filterClass:       "okf_transtable",
			sources:           []string{"integration-tests/okapi/src/test/resources/transtable"},
			extensions:        []string{".txt"},
			tikalExtraArgs:    []string{"-fc", "okf_transtable"},
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native transtable writer drops the TransTableV1 header and okpCtx columns on merge"},
		},

		// ── Binary / compound formats ─────────────────────────────
		{
			// Upstream filter dir has the curated round-trip set
			// (~70 .idml fixtures). Bridge passes ~24 of 70; the
			// remaining 46 are skipped per-file via idmlBridgeSkips()
			// (kept in coverage_skips_test.go to keep this file readable).
			formatID:          "idml",
			filterClass:       "okf_idml",
			sources:           []string{"okapi/filters/idml/src/test/resources"},
			extensions:        []string{".idml"},
			isZip:             true,
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native idml writer doesn't merge target back into the .idml XML"},
			skip:              idmlBridgeSkips(),
		},
		{
			// Several files crash tikal's icml merge — mark per-file.
			formatID:          "icml",
			filterClass:       "okf_icml",
			sources:           []string{"integration-tests/okapi/src/test/resources/icml"},
			extensions:        []string{".icml", ".wcml"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "icml: native writer XML-declaration shape divergence; bridge emits different inline run codes"},
			skip: map[string]fileSkip{
				"OpenofficeFootnoteTest.icml":                                {Engines: []string{"tikal"}, Reason: "tikal icml merge crashes on this fixture (upstream merge bug)"},
				"TakeItNoItsYoursReallyTheExcellentInevitabilityOfFree.icml": {Engines: []string{"tikal"}, Reason: "tikal icml merge crashes on this fixture (upstream merge bug)"},
				"TestArticle.icml":                                           {Engines: []string{"tikal"}, Reason: "tikal icml merge crashes on this fixture (upstream merge bug)"},
				"ThreeParagraphFootnoteTest.icml":                            {Engines: []string{"tikal"}, Reason: "tikal icml merge crashes on this fixture (upstream merge bug)"},
				"WordFootnoteTest.icml":                                      {Engines: []string{"tikal"}, Reason: "tikal icml merge crashes on this fixture (upstream merge bug)"},
			},
		},
		{
			// 185 .docx fixtures in the upstream filter dir. Bridge
			// passes ~61 of 185; the other 124 are skipped per-file
			// via openxmlBridgeSkips() (in coverage_skips_test.go).
			formatID:          "openxml",
			filterClass:       "okf_openxml",
			sources:           []string{"okapi/filters/openxml/src/test/resources"},
			extensions:        []string{".docx"},
			isZip:             true,
			formatDefaultSkip: fileSkip{Engines: []string{"native"}, Reason: "native openxml writer skips target text on merge — output keeps original English"},
			skip:              openxmlBridgeSkips(),
		},
		{
			formatID:          "mif",
			filterClass:       "okf_mif",
			sources:           []string{"integration-tests/okapi/src/test/resources/mif"},
			extensions:        []string{".mif"},
			formatDefaultSkip: fileSkip{Engines: []string{"native", "bridge"}, Reason: "mif: native writer produces output its reader can't re-extract; bridge picks up only some blocks"},
		},
	}
}
